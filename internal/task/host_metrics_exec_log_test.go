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

// TestHostSampler_fanout_writes_execution_log_samples — EL/M2-T3 acceptance:
// after two fanout() ticks, execution_log_samples has 2 rows for the
// registered runID and 0 rows for an attached-but-unregistered task.
func TestHostSampler_fanout_writes_execution_log_samples(t *testing.T) {
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

	hs := NewHostSampler(time.Second)
	hs.SetExecLogStore(store)

	// Two attached tasks: one is registered with an execution_log run id,
	// the other is intentionally left unregistered to assert the fanout
	// only writes for tasks the runner explicitly enrolled.
	registeredTask := "t-registered"
	unregisteredTask := "t-no-run"
	runID := "run-registered"

	hs.Attach(registeredTask, func() (float64, int, int) { return 1.25, 7, 13 })
	hs.Attach(unregisteredTask, func() (float64, int, int) { return 9.0, 99, 99 })
	hs.RegisterExecLogRun(registeredTask, runID)

	// Two synthetic ticks. We hand-call fanout instead of letting the
	// real loop fire so the test is deterministic and doesn't depend on
	// `ps` shell-out timing.
	for i := 0; i < 2; i++ {
		hs.fanout(HostMetricsSample{
			TS:         time.Now().UTC(),
			CPUPct:     12.5 + float64(i),
			RSSMB:      256.0 + float64(i),
			DiskUsedGB: 10.0,
		})
	}

	got, err := store.ListSamples(context.Background(), runID)
	if err != nil {
		t.Fatalf("ListSamples: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("registered run sample count = %d, want 2", len(got))
	}
	for i, s := range got {
		if s.RunID != runID {
			t.Fatalf("sample[%d].RunID = %q, want %q", i, s.RunID, runID)
		}
		if s.CostUSD != 1.25 {
			t.Fatalf("sample[%d].CostUSD = %v, want 1.25 (statFn output)", i, s.CostUSD)
		}
		if s.Turns != 7 || s.Actions != 13 {
			t.Fatalf("sample[%d] turns/actions = %d/%d, want 7/13", i, s.Turns, s.Actions)
		}
		if s.RSSMB == 0 || s.CPUPct == 0 {
			t.Fatalf("sample[%d] CPU/RSS empty: %+v", i, s)
		}
		if s.DiskUsedGB != 10.0 {
			t.Fatalf("sample[%d].DiskUsedGB = %v, want 10.0", i, s.DiskUsedGB)
		}
		if s.TS == 0 {
			t.Fatalf("sample[%d].TS = 0, want unix-ms timestamp", i)
		}
	}

	// Unregistered task must not have any rows in execution_log_samples
	// (its samples still live in the in-memory buffer that backs the
	// existing task_run_host_samples flush on Detach).
	stray, err := store.ListSamples(context.Background(), "run-not-registered")
	if err != nil {
		t.Fatalf("ListSamples (stray): %v", err)
	}
	if len(stray) != 0 {
		t.Fatalf("unregistered run sample count = %d, want 0", len(stray))
	}

	// Detach must drop the execRunIDs entry so a subsequent fanout
	// triggered before the goroutine fully exits doesn't double-write.
	if _, _ = hs.Detach(registeredTask); len(hs.execRunIDs) != 0 {
		t.Fatalf("execRunIDs after Detach = %v, want empty map", hs.execRunIDs)
	}
}

// TestHostSampler_fanout_no_exec_store_is_noop — when no execution_log
// store is wired, fanout must still buffer per-task samples for the
// legacy task_run_host_samples flush, but write nothing to execution_log_samples.
func TestHostSampler_fanout_no_exec_store_is_noop(t *testing.T) {
	hs := NewHostSampler(time.Second)
	// Deliberately do NOT call SetExecLogStore.
	taskID := "t-no-store"
	hs.Attach(taskID, func() (float64, int, int) { return 0, 0, 0 })
	// RegisterExecLogRun on a sampler with no store must also be a no-op
	// — the fanout's own `store == nil` guard short-circuits before
	// reading execRunIDs, so this just exercises the registration path.
	hs.RegisterExecLogRun(taskID, "run-orphan")

	hs.fanout(HostMetricsSample{TS: time.Now().UTC(), CPUPct: 1, RSSMB: 1})

	// The per-task buffer should still have grown — Detach returns it.
	samples, _ := hs.Detach(taskID)
	if len(samples) != 1 {
		t.Fatalf("in-memory buffer len = %d, want 1 (legacy task_run_host_samples flow must be untouched)", len(samples))
	}
}
