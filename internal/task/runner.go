package task

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/k8nstantin/OpenPraxis/internal/action"
	"github.com/k8nstantin/OpenPraxis/internal/comments"
	"github.com/k8nstantin/OpenPraxis/internal/entity"
	execution "github.com/k8nstantin/OpenPraxis/internal/execution"
	"github.com/k8nstantin/OpenPraxis/internal/relationships"
	"github.com/k8nstantin/OpenPraxis/internal/schedule"
	"github.com/k8nstantin/OpenPraxis/internal/settings"
	"github.com/k8nstantin/OpenPraxis/internal/templates"
)

// Event name and key constants for the onEvent broadcast callback.
// Using constants prevents typo-induced bugs when reading map keys in
// callers (e.g. the DAG chain gate in cmd/serve.go).
const (
	EventTaskCompleted = "task_completed"
	EventTaskStarted   = "task_started"
	EventTaskKilled    = "task_killed"
	EventTaskCancelled = "task_cancelled"
	EventTaskPaused    = "task_paused"
	EventTaskResumed   = "task_resumed"
	EventTaskProgress  = "task_progress"

	EventKeyTaskID = "task_id"
	EventKeyStatus = "status"
	EventKeyReason = "reason"
)

// RunningTask tracks an actively executing task.
type RunningTask struct {
	TaskID    string    `json:"task_id"`
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
	Output            []string `json:"-"` // ring buffer, not serialized
	// Model is the model id reported by the first assistant event — used to
	// pick a pricing table and for calibration after the run.
	Model string `json:"model"`
	// mu protects fields written by the Execute goroutine and read by
	// concurrent HTTP handlers: usageByMessage and Actions.
	mu             sync.RWMutex
	// usageByMessage tracks the last-seen usage per message id so we can
	// dedupe the repeated assistant events Claude Code emits while a single
	// logical message streams. Not serialized; live-estimate only.
	usageByMessage map[string]Usage
	cancel         context.CancelFunc
	cmd            *exec.Cmd
}

// Runner manages task execution — spawning agents and tracking running tasks.
//
// Every knob that shapes a task's execution (max_parallel, max_turns, timeout,
// model, agent, temperature, reasoning_effort, max_cost_usd, daily_budget_usd,
// retry_on_failure, approval_mode, allowed_tools) is resolved per-task at
// Execute time via the settings Resolver: it walks task → manifest → product
// → system so two products can have different caps. Standalone tasks (no
// manifest/product) fall through to the catalog system defaults.
type Runner struct {
	store    *Store
	actions  *action.Store
	resolver *settings.Resolver
	running  map[string]*RunningTask
	mu       sync.RWMutex
	onEvent  func(event string, data map[string]string) // broadcast callback

	// repoDir is the git repository root. Tasks execute in per-task worktrees
	// anchored to this repo via `git worktree add`, so each task starts from
	// a fresh checkout of origin/main and the operator's own working copy is
	// never touched. Empty means the runner will call os.Getwd() on the first
	// Execute — preserves old behavior for code paths that don't set it.
	repoDir string

	// warnOnce guards per-Runner log-once semantics for unsupported agent
	// flags (e.g. --temperature on Claude Code). Keyed by "<agent>:<knob>".
	warnOnce sync.Map

	// execReview is the optional post-completion checker for
	// comment comments. When wired, tasks that finish with
	// status=completed/reason=success are inspected — if no agent-authored
	// comment comment exists on the task, an amnesia flag is
	// recorded so the gap is visible on the dashboard. Leaving it nil
	// disables the check (existing behavior).
	execReview ExecutionReviewChecker

	// sourceNode is the originating node UUID used when recording amnesia
	// flags from the post-completion path. Empty is fine — amnesia.task_id
	// is what the task detail pulls on.
	sourceNode string

	// tmpl resolves per-section prompt bodies by walking
	// task → manifest → product → agent → system. Nil falls back to the
	// package defaults (existing behaviour + every existing test harness
	// that constructs a Runner without a DB-backed templates store).
	tmpl *templates.Resolver

	// hostSampler polls the serve process CPU/RSS and attributes each
	// sample to every task attached during its run. Nil → host metrics
	// are skipped (tests + pre-wire code paths). Wired via
	// SetHostSampler from cmd/serve.go at boot.
	hostSampler *HostSampler

	// entityStore is the SCD-2 entity store used for status updates
	// alongside the legacy task store. Nil → entity status updates are
	// skipped (pre-wire code paths).
	entityStore *entity.Store

	// executionLog is the unified run-history store. When non-nil, each
	// completed run is also recorded here alongside the legacy task_runs
	// table.
	executionLog *execution.Store

	// commentsStore is used to fetch the latest prompt
	// comment for a task when building the prompt. Nil → falls back to
	// task.Description from the tasks row.
	commentsStore *comments.Store

	// relsStore is the unified SCD-2 relationships store. The runner uses
	// it directly (rather than indirecting through r.store) because the
	// task package's legacy *Store is wired as nil in production — the
	// runner is fed relationships through SetRelationships at boot. Nil
	// disables the proposer trigger path.
	relsStore *relationships.Store

	// scheduleStore is the schedules table backing the cron-driven
	// dispatcher. The proposer trigger path inserts a one-shot schedule
	// row to fire the auto-created proposer task. Nil disables the
	// proposer trigger path.
	scheduleStore *schedule.Store
}

// SetHostSampler wires a started HostSampler onto the runner. Attach is
// called at spawn, Detach at completion; the accumulated samples land
// on task_run_host_samples + summary columns on task_runs.
func (r *Runner) SetHostSampler(hs *HostSampler) { r.hostSampler = hs }

// SetEntityStore wires the SCD-2 entity store onto the runner so status
// transitions are recorded in both the legacy tasks table and the entity
// store. Call once at startup before any Execute call.
func (r *Runner) SetEntityStore(es *entity.Store) { r.entityStore = es }

// SetExecutionLog wires the unified execution log store onto the runner.
// When set, each completed run is appended to execution_log alongside the
// legacy task_runs table.
func (r *Runner) SetExecutionLog(el *execution.Store) { r.executionLog = el }

// SetCommentsStore wires the comments store onto the runner so the prompt
// builder can fetch the latest prompt for a task. When nil,
// the runner uses task.Description from the tasks row directly.
func (r *Runner) SetCommentsStore(cs *comments.Store) { r.commentsStore = cs }

// SetRelationships wires the unified relationships store onto the runner.
// Nil disables any code path that requires DAG traversal (currently the
// proposer trigger path). Call once at startup.
func (r *Runner) SetRelationships(rs *relationships.Store) { r.relsStore = rs }

// SetScheduleStore wires the schedules table onto the runner so the
// proposer trigger path can enqueue a one-shot schedule for a freshly
// minted proposer task. Nil disables the proposer trigger path.
func (r *Runner) SetScheduleStore(ss *schedule.Store) { r.scheduleStore = ss }

// SetTemplateResolver wires the RC/M1 prompt-template resolver onto
// the runner. Nil disables scope-aware resolution — the runner uses the
// package defaults, preserving pre-RC/M1 byte-for-byte output.
func (r *Runner) SetTemplateResolver(t *templates.Resolver) { r.tmpl = t }

// ExecutionReviewChecker answers whether a task has at least one
// comment comment authored by the agent. Wired via
// SetExecutionReviewChecker; node.go adapts internal/comments.Store to
// this shape so the task package stays free of a comments import.
type ExecutionReviewChecker interface {
	HasAgentExecutionReview(ctx context.Context, taskID string) (bool, error)
}

// SetExecutionReviewChecker wires the post-completion comment
// gate. Nil disables it.
func (r *Runner) SetExecutionReviewChecker(c ExecutionReviewChecker) {
	r.execReview = c
}

// SetSourceNode records the node UUID used when the runner writes amnesia
// flags from its own code path (e.g. missing comment).
func (r *Runner) SetSourceNode(id string) { r.sourceNode = id }

