package web

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/action"
	"github.com/k8nstantin/OpenPraxis/internal/conversation"
	"github.com/k8nstantin/OpenPraxis/internal/execution"
	"github.com/k8nstantin/OpenPraxis/internal/node"

	"github.com/google/uuid"
)

// apiHook handles Claude Code hook events — reads transcript file directly, no in-memory state.
func apiHook(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var event struct {
			SessionID      string `json:"session_id"`
			HookEventName  string `json:"hook_event_name"`
			TranscriptPath string `json:"transcript_path"`
			CWD            string `json:"cwd"`
			Prompt         string `json:"prompt"`
		}

		bodyBytes, _ := io.ReadAll(r.Body)
		if len(bodyBytes) == 0 {
			writeJSON(w, map[string]string{"status": "ok"})
			return
		}
		if err := json.Unmarshal(bodyBytes, &event); err != nil {
			writeJSON(w, map[string]string{"status": "ok"})
			return
		}

		// Store live transcript path so the 5s MCP sampler can parse cumulative
		// token/cost data mid-session. transcript_path is sent on every hook event.
		if event.TranscriptPath != "" && event.SessionID != "" {
			n.SetTranscriptPath(event.SessionID, event.TranscriptPath)
		}

		// PostToolUse: write a throttled sample row to execution_log so
		// interactive session stats update without waiting for session end.
		// Throttled to once per 30s — PostToolUse fires on every tool call.
		if (event.HookEventName == "PostToolUse" || event.HookEventName == "AfterTool") &&
			event.SessionID != "" && event.TranscriptPath != "" &&
			n.ExecutionLog != nil && n.ShouldWriteSample(event.SessionID, 30*time.Second) {
			runUID := n.GetSessionRunUID(event.SessionID)
			if runUID == "" {
				runUID = uuid.Must(uuid.NewV7()).String()
			}
			if node.ParseLiveTranscript != nil {
				live := node.ParseLiveTranscript(event.TranscriptPath)
				totalTok := int64(live.InputTokens + live.OutputTokens + live.CacheReadTokens + live.CacheCreateTokens)
				cacheHit := float64(0)
				if totalTok > 0 {
					cacheHit = float64(live.CacheReadTokens) / float64(totalTok) * 100
				}
				cpuPct, rssMB := n.LatestSystemSample()
				sr := execution.Row{
					ID:                uuid.Must(uuid.NewV7()).String(),
					RunUID:            runUID,
					EntityUID:         event.SessionID,
					SessionID:         event.SessionID,
					Event:             execution.EventSample,
					Trigger:           "interactive",
					NodeID:            n.PeerID(),
					AgentRuntime:      n.AgentForSession(event.SessionID),
					Model:             live.Model,
					Turns:             live.Turns,
					Actions:           live.Actions,
					InputTokens:       int64(live.InputTokens),
					OutputTokens:      int64(live.OutputTokens),
					CacheReadTokens:   int64(live.CacheReadTokens),
					CacheCreateTokens: int64(live.CacheCreateTokens),
					CacheHitRatePct:   cacheHit,
					CPUPct:            cpuPct,
					RSSMB:             rssMB,
					CreatedBy:         "hook/post-tool-use",
				}
				_ = n.ExecutionLog.Insert(context.Background(), sr)
			}
		}

		// Record actions on PostToolUse/AfterTool + check visceral compliance
		if (event.HookEventName == "PostToolUse" || event.HookEventName == "AfterTool") && event.SessionID != "" {
			var toolName string
			var toolInput, toolResponse any
			var raw map[string]any
			if err := json.Unmarshal(bodyBytes, &raw); err != nil {
				slog.Warn("unmarshal hook body failed", "error", err)
			}
			toolName, _ = raw["tool_name"].(string)
			toolInput = raw["tool_input"]
			toolResponse = raw["tool_response"]
			cwd, _ := raw["cwd"].(string)
			if toolName != "" {
				if err := n.Actions.Record(event.SessionID, n.PeerID(), toolName, toolInput, toolResponse, cwd); err != nil {
					slog.Warn("record action failed", "error", err)
				}

				// Compliance + delusion checks go through a bounded worker
				// pool (cap 256 in-flight) so a chatty agent can't spawn
				// unbounded goroutines. The `compliance_checks_enabled`
				// knob (system-scope default true) lets operators opt out
				// entirely for high-throughput agents.
				if complianceChecksEnabled(r.Context(), n) {
					enqueueComplianceCheck(n, "visceral", event.SessionID, toolName, toolInput)
					enqueueComplianceCheck(n, "delusion", event.SessionID, toolName, toolInput)
				}
			}
		}

		// On Stop: check if this session ever confirmed visceral rules
		if (event.HookEventName == "Stop" || event.HookEventName == "AfterAgent") && event.SessionID != "" {
			confs, _ := n.Actions.ListConfirmations(100)
			confirmed := false
			for _, c := range confs {
				if c.SessionID != "" && strings.Contains(c.SessionID, event.SessionID[:min(8, len(event.SessionID))]) {
					confirmed = true
					break
				}
			}
			if !confirmed {
				// Check if visceral rules exist — only flag if there are rules to confirm
				rules, _ := n.Index.ListByType("visceral", 1)
				if len(rules) > 0 {
					sid := event.SessionID
					if len(sid) > 12 {
						sid = sid[:12]
					}
					// Only flag once per session — check if already flagged
					existing, _ := n.Actions.ListAmnesia("", 100)
					alreadyFlagged := false
					for _, a := range existing {
						if a.ToolName == "MISSING_VISCERAL_CONFIRM" && strings.Contains(a.SessionID, sid) {
							alreadyFlagged = true
							break
						}
					}
					if !alreadyFlagged {
						ruleCount := len(rules)
					allRules, _ := n.Index.ListByType("visceral", 100)
					ruleCount = len(allRules)
					ruleList := ""
					for i, r := range allRules {
						ruleList += fmt.Sprintf("\n  %d. [%s] %s", i+1, r.ID, r.L2)
					}
					if err := n.Actions.RecordAmnesia(event.SessionID, n.PeerID(), "", "", "SYSTEM", "SYSTEM",
							fmt.Sprintf("Session did not confirm visceral rules on startup. %d rules were NOT acknowledged.", ruleCount),
							"MISSING_VISCERAL_CONFIRM",
							fmt.Sprintf("Session %s started without calling visceral_rules + visceral_confirm. CWD: %s. Unacknowledged rules:%s", event.SessionID[:min(8, len(event.SessionID))], event.CWD, ruleList), 1.0, action.MatchTypeMissingConfirm, ""); err != nil {
						slog.Warn("record amnesia failed", "error", err)
					}
					}
				}
			}
		}

		// Heavy work goes async + only on SessionEnd. Stop fires after every
		// turn; running the full transcript re-parse + conversation re-save
		// per turn pegs the server at 500%+ CPU on long sessions and blanks
		// the dashboard. The right semantics: Stop = "this turn ended" (do
		// nothing expensive), SessionEnd = "session over" (now compute the
		// canonical totals + save the conversation).
		//
		// We respond immediately — the hook is fire-and-forget for the heavy
		// work. Operator-visible side effects land asynchronously in a
		// goroutine.
		if event.HookEventName != "SessionEnd" || event.TranscriptPath == "" || event.SessionID == "" {
			writeJSON(w, map[string]string{"status": "ok"})
			return
		}

		// Capture what we need so the goroutine doesn't reference the
		// request-scoped variables after the response is written.
		sessionID := event.SessionID
		transcriptPath := event.TranscriptPath
		cwd := event.CWD
		writeJSON(w, map[string]string{"status": "ok"})

		go func() {
			cost := parseTranscriptCost(transcriptPath)
			turns := readTranscript(transcriptPath)

			if cost.OutputTokens > 0 {
				tokenJSON := fmt.Sprintf(`{"input_tokens":%d,"output_tokens":%d,"cache_read":%d,"cache_create":%d,"model":"%s"}`,
					cost.InputTokens, cost.OutputTokens, cost.CacheReadTokens, cost.CacheCreateTokens, cost.Model)
				if err := n.Actions.Record(sessionID, n.PeerID(), "session_tokens", tokenJSON, "", cwd); err != nil {
					slog.Warn("record session tokens failed", "error", err)
				}
			}

			if n.ExecutionLog != nil && (cost.OutputTokens > 0 || len(turns) > 0) {
				runUID := uuid.Must(uuid.NewV7()).String()
				totalTokens := int64(cost.InputTokens + cost.OutputTokens + cost.CacheReadTokens + cost.CacheCreateTokens)
				cacheHitRate := float64(0)
				if totalTokens > 0 {
					cacheHitRate = float64(cost.CacheReadTokens) / float64(totalTokens) * 100
				}
				turnCount := 0
				actionCount := 0
				for _, t := range turns {
					if t.Role == "assistant" {
						turnCount++
					} else if t.Role == "user" && len(t.Content) > 6 && t.Content[:6] == "[tool:" {
						actionCount++
					}
				}
				row := execution.Row{
					ID:                uuid.Must(uuid.NewV7()).String(),
					RunUID:            runUID,
					EntityUID:         sessionID,
					SessionID:         sessionID,
					Event:             execution.EventCompleted,
					Trigger:           "interactive",
					NodeID:            n.PeerID(),
					AgentRuntime:      n.AgentForSession(sessionID),
					Model:             cost.Model,
					InputTokens:       int64(cost.InputTokens),
					OutputTokens:      int64(cost.OutputTokens),
					CacheReadTokens:   int64(cost.CacheReadTokens),
					CacheCreateTokens: int64(cost.CacheCreateTokens),
					CacheHitRatePct:   cacheHitRate,
					Turns:             turnCount,
					Actions:           actionCount,
					CreatedBy:         "hook/session-end",
				}
				if err := n.ExecutionLog.Insert(context.Background(), row); err != nil {
					slog.Warn("execution_log insert failed for session", "error", err)
				}
			}

			if len(turns) == 0 {
				return
			}
			title := fmt.Sprintf("session %s (ended)", sessionID[:min(8, len(sessionID))])
			convID := "hook-" + sessionID
			if err := n.UpdateConversation(context.Background(), convID, title, "claude-code", cwd, turns); err != nil {
				slog.Error("hook save failed", "error", err)
			} else {
				slog.Info("hook saved conversation", "title", title, "turns", len(turns))
			}
		}()
	}
}

