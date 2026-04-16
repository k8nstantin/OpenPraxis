package task

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"openpraxis/internal/action"
)

// RunningTask tracks an actively executing task.
type RunningTask struct {
	TaskID    string    `json:"task_id"`
	Marker    string    `json:"marker"`
	Title     string    `json:"title"`
	Manifest  string    `json:"manifest"`
	Agent     string    `json:"agent"`
	PID       int       `json:"pid"`
	Paused    bool      `json:"paused"`
	StartedAt time.Time `json:"started_at"`
	Actions   int       `json:"actions"`
	Lines     int       `json:"lines"`
	LastLine  string    `json:"last_line"`
	Output    []string  `json:"-"` // ring buffer, not serialized
	cancel    context.CancelFunc
	cmd       *exec.Cmd
}

// Runner manages task execution — spawning agents and tracking running tasks.
type Runner struct {
	store       *Store
	actions     *action.Store
	running     map[string]*RunningTask
	mu          sync.RWMutex
	maxParallel int
	onEvent     func(event string, data map[string]string) // broadcast callback
}

// NewRunner creates a task runner.
func NewRunner(store *Store, actions *action.Store, maxParallel int, onEvent func(string, map[string]string)) *Runner {
	if maxParallel <= 0 {
		maxParallel = 3
	}
	return &Runner{
		store:       store,
		actions:     actions,
		running:     make(map[string]*RunningTask),
		maxParallel: maxParallel,
		onEvent:     onEvent,
	}
}

// ListRunning returns currently executing tasks.
func (r *Runner) ListRunning() []*RunningTask {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*RunningTask
	for _, rt := range r.running {
		result = append(result, rt)
	}
	return result
}

// IsRunning checks if a task is currently executing.
func (r *Runner) IsRunning(taskID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.running[taskID]
	return ok
}

// RunningCount returns number of active executions.
func (r *Runner) RunningCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.running)
}