// enforceExecutionReview runs the M4-T10 post-completion gate. Only fires
// for status=completed/reason=success; skips entirely when the checker is
// not wired. Extracted from Execute so it can be tested directly.
func (r *Runner) enforceExecutionReview(bgCtx context.Context, taskID, status, reason string) {
	if r.execReview == nil {
		return
	}
	if status != "completed" || reason != "success" {
		return
	}
	checkCtx, checkCancel := context.WithTimeout(bgCtx, 5*time.Second)
	has, checkErr := r.execReview.HasAgentExecutionReview(checkCtx, taskID)
	checkCancel()
	switch {
	case checkErr != nil:
		slog.Warn("execution review lookup failed",
			"component", "runner", "task_id", taskID, "error", checkErr)
	case !has:
		slog.Warn("task completed but no comment comment — agent forgot the closing call",
			"component", "runner", "task_id", taskID)
		if r.actions != nil {
			if amnErr := r.actions.RecordAmnesia(
				"",            // sessionID
				r.sourceNode,  // sourceNode
				"",            // actionID
				taskID,        // taskID
				"exec-review", // ruleID (synthetic)
				"exec-review", // ruleMarker
				"Every completed task must post an comment comment via comment_add before finishing",
				"",  // toolName
				"",  // toolInput
				1.0, // score (max — the call was definitely missing)
				"rule",
				"missing_comment",
			); amnErr != nil {
				slog.Warn("record exec-review amnesia failed",
					"component", "runner", "task_id", taskID, "error", amnErr)
			}
		}
	}
}

// proposerStreakKey is the settings-table key under which the runner
// persists the per-manifest non-transient-failure streak counter for the
// proposer trigger. Underscore prefix marks it as internal runtime state
// (mirrors retryCountKey above) — operators do not edit this directly.
const proposerStreakKey = "_proposer_failure_streak"

// proposerTaskTitle is the entity title used when the runner auto-creates
// a meta-harness proposer task. Centralised so dashboards / tests can
// match against one canonical string.
const proposerTaskTitle = "meta-harness proposer"

// checkProposerTriggers updates the per-manifest non-transient-failure
// streak after a terminal run and, when either the streak or the cost
// threshold trips, auto-fires a meta-harness proposer task under the
// owning manifest. Wired off the ProposerEnabled knob; nil store fields
// short-circuit so the path is safe to invoke even on partially wired
// runners (tests / boot order).
//
// Failure classification is single-source-of-truth: transient failures
// (timeout / process_error / build_fail) reset the streak, non-transient
// failures (max_turns / deliverable_missing) advance it, and any other
// reason — success included — leaves the counter unchanged so a single
// successful run does NOT clear a building streak. The proposer fires
// at-or-above the configured streak length OR strictly above the cost
// threshold; on fire the streak resets so the proposer doesn't re-fire
// on the very next run.
func (r *Runner) checkProposerTriggers(ctx context.Context, t *Task, reason string, row *execution.Row, knobs runtimeKnobs) {
	if r.relsStore == nil || r.entityStore == nil || r.scheduleStore == nil {
		return
	}
	if r.resolver == nil || r.resolver.Store() == nil {
		return
	}

	// Find the parent manifest via the SCD-2 relationships graph (never
	// the legacy tasks.manifest_id column — it's been removed). A task
	// without a manifest parent has no scope to score, so the proposer
	// can't act on it; bail.
	edges, err := r.relsStore.ListIncoming(ctx, t.ID, relationships.EdgeOwns)
	if err != nil {
		slog.Warn("proposer trigger: list incoming edges failed",
			"component", "runner", "task_id", t.ID, "error", err)
		return
	}
	manifestID := ""
	for _, e := range edges {
		if e.SrcKind == relationships.KindManifest {
			manifestID = e.SrcID
			break
		}
	}
	if manifestID == "" {
		return
	}

	// Streak counter at manifest scope — survives runs of any task under
	// the manifest, which is the unit the proposer cares about.
	settingsStore := r.resolver.Store()
	streak := 0
	if entry, gerr := settingsStore.Get(ctx, settings.ScopeManifest, manifestID, proposerStreakKey); gerr == nil && entry.Value != "" {
		if uerr := json.Unmarshal([]byte(entry.Value), &streak); uerr != nil {
			slog.Warn("proposer trigger: decode streak failed, treating as 0",
				"component", "runner", "manifest_id", manifestID, "raw", entry.Value, "error", uerr)
			streak = 0
		}
	}
	switch reason {
	case execution.TerminalReasonMaxTurns, execution.TerminalReasonDeliverableMissing:
		streak++
	case execution.TerminalReasonTimeout, execution.TerminalReasonProcessError, execution.TerminalReasonBuildFail:
		streak = 0
	}
	if raw, merr := json.Marshal(streak); merr == nil {
		if serr := settingsStore.Set(ctx, settings.ScopeManifest, manifestID, proposerStreakKey, string(raw), "runner"); serr != nil {
			slog.Warn("proposer trigger: persist streak failed",
				"component", "runner", "manifest_id", manifestID, "error", serr)
		}
	}

	streakFired := knobs.ProposerTriggerFailureStreak > 0 && streak >= knobs.ProposerTriggerFailureStreak
	costFired := knobs.ProposerTriggerCostUSD > 0 && row != nil && row.EstimatedCostUSD > knobs.ProposerTriggerCostUSD
	if !streakFired && !costFired {
		return
	}

	// Reset the streak so a back-to-back failure on the very next run
	// doesn't re-fire the proposer before the previous one has had a
	// chance to land.
	if raw, merr := json.Marshal(0); merr == nil {
		if serr := settingsStore.Set(ctx, settings.ScopeManifest, manifestID, proposerStreakKey, string(raw), "runner"); serr != nil {
			slog.Warn("proposer trigger: reset streak failed",
				"component", "runner", "manifest_id", manifestID, "error", serr)
		}
	}

	proposer, err := r.entityStore.Create(entity.TypeTask, proposerTaskTitle, entity.StatusDraft, nil, "runner", "auto-fired by proposer trigger")
	if err != nil {
		slog.Warn("proposer trigger: create entity failed",
			"component", "runner", "manifest_id", manifestID, "error", err)
		return
	}
	if err := r.relsStore.Create(ctx, relationships.Edge{
		SrcKind:   relationships.KindManifest,
		SrcID:     manifestID,
		DstKind:   relationships.KindTask,
		DstID:     proposer.EntityUID,
		Kind:      relationships.EdgeOwns,
		CreatedBy: "runner",
		Reason:    "proposer auto-fired",
	}); err != nil {
		slog.Warn("proposer trigger: create owns edge failed",
			"component", "runner", "manifest_id", manifestID,
			"proposer_id", proposer.EntityUID, "error", err)
	}

	triggerReason := "failure streak"
	if !streakFired {
		triggerReason = "cost threshold"
	}
	if _, err := r.scheduleStore.Create(ctx, &schedule.Schedule{
		EntityKind: schedule.KindTask,
		EntityID:   proposer.EntityUID,
		RunAt:      time.Now().UTC().Format(time.RFC3339),
		CreatedBy:  "runner",
		Reason:     "proposer trigger: " + triggerReason,
	}); err != nil {
		slog.Warn("proposer trigger: schedule failed",
			"component", "runner", "manifest_id", manifestID,
			"proposer_id", proposer.EntityUID, "error", err)
	}
}

// NewRunner creates a task runner. The resolver is required — every knob that
// shapes task execution is looked up through it on every Execute call.
// repoDir is the git repo root tasks will clone worktrees from; pass "" to
// default to the server's process CWD at spawn time.
func NewRunner(store *Store, actions *action.Store, resolver *settings.Resolver, repoDir string, onEvent func(string, map[string]string)) *Runner {
	return &Runner{
		store:    store,
		actions:  actions,
		resolver: resolver,
		running:  make(map[string]*RunningTask),
		onEvent:  onEvent,
		repoDir:  repoDir,
	}
}