// readTranscript reads a transcript file (JSON or JSONL) and extracts user/assistant turns.
func readTranscript(path string) []conversation.Turn {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	// Try parsing as Gemini JSON object first
	var gemini struct {
		Model string `json:"model"`
		Turns []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"turns"`
	}
	if err := json.Unmarshal(data, &gemini); err == nil && len(gemini.Turns) > 0 {
		var turns []conversation.Turn
		for _, t := range gemini.Turns {
			if t.Content == "" {
				continue
			}
			role := t.Role
			text := t.Content

			// Truncate very long turns
			if len(text) > 2000 {
				text = text[:2000] + "..."
			}

			// Merge consecutive same-role turns
			if len(turns) > 0 && turns[len(turns)-1].Role == role {
				turns[len(turns)-1].Content += "\n" + text
			} else {
				turns = append(turns, conversation.Turn{Role: role, Content: text, Model: gemini.Model})
			}
		}
		return turns
	}

	// Fall back to Claude Code transcript (JSONL)
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var turns []conversation.Turn
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB line buffer

	for scanner.Scan() {
		var entry struct {
			Type    string `json:"type"`
			Message struct {
				Role    string `json:"role"`
				Model   string `json:"model"`
				Content any    `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}

		if entry.Type != "user" && entry.Type != "assistant" {
			continue
		}

		role := entry.Message.Role
		if role == "" {
			role = entry.Type
		}
		model := entry.Message.Model

		// Extract text content
		var text string
		switch c := entry.Message.Content.(type) {
		case string:
			text = c
		case []any:
			for _, item := range c {
				if m, ok := item.(map[string]any); ok {
					if m["type"] == "text" {
						if t, ok := m["text"].(string); ok {
							text += t + "\n"
						}
					} else if m["type"] == "tool_use" {
						name, _ := m["name"].(string)
						text += fmt.Sprintf("[tool: %s]\n", name)
					}
				}
			}
		}

		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}

		// Truncate very long turns
		if len(text) > 2000 {
			text = text[:2000] + "..."
		}

		// Merge consecutive same-role turns
		if len(turns) > 0 && turns[len(turns)-1].Role == role {
			turns[len(turns)-1].Content += "\n" + text
		} else {
			turns = append(turns, conversation.Turn{Role: role, Content: text, Model: model})
		}
	}

	return turns
}

