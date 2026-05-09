package execution

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "exec.db") + "?_journal_mode=WAL&_busy_timeout=5000"
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestInitSchema_idempotent(t *testing.T) {
	db := openTestDB(t)
	if err := InitSchema(db); err != nil {
		t.Fatalf("first InitSchema: %v", err)
	}
	if err := InitSchema(db); err != nil {
		t.Fatalf("second InitSchema: %v", err)
	}
}

func TestInsert_and_listByRun(t *testing.T) {
	db := openTestDB(t)
	if err := InitSchema(db); err != nil {
		t.Fatal(err)
	}
	s := NewStore(db)
	ctx := context.Background()
	exitCode := 0
	prNum := 42
	runUID := "run-uid-001"
	in := Row{
		ID:                 "test-row-001",
		RunUID:             runUID,
		EntityUID:          "task-abc",
		Event:              EventCompleted,
		RunNumber:          1,
		Trigger:            "manual",
		NodeID:             "node-x",
		TerminalReason:     "success",
		StartedAt:          1000,
		CompletedAt:        2000,
		DurationMS:         1000,
		TTFBMS:             50,
		ExitCode:           &exitCode,
		Error:              "",
		CancelledAt:        0,
		CancelledBy:        "",
		Provider:           "anthropic",
		Model:              "claude-sonnet-4-6",
		ModelContextSize:   200000,
		AgentRuntime:       "claude-code",
		AgentVersion:       "1.2.3",
		PricingVersion:     "v2",
		InputTokens:        100,
		OutputTokens:       200,
		CacheReadTokens:    50,
		CacheCreateTokens:  30,
		ReasoningTokens:    10,
		ToolUseTokens:      5,
		CostUSD:            0.005,
		EstimatedCostUSD:   0.006,
		CacheSavingsUSD:    0.001,
		CacheHitRatePct:    33.3,
		ContextWindowPct:   0.5,
		CostPerTurn:        0.0025,
		CostPerAction:      0.001,
		TokensPerTurn:      150.0,
		Turns:              2,
		Actions:            5,
		Errors:             0,
		Compactions:        1,
		ParallelTasks:      0,
		ToolCallsJSON:      `{"Read":3}`,
		LinesAdded:         10,
		LinesRemoved:       2,
		FilesChanged:       1,
		Commits:            1,
		PRNumber:           &prNum,
		Branch:             "feat/x",
		CommitSHA:          "abc123",
		WorktreePath:       "/tmp/wt",
		CPUPct:             12.5,
		RSSMB:              128.0,
		DiskUsedGB:         1.5,
		PeakCPUPct:         80.0,
		AvgCPUPct:          40.0,
		PeakRSSMB:          512.0,
		AvgRSSMB:           256.0,
		CreatedBy:          "test",
		CreatedAt:          time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := s.Insert(ctx, in); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	rows, err := s.ListByRun(ctx, runUID)
	if err != nil {
		t.Fatalf("ListByRun: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	got := rows[0]
	if got.ID != in.ID {
		t.Errorf("ID: got %q want %q", got.ID, in.ID)
	}
	if got.EntityUID != in.EntityUID {
		t.Errorf("EntityUID: got %q want %q", got.EntityUID, in.EntityUID)
	}
	if got.Event != in.Event {
		t.Errorf("Event: got %q want %q", got.Event, in.Event)
	}
	if got.CostUSD != in.CostUSD {
		t.Errorf("CostUSD: got %v want %v", got.CostUSD, in.CostUSD)
	}
	if got.ExitCode == nil || *got.ExitCode != *in.ExitCode {
		t.Errorf("ExitCode mismatch")
	}
	if got.PRNumber == nil || *got.PRNumber != *in.PRNumber {
		t.Errorf("PRNumber mismatch")
	}
	if got.ToolCallsJSON != in.ToolCallsJSON {
		t.Errorf("ToolCallsJSON: got %q want %q", got.ToolCallsJSON, in.ToolCallsJSON)
	}
	if got.CPUPct != in.CPUPct {
		t.Errorf("CPUPct: got %v want %v", got.CPUPct, in.CPUPct)
	}
}

func TestLatestByRun(t *testing.T) {
	db := openTestDB(t)
	if err := InitSchema(db); err != nil {
		t.Fatal(err)
	}
	s := NewStore(db)
	ctx := context.Background()
	runUID := "run-latest-test"

	for i, event := range []string{EventStarted, EventSample, EventCompleted} {
		r := Row{
			ID:        fmt.Sprintf("row-%d", i),
			RunUID:    runUID,
			EntityUID: "entity-1",
			Event:     event,
			CreatedAt: time.Now().Add(time.Duration(i) * time.Millisecond).UTC().Format(time.RFC3339Nano),
		}
		if err := s.Insert(ctx, r); err != nil {
			t.Fatalf("Insert %d: %v", i, err)
		}
	}

	latest, err := s.LatestByRun(ctx, runUID)
	if err != nil {
		t.Fatalf("LatestByRun: %v", err)
	}
	if latest.Event != EventCompleted {
		t.Errorf("expected latest event %q, got %q", EventCompleted, latest.Event)
	}
}

func TestLatestByRun_notFound(t *testing.T) {
	db := openTestDB(t)
	if err := InitSchema(db); err != nil {
		t.Fatal(err)
	}
	s := NewStore(db)
	_, err := s.LatestByRun(context.Background(), "no-such-run")
	if err != ErrRunNotFound {
		t.Errorf("expected ErrRunNotFound, got %v", err)
	}
}

func TestListByEntity_order(t *testing.T) {
	db := openTestDB(t)
	if err := InitSchema(db); err != nil {
		t.Fatal(err)
	}
	s := NewStore(db)
	ctx := context.Background()
	base := time.Now().UTC()
	for i, offset := range []time.Duration{0, 200 * time.Millisecond, 100 * time.Millisecond} {
		r := Row{
			ID:        fmt.Sprintf("run-%d", i),
			RunUID:    fmt.Sprintf("run-%d", i),
			EntityUID: "entity-1",
			Event:     EventCompleted,
			CreatedAt: base.Add(offset).Format(time.RFC3339Nano),
		}
		if err := s.Insert(ctx, r); err != nil {
			t.Fatalf("Insert %d: %v", i, err)
		}
	}
	rows, err := s.ListByEntity(ctx, "entity-1", 10)
	if err != nil {
		t.Fatalf("ListByEntity: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	// created_at DESC: offset 200ms, 100ms, 0ms
	if rows[0].ID != "run-1" || rows[1].ID != "run-2" || rows[2].ID != "run-0" {
		t.Errorf("wrong order: %v %v %v", rows[0].ID, rows[1].ID, rows[2].ID)
	}
}

func TestInsertOutput_and_listOutput(t *testing.T) {
	db := openTestDB(t)
	if err := InitSchema(db); err != nil {
		t.Fatal(err)
	}
	s := NewStore(db)
	ctx := context.Background()
	runUID := "out-run-1"
	for _, tc := range []struct {
		seq   int
		chunk string
	}{
		{2, "second"},
		{0, "first"},
		{1, "middle"},
	} {
		if err := s.InsertOutput(ctx, runUID, tc.chunk, tc.seq, "test"); err != nil {
			t.Fatalf("InsertOutput seq=%d: %v", tc.seq, err)
		}
	}
	chunks, err := s.ListOutput(ctx, runUID)
	if err != nil {
		t.Fatalf("ListOutput: %v", err)
	}
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	// seq ASC: 0, 1, 2
	if chunks[0].Chunk != "first" || chunks[1].Chunk != "middle" || chunks[2].Chunk != "second" {
		t.Errorf("wrong order: %v %v %v", chunks[0].Chunk, chunks[1].Chunk, chunks[2].Chunk)
	}
}

func TestLookupModel_known(t *testing.T) {
	info := LookupModel("claude-opus-4-7")
	if info.Provider != "anthropic" {
		t.Errorf("Provider: got %q want %q", info.Provider, "anthropic")
	}
	if info.ContextWindowSize != 1_000_000 {
		t.Errorf("ContextWindowSize: got %d want %d", info.ContextWindowSize, 1_000_000)
	}
}

func TestLookupModel_unknown(t *testing.T) {
	info := LookupModel("some-future-model-xyz")
	if info.Provider != "unknown" {
		t.Errorf("Provider: got %q want %q", info.Provider, "unknown")
	}
	if info.ContextWindowSize != 200_000 {
		t.Errorf("ContextWindowSize: got %d want %d", info.ContextWindowSize, 200_000)
	}
}

func TestEmptyEntityUID_rejected(t *testing.T) {
	db := openTestDB(t)
	if err := InitSchema(db); err != nil {
		t.Fatal(err)
	}
	s := NewStore(db)
	err := s.Insert(context.Background(), Row{RunUID: "r", Event: EventStarted})
	if err != ErrEmptyEntityID {
		t.Errorf("expected ErrEmptyEntityID, got %v", err)
	}
}

// seedTaskRuns creates the task_runs table and inserts rows for backfill tests.
func seedTaskRuns(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS task_runs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id TEXT NOT NULL,
		run_number INTEGER NOT NULL DEFAULT 0,
		output TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT '',
		actions INTEGER NOT NULL DEFAULT 0,
		lines INTEGER NOT NULL DEFAULT 0,
		cost_usd REAL NOT NULL DEFAULT 0,
		turns INTEGER NOT NULL DEFAULT 0,
		started_at TEXT NOT NULL DEFAULT '',
		completed_at TEXT NOT NULL DEFAULT '',
		input_tokens INTEGER NOT NULL DEFAULT 0,
		output_tokens INTEGER NOT NULL DEFAULT 0,
		cache_read_tokens INTEGER NOT NULL DEFAULT 0,
		cache_create_tokens INTEGER NOT NULL DEFAULT 0,
		model TEXT NOT NULL DEFAULT '',
		pricing_version TEXT NOT NULL DEFAULT '',
		duration_ms INTEGER NOT NULL DEFAULT 0,
		agent_runtime TEXT NOT NULL DEFAULT '',
		agent_version TEXT NOT NULL DEFAULT '',
		peak_cpu_pct REAL NOT NULL DEFAULT 0,
		avg_cpu_pct REAL NOT NULL DEFAULT 0,
		peak_rss_mb REAL NOT NULL DEFAULT 0,
		avg_rss_mb REAL NOT NULL DEFAULT 0,
		errors INTEGER NOT NULL DEFAULT 0,
		compactions INTEGER NOT NULL DEFAULT 0,
		files_changed INTEGER NOT NULL DEFAULT 0,
		lines_added INTEGER NOT NULL DEFAULT 0,
		lines_removed INTEGER NOT NULL DEFAULT 0,
		exit_code INTEGER NOT NULL DEFAULT 0,
		cancelled_at TEXT NOT NULL DEFAULT '',
		cancelled_by TEXT NOT NULL DEFAULT '',
		branch TEXT NOT NULL DEFAULT '',
		commit_sha TEXT NOT NULL DEFAULT '',
		commits INTEGER NOT NULL DEFAULT 0,
		pr_number INTEGER NOT NULL DEFAULT 0,
		worktree_path TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		t.Fatalf("seedTaskRuns: create table: %v", err)
	}
}

func insertTaskRun(t *testing.T, db *sql.DB, taskID string, runNum int, status string, startedAt time.Time, cost float64, turns, actions int) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO task_runs
		(task_id, run_number, output, status, actions, lines, cost_usd, turns,
		 started_at, completed_at, input_tokens, output_tokens,
		 cache_read_tokens, cache_create_tokens, model, pricing_version,
		 duration_ms, agent_runtime, agent_version,
		 peak_cpu_pct, avg_cpu_pct, peak_rss_mb, avg_rss_mb,
		 errors, compactions, files_changed, lines_added, lines_removed,
		 exit_code, cancelled_at, cancelled_by,
		 branch, commit_sha, commits, pr_number, worktree_path)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		taskID, runNum, "done", status, actions, 0, cost, turns,
		startedAt.UTC().Format(time.RFC3339),
		startedAt.Add(10*time.Second).UTC().Format(time.RFC3339),
		100, 50, 20, 10, "claude-sonnet-4-6", "v1-2026-04",
		10000, "claude-code", "",
		70.0, 40.0, 256.0, 200.0,
		0, 0, 2, 5, 1,
		0, "", "",
		"feat/x", "abc123", 1, 0, "/tmp/wt",
	)
	if err != nil {
		t.Fatalf("insertTaskRun: %v", err)
	}
}

// insertTerminalRow is a small helper used by pass-rate / frontier tests.
// It writes a terminal execution_log row with the given event, terminal_reason,
// turns, cost, and an explicit created_at so window-filter tests can backdate.
func insertTerminalRow(t *testing.T, s *Store, runID, entityUID, event, reason string, turns int, cost float64, createdAt time.Time) {
	t.Helper()
	if err := s.Insert(context.Background(), Row{
		ID:             runID,
		RunUID:         runID,
		EntityUID:      entityUID,
		Event:          event,
		TerminalReason: reason,
		Turns:          turns,
		CostUSD:        cost,
		CreatedAt:      createdAt.UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("insert %s: %v", runID, err)
	}
}

func TestFrontierByManifest_emptyTaskIDs(t *testing.T) {
	db := openTestDB(t)
	if err := InitSchema(db); err != nil {
		t.Fatal(err)
	}
	s := NewStore(db)

	got, err := s.FrontierByManifest(context.Background(), nil, 30)
	if err != nil {
		t.Fatalf("FrontierByManifest(nil): %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil empty map, got nil")
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(got))
	}

	got, err = s.FrontierByManifest(context.Background(), []string{}, 30)
	if err != nil {
		t.Fatalf("FrontierByManifest([]): %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil empty map, got nil")
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(got))
	}
}

func TestPassRateByEntity_singleSuccess(t *testing.T) {
	db := openTestDB(t)
	if err := InitSchema(db); err != nil {
		t.Fatal(err)
	}
	s := NewStore(db)
	now := time.Now().UTC()
	insertTerminalRow(t, s, "run-1", "task-pass", EventCompleted, TerminalReasonSuccess, 4, 0.10, now)

	got, err := s.PassRateByEntity(context.Background(), "task-pass", 30)
	if err != nil {
		t.Fatalf("PassRateByEntity: %v", err)
	}
	if got.EntityUID != "task-pass" {
		t.Errorf("EntityUID: got %q want task-pass", got.EntityUID)
	}
	if got.TotalRuns != 1 || got.SuccessRuns != 1 {
		t.Errorf("counts: total=%d success=%d, want 1/1", got.TotalRuns, got.SuccessRuns)
	}
	if got.PassRate != 1.0 {
		t.Errorf("PassRate: got %v want 1.0", got.PassRate)
	}
	if got.FailedRuns != 0 || got.MaxTurnsRuns != 0 || got.TimeoutRuns != 0 {
		t.Errorf("non-success buckets: failed=%d max_turns=%d timeout=%d, want 0",
			got.FailedRuns, got.MaxTurnsRuns, got.TimeoutRuns)
	}
}

func TestPassRateByEntity_singleMaxTurns(t *testing.T) {
	db := openTestDB(t)
	if err := InitSchema(db); err != nil {
		t.Fatal(err)
	}
	s := NewStore(db)
	now := time.Now().UTC()
	insertTerminalRow(t, s, "run-mt", "task-mt", EventCompleted, TerminalReasonMaxTurns, 100, 0.50, now)

	got, err := s.PassRateByEntity(context.Background(), "task-mt", 30)
	if err != nil {
		t.Fatalf("PassRateByEntity: %v", err)
	}
	if got.TotalRuns != 1 || got.MaxTurnsRuns != 1 || got.SuccessRuns != 0 {
		t.Errorf("counts: total=%d success=%d max_turns=%d, want 1/0/1",
			got.TotalRuns, got.SuccessRuns, got.MaxTurnsRuns)
	}
	if got.PassRate != 0.0 {
		t.Errorf("PassRate: got %v want 0.0", got.PassRate)
	}
}

func TestPassRateByEntity_mixed(t *testing.T) {
	db := openTestDB(t)
	if err := InitSchema(db); err != nil {
		t.Fatal(err)
	}
	s := NewStore(db)
	now := time.Now().UTC()
	insertTerminalRow(t, s, "run-a", "task-mix", EventCompleted, TerminalReasonSuccess, 5, 0.10, now)
	insertTerminalRow(t, s, "run-b", "task-mix", EventCompleted, TerminalReasonMaxTurns, 100, 0.30, now.Add(time.Second))

	got, err := s.PassRateByEntity(context.Background(), "task-mix", 30)
	if err != nil {
		t.Fatalf("PassRateByEntity: %v", err)
	}
	if got.TotalRuns != 2 {
		t.Errorf("TotalRuns: got %d want 2", got.TotalRuns)
	}
	if got.SuccessRuns != 1 || got.MaxTurnsRuns != 1 {
		t.Errorf("counts: success=%d max_turns=%d, want 1/1", got.SuccessRuns, got.MaxTurnsRuns)
	}
	if got.PassRate != 0.5 {
		t.Errorf("PassRate: got %v want 0.5", got.PassRate)
	}
}

func TestPassRateByEntity_neverRun(t *testing.T) {
	db := openTestDB(t)
	if err := InitSchema(db); err != nil {
		t.Fatal(err)
	}
	s := NewStore(db)

	got, err := s.PassRateByEntity(context.Background(), "task-ghost", 30)
	if err != nil {
		t.Fatalf("PassRateByEntity: %v", err)
	}
	if got.EntityUID != "task-ghost" {
		t.Errorf("EntityUID: got %q want task-ghost", got.EntityUID)
	}
	if got.TotalRuns != 0 {
		t.Errorf("TotalRuns: got %d want 0", got.TotalRuns)
	}
	if got.PassRate != 0.0 {
		t.Errorf("PassRate: got %v want 0.0", got.PassRate)
	}
}

func TestPassRateByEntity_classification(t *testing.T) {
	db := openTestDB(t)
	if err := InitSchema(db); err != nil {
		t.Fatal(err)
	}
	s := NewStore(db)
	now := time.Now().UTC()
	// One of each terminal-reason bucket plus an arbitrary other reason.
	insertTerminalRow(t, s, "run-s", "task-c", EventCompleted, TerminalReasonSuccess, 1, 0, now)
	insertTerminalRow(t, s, "run-empty", "task-c", EventCompleted, "", 1, 0, now.Add(time.Millisecond))
	insertTerminalRow(t, s, "run-mt", "task-c", EventCompleted, TerminalReasonMaxTurns, 1, 0, now.Add(2*time.Millisecond))
	insertTerminalRow(t, s, "run-to", "task-c", EventFailed, TerminalReasonTimeout, 1, 0, now.Add(3*time.Millisecond))
	insertTerminalRow(t, s, "run-be", "task-c", EventFailed, "build_fail", 1, 0, now.Add(4*time.Millisecond))
	insertTerminalRow(t, s, "run-pe", "task-c", EventFailed, "process_error", 1, 0, now.Add(5*time.Millisecond))

	got, err := s.PassRateByEntity(context.Background(), "task-c", 30)
	if err != nil {
		t.Fatalf("PassRateByEntity: %v", err)
	}
	if got.TotalRuns != 6 {
		t.Errorf("TotalRuns: got %d want 6", got.TotalRuns)
	}
	if got.SuccessRuns != 2 {
		t.Errorf("SuccessRuns: got %d want 2 (success + empty reason)", got.SuccessRuns)
	}
	if got.MaxTurnsRuns != 1 {
		t.Errorf("MaxTurnsRuns: got %d want 1", got.MaxTurnsRuns)
	}
	if got.TimeoutRuns != 1 {
		t.Errorf("TimeoutRuns: got %d want 1", got.TimeoutRuns)
	}
	if got.FailedRuns != 2 {
		t.Errorf("FailedRuns: got %d want 2 (build_fail + process_error)", got.FailedRuns)
	}
}

func TestPassRateByEntity_windowFiltersOldRows(t *testing.T) {
	db := openTestDB(t)
	if err := InitSchema(db); err != nil {
		t.Fatal(err)
	}
	s := NewStore(db)
	now := time.Now().UTC()
	// One recent (in-window), one 48h old (out-of-window for windowDays=1).
	insertTerminalRow(t, s, "run-recent", "task-win", EventCompleted, TerminalReasonSuccess, 1, 0, now)
	insertTerminalRow(t, s, "run-old", "task-win", EventCompleted, TerminalReasonMaxTurns, 1, 0, now.Add(-48*time.Hour))

	got, err := s.PassRateByEntity(context.Background(), "task-win", 1)
	if err != nil {
		t.Fatalf("PassRateByEntity windowDays=1: %v", err)
	}
	if got.TotalRuns != 1 {
		t.Errorf("windowDays=1 TotalRuns: got %d want 1 (old row should be excluded)", got.TotalRuns)
	}
	if got.SuccessRuns != 1 {
		t.Errorf("windowDays=1 SuccessRuns: got %d want 1", got.SuccessRuns)
	}

	// windowDays=30 includes both rows.
	got, err = s.PassRateByEntity(context.Background(), "task-win", 30)
	if err != nil {
		t.Fatalf("PassRateByEntity windowDays=30: %v", err)
	}
	if got.TotalRuns != 2 {
		t.Errorf("windowDays=30 TotalRuns: got %d want 2", got.TotalRuns)
	}

	// windowDays<=0 disables the filter — both rows included.
	got, err = s.PassRateByEntity(context.Background(), "task-win", 0)
	if err != nil {
		t.Fatalf("PassRateByEntity windowDays=0: %v", err)
	}
	if got.TotalRuns != 2 {
		t.Errorf("windowDays=0 TotalRuns: got %d want 2 (no filter)", got.TotalRuns)
	}
}

func TestFrontierByManifest_perTaskMap(t *testing.T) {
	db := openTestDB(t)
	if err := InitSchema(db); err != nil {
		t.Fatal(err)
	}
	s := NewStore(db)
	now := time.Now().UTC()
	insertTerminalRow(t, s, "run-a1", "task-a", EventCompleted, TerminalReasonSuccess, 3, 0.10, now)
	insertTerminalRow(t, s, "run-a2", "task-a", EventFailed, TerminalReasonTimeout, 5, 0.20, now.Add(time.Second))
	insertTerminalRow(t, s, "run-b1", "task-b", EventCompleted, TerminalReasonMaxTurns, 100, 0.50, now)
	// task-c has zero runs and should be absent from result.

	got, err := s.FrontierByManifest(context.Background(), []string{"task-a", "task-b", "task-c"}, 30)
	if err != nil {
		t.Fatalf("FrontierByManifest: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got): %d want 2 (task-c absent)", len(got))
	}
	if _, ok := got["task-c"]; ok {
		t.Error("task-c should be absent from result map (no runs)")
	}
	a := got["task-a"]
	if a.TotalRuns != 2 || a.SuccessRuns != 1 || a.TimeoutRuns != 1 {
		t.Errorf("task-a: total=%d success=%d timeout=%d, want 2/1/1",
			a.TotalRuns, a.SuccessRuns, a.TimeoutRuns)
	}
	if a.PassRate != 0.5 {
		t.Errorf("task-a PassRate: got %v want 0.5", a.PassRate)
	}
	b := got["task-b"]
	if b.TotalRuns != 1 || b.MaxTurnsRuns != 1 || b.PassRate != 0.0 {
		t.Errorf("task-b: total=%d max_turns=%d pass=%v, want 1/1/0.0",
			b.TotalRuns, b.MaxTurnsRuns, b.PassRate)
	}
}

func TestPassRateByEntity_emptyEntityUID(t *testing.T) {
	db := openTestDB(t)
	if err := InitSchema(db); err != nil {
		t.Fatal(err)
	}
	s := NewStore(db)
	if _, err := s.PassRateByEntity(context.Background(), "", 30); err != ErrEmptyEntityID {
		t.Errorf("expected ErrEmptyEntityID, got %v", err)
	}
}

func TestBackfill_idempotent(t *testing.T) {
	db := openTestDB(t)
	if err := InitSchema(db); err != nil {
		t.Fatal(err)
	}
	seedTaskRuns(t, db)
	ctx := context.Background()
	started := time.Now().UTC().Truncate(time.Second)
	insertTaskRun(t, db, "task-001", 1, "completed", started, 0.01, 3, 5)
	insertTaskRun(t, db, "task-001", 2, "failed", started.Add(time.Minute), 0.005, 1, 2)

	n1, err := BackfillFromTaskRuns(ctx, db)
	if err != nil {
		t.Fatalf("first backfill: %v", err)
	}
	if n1 != 2 {
		t.Errorf("first backfill: expected 2 rows, got %d", n1)
	}

	n2, err := BackfillFromTaskRuns(ctx, db)
	if err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	if n2 != 0 {
		t.Errorf("second backfill should be no-op, got %d", n2)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM execution_log`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("row count after double backfill: got %d want 2", count)
	}
}

func TestBackfill_field_mapping(t *testing.T) {
	db := openTestDB(t)
	if err := InitSchema(db); err != nil {
		t.Fatal(err)
	}
	seedTaskRuns(t, db)
	ctx := context.Background()

	started := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	insertTaskRun(t, db, "task-xyz", 3, "completed", started, 0.025, 5, 10)

	if _, err := BackfillFromTaskRuns(ctx, db); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	s := NewStore(db)
	rows, err := s.ListByEntity(ctx, "task-xyz", 10)
	if err != nil {
		t.Fatalf("ListByEntity: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	r := rows[0]

	if r.EntityUID != "task-xyz" {
		t.Errorf("EntityUID: got %q want task-xyz", r.EntityUID)
	}
	if r.Event != EventCompleted {
		t.Errorf("Event: got %q want %q", r.Event, EventCompleted)
	}
	if r.RunNumber != 3 {
		t.Errorf("RunNumber: got %d want 3", r.RunNumber)
	}
	if r.TerminalReason != "success" {
		t.Errorf("TerminalReason: got %q want success", r.TerminalReason)
	}
	if r.CostUSD != 0.025 {
		t.Errorf("CostUSD: got %v want 0.025", r.CostUSD)
	}
	if r.Turns != 5 {
		t.Errorf("Turns: got %d want 5", r.Turns)
	}
	if r.Actions != 10 {
		t.Errorf("Actions: got %d want 10", r.Actions)
	}
	wantStartedAt := started.UnixMilli()
	if r.StartedAt != wantStartedAt {
		t.Errorf("StartedAt: got %d want %d", r.StartedAt, wantStartedAt)
	}
	if r.Provider != "anthropic" {
		t.Errorf("Provider: got %q want anthropic", r.Provider)
	}
	if r.Model != "claude-sonnet-4-6" {
		t.Errorf("Model: got %q want claude-sonnet-4-6", r.Model)
	}
	if r.ModelContextSize != 200_000 {
		t.Errorf("ModelContextSize: got %d want 200000", r.ModelContextSize)
	}
	// Derived: CacheHitRatePct = 20/(100+20)*100 ≈ 16.666...
	if r.CacheHitRatePct <= 0 {
		t.Errorf("CacheHitRatePct: got %v want > 0", r.CacheHitRatePct)
	}
	if r.CostPerTurn <= 0 {
		t.Errorf("CostPerTurn: got %v want > 0", r.CostPerTurn)
	}
	if r.Branch != "feat/x" {
		t.Errorf("Branch: got %q want feat/x", r.Branch)
	}
	if r.CommitSHA != "abc123" {
		t.Errorf("CommitSHA: got %q want abc123", r.CommitSHA)
	}
}
