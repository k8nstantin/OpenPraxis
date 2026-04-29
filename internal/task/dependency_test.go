package task

import (
	"errors"
	"strings"
	"testing"
)

// mustCreateTask is a terse Create helper for tests that don't care
// about most fields. Returns the full task row so the caller can ID
// it back and check status.
func mustCreateTask(t *testing.T, s *Store, title, dependsOn string) *Task {
	t.Helper()
	tk, err := s.Create("mf-1", title, "", "once", "claude-code", "node", "tester", dependsOn)
	if err != nil {
		t.Fatalf("Create(%q): %v", title, err)
	}
	return tk
}

// force overwrites a task's status directly, bypassing the state
// machine. Needed for fixtures where we want the parent to already
// be "completed" without running the whole runner loop.
func force(t *testing.T, s *Store, id, status string) {
	t.Helper()
	if _, err := s.db.Exec(`UPDATE tasks SET status = ? WHERE id = ?`, status, id); err != nil {
		t.Fatalf("force status %s: %v", status, err)
	}
}

// TestSetDependency_ClearsDepAndFlipsWaitingToPending — clearing a
// dep on a task that was waiting (the typical "operator realized
// the chain was wrong and removed the block") must flip the task
// to pending and wipe block_reason. Option B locked earlier in
// session means we do NOT auto-schedule; operator arms.
func TestSetDependency_ClearsDepAndFlipsWaitingToPending(t *testing.T) {
	s := openRepoTestStore(t)
	parent := mustCreateTask(t, s, "parent", "")
	child := mustCreateTask(t, s, "child", parent.ID) // seeded as waiting per #77

	if got := readStatus(t, s, child.ID); got != "waiting" {
		t.Fatalf("initial child status = %q, want waiting", got)
	}

	if err := s.SetDependency(child.ID, ""); err != nil {
		t.Fatalf("SetDependency clear: %v", err)
	}
	if got := readStatus(t, s, child.ID); got != "pending" {
		t.Errorf("child status after clear = %q, want pending", got)
	}
	if br := readBlockReason(t, s, child.ID); br != "" {
		t.Errorf("block_reason = %q, want empty after clear", br)
	}
}

// TestSetDependency_ParentCompletedLandsScheduled — setting a dep
// on a task whose parent is already completed must land the task
// in scheduled (the dep is already satisfied). The pre-fix handler
// always wrote waiting; this asserts the Store now does the right
// thing regardless of caller.
func TestSetDependency_ParentCompletedLandsScheduled(t *testing.T) {
	s := openRepoTestStore(t)
	parent := mustCreateTask(t, s, "parent", "")
	force(t, s, parent.ID, "completed")

	child := mustCreateTask(t, s, "orphan", "")
	if got := readStatus(t, s, child.ID); got != "pending" {
		t.Fatalf("initial orphan status = %q, want pending", got)
	}

	if err := s.SetDependency(child.ID, parent.ID); err != nil {
		t.Fatalf("SetDependency: %v", err)
	}
	if got := readStatus(t, s, child.ID); got != "scheduled" {
		t.Errorf("child status = %q, want scheduled (parent already done)", got)
	}
	if br := readBlockReason(t, s, child.ID); br != "" {
		t.Errorf("block_reason = %q, want empty (dep is satisfied)", br)
	}
}

// TestSetDependency_ParentOpenLandsWaitingWithBlockReason — the
// headline case. Parent still open → child parks in waiting with
// populated block_reason that the #85 UI surfaces as a visible bar.
func TestSetDependency_ParentOpenLandsWaitingWithBlockReason(t *testing.T) {
	s := openRepoTestStore(t)
	parent := mustCreateTask(t, s, "parent", "")
	// parent stays at the default 'pending'

	child := mustCreateTask(t, s, "orphan", "")
	if err := s.SetDependency(child.ID, parent.ID); err != nil {
		t.Fatalf("SetDependency: %v", err)
	}
	if got := readStatus(t, s, child.ID); got != "waiting" {
		t.Errorf("status = %q, want waiting", got)
	}
	br := readBlockReason(t, s, child.ID)
	if !strings.Contains(br, "not completed") {
		t.Errorf("block_reason = %q, want 'not completed' phrasing", br)
	}
	if !strings.Contains(br, parent.ID[:12]) {
		t.Errorf("block_reason = %q, want parent marker %q", br, parent.ID[:12])
	}
}

// TestSetDependency_SelfLoopRejected — sentinel error, row unchanged.
func TestSetDependency_SelfLoopRejected(t *testing.T) {
	s := openRepoTestStore(t)
	tk := mustCreateTask(t, s, "solo", "")

	err := s.SetDependency(tk.ID, tk.ID)
	if !errors.Is(err, ErrTaskDepSelfLoop) {
		t.Fatalf("err = %v, want ErrTaskDepSelfLoop", err)
	}
	if got := readStatus(t, s, tk.ID); got != "pending" {
		t.Errorf("status mutated after rejected self-loop: %q", got)
	}
}