// RecoverInFlight deals with tasks left in `running` or `paused` status from
// a prior `openpraxis serve` process. For each orphan it resolves the
// `on_restart_behavior` knob at the task's own scope (task → manifest →
// product → system) and applies one of:
//
//   - stop    — mark failed with a diagnostic reason, operator re-fires manually.
//   - restart — reset to scheduled with next_run_at=now so the scheduler picks
//               it up on its next tick.
//   - fail    — mark failed with no auto-recovery hint, requires explicit ack.
//
// Safe to call multiple times; a second call finds no rows in running/paused
// state and returns without touching the DB. Must run before the scheduler
// starts — otherwise a `restart`-eligible orphan races with the first tick.
func (r *Runner) RecoverInFlight(ctx context.Context) error {
	if r.store == nil {
		return nil
	}
	orphans, err := r.store.listOrphanRunningTasks()
	if err != nil {
		return fmt.Errorf("list orphan tasks: %w", err)
	}
	var errs []error
	for _, t := range orphans {
		if err := r.recoverOneOrphan(ctx, t.ID, 0, 0, 0, ""); err != nil {
			errs = append(errs, fmt.Errorf("recover task %s: %w", t.ID, err))
		}
	}
	if len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = e.Error()
		}
		return fmt.Errorf("recovery had %d error(s): %s", len(errs), strings.Join(msgs, "; "))
	}
	return nil
}

func (r *Runner) recoverOneOrphan(ctx context.Context, taskID string, pid, actions, lines int, startedAt string) error {
	behavior := "stop"
	if r.resolver != nil {
		scope, err := r.resolver.NormalizeScope(ctx, settings.Scope{TaskID: taskID})
		if err != nil {
			slog.Warn("recover: normalize scope failed, defaulting to stop",
				"component", "runner", "task_id", taskID, "error", err)
		} else {
			resolved, err := r.resolver.Resolve(ctx, scope, "on_restart_behavior")
			if err != nil {
				slog.Warn("recover: resolve on_restart_behavior failed, defaulting to stop",
					"component", "runner", "task_id", taskID, "error", err)
			} else if s, ok := resolved.Value.(string); ok && s != "" {
				// The resolver does not consult system-scope rows — it
				// falls through from product to the catalog default. If
				// the walk ended at system (catalog default), still
				// prefer an explicit system-scope row when present.
				if resolved.Source == settings.ScopeSystem && r.resolver.Store() != nil {
					if entry, gerr := r.resolver.Store().Get(ctx, settings.ScopeSystem, "", "on_restart_behavior"); gerr == nil && entry.Value != "" {
						var v string
						if jerr := json.Unmarshal([]byte(entry.Value), &v); jerr == nil && v != "" {
							s = v
						}
					}
				}
				behavior = s
			}
		}
	}

	switch behavior {
	case "restart":
		if err := r.store.RecoverAsScheduled(taskID, "serve restart: rescheduled per on_restart_behavior=restart"); err != nil {
			return fmt.Errorf("reschedule orphan: %w", err)
		}
		slog.Info("recover: orphan rescheduled", "task_id", taskID)
	case "fail":
		if err := r.store.RecoverAsFailed(taskID, "serve restart: no auto-recovery; operator must re-fire"); err != nil {
			return fmt.Errorf("mark failed (strict): %w", err)
		}
		slog.Info("recover: orphan marked failed (strict)", "task_id", taskID)
	default: // "stop"
		if err := r.store.RecoverAsFailed(taskID, "serve restart: task was still running when the server stopped"); err != nil {
			return fmt.Errorf("mark failed: %w", err)
		}
		slog.Info("recover: orphan marked failed", "task_id", taskID)
	}
	return nil
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

// LiveTurnCount returns the in-memory turn count for a running task — the
// number of distinct assistant message ids seen so far on its stdout. Updates
// immediately at each turn boundary (before task_turn_started fires), so
// dashboards polling /api/execution/live see the new value within ~1s instead
// of waiting for the next 5s execution_log sample. Returns (0,false) when the
// task is not currently in r.running.
func (r *Runner) LiveTurnCount(taskID string) (int, bool) {
	// Two-level lock: r.mu guards r.running (the map of RunningTasks),
	// rt.mu guards fields written by the Execute goroutine (usageByMessage, Actions).
	r.mu.RLock()
	rt, ok := r.running[taskID]
	r.mu.RUnlock()
	if !ok {
		return 0, false
	}
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return len(rt.usageByMessage), true
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

// resolvedFloat mirrors resolvedInt for float knobs (max_cost_usd,
// daily_budget_usd, temperature). Same resolver-decode vs system-default type
// asymmetry applies — the resolver returns float64 for explicit rows but
// catalog defaults may be typed as int in source. Defensive coercion keeps
// callers from panicking on a future catalog change.
func resolvedFloat(v interface{}) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	}
	return 0, fmt.Errorf("expected float value, got %T", v)
}

// resolvedStr pulls a string out of a resolver Value. Used for string/enum
// knobs (default_agent, default_model, reasoning_effort, approval_mode).
// Empty string is a valid value — callers distinguish "not set" from "" via
// the Resolved.Source field, not the value itself.
func resolvedStr(v interface{}) (string, error) {
	if v == nil {
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("expected string value, got %T", v)
	}
	return s, nil
}

// resolvedStrSlice decodes a multiselect knob (currently just allowed_tools)
// into []string. The resolver's decodeMultiselect already produces []string
// for explicit rows, but catalog defaults are typed []string in source too —
// accept both plus []interface{} for defensive symmetry.
func resolvedStrSlice(v interface{}) ([]string, error) {
	switch x := v.(type) {
	case []string:
		return x, nil
	case []interface{}:
		out := make([]string, 0, len(x))
		for i, item := range x {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("expected []string at index %d, got %T", i, item)
			}
			out = append(out, s)
		}
		return out, nil
	case nil:
		return nil, nil
	}
	return nil, fmt.Errorf("expected []string value, got %T", v)
}

// defaultAllowedTools is the baseline tool allowlist used when the resolver
// returns an empty slice. Mirrors the original hardcoded list plus the core
// Bash/Read/Write/Edit/Glob/Grep file tools — any narrowing happens by
// setting allowed_tools at product/manifest/task scope, not here.
func defaultAllowedTools() []string {
	return []string{
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
		// comment_add is referenced in the runner's mandatory closing
		// prompt — every task must be able to post its comment.
		"mcp__openpraxis__comment_add",
	}
}

// warnUnsupported logs a one-time warning per (agent, knob) pair when the
// resolved value asks for a knob the agent CLI does not support. Keeps the
// logs readable — without the guard a chatty task would spam one warning per
// dispatch.
func (r *Runner) warnUnsupported(agent, knob string, value interface{}) {
	key := agent + ":" + knob
	if _, loaded := r.warnOnce.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	slog.Warn("agent does not support knob, skipping",
		"component", "runner", "agent", agent, "knob", knob, "value", value)
}

// runtimeKnobs is the decoded form of resolver.ResolveAll output used by the
// runner. Keeping it in a struct instead of reaching into the map on every
// line in Execute makes the flow above readable and keeps each knob's fallback
// semantics localized in decodeRuntimeKnobs.
type runtimeKnobs struct {
	MaxTurns        int
	TimeoutMinutes  int
	DefaultAgent    string
	DefaultModel    string
	Temperature     float64
	ReasoningEffort string
	RetryOnFailure  int
	ApprovalMode    string
	AllowedTools    []string
	BranchPrefix       string
	BranchRemote       string
	BranchStrategy     string
	AutoPushOnComplete string // "always" | "on_success" | "never"
	WorktreeBaseDir    string

	// Prompt-context knobs — drive the prior_context section of the agent
	// prompt. Limits and budget controls are catalog-driven so operators
	// can dial the prior-runs / prior-comments injection per scope without
	// a code change. See internal/settings/catalog.go for descriptions.
	PromptMaxCommentChars    int
	PromptMaxContextPct      float64
	PromptPriorRunsLimit     int
	PromptPriorCommentsLimit int
	PromptBuildTimeoutSecs   int

	// Proposer-loop knobs — drive the auto-firing meta-harness proposer.
	// The catalog stores ProposerEnabled as an enum ("true"/"false"); the
	// decoder normalises it to a bool so call sites read a real flag.
	// ProposerTriggerFailureStreak == 0 disables streak-based firing;
	// ProposerTriggerCostUSD == 0 disables cost-based firing. Both off
	// means the trigger check is inert even if ProposerEnabled is true.
	ProposerEnabled              bool
	ProposerTriggerFailureStreak int
	ProposerTriggerCostUSD       float64
}

