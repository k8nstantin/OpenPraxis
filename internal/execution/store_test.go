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

func TestInsert_and_get(t *testing.T) {
	db := openTestDB(t)
	if err := InitSchema(db); err != nil {
		t.Fatal(err)
	}
	s := NewStore(db)
	ctx := context.Background()
	exitCode := 0
	prNum := 42
	in := Row{
		ID:                "test-run-001",
		EntityKind:        KindTask,
		EntityID:          "task-abc",
		RunNumber:         1,
		Trigger:           "manual",
		NodeID:            "node-x",
		Status:            "completed",
		TerminalReason:    "success",
		StartedAt:         1000,
		CompletedAt:       2000,
		DurationMS:        1000,
		TTFBMS:            50,
		ExitCode:          &exitCode,
		Error:             "",
		CancelledAt:       0,
		CancelledBy:       "",
		Provider:          "anthropic",
		Model:             "claude-sonnet-4-6",
		ModelContextSize:  200000,
		AgentRuntime:      "claude-code",
		AgentVersion:      "1.2.3",
		PricingVersion:    "v2",
		InputTokens:       100,
		OutputTokens:      200,
		CacheReadTokens:   50,
		CacheCreateTokens: 30,
		ReasoningTokens:   10,
		ToolUseTokens:     5,
		CostUSD:           0.005,
		EstimatedCostUSD:  0.006,
		CacheSavingsUSD:   0.001,
		CacheHitRatePct:   33.3,
		ContextWindowPct:  0.5,
		CostPerTurn:       0.0025,
		CostPerAction:     0.001,
		TokensPerTurn:     150.0,
		Turns:             2,
		Actions:           5,
		Errors:            0,
		Compactions:       1,
		ParallelTasks:     0,
		ToolCallsJSON:     `{"Read":3}`,
		LinesAdded:        10,
		LinesRemoved:      2,
		FilesChanged:      1,
		Commits:           1,
		PRNumber:          &prNum,
		Branch:            "feat/x",
		CommitSHA:         "abc123",
		WorktreePath:      "/tmp/wt",
		PeakCPUPct:        80.0,
		AvgCPUPct:         40.0,
		PeakRSSMB:         512.0,
		AvgRSSMB:          256.0,
		DiskUsedGB:        1.5,
		LastOutput:        "done",
	}
	if err := s.Insert(ctx, in); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	got, err := s.Get(ctx, in.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != in.ID {
		t.Errorf("ID: got %q want %q", got.ID, in.ID)
	}
	if got.EntityKind != in.EntityKind {
		t.Errorf("EntityKind: got %q want %q", got.EntityKind, in.EntityKind)
	}
	if got.EntityID != in.EntityID {
		t.Errorf("EntityID: got %q want %q", got.EntityID, in.EntityID)
	}
	if got.Status != in.Status {
		t.Errorf("Status: got %q want %q", got.Status, in.Status)
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
	if got.LastOutput != in.LastOutput {
		t.Errorf("LastOutput: got %q want %q", got.LastOutput, in.LastOutput)
	}
}

func TestListByEntity_order(t *testing.T) {
	db := openTestDB(t)
	if err := InitSchema(db); err != nil {
		t.Fatal(err)
	}
	s := NewStore(db)
	ctx := context.Background()
	for i, ts := range []int64{100, 300, 200} {
		r := Row{
			ID:        fmt.Sprintf("run-%d", i),
			EntityKind: KindTask,
			EntityID:  "entity-1",
			StartedAt: ts,
		}
		if err := s.Insert(ctx, r); err != nil {
			t.Fatalf("Insert %d: %v", i, err)
		}
	}
	rows, err := s.ListByEntity(ctx, KindTask, "entity-1", 10)
	if err != nil {
		t.Fatalf("ListByEntity: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	// started_at DESC: 300, 200, 100
	if rows[0].StartedAt != 300 || rows[1].StartedAt != 200 || rows[2].StartedAt != 100 {
		t.Errorf("wrong order: %v %v %v", rows[0].StartedAt, rows[1].StartedAt, rows[2].StartedAt)
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

func TestInsertSample_and_list(t *testing.T) {
	db := openTestDB(t)
	if err := InitSchema(db); err != nil {
		t.Fatal(err)
	}
	s := NewStore(db)
	ctx := context.Background()
	for _, sm := range []Sample{
		{RunID: "r1", TS: 200, CPUPct: 50, RSSMB: 256},
		{RunID: "r1", TS: 100, CPUPct: 30, RSSMB: 128},
	} {
		if err := s.InsertSample(ctx, sm); err != nil {
			t.Fatalf("InsertSample: %v", err)
		}
	}
	samples, err := s.ListSamples(ctx, "r1")
	if err != nil {
		t.Fatalf("ListSamples: %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(samples))
	}
	// ts ASC: 100, 200
	if samples[0].TS != 100 || samples[1].TS != 200 {
		t.Errorf("wrong order: %v %v", samples[0].TS, samples[1].TS)
	}
	if samples[0].CPUPct != 30 {
		t.Errorf("CPUPct: got %v want 30", samples[0].CPUPct)
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
	rows, err := s.ListByEntity(ctx, KindTask, "task-xyz", 10)
	if err != nil {
		t.Fatalf("ListByEntity: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	r := rows[0]

	if r.EntityKind != KindTask {
		t.Errorf("EntityKind: got %q want %q", r.EntityKind, KindTask)
	}
	if r.EntityID != "task-xyz" {
		t.Errorf("EntityID: got %q want task-xyz", r.EntityID)
	}
	if r.RunNumber != 3 {
		t.Errorf("RunNumber: got %d want 3", r.RunNumber)
	}
	if r.Status != "completed" {
		t.Errorf("Status: got %q want completed", r.Status)
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
