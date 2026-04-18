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

	"github.com/k8nstantin/OpenPraxis/internal/action"
	"github.com/k8nstantin/OpenPraxis/internal/settings"
)

// RunningTask tracks an actively executing task.
type RunningTask struct {
	TaskID    string    `json:"task_id"`
	Marker    string    `json:"marker"`
	Title     string    `json:"title"`
	Manifest  string    `json:"manifest"`
	// ProductID is the resolved product scope for the task — empty for
	// standalone tasks with no manifest. Used by the per-product dispatch cap.
	ProductID string    `json:"product_id"`
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
//
// The max_parallel dispatch cap is resolved per-task at Execute time via the
// settings Resolver: it walks task → manifest → product → system so two
// products can have different caps. Standalone tasks (no manifest/product)
// fall through to the catalog system default.
type Runner struct {
	store    *Store
	actions  *action.Store
	resolver *settings.Resolver
	running  map[string]*RunningTask
	mu       sync.RWMutex
	onEvent  func(event string, data map[string]string) // broadcast callback
}

// NewRunner creates a task runner. The resolver is required — dispatch caps
// are looked up through it on every Execute call.
func NewRunner(store *Store, actions *action.Store, resolver *settings.Resolver, onEvent func(string, map[string]string)) *Runner {
	return &Runner{
		store:    store,
		actions:  actions,
		resolver: resolver,
		running:  make(map[string]*RunningTask),
		onEvent:  onEvent,
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

// RunningCountForProduct returns the number of active executions whose
// resolved ProductID matches productID. Standalone tasks (ProductID == "")
// share their own pool — pass "" to count them.
func (r *Runner) RunningCountForProduct(productID string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := 0
	for _, rt := range r.running {
		if rt.ProductID == productID {
			n++
		}
	}
	return n
}

// resolveMaxParallel walks settings task → manifest → product → system for
// the max_parallel knob at the task's scope. Returns the normalized scope
// (with ManifestID/ProductID auto-filled) and the int cap. Extracted so the
// dispatch gate is unit-testable without spawning an agent.
func (r *Runner) resolveMaxParallel(ctx context.Context, taskID string) (settings.Scope, int, error) {
	if r.resolver == nil {
		return settings.Scope{}, 0, fmt.Errorf("runner has no settings resolver")
	}
	scope, err := r.resolver.NormalizeScope(ctx, settings.Scope{TaskID: taskID})
	if err != nil {
		return scope, 0, fmt.Errorf("normalize scope: %w", err)
	}
	resolved, err := r.resolver.Resolve(ctx, scope, "max_parallel")
	if err != nil {
		return scope, 0, fmt.Errorf("resolve max_parallel: %w", err)
	}
	cap, err := resolvedInt(resolved.Value)
	if err != nil {
		return scope, 0, fmt.Errorf("max_parallel: %w", err)
	}
	return scope, cap, nil
}

// resolvedInt coerces a resolver-returned Value to a Go int. The resolver
// decodes explicit settings rows as int64 (encoding/json round-trip), while
// system-default fallthrough returns the catalog's raw Go int. We accept both
// plus float64 for defensive symmetry with any future catalog changes.
func resolvedInt(v interface{}) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		return int(n), nil
	}
	return 0, fmt.Errorf("expected int value, got %T", v)
}

// Execute spawns an autonomous agent for a task.
func (r *Runner) Execute(t *Task, manifestTitle, manifestContent, visceralRules string) error {
	if r.IsRunning(t.ID) {
		return fmt.Errorf("task already running")
	}

	// Resolve per-product cap via the settings walker. Runs BEFORE we spawn
	// so a cap breach returns an error instead of starting a process we'd
	// immediately have to kill. A background context is fine — the lookups
	// are cheap and the task's own context is built below.
	scope, cap, err := r.resolveMaxParallel(context.Background(), t.ID)
	if err != nil {
		return err
	}
	if r.RunningCountForProduct(scope.ProductID) >= cap {
		return fmt.Errorf("max parallel tasks reached for product %s (%d)", scope.ProductID, cap)
	}

	// Build the prompt
	prompt := buildPrompt(t, manifestTitle, manifestContent, visceralRules)

	// Build allowed tools list — OpenPraxis MCP tools + standard tools.
	// settings_set is deliberately NOT allowlisted: agents should consult their
	// own budgets via settings_get/resolve but must not mutate them mid-run.
	allowedTools := []string{
		"Bash", "Read", "Write", "Edit", "Glob", "Grep",
		"mcp__openpraxis__memory_store",
		"mcp__openpraxis__memory_search",
		"mcp__openpraxis__memory_recall",
		"mcp__openpraxis__visceral_rules",
		"mcp__openpraxis__visceral_confirm",
		"mcp__openpraxis__manifest_get",
		"mcp__openpraxis__conversation_save",
		"mcp__openpraxis__settings_get",
		"mcp__openpraxis__settings_resolve",
		"mcp__openpraxis__settings_catalog",
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
		ProductID: scope.ProductID,
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

		// Dependent-task activation is deferred to the watcher audit path
		// (cmd/serve.go). Activating here races the audit: a task the runner
		// marks "completed" can be downgraded to "failed" by the 5s-later
		// watcher check, but any dependents activated in the interim would
		// already be scheduled/running.

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

// Cancel kills a running task's process, marks it cancelled.
// For tasks that are not in the runner's in-memory map (e.g. queued scheduled),
// it just sets the status — the scheduler will skip cancelled tasks.
func (r *Runner) Cancel(taskID string) error {
	r.mu.RLock()
	rt, ok := r.running[taskID]
	r.mu.RUnlock()
	if ok && rt.cmd != nil && rt.cmd.Process != nil {
		slog.Info("cancelling task — killing process", "component", "runner", "marker", rt.Marker, "pid", rt.PID)
		if rt.Paused {
			_ = rt.cmd.Process.Signal(syscall.SIGCONT)
		}
		if err := rt.cmd.Process.Kill(); err != nil {
			slog.Error("kill process failed", "component", "runner", "marker", rt.Marker, "error", err)
		}
		if rt.cancel != nil {
			rt.cancel()
		}
	}
	if err := r.store.UpdateStatus(taskID, "cancelled"); err != nil {
		return fmt.Errorf("update status to cancelled: %w", err)
	}
	if r.onEvent != nil {
		r.onEvent("task_cancelled", map[string]string{"task_id": taskID})
	}
	return nil
}

// StartActionWatcher polls the tasks table for cross-process action_request signals
// (pause/resume/cancel) and applies them to tasks this runner owns. Safe to call once
// from serve after InitRunner.
func (r *Runner) StartActionWatcher(interval time.Duration) {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for range t.C {
			reqs, err := r.store.ListActionRequests()
			if err != nil {
				slog.Error("list action requests failed", "component", "runner", "error", err)
				continue
			}
			for _, req := range reqs {
				switch req.Action {
				case "pause":
					if err := r.Pause(req.TaskID); err != nil {
						slog.Warn("pause action failed", "component", "runner", "task_id", req.TaskID, "error", err)
					}
				case "resume":
					if err := r.Resume(req.TaskID); err != nil {
						slog.Warn("resume action failed", "component", "runner", "task_id", req.TaskID, "error", err)
					}
				case "cancel":
					if err := r.Cancel(req.TaskID); err != nil {
						slog.Warn("cancel action failed", "component", "runner", "task_id", req.TaskID, "error", err)
					}
				default:
					slog.Warn("unknown action request", "component", "runner", "task_id", req.TaskID, "action", req.Action)
				}
				if err := r.store.ClearActionRequest(req.TaskID); err != nil {
					slog.Error("clear action request failed", "component", "runner", "task_id", req.TaskID, "error", err)
				}
			}
		}
	}()
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
