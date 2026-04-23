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

	"github.com/k8nstantin/OpenPraxis/internal/action"
	"github.com/k8nstantin/OpenPraxis/internal/conversation"
	"github.com/k8nstantin/OpenPraxis/internal/node"
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

		// Record actions on PostToolUse + check visceral compliance
		if event.HookEventName == "PostToolUse" && event.SessionID != "" {
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

				// Visceral compliance check — compare action against rules
				go checkVisceralCompliance(n, event.SessionID, toolName, toolInput)

				// Manifest delusion check — is the action related to any active manifest?
				go checkManifestDelusion(n, event.SessionID, toolName, toolInput)
			}
		}

		// On Stop: check if this session ever confirmed visceral rules
		if event.HookEventName == "Stop" && event.SessionID != "" {
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
						marker := ""
						if len(r.ID) >= 12 {
							marker = r.ID[:12]
						}
						ruleList += fmt.Sprintf("\n  %d. [%s] %s", i+1, marker, r.L2)
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
			if cost.OutputTokens > 0 {
				costJSON := fmt.Sprintf(`{"input_tokens":%d,"output_tokens":%d,"cache_read":%d,"cache_create":%d,"cost_usd":%.6f,"model":"%s"}`,
					cost.InputTokens, cost.OutputTokens, cost.CacheReadTokens, cost.CacheCreateTokens, cost.CostUSD, cost.Model)
				if err := n.Actions.Record(sessionID, n.PeerID(), "session_cost", costJSON, "", cwd); err != nil {
					slog.Warn("record session cost failed", "error", err)
				} else {
					slog.Info("session cost recorded", "session_id", sessionID[:min(12, len(sessionID))],
						"cost_usd", cost.CostUSD, "input_tokens", cost.InputTokens, "output_tokens", cost.OutputTokens,
						"cache_read_tokens", cost.CacheReadTokens, "cache_create_tokens", cost.CacheCreateTokens, "model", cost.Model)
				}
			}

			turns := readTranscript(transcriptPath)
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

// readTranscript reads a Claude Code transcript JSONL file and extracts user/assistant turns.
func readTranscript(path string) []conversation.Turn {
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

// sessionCost holds parsed token usage and computed cost from a transcript.
type sessionCost struct {
	InputTokens       int
	OutputTokens      int
	CacheReadTokens   int
	CacheCreateTokens int
	CostUSD           float64
	Model             string
}

// parseTranscriptCost reads a Claude Code transcript JSONL and sums all token usage.
func parseTranscriptCost(path string) sessionCost {
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

	// Compute cost based on model pricing
	// Opus: $15/M input, $75/M output, cache read $1.50/M, cache create $18.75/M
	// Sonnet: $3/M input, $15/M output, cache read $0.30/M, cache create $3.75/M
	inputRate := 15.0   // per million
	outputRate := 75.0
	cacheReadRate := 1.5
	cacheCreateRate := 18.75
	if strings.Contains(cost.Model, "sonnet") {
		inputRate = 3.0
		outputRate = 15.0
		cacheReadRate = 0.30
		cacheCreateRate = 3.75
	} else if strings.Contains(cost.Model, "haiku") {
		inputRate = 0.25
		outputRate = 1.25
		cacheReadRate = 0.025
		cacheCreateRate = 0.3125
	}

	cost.CostUSD = float64(cost.InputTokens)/1_000_000*inputRate +
		float64(cost.OutputTokens)/1_000_000*outputRate +
		float64(cost.CacheReadTokens)/1_000_000*cacheReadRate +
		float64(cost.CacheCreateTokens)/1_000_000*cacheCreateRate

	return cost
}
