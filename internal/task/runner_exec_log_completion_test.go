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

// TestUpdateCompletion_writes_all_fields — EL/M2-T4 acceptance: after a
// recorded run-start row, the completion helper must populate every
// completion-side column on the same execution_log row. This exercises the
// helper directly (no subprocess) so the test stays fast and deterministic.
func TestUpdateCompletion_writes_all_fields(t *testing.T) {
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

	taskID := "t-exec-complete"
	tk := &Task{ID: taskID, Title: "exec complete", Agent: "claude-code", Schedule: "once"}
	rt := &RunningTask{
		TaskID:    taskID,
		Title:     tk.Title,
		Agent:     tk.Agent,
		StartedAt: time.Now().Add(-2 * time.Second),
		Model:     "claude-sonnet-4-6",
		Actions:   12,
	}

	// Seed the at-start row so UpdateCompletion has a target.
	r.recordExecLogStart(context.Background(), rt, tk, "")

	in := completionInput{
		status:   "completed",
		reason:   "success",
		exitCode: 0,
		costUSD:  0.04,
		numTurns: 4,
		usage: Usage{
			InputTokens:         1_500,
			OutputTokens:        800,
			CacheReadTokens:     6_000,
			CacheCreationTokens: 200,
		},
		output: "final result",
		hostMetrics: HostMetrics{
			PeakCPUPct: 73.5,
			AvgCPUPct:  41.2,
			PeakRSSMB:  912,
			AvgRSSMB:   544,
		},
		// workDir empty → gitChurn returns zero result without forking.
	}
	r.recordExecLogCompletion(context.Background(), rt, in)

	got, err := store.Get(context.Background(), rt.ExecLogID)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}

	// Status / terminal
	if got.Status != "completed" {
		t.Errorf("status = %q, want completed", got.Status)
	}
	if got.TerminalReason != "success" {
		t.Errorf("terminal_reason = %q, want success", got.TerminalReason)
	}
	if got.CompletedAt == 0 {
		t.Errorf("completed_at = 0, want non-zero")
	}
	if got.DurationMS <= 0 {
		t.Errorf("duration_ms = %d, want > 0", got.DurationMS)
	}
	if got.ExitCode == nil || *got.ExitCode != 0 {
		t.Errorf("exit_code = %v, want 0", got.ExitCode)
	}

	// Tokens / cost
	if got.InputTokens != 1500 || got.OutputTokens != 800 ||
		got.CacheReadTokens != 6000 || got.CacheCreateTokens != 200 {
		t.Errorf("tokens mismatch: in=%d out=%d cr=%d cc=%d",
			got.InputTokens, got.OutputTokens, got.CacheReadTokens, got.CacheCreateTokens)
	}
	if got.CostUSD != 0.04 {
		t.Errorf("cost_usd = %v, want 0.04", got.CostUSD)
	}
	if got.Turns != 4 || got.Actions != 12 {
		t.Errorf("turns/actions = %d/%d, want 4/12", got.Turns, got.Actions)
	}
	if got.LastOutput != "final result" {
		t.Errorf("last_output = %q, want %q", got.LastOutput, "final result")
	}
	if got.PricingVersion == "" {
		t.Errorf("pricing_version empty, want stamped")
	}

	// Derived ratios — verify ComputeDerived ran through and the values
	// got persisted (exact arithmetic is in derived_test.go).
	if got.CacheHitRatePct == 0 {
		t.Errorf("cache_hit_rate_pct = 0, want >0")
	}
	if got.ContextWindowPct == 0 {
		t.Errorf("context_window_pct = 0, want >0")
	}
	if got.CostPerTurn == 0 {
		t.Errorf("cost_per_turn = 0, want >0")
	}
	if got.CostPerAction == 0 {
		t.Errorf("cost_per_action = 0, want >0")
	}
	if got.TokensPerTurn == 0 {
		t.Errorf("tokens_per_turn = 0, want >0")
	}
	if got.CacheSavingsUSD == 0 {
		t.Errorf("cache_savings_usd = 0, want >0")
	}

	// Host metrics rollup
	if got.PeakCPUPct != 73.5 || got.AvgCPUPct != 41.2 ||
		got.PeakRSSMB != 912 || got.AvgRSSMB != 544 {
		t.Errorf("host metrics mismatch: peakCPU=%v avgCPU=%v peakRSS=%v avgRSS=%v",
			got.PeakCPUPct, got.AvgCPUPct, got.PeakRSSMB, got.AvgRSSMB)
	}

	// Empty workDir → git fields stay zero / empty (not failures).
	if got.Branch != "" || got.CommitSHA != "" {
		t.Errorf("expected empty git fields with empty workDir, got branch=%q sha=%q",
			got.Branch, got.CommitSHA)
	}
}

// TestUpdateCompletion_no_store_is_noop — without a wired execLog store, the
// helper must not panic and must do nothing (no row, no log spam).
func TestUpdateCompletion_no_store_is_noop(t *testing.T) {
	r, _, _, _ := newRunnerHarness(t)
	// Deliberately do NOT call SetExecutionLog.

	rt := &RunningTask{TaskID: "t-noop", StartedAt: time.Now()}
	r.recordExecLogCompletion(context.Background(), rt, completionInput{status: "completed"})
}

// TestUpdateCompletion_no_exec_log_id_is_noop — recordExecLogStart did not
// fire (e.g. the store insert failed) so rt.ExecLogID is empty. The
// completion helper must short-circuit.
func TestUpdateCompletion_no_exec_log_id_is_noop(t *testing.T) {
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
	r.SetExecutionLog(executionlog.NewStore(db))

	rt := &RunningTask{TaskID: "t-empty-id", StartedAt: time.Now()} // no ExecLogID
	r.recordExecLogCompletion(context.Background(), rt, completionInput{status: "completed"})
}

// TestParseShortstat — git diff --shortstat parsing is reused for the run
// completion stats. Cover the canonical mixed-case + the edge cases where
// only insertions or only deletions land.
func TestParseShortstat(t *testing.T) {
	cases := []struct {
		in            string
		files, ins, del int
	}{
		{"3 files changed, 42 insertions(+), 7 deletions(-)", 3, 42, 7},
		{"1 file changed, 5 insertions(+)", 1, 5, 0},
		{"1 file changed, 9 deletions(-)", 1, 0, 9},
		{"", 0, 0, 0},
	}
	for _, tc := range cases {
		var f, a, d int
		parseShortstat(tc.in, &f, &a, &d)
		if f != tc.files || a != tc.ins || d != tc.del {
			t.Errorf("parseShortstat(%q) = (%d,%d,%d), want (%d,%d,%d)",
				tc.in, f, a, d, tc.files, tc.ins, tc.del)
		}
	}
}
