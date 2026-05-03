package task

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	executionlog "github.com/k8nstantin/OpenPraxis/internal/execution"
)

// TestRunStart_writes_exec_log — EL/M2-T1 acceptance: when the runner has an
// execution_log store wired and the run-start helper fires, exactly one row
// lands in execution_log for the task with status=running and the identity +
// agent + worktree fields the rest of M2 will update on completion.
func TestRunStart_writes_exec_log(t *testing.T) {
	r, _, _, _ := newRunnerHarness(t)

	// Open a separate sqlite for the execution_log store so this test stays
	// independent of the harness's schema.
	dbPath := filepath.Join(t.TempDir(), "execlog.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := executionlog.InitSchema(db); err != nil {
		t.Fatalf("execution.InitSchema: %v", err)
	}
	store := executionlog.NewStore(db)
	r.SetExecutionLog(store)
	r.SetSourceNode("node-test")

	taskID := "t-exec-start"
	tk := &Task{ID: taskID, Title: "exec start", Agent: "claude-code", Schedule: "once"}
	rt := &RunningTask{
		TaskID:    taskID,
		Title:     tk.Title,
		Agent:     tk.Agent,
		StartedAt: time.Now(),
		Model:     "claude-opus-4-7",
	}
	workDir := "/tmp/wt-exec-start"

	r.recordExecLogStart(context.Background(), rt, tk, workDir)

	if rt.ExecLogID == "" {
		t.Fatalf("recordExecLogStart did not stamp ExecLogID on RunningTask")
	}

	rows, err := store.ListByEntity(context.Background(), executionlog.KindTask, taskID, 10)
	if err != nil {
		t.Fatalf("ListByEntity: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("ListByEntity rows = %d, want 1", len(rows))
	}
	got := rows[0]
	if got.ID != rt.ExecLogID {
		t.Fatalf("row id = %q, want %q (rt.ExecLogID)", got.ID, rt.ExecLogID)
	}
	if got.Status != "running" {
		t.Fatalf("row status = %q, want %q", got.Status, "running")
	}
	if got.EntityKind != executionlog.KindTask {
		t.Fatalf("entity_kind = %q, want %q", got.EntityKind, executionlog.KindTask)
	}
	if got.EntityID != taskID {
		t.Fatalf("entity_id = %q, want %q", got.EntityID, taskID)
	}
	if got.AgentRuntime != "claude-code" {
		t.Fatalf("agent_runtime = %q, want %q", got.AgentRuntime, "claude-code")
	}
	if got.WorktreePath != workDir {
		t.Fatalf("worktree_path = %q, want %q", got.WorktreePath, workDir)
	}
	if got.NodeID != "node-test" {
		t.Fatalf("node_id = %q, want %q", got.NodeID, "node-test")
	}
	if got.Model != "claude-opus-4-7" {
		t.Fatalf("model = %q, want %q", got.Model, "claude-opus-4-7")
	}
	if got.Provider != "anthropic" {
		t.Fatalf("provider = %q, want %q (LookupModel should have filled this)", got.Provider, "anthropic")
	}
	if got.ModelContextSize != 1_000_000 {
		t.Fatalf("model_context_size = %d, want %d", got.ModelContextSize, 1_000_000)
	}
	if got.Trigger != "once" {
		t.Fatalf("trigger = %q, want %q", got.Trigger, "once")
	}
	if got.StartedAt == 0 {
		t.Fatalf("started_at = 0, want non-zero (rt.StartedAt.UnixMilli)")
	}
}

// TestRunStart_no_store_is_noop — when no execution_log store is wired, the
// helper must be a no-op and must not stamp ExecLogID.
func TestRunStart_no_store_is_noop(t *testing.T) {
	r, _, _, _ := newRunnerHarness(t)
	// Deliberately do NOT call SetExecutionLog.

	rt := &RunningTask{TaskID: "t-noop", StartedAt: time.Now()}
	tk := &Task{ID: "t-noop", Agent: "claude-code"}
	r.recordExecLogStart(context.Background(), rt, tk, "/tmp/wt-noop")

	if rt.ExecLogID != "" {
		t.Fatalf("ExecLogID = %q, want empty (no store wired)", rt.ExecLogID)
	}
}

// TestTTFB_nonzero — EL/M2-T2 acceptance: after a run-start row is in place
// and the first assistant-event handler fires recordExecLogTTFB, the row
// should carry a positive ttfb_ms and remain in status=running (TTFB does
// not flip terminal state).
func TestTTFB_nonzero(t *testing.T) {
	r, _, _, _ := newRunnerHarness(t)

	dbPath := filepath.Join(t.TempDir(), "execlog.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := executionlog.InitSchema(db); err != nil {
		t.Fatalf("execution.InitSchema: %v", err)
	}
	store := executionlog.NewStore(db)
	r.SetExecutionLog(store)
	r.SetSourceNode("node-test")

	taskID := "t-ttfb"
	tk := &Task{ID: taskID, Title: "ttfb", Agent: "claude-code", Schedule: "once"}
	rt := &RunningTask{
		TaskID:    taskID,
		Title:     tk.Title,
		Agent:     tk.Agent,
		StartedAt: time.Now(),
		Model:     "claude-opus-4-7",
	}
	r.recordExecLogStart(context.Background(), rt, tk, "/tmp/wt-ttfb")
	if rt.ExecLogID == "" {
		t.Fatalf("recordExecLogStart did not stamp ExecLogID")
	}

	// Simulate first-assistant-event arrival after a measurable delay so
	// time.Since(rt.StartedAt) is guaranteed positive on every platform.
	time.Sleep(2 * time.Millisecond)
	rt.ttfbOnce.Do(func() {
		r.recordExecLogTTFB(context.Background(), rt)
	})

	got, err := store.Get(context.Background(), rt.ExecLogID)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if got.TTFBMS <= 0 {
		t.Fatalf("ttfb_ms = %d, want > 0", got.TTFBMS)
	}
	if got.Status != "running" {
		t.Fatalf("status = %q after ttfb stamp, want %q (TTFB must not flip terminal state)", got.Status, "running")
	}

	// Re-firing the once-guard must be a no-op — TTFB stays as the first-event
	// measurement. We capture the value before re-firing and assert it is
	// unchanged afterwards.
	first := got.TTFBMS
	time.Sleep(5 * time.Millisecond)
	rt.ttfbOnce.Do(func() {
		r.recordExecLogTTFB(context.Background(), rt)
	})
	got2, err := store.Get(context.Background(), rt.ExecLogID)
	if err != nil {
		t.Fatalf("store.Get (second): %v", err)
	}
	if got2.TTFBMS != first {
		t.Fatalf("ttfb_ms re-fired: was %d, now %d — sync.Once is supposed to gate this", first, got2.TTFBMS)
	}
}

// TestTTFB_no_execlog_id_is_noop — when the runner was constructed without an
// execution_log store wired (so rt.ExecLogID stays empty), recordExecLogTTFB
// must return silently and write nothing.
func TestTTFB_no_execlog_id_is_noop(t *testing.T) {
	r, _, _, _ := newRunnerHarness(t)

	dbPath := filepath.Join(t.TempDir(), "execlog.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := executionlog.InitSchema(db); err != nil {
		t.Fatalf("execution.InitSchema: %v", err)
	}
	store := executionlog.NewStore(db)
	// Wire the store so we can verify no rows land for the unstamped task.
	r.SetExecutionLog(store)

	rt := &RunningTask{TaskID: "t-ttfb-noop", StartedAt: time.Now()}
	// Deliberately leave rt.ExecLogID == "" — recordExecLogStart was not
	// called, so the helper should bail before issuing any UPDATE.
	r.recordExecLogTTFB(context.Background(), rt)

	rows, err := store.ListByEntity(context.Background(), executionlog.KindTask, "t-ttfb-noop", 10)
	if err != nil {
		t.Fatalf("ListByEntity: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("ListByEntity rows = %d, want 0 (nothing should have landed)", len(rows))
	}
}
