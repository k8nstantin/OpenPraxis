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

	"github.com/google/uuid"
	"github.com/k8nstantin/OpenPraxis/internal/action"
	"github.com/k8nstantin/OpenPraxis/internal/activity"
	"github.com/k8nstantin/OpenPraxis/internal/comments"
	"github.com/k8nstantin/OpenPraxis/internal/entity"
	execution "github.com/k8nstantin/OpenPraxis/internal/execution"
	"github.com/k8nstantin/OpenPraxis/internal/settings"
	"github.com/k8nstantin/OpenPraxis/internal/templates"
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
	activityLog  *activity.Store

	// commentsStore is used to fetch the latest prompt
	// comment for a task when building the prompt. Nil → falls back to
	// task.Description from the tasks row.
	commentsStore *comments.Store
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

// SetActivityLog wires the append-only activity log onto the runner.
func (r *Runner) SetActivityLog(al *activity.Store) { r.activityLog = al }

// SetCommentsStore wires the comments store onto the runner so the prompt
// builder can fetch the latest prompt for a task. When nil,
// the runner uses task.Description from the tasks row directly.
func (r *Runner) SetCommentsStore(cs *comments.Store) { r.commentsStore = cs }

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

// NewRunner creates a task runner. The resolver is required — every knob that
// shapes task execution is looked up through it on every Execute call.
// repoDir is the git repo root tasks will clone worktrees from; pass "" to
// default to the server's process CWD at spawn time.
func NewRunner(actions *action.Store, resolver *settings.Resolver, repoDir string, onEvent func(string, map[string]string)) *Runner {
	return &Runner{
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
// RecoverInFlight is a no-op. In-flight recovery now relies on execution_log:
// any run with a started event but no terminal event is considered orphaned
// and will be retried by the next schedule tick.
func (r *Runner) RecoverInFlight(_ context.Context) error { return nil }

// recoverOneOrphan is unused — kept for compilation until full cleanup.
func (r *Runner) recoverOneOrphan(ctx context.Context, taskID string, pid, actions, lines int, startedAt string) {
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
		slog.Info("recover: orphan will be re-fired by schedule runner", "task_id", taskID, "prior_pid", pid)
	default:
		slog.Info("recover: orphan task logged as failed in execution_log", "task_id", taskID, "prior_pid", pid)
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
	BranchPrefix    string
	WorktreeBaseDir string
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
	if k.WorktreeBaseDir, err = resolvedStr(all["worktree_base_dir"].Value); err != nil {
		return k, fmt.Errorf("worktree_base_dir: %w", err)
	}
	if k.WorktreeBaseDir == "" {
		k.WorktreeBaseDir = workspaceRoot
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
	prompt, err := buildPrompt(t, manifestTitle, manifestContent, visceralRules, knobs.BranchPrefix, r.tmpl)
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

	if r.activityLog != nil {
		_, _ = r.activityLog.Insert(bgCtx, activity.Row{
			EntityUID:  t.ID,
			Event:      activity.EventRunStarted,
			Actor:      "agent",
			Summary:    "Run started: " + t.Title,
			RunUID:     runUID,
			Trigger:    "schedule",
			NodeID:     r.sourceNode,
		})
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
			return 0, len(rtRef.usageByMessage), rtRef.Actions
		})
	}

	// Read output in background
	retryCap := knobs.RetryOnFailure
	go func() {
		defer func() {
			r.mu.Lock()
			delete(r.running, t.ID)
			r.mu.Unlock()
			cancel()
			// Clean up persisted runtime state — task is done
			// runtime_state table removed
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
				r.onEvent("task_progress", map[string]string{
					"task_id": t.ID, "actions": fmt.Sprintf("%d", rt.Actions),
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

		// Retry on transient failure. Counter persists across restarts via
		// the settings table so a crash mid-retry does not reset progress.
		// Watcher-driven downgrades (completed→failed via deliverable audit)
		// are intentionally NOT retried — they run in cmd/serve.go's audit
		// callback, which is the documented single retry decision point for
		// those cases. This path only handles the runner's own detection.
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

		// Write to activity_log — run_completed or run_failed.
		if r.activityLog != nil {
			actEvent := activity.EventRunCompleted
			if status == "failed" {
				actEvent = activity.EventRunFailed
			}
			// Aggregate usage from usageByMessage (same as the host sampler).
			var totalIn, totalOut int64
			r.mu.RLock()
			for _, u := range rt.usageByMessage {
				totalIn += int64(u.InputTokens)
				totalOut += int64(u.OutputTokens)
			}
			turns := len(rt.usageByMessage)
			r.mu.RUnlock()

			_, _ = r.activityLog.Insert(bgCtx, activity.Row{
				EntityUID:    t.ID,
				Event:        actEvent,
				Actor:        "agent",
				Summary:      fmt.Sprintf("Run %s: %s (reason=%s)", status, t.Title, reason),
				RunUID:       runUID,
				Trigger:      "schedule",
				NodeID:       r.sourceNode,
				DurationMS:   time.Since(rt.StartedAt).Milliseconds(),
				Turns:        turns,
				Actions:      rt.Actions,
				InputTokens:  totalIn,
				OutputTokens: totalOut,
				Error:        reason,
			})
		}

		if r.onEvent != nil {
			r.onEvent("task_completed", map[string]string{
				"task_id": t.ID, "status": status, "reason": reason,
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

// StartActionWatcher is a no-op. Pause/Resume/Cancel are now driven
// via direct HTTP calls to the runner's in-memory state.
func (r *Runner) StartActionWatcher(_ time.Duration) {}

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

// buildPrompt assembles the runner's task prompt from the seven prompt
// sections (RC/M1 read substrate). Each section body is resolved via
// tmpl (task → manifest → product → agent → system) and rendered as a
// text/template against a shared PromptData payload.
//
// When tmpl is nil, or a section resolves to "", the package defaults
// in internal/templates are used so every historical test harness + the
// snapshot test produce byte-identical output to the pre-RC/M1
// hardcoded writer.
func buildPrompt(t *Task, manifestTitle, manifestContent, visceralRules, branchPfx string, tmpl *templates.Resolver) (string, error) {
	if branchPfx == "" {
		branchPfx = "openpraxis"
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
		Now:           time.Now(),
	}

	defaults := templates.SystemDefaults()
	ctx := context.Background()

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