// decodeRuntimeKnobs pulls the 11 execution-shaping knobs out of a
// resolver.ResolveAll map, applying the defensive coercion helpers so any
// catalog-or-storage type drift surfaces as a wrapped error rather than a
// panic. max_parallel is intentionally absent — it is resolved separately by
// resolveMaxParallel before this function runs.
func decodeRuntimeKnobs(all map[string]settings.Resolved) (runtimeKnobs, error) {
	var k runtimeKnobs
	var err error

	if k.MaxTurns, err = resolvedInt(all["max_turns"].Value); err != nil {
		return k, fmt.Errorf("max_turns: %w", err)
	}
	if k.TimeoutMinutes, err = resolvedInt(all["timeout_minutes"].Value); err != nil {
		return k, fmt.Errorf("timeout_minutes: %w", err)
	}
	if k.DefaultAgent, err = resolvedStr(all["default_agent"].Value); err != nil {
		return k, fmt.Errorf("default_agent: %w", err)
	}
	if k.DefaultModel, err = resolvedStr(all["default_model"].Value); err != nil {
		return k, fmt.Errorf("default_model: %w", err)
	}
	if k.Temperature, err = resolvedFloat(all["temperature"].Value); err != nil {
		return k, fmt.Errorf("temperature: %w", err)
	}
	if k.ReasoningEffort, err = resolvedStr(all["reasoning_effort"].Value); err != nil {
		return k, fmt.Errorf("reasoning_effort: %w", err)
	}
	if k.RetryOnFailure, err = resolvedInt(all["retry_on_failure"].Value); err != nil {
		return k, fmt.Errorf("retry_on_failure: %w", err)
	}
	if k.ApprovalMode, err = resolvedStr(all["approval_mode"].Value); err != nil {
		return k, fmt.Errorf("approval_mode: %w", err)
	}
	if k.AllowedTools, err = resolvedStrSlice(all["allowed_tools"].Value); err != nil {
		return k, fmt.Errorf("allowed_tools: %w", err)
	}
	if k.BranchPrefix, err = resolvedStr(all["branch_prefix"].Value); err != nil {
		return k, fmt.Errorf("branch_prefix: %w", err)
	}
	if k.BranchPrefix == "" {
		k.BranchPrefix = "openpraxis"
	}
	if k.BranchRemote, err = resolvedStr(all["branch_remote"].Value); err != nil {
		return k, fmt.Errorf("branch_remote: %w", err)
	}
	if k.BranchRemote == "" {
		k.BranchRemote = "github"
	}
	if k.BranchStrategy, err = resolvedStr(all["branch_strategy"].Value); err != nil {
		return k, fmt.Errorf("branch_strategy: %w", err)
	}
	if k.BranchStrategy == "" {
		k.BranchStrategy = "task"
	}
	if k.AutoPushOnComplete, err = resolvedStr(all["auto_push_on_complete"].Value); err != nil {
		return k, fmt.Errorf("auto_push_on_complete: %w", err)
	}
	if k.AutoPushOnComplete == "" {
		k.AutoPushOnComplete = "always"
	}
	if k.WorktreeBaseDir, err = resolvedStr(all["worktree_base_dir"].Value); err != nil {
		return k, fmt.Errorf("worktree_base_dir: %w", err)
	}
	if k.WorktreeBaseDir == "" {
		k.WorktreeBaseDir = workspaceRoot
	}
	if k.PromptMaxCommentChars, err = resolvedInt(all["prompt_max_comment_chars"].Value); err != nil {
		return k, fmt.Errorf("prompt_max_comment_chars: %w", err)
	}
	if k.PromptMaxContextPct, err = resolvedFloat(all["prompt_max_context_pct"].Value); err != nil {
		return k, fmt.Errorf("prompt_max_context_pct: %w", err)
	}
	if k.PromptPriorRunsLimit, err = resolvedInt(all["prompt_prior_runs_limit"].Value); err != nil {
		return k, fmt.Errorf("prompt_prior_runs_limit: %w", err)
	}
	if k.PromptPriorCommentsLimit, err = resolvedInt(all["prompt_prior_comments_limit"].Value); err != nil {
		return k, fmt.Errorf("prompt_prior_comments_limit: %w", err)
	}
	if k.PromptBuildTimeoutSecs, err = resolvedInt(all["prompt_build_timeout_seconds"].Value); err != nil {
		return k, fmt.Errorf("prompt_build_timeout_seconds: %w", err)
	}
	// proposer_enabled is stored as an enum ("true"/"false") so the catalog
	// can render a dropdown; normalise to bool here so the runner toggles
	// behaviour off the typed flag. A non-"true" value (including unset)
	// disables the proposer trigger path entirely — fail-closed default.
	enabledStr, err := resolvedStr(all["proposer_enabled"].Value)
	if err != nil {
		return k, fmt.Errorf("proposer_enabled: %w", err)
	}
	k.ProposerEnabled = enabledStr == "true"
	if k.ProposerTriggerFailureStreak, err = resolvedInt(all["proposer_trigger_failure_streak"].Value); err != nil {
		return k, fmt.Errorf("proposer_trigger_failure_streak: %w", err)
	}
	if k.ProposerTriggerCostUSD, err = resolvedFloat(all["proposer_trigger_cost_usd"].Value); err != nil {
		return k, fmt.Errorf("proposer_trigger_cost_usd: %w", err)
	}
	return k, nil
}

// retryCountKey is the settings-table key under which the runner persists the
// number of retries already attempted for a task. Underscore prefix marks it
// as internal runtime state (not a user-configurable knob in the catalog) —
// the same convention used elsewhere in the repo for private keys.
const retryCountKey = "_retry_count"

// getRetryCount reads the persisted retry counter for a task from the
// settings table. Missing rows return 0. Parse errors are logged and treated
// as 0 — a corrupted counter should not block a retry.
func (r *Runner) getRetryCount(ctx context.Context, taskID string) int {
	if r.resolver == nil || r.resolver.Store() == nil {
		return 0
	}
	entry, err := r.resolver.Store().Get(ctx, settings.ScopeTask, taskID, retryCountKey)
	if err != nil {
		return 0
	}
	var n int
	if uerr := json.Unmarshal([]byte(entry.Value), &n); uerr != nil {
		slog.Warn("decode retry count failed, treating as 0",
			"component", "runner", "task_id", taskID, "raw", entry.Value, "error", uerr)
		return 0
	}
	return n
}

// setRetryCount persists the retry counter for a task. Non-fatal — failure
// to persist just means a restart could reset the counter, which is a minor
// edge case the retry cap naturally bounds.
func (r *Runner) setRetryCount(ctx context.Context, taskID string, n int) {
	if r.resolver == nil || r.resolver.Store() == nil {
		return
	}
	raw, err := json.Marshal(n)
	if err != nil {
		slog.Warn("encode retry count failed", "component", "runner", "task_id", taskID, "error", err)
		return
	}
	if err := r.resolver.Store().Set(ctx, settings.ScopeTask, taskID, retryCountKey, string(raw), "runner"); err != nil {
		slog.Warn("persist retry count failed", "component", "runner", "task_id", taskID, "error", err)
	}
}

// chooseAgent picks the agent CLI to spawn. Per-task override (t.Agent) wins
// over the resolved default so power users can pin a single task to a
// different runtime without mutating product/manifest scope. Extracted so
// the override rule is unit-testable without spawning a process.
func chooseAgent(taskAgent, resolvedDefault string) string {
	if taskAgent != "" {
		return taskAgent
	}
	return resolvedDefault
}

// shouldRetry answers whether the runner should requeue a completed run.
// Extracted from Execute's post-Wait block so all three retry-decision inputs
// (status, reason, attempts-so-far) are testable in isolation.
func shouldRetry(status, reason string, attempts, cap int) bool {
	if status != "failed" {
		return false
	}
	if cap <= 0 || attempts >= cap {
		return false
	}
	return isTransientFailure(reason)
}