// Execute spawns an autonomous agent for a task.
func (r *Runner) Execute(t *Task, manifestTitle, manifestContent, visceralRules string) error {
	if r.RunningCount() >= r.maxParallel {
		return fmt.Errorf("max parallel tasks reached (%d)", r.maxParallel)
	}
	if r.IsRunning(t.ID) {
		return fmt.Errorf("task already running")
	}

	// Build the prompt
	prompt := buildPrompt(t, manifestTitle, manifestContent, visceralRules)

	// Build allowed tools list — OpenPraxis MCP tools + standard tools
	allowedTools := []string{
		"Bash", "Read", "Write", "Edit", "Glob", "Grep",
		"mcp__openpraxis__memory_store",
		"mcp__openpraxis__memory_search",
		"mcp__openpraxis__memory_recall",
		"mcp__openpraxis__visceral_rules",
		"mcp__openpraxis__visceral_confirm",
		"mcp__openpraxis__manifest_get",
		"mcp__openpraxis__conversation_save",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)

	maxTurns := t.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 50
	}

	args := []string{
		"-p", prompt,
		"--output-format", "stream-json",
		"--verbose",
		"--max-turns", fmt.Sprintf("%d", maxTurns),
		"--allowedTools", strings.Join(allowedTools, ","),
	}

	cmd := exec.CommandContext(ctx, "claude", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("start agent: %w", err)
	}

	marker := t.ID
	if len(marker) >= 12 {
		marker = marker[:12]
	}

	rt := &RunningTask{
		TaskID:    t.ID,
		Marker:    marker,
		Title:     t.Title,
		Manifest:  manifestTitle,
		Agent:     t.Agent,
		PID:       cmd.Process.Pid,
		StartedAt: time.Now(),
		Output:    make([]string, 0, 200),
		cancel:    cancel,
		cmd:       cmd,
	}

	r.mu.Lock()
	r.running[t.ID] = rt
	r.mu.Unlock()

	if err := r.store.UpdateStatus(t.ID, "running"); err != nil {
		slog.Error("update status to running failed", "component", "runner", "marker", marker, "error", err)
	}

	// Persist runtime state to SQLite — survives restarts
	if err := r.store.SaveRuntimeState(t.ID, marker, t.Title, manifestTitle, t.Agent, cmd.Process.Pid, false, 0, 0, "", rt.StartedAt); err != nil {
		slog.Error("save runtime state failed", "component", "runner", "marker", marker, "error", err)
	}

	if r.onEvent != nil {
		r.onEvent("task_started", map[string]string{
			"task_id": t.ID, "title": t.Title, "manifest": manifestTitle,
		})
	}

	slog.Info("task started", "component", "runner", "marker", marker, "title", t.Title, "pid", cmd.Process.Pid)

	// Read output in background
	go func() {
		defer func() {
			r.mu.Lock()
			delete(r.running, t.ID)
			r.mu.Unlock()
			cancel()
			// Clean up persisted runtime state — task is done
			if err := r.store.DeleteRuntimeState(t.ID); err != nil {
				slog.Error("delete runtime state failed", "component", "runner", "marker", marker, "error", err)
			}
		}()

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB line buffer
		var allOutput strings.Builder

		// Track pending tool_use blocks to pair with tool_result
		pendingTools := make(map[string]pendingToolCall) // keyed by tool_use ID

		for scanner.Scan() {
			line := scanner.Text()
			rt.Lines++

			// Keep last 200 lines
			if len(rt.Output) >= 200 {
				rt.Output = rt.Output[1:]
			}
			rt.Output = append(rt.Output, line)
			rt.LastLine = line
			allOutput.WriteString(line)
			allOutput.WriteString("\n")

			// Parse stream-json events
			var event map[string]any
			if err := json.Unmarshal([]byte(line), &event); err == nil {
				eventType, _ := event["type"].(string)

				if eventType == "assistant" {
					// Extract tool_use blocks from assistant message
					if msg, ok := event["message"].(map[string]any); ok {
						if content, ok := msg["content"].([]any); ok {
							for _, block := range content {
								if bm, ok := block.(map[string]any); ok {
									if bm["type"] == "tool_use" {
										rt.Actions++
										toolName, _ := bm["name"].(string)
										toolID, _ := bm["id"].(string)
										toolInput := marshalJSON(bm["input"])

										// Record action immediately (response filled when tool_result arrives)
										var actionID int64
										if r.actions != nil {
											var recErr error
											actionID, recErr = r.actions.RecordForTask(t.ID, t.SourceNode, toolName, toolInput, "", "")
											if recErr != nil {
												slog.Error("record action failed", "component", "runner", "marker", marker, "error", recErr)
											}
										}

										// Store pending for pairing with result (keyed by tool_use ID)
										pendingTools[toolID] = pendingToolCall{
											name:     toolName,
											input:    toolInput,
											actionID: actionID,
										}

										if r.onEvent != nil {
											r.onEvent("task_action", map[string]string{
												"task_id": t.ID, "tool": toolName,
											})
										}
									}
								}
							}
						}
					}
				}

				// Tool results come in "user" events (message.content[].type == "tool_result")
				if eventType == "user" {
					if msg, ok := event["message"].(map[string]any); ok {
						if content, ok := msg["content"].([]any); ok {
							for _, block := range content {
								if bm, ok := block.(map[string]any); ok {
									if bm["type"] == "tool_result" {
										toolUseID, _ := bm["tool_use_id"].(string)
										resultContent := marshalJSON(bm["content"])
										if pending, exists := pendingTools[toolUseID]; exists {
											if r.actions != nil && pending.actionID > 0 {
												if err := r.actions.UpdateResponseByID(pending.actionID, resultContent); err != nil {
													slog.Error("update action response failed", "component", "runner", "marker", marker, "error", err)
												}
											}
											delete(pendingTools, toolUseID)
										}
									}
								}
							}
						}
					}
				}

				if eventType == "result" {
					if r.onEvent != nil {
						r.onEvent("task_progress", map[string]string{
							"task_id": t.ID, "line": "Task completed",
						})
					}
				}
			}

			// Broadcast progress
			if rt.Lines%5 == 0 && r.onEvent != nil {
				r.onEvent("task_progress", map[string]string{
					"task_id": t.ID, "actions": fmt.Sprintf("%d", rt.Actions),
				})
			}

			// Persist runtime state every 10 lines
			if rt.Lines%10 == 0 {
				if err := r.store.UpdateRuntimeState(t.ID, rt.Actions, rt.Lines, rt.LastLine, rt.Paused); err != nil {
					slog.Error("update runtime state failed", "component", "runner", "marker", marker, "error", err)
				}
			}
		}

		// Wait for process to finish
		err := cmd.Wait()
		output := allOutput.String()

		status := "completed"
		if err != nil {
			// Check if the result JSON contains terminal_reason: "max_turns"
			if detectMaxTurns(output) {
				status = "completed"
				slog.Info("task hit max turns limit", "component", "runner", "marker", marker, "actions", rt.Actions, "lines", rt.Lines)
			} else {
				status = "failed"
				slog.Error("task failed", "component", "runner", "marker", marker, "error", err)
			}
		} else {
			// Also check completed output — some versions don't error on max_turns
			if detectMaxTurns(output) {
				status = "completed"
				slog.Info("task hit max turns limit", "component", "runner", "marker", marker, "actions", rt.Actions, "lines", rt.Lines)
			} else {
				slog.Info("task completed", "component", "runner", "marker", marker, "actions", rt.Actions, "lines", rt.Lines)
			}
		}

		// Parse cost from the result event
		costUSD, numTurns := ParseCostFromOutput(output)

		// Record the run with history — always use real status, not "scheduled"
		if err := r.store.RecordRun(t.ID, output, status, rt.Actions, rt.Lines, costUSD, numTurns, rt.StartedAt); err != nil {
			slog.Error("record run failed", "component", "runner", "marker", marker, "error", err)
		}
		if !IsOneShot(t.Schedule) {
			// Compute next run for recurring
			nextRun := ComputeNextRun(t.Schedule)
			if !nextRun.IsZero() {
				if err := r.store.SetNextRun(t.ID, nextRun.Format(time.RFC3339)); err != nil {
					slog.Error("set next run failed", "component", "runner", "marker", marker, "error", err)
				}
			}
		}

		// Activate dependent tasks
		if status == "completed" || status == "max_turns" {
			activated, _ := r.store.ActivateDependents(t.ID)
			if activated > 0 {
				slog.Info("activated dependent tasks", "component", "runner", "marker", marker, "count", activated)
			}
		}

		if r.onEvent != nil {
			r.onEvent("task_completed", map[string]string{
				"task_id": t.ID, "status": status,
			})
		}
	}()

	return nil
}

