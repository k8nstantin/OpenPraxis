package task

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/settings"
)

// insertRunningTask seeds a task row directly in the running status so
// RecoverInFlight has something to classify. Bypasses Store.Create
// because that path forces status='pending'. PR/M3 dropped the legacy
// manifest_id column from tasks; ownership wiring lives in the
// relationships store now.
func insertRunningTask(t *testing.T, db *sql.DB, taskID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`INSERT INTO tasks (id, title, description, schedule, status, agent, source_node, created_by, created_at, updated_at)
		VALUES (?, 'orphan', '', 'once', 'running', 'claude-code', '', 'test', ?, ?)`,
		taskID, now, now)
	if err != nil {
		t.Fatalf("insert running task: %v", err)
	}
}

// TestBuildPrompt_BranchPrefixOverride verifies the resolved
// `branch_prefix` knob renders into the <git_workflow> block. The
// manifest's acceptance bullet 4 (`branch_prefix=qa` → `git checkout -b
// qa/<id-prefix>`) is the concrete shape we assert.
func TestBuildPrompt_BranchPrefixOverride(t *testing.T) {
	task := &Task{ID: "019dba9f-9c5b-76ba-8221-e7e11093887f", Title: "T", Description: "D"}
	got, err := buildPrompt(task, "M", "m body", "", "qa", nil)
	if err != nil {
		t.Fatalf("buildPrompt: %v", err)
	}
	wantBranchLine := "git checkout -b qa/019dba9f-9c5"
	wantPushLine := "git push -u origin qa/019dba9f-9c5"
	if !strings.Contains(got, wantBranchLine) {
		t.Errorf("prompt missing %q; got:\n%s", wantBranchLine, got)
	}
	if !strings.Contains(got, wantPushLine) {
		t.Errorf("prompt missing %q; got:\n%s", wantPushLine, got)
	}
	// Sanity: the default "openpraxis" prefix must not leak in when the
	// override is active.
	if strings.Contains(got, "openpraxis/019dba9f-9c5") {
		t.Errorf("prompt leaked default prefix even though override=qa; got:\n%s", got)
	}
}

// TestBuildPrompt_EmptyBranchPrefixFallsBack keeps the legacy default
// behaviour when the knob resolves to an empty string (e.g. a badly
// seeded DB).
func TestBuildPrompt_EmptyBranchPrefixFallsBack(t *testing.T) {
	task := &Task{ID: "019dba9f-9c5b-76ba-8221-e7e11093887f", Title: "T"}
	got, err := buildPrompt(task, "M", "m body", "", "", nil)
	if err != nil {
		t.Fatalf("buildPrompt: %v", err)
	}
	if !strings.Contains(got, "git checkout -b openpraxis/019dba9f-9c5") {
		t.Errorf("empty prefix should fall back to openpraxis; got:\n%s", got)
	}
}

// TestRunner_RecoverInFlight_StopMarksFailed asserts the default
// on_restart_behavior ("stop") marks orphans as failed with a
// diagnostic reason.
func TestRunner_RecoverInFlight_StopMarksFailed(t *testing.T) {
	r, _, _, _ := newRunnerHarness(t)

	insertRunningTask(t, r.store.db, "019dbaa4-orph-stop-aaaaaaaaaaaa")
	if err := r.store.SaveRuntimeState("019dbaa4-orph-stop-aaaaaaaaaaaa", "T", "", "claude-code", 12345, false, 3, 40, "last", time.Now()); err != nil {
		t.Fatalf("SaveRuntimeState: %v", err)
	}

	if err := r.RecoverInFlight(context.Background()); err != nil {
		t.Fatalf("RecoverInFlight: %v", err)
	}

	got, err := r.store.Get("019dbaa4-orph-stop-aaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != "failed" {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if !strings.Contains(got.LastOutput, "serve restart") {
		t.Errorf("LastOutput missing diagnostic; got %q", got.LastOutput)
	}
}