// isTransientFailure classifies a run's terminal reason. Transient failures
// (process crash, timeout, build/test failure) are eligible for retry because
// a rerun might succeed; non-transient ones (max_turns exhausted, deliverable
// missing) won't improve with a rerun and would just burn cost.
func isTransientFailure(reason string) bool {
	switch reason {
	case "max_turns", "deliverable_missing":
		return false
	case "timeout", "build_fail", "process_error":
		return true
	default:
		// Unknown reasons: default to transient. Better to retry once than
		// to silently swallow a failure that the user expected to auto-heal.
		return true
	}
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
	bgCtx := context.Background()
	scope, maxPar, err := r.resolveMaxParallel(bgCtx, t.ID)
	if err != nil {
		return err
	}
	if r.RunningCountForProduct(scope.ProductID) >= maxPar {
		return fmt.Errorf("max parallel tasks reached for product %s (%d)", scope.ProductID, maxPar)
	}

	// Resolve every other knob in one DB query round-trip. Scope was already
	// normalized by resolveMaxParallel, so ResolveAll runs three ListScope
	// queries max regardless of catalog size.
	all, err := r.resolver.ResolveAll(bgCtx, scope)
	if err != nil {
		return fmt.Errorf("resolve knobs: %w", err)
	}

	knobs, err := decodeRuntimeKnobs(all)
	if err != nil {
		return fmt.Errorf("decode knobs: %w", err)
	}

	// Build the prompt
	prompt, err := buildPrompt(t, manifestTitle, manifestContent, visceralRules, knobs, r.tmpl, r.executionLog, r.commentsStore)
	if err != nil {
		return fmt.Errorf("build prompt: %w", err)
	}


	// Allowed tools: catalog-resolved list wins over the baseline. Empty
	// resolution (no catalog default set) falls back to defaultAllowedTools
	// so existing tasks keep working without an explicit multiselect row.
	allowedTools := knobs.AllowedTools
	if len(allowedTools) == 0 {
		allowedTools = defaultAllowedTools()
	}

	// Wall-clock timeout sourced from the resolver (minutes → Duration).
	// The catalog default (30) and type (int) are enforced at schema write
	// time, so knobs.TimeoutMinutes is always > 0 on the resolver snapshot
	// path. A regression to <=0 would fail decodeRuntimeKnobs before this
	// line is reached.
	timeout := time.Duration(knobs.TimeoutMinutes) * time.Minute
	ctx, cancel := context.WithTimeout(bgCtx, timeout)

	// Per-task override on Agent still beats the resolved default: letting a
	// user pin a specific task to e.g. "codex" without mutating scope config
	// is the whole reason we keep the column.
	agent := chooseAgent(t.Agent, knobs.DefaultAgent)

	// max_turns: post-M4-T14, 100% resolver-driven. Legacy per-task column
	// override (t.MaxTurns) was retired along with the tasks.max_turns
	// column. Per-task overrides now live in the settings table at task
	// scope and are already folded into knobs.MaxTurns by ResolveAll.
	maxTurns := knobs.MaxTurns

	var bin string
	var args []string

	if agent == "gemini-cli" {
		bin = "gemini"
		args = []string{
			"-p", prompt,
			"--output-format", "stream-json",
			"--allowed-tools", strings.Join(allowedTools, ","),
		}
	} else {
		bin = "claude"
		args = []string{
			"-p", prompt,
			"--output-format", "stream-json",
			"--verbose",
			"--max-turns", fmt.Sprintf("%d", maxTurns),
			"--allowedTools", strings.Join(allowedTools, ","),
		}
	}

	// Pass --model when the resolver yields a non-empty model id. Empty
	// default leaves the agent on its own default model.
	if knobs.DefaultModel != "" {
		args = append(args, "--model", knobs.DefaultModel)
	}

	// reasoning_effort: map catalog values (minimal/low/medium/high) onto
	// Claude's --effort values (low/medium/high/xhigh/max). "minimal" has
	// no Claude equivalent and is skipped with a one-time warning.
	if agent == "claude-code" {
		switch knobs.ReasoningEffort {
		case "low", "medium", "high":
			args = append(args, "--effort", knobs.ReasoningEffort)
		case "minimal":
			r.warnUnsupported(agent, "reasoning_effort=minimal", knobs.ReasoningEffort)
		case "":
			// no-op: system default already handled by agent
		default:
			r.warnUnsupported(agent, "reasoning_effort", knobs.ReasoningEffort)
		}
	} else if knobs.ReasoningEffort != "" {
		r.warnUnsupported(agent, "reasoning_effort", knobs.ReasoningEffort)
	}

	// temperature: Claude Code does not expose a --temperature flag. Warn
	// once per runner instance when a non-default value is configured so
	// operators know their knob is inert.
	if knobs.Temperature > 0 && knobs.Temperature != 0.2 {
		r.warnUnsupported(agent, "temperature", knobs.Temperature)
	}

	// approval_mode: Claude Code's --permission-mode has a different value
	// space (acceptEdits/auto/bypassPermissions/default/dontAsk/plan) than
	// the catalog (auto/manual/on-failure). "auto" preserves current
	// behavior (no flag, runs unattended under -p); other values warn.
	if knobs.ApprovalMode != "" && knobs.ApprovalMode != "auto" {
		r.warnUnsupported(agent, "approval_mode", knobs.ApprovalMode)
	}

	// Materialize a dedicated git worktree for this task off origin/main.
	// The agent runs inside it, so its branch is always based on a clean,
	// up-to-date main — no stacking on a previous task's branch, no risk
	// of clobbering the operator's own working copy at the repo root.
	workDir, baseSHA, err := r.prepareTaskWorkspace(t.ID, knobs.WorktreeBaseDir)
	if err != nil {
		cancel()
		return fmt.Errorf("prepare task workspace: %w", err)
	}
	slog.Info("task workspace ready",
		"component", "runner", "task_id", t.ID,
		"workdir", workDir, "base_sha", baseSHA)

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = workDir
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		r.cleanupTaskWorkspace(workDir)
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		r.cleanupTaskWorkspace(workDir)
		return fmt.Errorf("start agent: %w", err)
	}

	rt := &RunningTask{
		TaskID:         t.ID,
		Title:          t.Title,
		Manifest:       manifestTitle,
		ProductID:      scope.ProductID,
		Agent:          t.Agent,
		PID:            cmd.Process.Pid,
		StartedAt:      time.Now(),
		Output:         make([]string, 0, 200),
		usageByMessage: make(map[string]Usage),
		cancel:         cancel,
		cmd:            cmd,
	}

	r.mu.Lock()
	r.running[t.ID] = rt
	r.mu.Unlock()

	// status tracked via execution_log
	if r.entityStore != nil {
		if e, _ := r.entityStore.Get(t.ID); e != nil {
			if uerr := r.entityStore.Update(t.ID, e.Title, entity.StatusActive, e.Tags, "runner", "task started"); uerr != nil {
				slog.Warn("entity store: update to active failed", "component", "runner", "task_id", t.ID, "error", uerr)
			}
		}
	}

	// Generate a stable run UID (UUID v7) that ties the started, sample,
	// and completed/failed rows in execution_log together. Generated here
	// so the goroutine closes over it.
	runUID := uuid.Must(uuid.NewV7()).String()

	// Record the started event in execution_log so crash-recovery can
	// see which tasks were in-flight at the time of the crash.
	if r.executionLog != nil {
		startRow := execution.Row{
			RunUID:        runUID,
			EntityUID:     t.ID,
			Event:         execution.EventStarted,
			Trigger:       "schedule",
			NodeID:        r.sourceNode,
			AgentRuntime:  agent,
			StartedAt:     rt.StartedAt.UnixMilli(),
			WorktreePath:  workDir,
			Branch:        "", // will be filled after first commit
			ToolCallsJSON: "{}",
			CreatedBy:     "runner",
			AgentPID:      cmd.Process.Pid,
		}
		if err := r.executionLog.Insert(bgCtx, startRow); err != nil {
			slog.Warn("execution_log: insert started event failed", "component", "runner", "task_id", t.ID, "error", err)
		}
	}

	if r.onEvent != nil {
		r.onEvent("task_started", map[string]string{
			"task_id": t.ID, "title": t.Title, "manifest": manifestTitle,
		})
	}

	slog.Info("task started", "component", "runner", "task_id", t.ID, "title", t.Title, "pid", cmd.Process.Pid)

	// Begin host CPU/RSS sampling for this task. Samples accumulate in
	// the sampler's per-task buffer until the completion path calls
	// Detach + RecordHostMetrics. Nil sampler → no-op (tests + pre-wire
	// code paths).
	if r.hostSampler != nil {
		// The callback closes over rt so samples capture live cost / turns
		// (unique message ids) / actions alongside host CPU/RSS on the
		// same 5s cadence — powers the Run Stats 5-aligned sparklines.
		rtRef := rt
		r.hostSampler.Attach(t.ID, func() (float64, int, int) {
			rtRef.mu.RLock()
			turns := len(rtRef.usageByMessage)
			actions := rtRef.Actions
			rtRef.mu.RUnlock()
			return 0, turns, actions
		})
	}

	// Read output in background
	retryCap := knobs.RetryOnFailure
	go func() {
		// finalStatus is set after the agent exits and is read by the defer
		// to decide whether to auto-push. Pointer so the defer captures it.
		var finalStatus string
		defer func() {
			r.mu.Lock()
			delete(r.running, t.ID)
			r.mu.Unlock()
			cancel()
			// Auto-push: ensure the agent's branch reaches the remote before
			// the worktree is destroyed. Controlled by auto_push_on_complete:
			//   "always"     — push regardless of outcome (default, safest)
			//   "on_success" — push only when task completed successfully
			//   "never"      — skip runner-level push, agent is responsible
			shouldPush := false
			switch knobs.AutoPushOnComplete {
			case "always":
				shouldPush = true
			case "on_success":
				shouldPush = finalStatus == execution.EventCompleted
			}
			if shouldPush && workDir != "" && knobs.BranchRemote != "" {
				branchName := knobs.BranchPrefix + "/" + t.ID
				out, pushErr := runGit(workDir, "push", "--set-upstream", knobs.BranchRemote, branchName)
				if pushErr != nil {
					slog.Warn("auto-push failed — branch may be local only",
						"component", "runner", "task_id", t.ID[:12],
						"branch", branchName, "remote", knobs.BranchRemote,
						"error", pushErr, "output", out)
				} else {
					slog.Info("auto-push succeeded", "component", "runner",
						"task_id", t.ID[:12], "branch", branchName, "remote", knobs.BranchRemote)
				}
			}
			// Remove the per-task worktree directory. The agent's branch
			// stays intact in the shared .git, so the PR keeps working.
			r.cleanupTaskWorkspace(workDir)
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
					// Detect a new turn boundary. Claude Code re-emits the same
					// assistant message id as it streams chunks, so dedupe on
					// message.id via the existing usageByMessage map. Firing on
					// every assistant line would overcount vs. result.num_turns.
					// rt.mu guards usageByMessage — held only for the map op,
					// released before calling onEvent to avoid lock inversion.
					if msg, ok := event["message"].(map[string]any); ok {
						if msgID, _ := msg["id"].(string); msgID != "" {
							rt.mu.Lock()
							_, seen := rt.usageByMessage[msgID]
							if !seen {
								rt.usageByMessage[msgID] = Usage{}
							}
							turnCount := len(rt.usageByMessage)
							rt.mu.Unlock()
							if !seen {
								// Persist turn boundary to execution_log so turn history
								// survives beyond the in-memory RunningTask lifetime.
								if r.executionLog != nil {
									_ = r.executionLog.Insert(bgCtx, execution.Row{
										RunUID:       runUID,
										EntityUID:    t.ID,
										Event:        execution.EventTurn,
										Trigger:      "agent",
										NodeID:       r.sourceNode,
										AgentRuntime: agent,
										Turns:        turnCount,
										CreatedBy:    "runner",
									})
								}
								if r.onEvent != nil {
									r.onEvent("task_turn_started", map[string]string{
										"task_id": t.ID,
										"turn":    strconv.Itoa(turnCount),
									})
								}
							}
						}
					}

					// Extract tool_use blocks from assistant message
					if msg, ok := event["message"].(map[string]any); ok {
						if content, ok := msg["content"].([]any); ok {
							for _, block := range content {
								if bm, ok := block.(map[string]any); ok {
									if bm["type"] == "tool_use" {
										rt.mu.Lock()
										rt.Actions++
										rt.mu.Unlock()
										toolName, _ := bm["name"].(string)
										toolID, _ := bm["id"].(string)
										toolInput := marshalJSON(bm["input"])

										// Record action with current turn number.
										var actionID int64
										if r.actions != nil {
											rt.mu.RLock()
											currentTurn := len(rt.usageByMessage)
											rt.mu.RUnlock()
											var recErr error
											actionID, recErr = r.actions.RecordForTask(t.ID, t.SourceNode, toolName, toolInput, "", "", currentTurn)
											if recErr != nil {
												slog.Error("record action failed", "component", "runner", "task_id", t.ID, "error", recErr)
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
											r.onEvent("task_turn_tool", map[string]string{
												"task_id":   t.ID,
												"tool_name": toolName,
												"tool_id":   toolID,
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
													slog.Error("update action response failed", "component", "runner", "task_id", t.ID, "error", err)
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

			// Live cost tracking.
			//
			// Real Claude Code stream-json only puts total_cost_usd on the
			// terminal result event, so the cost cap would have nothing to
			// read mid-run if we relied on it alone. Instead, each assistant
			// event carries message.usage (input/output/cache tokens); we
			// dedupe by message.id (Claude Code re-emits the same message as
			// Track model from assistant events for metadata.
			if event != nil {
				if _, model, _, ok := parseAssistantUsage(event); ok {
					if rt.Model == "" && model != "" {
						rt.Model = model
					}
				}
			}

			// Broadcast progress
			if rt.Lines%5 == 0 && r.onEvent != nil {
				rt.mu.RLock()
				actions := rt.Actions
				rt.mu.RUnlock()
				r.onEvent("task_progress", map[string]string{
					"task_id": t.ID, "actions": fmt.Sprintf("%d", actions),
				})
			}

			// Persist runtime state every 10 lines
			if rt.Lines%10 == 0 {
				// runtime_state table removed
			}
		}

		// Wait for process to finish
		waitErr := cmd.Wait()
		output := allOutput.String()

		// Classify the outcome. Reason drives both the recorded run status
		// and whether retry fires. Order matters: cost-cap overrides a
		// max_turns/timeout detection because the kill happened first.
		var (
			status string
			reason string
		)
		switch {
		case detectMaxTurns(output):
			status = "completed"
			reason = "max_turns"
			slog.Info("task hit max turns limit", "component", "runner", "task_id", t.ID,
				"actions", rt.Actions, "lines", rt.Lines)
		case ctx.Err() == context.DeadlineExceeded:
			status = "failed"
			reason = "timeout"
			slog.Error("task timed out", "component", "runner", "task_id", t.ID,
				"actions", rt.Actions, "lines", rt.Lines)
		case waitErr != nil:
			status = "failed"
			reason = "process_error"
			slog.Error("task failed", "component", "runner", "task_id", t.ID, "error", waitErr)
		default:
			status = "completed"
			reason = "success"
			slog.Info("task completed", "component", "runner", "task_id", t.ID,
				"actions", rt.Actions, "lines", rt.Lines)
		}
		// Capture for the auto-push defer which runs after this goroutine exits.
		finalStatus = status

		_, numTurns := ParseCostFromOutput(output)
		costUSD := float64(0)

		// Compute final per-token usage from stream-json (preferred) or
		// the summed per-message usage we tracked during the run. Used
		// both for pricing calibration AND for the denormalised columns
		// on task_runs (so dashboards / future cost recompute don't
		// re-parse the output blob).
		var finalUsage Usage
		if ru, ok := parseFinalResultUsage(output); ok {
			finalUsage = ru
		} else {
			for _, mu := range rt.usageByMessage {
				finalUsage = finalUsage.Add(mu)
			}
		}

		// Calibrate model pricing from the authoritative final cost so the
		// next run's live estimate is closer to reality.
		if costUSD > 0 {
			// model_pricing table removed
		}

		// Record the completed/failed run in execution_log and update the
		// tasks row (run_count, last_run_at, last_output, status).
		// task run stats tracked in execution_log

		// completedRunRow exposes the just-built terminal Row (with
		// ComputeDerived already applied) to post-completion paths that
		// need cost / token / turn fields — currently the proposer
		// trigger check below. Stays nil when execution logging is off.
		var completedRunRow *execution.Row
		if r.executionLog != nil {
			completedAtMS := time.Now().UnixMilli()
			durationMS := completedAtMS - rt.StartedAt.UnixMilli()
			if durationMS < 0 {
				durationMS = 0
			}
			terminalEvent := execution.EventCompleted
			terminalReason := reason
			errMsg := ""
			if status == "failed" {
				terminalEvent = execution.EventFailed
				if waitErr != nil {
					errMsg = waitErr.Error()
				}
			}

			// Exit code
			var exitCodeVal *int
			if cmd.ProcessState != nil {
				code := cmd.ProcessState.ExitCode()
				exitCodeVal = &code
			}

			// Git stats: lines+/-, files changed, commits, SHA, branch
			linesAdded, linesRemoved, filesChanged, commits, headSHA, branchName := RunGitStats(workDir, baseSHA)

			// Parse additional metrics from stream-json output
			compactions, errorCount, reasoningTokens, ttfbMS := ParseOutputMetrics(output)
			testsRun, testsPassed, testsFailed := ParseTestsFromOutput(output)

			modelInfo := execution.LookupModel(rt.Model)

			// Host metrics: peak/avg CPU/RSS from samples collected during the run
			var peakCPU, avgCPU, peakRSS, avgRSS float64
			var diskUsedGB float64
			var hostSamples []HostMetricsSample
			if r.hostSampler != nil {
				hostSamples, _ = r.hostSampler.Detach(t.ID)
				for _, s := range hostSamples {
					if s.CPUPct > peakCPU {
						peakCPU = s.CPUPct
					}
					if s.RSSMB > peakRSS {
						peakRSS = s.RSSMB
					}
					avgCPU += s.CPUPct
					avgRSS += s.RSSMB
					if s.DiskUsedGB > diskUsedGB {
						diskUsedGB = s.DiskUsedGB
					}
				}
				if n := len(hostSamples); n > 0 {
					avgCPU /= float64(n)
					avgRSS /= float64(n)
				}
			}

			// Run number derived from execution_log (count of prior terminal events)
			runNumber := 0
			if r.executionLog != nil {
				if prior, err := r.executionLog.ListByEntity(bgCtx, t.ID, 1000); err == nil {
					for _, row := range prior {
						if row.Event == execution.EventCompleted || row.Event == execution.EventFailed {
							runNumber++
						}
					}
				}
			}

			completedRow := execution.Row{
				RunUID:            runUID,
				EntityUID:         t.ID,
				Event:             terminalEvent,
				RunNumber:         runNumber,
				Trigger:           "schedule",
				NodeID:            r.sourceNode,
				TerminalReason:    terminalReason,
				StartedAt:         rt.StartedAt.UnixMilli(),
				CompletedAt:       completedAtMS,
				DurationMS:        durationMS,
				TTFBMS:            ttfbMS,
				ExitCode:          exitCodeVal,
				Error:             errMsg,
				Model:             rt.Model,
				AgentRuntime:      agent,
				Provider:          modelInfo.Provider,
				ModelContextSize:  modelInfo.ContextWindowSize,
				PricingVersion:    PricingVersion,
				InputTokens:       int64(finalUsage.InputTokens),
				OutputTokens:      int64(finalUsage.OutputTokens),
				CacheReadTokens:   int64(finalUsage.CacheReadTokens),
				CacheCreateTokens: int64(finalUsage.CacheCreationTokens),
				ReasoningTokens:   reasoningTokens,
				CostUSD:           costUSD,
				Turns:             numTurns,
				Actions:           rt.Actions,
				Errors:            errorCount,
				Compactions:       compactions,
				ToolCallsJSON:     "{}",
				LinesAdded:        linesAdded,
				LinesRemoved:      linesRemoved,
				FilesChanged:      filesChanged,
				Commits:           commits,
				CommitSHA:         headSHA,
				Branch:            branchName,
				WorktreePath:      workDir,
				TestsRun:          testsRun,
				TestsPassed:       testsPassed,
				TestsFailed:       testsFailed,
				PeakCPUPct:        peakCPU,
				AvgCPUPct:         avgCPU,
				PeakRSSMB:         peakRSS,
				AvgRSSMB:          avgRSS,
				DiskUsedGB:        diskUsedGB,
				CreatedBy:         "runner",
			}
			execution.ComputeDerived(&completedRow)
			completedRunRow = &completedRow
			if err := r.executionLog.Insert(bgCtx, completedRow); err != nil {
				slog.Warn("execution_log: insert completed event failed", "component", "runner", "task_id", t.ID, "error", err)
			}

			// Persist host CPU/RSS samples as individual EventSample rows.
			for _, smp := range hostSamples {
				sampleRow := execution.Row{
					RunUID:        runUID,
					EntityUID:     t.ID,
					Event:         execution.EventSample,
					Trigger:       "schedule",
					NodeID:        r.sourceNode,
					AgentRuntime:  agent,
					StartedAt:     rt.StartedAt.UnixMilli(),
					CPUPct:        smp.CPUPct,
					RSSMB:         smp.RSSMB,
					DiskUsedGB:    smp.DiskUsedGB,
					NetRxMbps:     smp.NetRxMbps,
					NetTxMbps:     smp.NetTxMbps,
					DiskReadMBps:  smp.DiskReadMBps,
					DiskWriteMBps: smp.DiskWriteMBps,
					MemUsedMB:     smp.MemUsedMB,
					MemTotalMB:    smp.MemTotalMB,
					LoadAvg1m:     smp.LoadAvg1m,
					CostUSD:       smp.CostUSD,
					Turns:         smp.Turns,
					Actions:       smp.Actions,
					ToolCallsJSON: "{}",
					CreatedBy:     "runner",
					CreatedAt:     smp.TS.UTC().Format(time.RFC3339Nano),
				}
				if err := r.executionLog.Insert(bgCtx, sampleRow); err != nil {
					slog.Warn("execution_log: insert sample event failed", "component", "runner", "task_id", t.ID, "error", err)
				}
			}
		} else if r.hostSampler != nil {
			// Drain the sampler even when execution_log is not wired so buffers don't grow.
			r.hostSampler.Detach(t.ID) //nolint:errcheck
		}

		// Post-completion execution-review gate. Successful completions must
		// carry at least one agent-authored comment comment on the
		// task. When missing, log and record an amnesia flag so the gap
		// surfaces on the dashboard. Non-blocking — the watcher still has
		// final say on the completion status.
		r.enforceExecutionReview(bgCtx, t.ID, status, reason)

		// Proposer-loop trigger. Off by default; the catalog enum +
		// per-scope override let operators enable it without restart.
		// Failure-streak and cost thresholds are evaluated per parent
		// manifest — if either trips, the runner spawns a proposer task
		// and queues it on the schedules table. See checkProposerTriggers.
		if knobs.ProposerEnabled {
			r.checkProposerTriggers(bgCtx, t, reason, completedRunRow, knobs)
		}

		// Retry on transient failure. Counter persists across restarts via
		// the settings table so a crash mid-retry does not reset progress.
		retried := false
		attempts := r.getRetryCount(bgCtx, t.ID)
		if shouldRetry(status, reason, attempts, retryCap) {
			r.setRetryCount(bgCtx, t.ID, attempts+1)
			// Requeue immediately — the scheduler will pick up the row
			// on its next tick. IsOneShot tasks are retried with the
			// same next_run_at so they remain one-shot from the user's
			// perspective; the retry is a runner-internal decision.
			// Retry: the schedules table will re-fire this entity.
			// The schedule runner handles next_run_at; we just signal intent.
			slog.Info("retrying task after transient failure",
				"component", "runner", "task_id", t.ID,
				"reason", reason, "attempt", attempts+1, "cap", retryCap)
			_ = retried
		} else if status == "failed" && retryCap > 0 {
			slog.Info("retry skipped",
				"component", "runner", "task_id", t.ID,
				"reason", reason, "attempts", attempts, "cap", retryCap)
		}

		// For recurring tasks that did not retry, compute the next scheduled
		// run as normal.
		// Recurring next-run is managed by the schedules table + cron runner.

		// Dependent-task activation is deferred to the watcher audit path
		// (cmd/serve.go). Activating here races the audit: a task the runner
		// marks "completed" can be downgraded to "failed" by the 5s-later
		// watcher check, but any dependents activated in the interim would
		// already be scheduled/running.

		// Remove from running map before firing task_completed so the
		// DAG chain dispatch sees RunningCount()==0 and can start the
		// next task without hitting the concurrency guard. The defer's
		// delete is a safe no-op double-remove.
		r.mu.Lock()
		delete(r.running, t.ID)
		r.mu.Unlock()

		if r.onEvent != nil {
			r.onEvent(EventTaskCompleted, map[string]string{
				EventKeyTaskID: t.ID,
				EventKeyStatus: status,
				EventKeyReason: reason,
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

	slog.Info("killing task", "component", "runner", "task_id", rt.TaskID)
	rt.cancel()
	if rt.cmd != nil && rt.cmd.Process != nil {
		if err := rt.cmd.Process.Kill(); err != nil {
			slog.Error("kill process failed", "component", "runner", "task_id", rt.TaskID, "error", err)
		}
	}
	// status tracked via execution_log

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

	slog.Info("pausing task", "component", "runner", "task_id", rt.TaskID, "pid", rt.PID)
	if err := rt.cmd.Process.Signal(syscall.SIGSTOP); err != nil {
		return fmt.Errorf("SIGSTOP failed: %w", err)
	}

	rt.Paused = true
	// status tracked via execution_log
	// runtime_state table removed

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
		slog.Info("cancelling task — killing process", "component", "runner", "task_id", rt.TaskID, "pid", rt.PID)
		if rt.Paused {
			_ = rt.cmd.Process.Signal(syscall.SIGCONT)
		}
		if err := rt.cmd.Process.Kill(); err != nil {
			slog.Error("kill process failed", "component", "runner", "task_id", rt.TaskID, "error", err)
		}
		if rt.cancel != nil {
			rt.cancel()
		}
	}
	// status tracked via execution_log
	if r.onEvent != nil {
		r.onEvent("task_cancelled", map[string]string{"task_id": taskID})
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

	slog.Info("resuming task", "component", "runner", "task_id", rt.TaskID, "pid", rt.PID)
	if err := rt.cmd.Process.Signal(syscall.SIGCONT); err != nil {
		return fmt.Errorf("SIGCONT failed: %w", err)
	}

	rt.Paused = false
	// status tracked via execution_log
	// runtime_state table removed

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

// defaultModelContextChars is the fallback context-window size used for
// prompt budget calculations. Multiplied by knobs.PromptMaxContextPct to
// yield the byte budget that PriorRuns + OtherComments must fit within.
// Hardcoded number is centralized here so tuning is a single edit.
const defaultModelContextChars = 200_000

// buildPrompt assembles the runner's task prompt from the prompt sections
// (RC/M1 read substrate). Each section body is resolved via tmpl
// (task → manifest → product → agent → system) and rendered as a
// text/template against a shared PromptData payload.
//
// When tmpl is nil, or a section resolves to "", the package defaults
// in internal/templates are used so every historical test harness + the
// snapshot test produce byte-identical output to the pre-RC/M1
// hardcoded writer.
//
// execLog and commentsStore feed the prior_context section with run
// digests and prior agent comments respectively. Either may be nil — the
// section template guards on empty slices and renders nothing in that
// case. knobs.PromptBuildTimeoutSecs bounds the history queries so a
// slow DB cannot stall task dispatch.
func buildPrompt(t *Task, manifestTitle, manifestContent, visceralRules string,
	knobs runtimeKnobs, tmpl *templates.Resolver,
	execLog *execution.Store, commentsStore *comments.Store) (string, error) {
	branchPfx := knobs.BranchPrefix
	if branchPfx == "" {
		branchPfx = "openpraxis"
	}
	branchRemote := knobs.BranchRemote
	if branchRemote == "" {
		branchRemote = "github"
	}
	// Resolve the branch name from strategy. For task strategy, compose
	// prefix/taskID. For manifest/product, the shared branch name is
	// injected by the dispatcher in cmd/serve.go — we leave Branch empty
	// here so the dispatcher's injection takes precedence. The template
	// falls back to {{.BranchPrefix}}/{{.Task.ID}} when Branch is "".
	branch := ""
	if knobs.BranchStrategy == "task" || knobs.BranchStrategy == "" {
		branch = branchPfx + "/" + t.ID
	}
	data := templates.PromptData{
		Task: templates.TaskView{
			ID:          t.ID,
			Title:       t.Title,
			Description: t.Description,
			Agent:       t.Agent,
		},
		Manifest: templates.ManifestView{
			ID:      t.ManifestID,
			Title:   manifestTitle,
			Content: manifestContent,
		},
		VisceralRules: visceralRules,
		BranchPrefix:  branchPfx,
		BranchRemote:  branchRemote,
		Branch:        branch,
		Now:           time.Now(),
	}

	timeoutSecs := knobs.PromptBuildTimeoutSecs
	if timeoutSecs <= 0 {
		timeoutSecs = 5
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	if execLog != nil && knobs.PromptPriorRunsLimit > 0 {
		rows, _ := execLog.ListByEntity(ctx, t.ID, knobs.PromptPriorRunsLimit)
		for i, row := range rows {
			runShort := row.RunUID
			if len(runShort) > 8 {
				runShort = runShort[:8]
			}
			line := fmt.Sprintf("Run #%d (%s) — %s/%s — %d turns, %d actions, $%.3f, %dms. Branch: %s. Lines: +%d/-%d.",
				i+1, runShort, row.Event, row.TerminalReason,
				row.Turns, row.Actions, row.EstimatedCostUSD, row.DurationMS,
				row.Branch, row.LinesAdded, row.LinesRemoved)
			if knobs.PromptMaxCommentChars > 0 && len(line) > knobs.PromptMaxCommentChars {
				line = line[:knobs.PromptMaxCommentChars]
			}
			data.PriorRuns = append(data.PriorRuns, line)
		}
	}

	if commentsStore != nil && knobs.PromptPriorCommentsLimit > 0 {
		ct := comments.TypeComment
		cms, _ := commentsStore.List(ctx, comments.TargetEntity, t.ID, knobs.PromptPriorCommentsLimit, &ct)
		for _, c := range cms {
			body := c.Body
			if knobs.PromptMaxCommentChars > 0 && len(body) > knobs.PromptMaxCommentChars {
				body = body[:knobs.PromptMaxCommentChars] + "\n[truncated]"
			}
			data.OtherComments = append(data.OtherComments, body)
		}
	}

	if knobs.PromptMaxContextPct > 0 {
		budget := int(float64(defaultModelContextChars) * knobs.PromptMaxContextPct)
		used := 0
		for _, s := range data.PriorRuns {
			used += len(s)
		}
		for _, s := range data.OtherComments {
			used += len(s)
		}
		for used > budget && len(data.PriorRuns) > 0 {
			used -= len(data.PriorRuns[len(data.PriorRuns)-1])
			data.PriorRuns = data.PriorRuns[:len(data.PriorRuns)-1]
		}
		for used > budget && len(data.OtherComments) > 0 {
			used -= len(data.OtherComments[len(data.OtherComments)-1])
			data.OtherComments = data.OtherComments[:len(data.OtherComments)-1]
		}
	}

	defaults := templates.SystemDefaults()

	var b strings.Builder
	for _, section := range templates.Sections {
		body := ""
		if tmpl != nil {
			resolved, err := tmpl.Resolve(ctx, section, t.ID)
			if err != nil {
				return "", fmt.Errorf("resolve %s: %w", section, err)
			}
			body = resolved
		}
		if body == "" {
			body = defaults[section]
		}
		if body == "" {
			continue
		}
		rendered, err := templates.Render(body, data)
		if err != nil {
			return "", fmt.Errorf("render %s: %w", section, err)
		}
		b.WriteString(rendered)
	}
	return b.String(), nil
}

