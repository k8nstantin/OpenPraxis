package execution

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

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