// pendingToolCall tracks a tool_use awaiting its tool_result.
type pendingToolCall struct {
	name     string
	input    string
	actionID int64 // row ID in actions table for precise response pairing
}

// marshalJSON serializes any value to JSON string.
func marshalJSON(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// detectMaxTurns checks if the stream-json output contains a result with terminal_reason "max_turns".
func detectMaxTurns(output string) bool {
	// Scan from the end — result event is typically the last JSON line
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0 && i >= len(lines)-20; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if event["type"] == "result" {
			// Check terminal_reason first (most reliable)
			if reason, ok := event["terminal_reason"].(string); ok && reason == "max_turns" {
				return true
			}
			// Check subtype — can be "max_turns" or "error_max_turns"
			if reason, ok := event["subtype"].(string); ok && strings.Contains(reason, "max_turns") {
				return true
			}
			if reason, ok := event["stop_reason"].(string); ok && reason == "max_turns" {
				return true
			}
		}
	}
	return false
}

// Kill force-stops a running task.
func (r *Runner) Kill(taskID string) error {
	r.mu.RLock()
	rt, ok := r.running[taskID]
	r.mu.RUnlock()
	if !ok {
		return fmt.Errorf("task not running")
	}

	slog.Info("killing task", "component", "runner", "marker", rt.Marker)
	rt.cancel()
	if rt.cmd != nil && rt.cmd.Process != nil {
		if err := rt.cmd.Process.Kill(); err != nil {
			slog.Error("kill process failed", "component", "runner", "marker", rt.Marker, "error", err)
		}
	}
	if err := r.store.UpdateStatus(taskID, "cancelled"); err != nil {
		slog.Error("update status to cancelled failed", "component", "runner", "marker", rt.Marker, "error", err)
	}

	r.mu.Lock()
	delete(r.running, taskID)
	r.mu.Unlock()

	if r.onEvent != nil {
		r.onEvent("task_killed", map[string]string{"task_id": taskID})
	}
	return nil
}