// TestSetDependency_DirectCycleRejected — A→B exists, set B→A →
// rejected. Error message names the rejected pair so the operator
// sees which edge was refused.
func TestSetDependency_DirectCycleRejected(t *testing.T) {
	s := openRepoTestStore(t)
	a := mustCreateTask(t, s, "A", "")
	b := mustCreateTask(t, s, "B", a.ID) // B depends on A

	err := s.SetDependency(a.ID, b.ID) // attempt A → B
	if !errors.Is(err, ErrTaskDepCycle) {
		t.Fatalf("err = %v, want ErrTaskDepCycle", err)
	}
	if !strings.Contains(err.Error(), a.ID[:12]) || !strings.Contains(err.Error(), b.ID[:12]) {
		t.Errorf("cycle error doesn't name the rejected pair: %v", err)
	}
}

// TestSetDependency_TransitiveCycleRejected — A → B → C. Setting
// C → A would close a length-3 cycle; DFS must catch it.
func TestSetDependency_TransitiveCycleRejected(t *testing.T) {
	s := openRepoTestStore(t)
	a := mustCreateTask(t, s, "A", "")
	b := mustCreateTask(t, s, "B", a.ID)
	c := mustCreateTask(t, s, "C", b.ID)

	err := s.SetDependency(a.ID, c.ID) // A → C would make A → C → B → A
	if !errors.Is(err, ErrTaskDepCycle) {
		t.Fatalf("err = %v, want ErrTaskDepCycle on transitive cycle", err)
	}
}

// TestSetDependency_SCDHistoryAccumulates — every non-clear
// SetDependency call lands one row in the unified relationships SCD-2
// store; the matching close (when the dep changes or is cleared)
// stamps valid_to on the prior row. ListDepHistory returns the rows
// newest-first.
//
// Post-PR/M3 the "cleared" sentinel row that the legacy
// task_dependency table emitted is no longer present — the
// relationships store represents "no current dep" by absence of any
// row with valid_to=''. The audit trail still tells the full story:
// two closed rows (P1 → P2) ordered newest-first.
func TestSetDependency_SCDHistoryAccumulates(t *testing.T) {
	s := openRepoTestStore(t)
	parent1 := mustCreateTask(t, s, "P1", "")
	parent2 := mustCreateTask(t, s, "P2", "")
	child := mustCreateTask(t, s, "child", "")

	if err := s.SetDependency(child.ID, parent1.ID); err != nil {
		t.Fatalf("set to P1: %v", err)
	}
	if err := s.SetDependency(child.ID, parent2.ID); err != nil {
		t.Fatalf("change to P2: %v", err)
	}
	if err := s.SetDependency(child.ID, ""); err != nil {
		t.Fatalf("clear: %v", err)
	}

	hist, err := s.ListDepHistory(child.ID, 10)
	if err != nil {
		t.Fatalf("ListDepHistory: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("history rows = %d, want 2 (P1 + P2 closed; clear is absence)", len(hist))
	}
	// Newest first: P2 closed, then P1 closed. Both must have valid_to
	// stamped (no current dep — clear closed P2's row).
	if hist[0].DependsOn != parent2.ID {
		t.Errorf("newest row depends_on = %q, want %q", hist[0].DependsOn, parent2.ID)
	}
	if hist[1].DependsOn != parent1.ID {
		t.Errorf("second row depends_on = %q, want %q", hist[1].DependsOn, parent1.ID)
	}
	if hist[0].ValidTo == "" {
		t.Errorf("P2 row valid_to = empty; clear should have stamped it")
	}
	if hist[1].ValidTo == "" {
		t.Errorf("P1 row valid_to = empty; superseded by P2 should have stamped it")
	}
}

// TestBackfillTaskDepSCD_Idempotent — legacy tasks with a non-empty
// depends_on column but no SCD row get one seeded on first run;
// second run is a no-op.
func TestBackfillTaskDepSCD_Idempotent(t *testing.T) {
	s := openRepoTestStore(t)
	// Manual insert: mimic a pre-SCD row — tasks.depends_on set,
	// task_dependency empty.
	parent := mustCreateTask(t, s, "P", "")
	orphan, _ := s.Create("mf-1", "legacy", "", "once", "claude-code", "node", "t", "")
	if _, err := s.db.Exec(`UPDATE tasks SET depends_on = ? WHERE id = ?`, parent.ID, orphan.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM task_dependency WHERE task_id = ?`, orphan.ID); err != nil {
		t.Fatal(err)
	}

	// Store.NewStore already ran the backfill, so force a second
	// round to assert idempotency on top of whatever the legacy
	// state produced.
	n, err := s.BackfillTaskDepSCD()
	if err != nil {
		t.Fatalf("first explicit backfill: %v", err)
	}
	// Exactly one row seeded — the legacy orphan.
	if n != 1 {
		t.Errorf("first backfill inserted %d, want 1", n)
	}
	n, err = s.BackfillTaskDepSCD()
	if err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	if n != 0 {
		t.Errorf("second backfill inserted %d, want 0 (idempotent)", n)
	}
}