// TestRunner_RecoverInFlight_RestartReschedules asserts that setting
// on_restart_behavior=restart at system scope resets orphans to
// `scheduled` with next_run_at=now so the scheduler re-fires them.
func TestRunner_RecoverInFlight_RestartReschedules(t *testing.T) {
	r, store, _, _ := newRunnerHarness(t)

	raw, _ := json.Marshal("restart")
	if err := store.Set(context.Background(), settings.ScopeSystem, "", "on_restart_behavior", string(raw), "test"); err != nil {
		t.Fatalf("set on_restart_behavior: %v", err)
	}

	insertRunningTask(t, r.store.db, "019dbaa4-orph-restart-aaaaaa")

	if err := r.RecoverInFlight(context.Background()); err != nil {
		t.Fatalf("RecoverInFlight: %v", err)
	}

	got, err := r.store.Get("019dbaa4-orph-restart-aaaaaa")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != "scheduled" {
		t.Fatalf("status = %q, want scheduled", got.Status)
	}
	if got.NextRunAt == "" {
		t.Fatalf("NextRunAt empty; want a timestamp so the scheduler picks it up")
	}
}

// TestRunner_RecoverInFlight_FailStaysFailed asserts the strictest
// mode leaves tasks failed without the auto-recovery hint.
func TestRunner_RecoverInFlight_FailStaysFailed(t *testing.T) {
	r, store, _, _ := newRunnerHarness(t)

	raw, _ := json.Marshal("fail")
	if err := store.Set(context.Background(), settings.ScopeSystem, "", "on_restart_behavior", string(raw), "test"); err != nil {
		t.Fatalf("set on_restart_behavior: %v", err)
	}

	insertRunningTask(t, r.store.db, "019dbaa4-orph-fail-aaaaaaaaaa")

	if err := r.RecoverInFlight(context.Background()); err != nil {
		t.Fatalf("RecoverInFlight: %v", err)
	}

	got, err := r.store.Get("019dbaa4-orph-fail-aaaaaaaaaa")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != "failed" {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if !strings.Contains(got.LastOutput, "no auto-recovery") {
		t.Errorf("LastOutput missing strict-mode hint; got %q", got.LastOutput)
	}
}

// TestScheduler_ResolveTick_ReadsSystemKnob asserts that setting
// scheduler_tick_seconds at system scope changes the tick duration
// returned by resolveTick — which in turn drives Start's loop.
func TestScheduler_ResolveTick_ReadsSystemKnob(t *testing.T) {
	r, store, _, _ := newRunnerHarness(t)

	s := NewScheduler(r.store, 10*time.Second, nil, nil)
	s.SetResolver(r.resolver)

	raw, _ := json.Marshal(2)
	if err := store.Set(context.Background(), settings.ScopeSystem, "", "scheduler_tick_seconds", string(raw), "test"); err != nil {
		t.Fatalf("set scheduler_tick_seconds: %v", err)
	}
	if got := s.resolveTick(); got != 2*time.Second {
		t.Errorf("resolveTick = %v, want 2s", got)
	}

	raw, _ = json.Marshal(30)
	if err := store.Set(context.Background(), settings.ScopeSystem, "", "scheduler_tick_seconds", string(raw), "test"); err != nil {
		t.Fatalf("update scheduler_tick_seconds: %v", err)
	}
	if got := s.resolveTick(); got != 30*time.Second {
		t.Errorf("resolveTick after update = %v, want 30s", got)
	}

	// Below-floor values get clamped to the 2s floor.
	raw, _ = json.Marshal(1)
	if err := store.Set(context.Background(), settings.ScopeSystem, "", "scheduler_tick_seconds", string(raw), "test"); err != nil {
		t.Fatalf("update below-floor: %v", err)
	}
	// The catalog's slider_min=2 is a UI hint; the store accepts any int, so
	// we enforce the floor in resolveTick itself.
	if got := s.resolveTick(); got != schedulerTickFloor {
		t.Errorf("resolveTick with below-floor knob = %v, want %v", got, schedulerTickFloor)
	}
}

// TestResolveWorkspacePath covers the three path shapes the runner
// honours: empty (default), relative (joined onto repoDir), and
// absolute (verbatim).
func TestResolveWorkspacePath(t *testing.T) {
	repo := "/srv/repo"
	taskID := "019dbaa4-abc"

	if got := resolveWorkspacePath(repo, "", taskID); got != "/srv/repo/.openpraxis-work/019dbaa4-abc" {
		t.Errorf("empty baseDir: got %q", got)
	}
	if got := resolveWorkspacePath(repo, "custom-dir", taskID); got != "/srv/repo/custom-dir/019dbaa4-abc" {
		t.Errorf("relative baseDir: got %q", got)
	}
	if got := resolveWorkspacePath(repo, "/tmp/openpraxis-runs", taskID); got != "/tmp/openpraxis-runs/019dbaa4-abc" {
		t.Errorf("absolute baseDir: got %q", got)
	}
}