// Pause sends SIGSTOP to a running task's process, freezing the agent.
func (r *Runner) Pause(taskID string) error {
	r.mu.RLock()
	rt, ok := r.running[taskID]
	r.mu.RUnlock()
	if !ok {
		return fmt.Errorf("task not running")
	}
	if rt.Paused {
		return fmt.Errorf("task already paused")
	}
	if rt.cmd == nil || rt.cmd.Process == nil {
		return fmt.Errorf("no process to pause")
	}

	slog.Info("pausing task", "component", "runner", "marker", rt.Marker, "pid", rt.PID)
	if err := rt.cmd.Process.Signal(syscall.SIGSTOP); err != nil {
		return fmt.Errorf("SIGSTOP failed: %w", err)
	}

	rt.Paused = true
	if err := r.store.UpdateStatus(taskID, "paused"); err != nil {
		slog.Error("update status to paused failed", "component", "runner", "marker", rt.Marker, "error", err)
	}
	if err := r.store.UpdateRuntimeState(taskID, rt.Actions, rt.Lines, rt.LastLine, true); err != nil {
		slog.Error("update runtime state failed", "component", "runner", "marker", rt.Marker, "state", "paused", "error", err)
	}

	if r.onEvent != nil {
		r.onEvent("task_paused", map[string]string{"task_id": taskID})
	}
	return nil
}

// Resume sends SIGCONT to a paused task's process, resuming the agent.
func (r *Runner) Resume(taskID string) error {
	r.mu.RLock()
	rt, ok := r.running[taskID]
	r.mu.RUnlock()
	if !ok {
		return fmt.Errorf("task not in running map")
	}
	if !rt.Paused {
		return fmt.Errorf("task is not paused")
	}
	if rt.cmd == nil || rt.cmd.Process == nil {
		return fmt.Errorf("no process to resume")
	}

	slog.Info("resuming task", "component", "runner", "marker", rt.Marker, "pid", rt.PID)
	if err := rt.cmd.Process.Signal(syscall.SIGCONT); err != nil {
		return fmt.Errorf("SIGCONT failed: %w", err)
	}

	rt.Paused = false
	if err := r.store.UpdateStatus(taskID, "running"); err != nil {
		slog.Error("update status to running failed", "component", "runner", "marker", rt.Marker, "error", err)
	}
	if err := r.store.UpdateRuntimeState(taskID, rt.Actions, rt.Lines, rt.LastLine, false); err != nil {
		slog.Error("update runtime state failed", "component", "runner", "marker", rt.Marker, "state", "resumed", "error", err)
	}

	if r.onEvent != nil {
		r.onEvent("task_resumed", map[string]string{"task_id": taskID})
	}
	return nil
}

// GetOutput returns the recent output lines for a running task.
func (r *Runner) GetOutput(taskID string) ([]string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rt, ok := r.running[taskID]
	if !ok {
		return nil, false
	}
	return rt.Output, true
}

func buildPrompt(t *Task, manifestTitle, manifestContent, visceralRules string) string {
	var b strings.Builder
	b.WriteString("You are executing a scheduled task for OpenPraxis.\n\n")

	if visceralRules != "" {
		b.WriteString("## Visceral Rules (MANDATORY — follow without exception)\n")
		b.WriteString(visceralRules)
		b.WriteString("\n\n")
	}

	b.WriteString(fmt.Sprintf("## Manifest: %s\n", manifestTitle))
	b.WriteString(manifestContent)
	b.WriteString("\n\n")

	b.WriteString(fmt.Sprintf("## Task: %s\n", t.Title))
	if t.Description != "" {
		b.WriteString(t.Description)
		b.WriteString("\n")
	}

	b.WriteString("\n## Instructions\n")
	b.WriteString("Follow the manifest spec exactly. Work autonomously.\n")
	b.WriteString("Call visceral_rules and visceral_confirm first.\n\n")

	// Git isolation: each task gets its own branch and PR
	marker := t.ID
	if len(marker) >= 12 {
		marker = marker[:12]
	}
	b.WriteString("## Git Workflow (MANDATORY)\n")
	b.WriteString(fmt.Sprintf("1. Before making ANY code changes, create a new branch:\n"))
	b.WriteString(fmt.Sprintf("   git checkout -b openpraxis/%s\n", marker))
	b.WriteString("2. Make all your changes on this branch.\n")
	b.WriteString("3. Commit your work with a descriptive message.\n")
	b.WriteString(fmt.Sprintf("4. Push the branch: git push -u origin openpraxis/%s\n", marker))
	b.WriteString("5. Create a pull request using: gh pr create --title \"<title>\" --body \"<summary>\"\n")
	b.WriteString("6. Include the PR URL in your final output.\n")
	b.WriteString("NEVER work on an existing branch. NEVER push to main. Each task gets its own branch and PR.\n\n")

	b.WriteString("Report completion when done.\n")

	return b.String()
}