// sessionCost holds parsed token usage from a transcript.
type sessionCost struct {
	InputTokens       int
	OutputTokens      int
	CacheReadTokens   int
	CacheCreateTokens int
	Model             string
}

// parseTranscriptCost reads a transcript (JSON or JSONL) and sums all token usage.
func parseTranscriptCost(path string) sessionCost {
	data, err := os.ReadFile(path)
	if err != nil {
		return sessionCost{}
	}

	// Try parsing as Gemini JSON object first
	var gemini struct {
		Model      string `json:"model"`
		TotalUsage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		} `json:"totalUsage"`
	}
	if err := json.Unmarshal(data, &gemini); err == nil && gemini.TotalUsage.OutputTokens > 0 {
		return sessionCost{
			InputTokens:       gemini.TotalUsage.InputTokens,
			OutputTokens:      gemini.TotalUsage.OutputTokens,
			CacheReadTokens:   gemini.TotalUsage.CacheReadInputTokens,
			CacheCreateTokens: gemini.TotalUsage.CacheCreationInputTokens,
			Model:             gemini.Model,
		}
	}

	// Fall back to Claude Code transcript (JSONL)
	f, err := os.Open(path)
	if err != nil {
		return sessionCost{}
	}
	defer f.Close()

	var cost sessionCost
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		var entry struct {
			Type    string `json:"type"`
			Message struct {
				Model string `json:"model"`
				Usage struct {
					InputTokens              int `json:"input_tokens"`
					OutputTokens             int `json:"output_tokens"`
					CacheReadInputTokens     int `json:"cache_read_input_tokens"`
					CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if entry.Type != "assistant" {
			continue
		}
		u := entry.Message.Usage
		cost.InputTokens += u.InputTokens
		cost.OutputTokens += u.OutputTokens
		cost.CacheReadTokens += u.CacheReadInputTokens
		cost.CacheCreateTokens += u.CacheCreationInputTokens
		if entry.Message.Model != "" {
			cost.Model = entry.Message.Model
		}
	}

	return cost
}

func init() {
	// Wire the transcript parser into the node package so the MCP sampler
	// (which imports node but not web) can call it without a circular import.
	// Parses both token/cost AND turn/action counts from the live transcript.
	node.ParseLiveTranscript = func(path string) node.LiveSessionCost {
		c := parseTranscriptCost(path)
		turns := readTranscript(path)
		turnCount, actionCount := 0, 0
		for _, t := range turns {
			if t.Role == "assistant" {
				turnCount++
				// readTranscript formats each tool_use block as "[tool: name]\n"
				// on assistant turns — count occurrences to get action count.
				actionCount += strings.Count(t.Content, "[tool:")
			}
		}
		return node.LiveSessionCost{
			InputTokens:       c.InputTokens,
			OutputTokens:      c.OutputTokens,
			CacheReadTokens:   c.CacheReadTokens,
			CacheCreateTokens: c.CacheCreateTokens,
			Model:             c.Model,
			Turns:             turnCount,
			Actions:           actionCount,
		}
	}
}
