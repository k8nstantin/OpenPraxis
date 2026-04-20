package task

import (
	"context"
	"testing"
)

// readNextRunAt pulls the raw tasks.next_run_at column for a given id. We
// go direct to the DB rather than via scanTask because scanTask projects
// through the standard column list and we care about this specific field.
func readNextRunAt(t *testing.T, s *Store, id string) string {
	t.Helper()
	var next string
	if err := s.db.QueryRow(`SELECT next_run_at FROM tasks WHERE id = ?`, id).Scan(&next); err != nil {
		t.Fatalf("read next_run_at for %s: %v", id, err)
	}
	return next
}

// TestSetDependency_SchedulesTaskSetsNextRunAt — regression for #114.
// When SetDependency detects the parent is completed it flips the task
// to StatusScheduled. Prior to this fix the UPDATE wrote only the
// status column, leaving next_run_at empty and the task invisible to
// the scheduler's dequeue query. This test asserts the column is
// populated on the scheduled-land path.
func TestSetDependency_SchedulesTaskSetsNextRunAt(t *testing.T) {
	s := openRepoTestStore(t)
	parent, _ := s.Create("", "parent", "", "once", "claude-code", "node", "t", "")
	if _, err := s.db.Exec(`UPDATE tasks SET status='completed' WHERE id = ?`, parent.ID); err != nil {
		t.Fatalf("force parent completed: %v", err)
	}
	child, _ := s.Create("", "child", "", "once", "claude-code", "node", "t", "")
	// Force child back to pending so SetDependency's parent-completed
	// path takes the scheduled branch (Create would have already
	// scheduled it given the parent is completed; we want to exercise
	// the SetDependency path explicitly).
	if _, err := s.db.Exec(`UPDATE tasks SET status='pending', depends_on='' WHERE id = ?`, child.ID); err != nil {
		t.Fatalf("reset child: %v", err)
	}

	if err := s.SetDependency(child.ID, parent.ID); err != nil {
		t.Fatalf("SetDependency: %v", err)
	}
	if got := readStatus(t, s, child.ID); got != "scheduled" {
		t.Fatalf("status = %q, want scheduled", got)
	}
	if got := readNextRunAt(t, s, child.ID); got == "" {
		t.Fatalf("next_run_at is empty; scheduler would never pick this task up (bug #114 regression)")
	}
}

// TestSetDependency_ClearPath_LeavesNextRunAtEmpty — symmetric check:
// when SetDependency clears a dep and the task ends up in pending
// (because it was waiting), next_run_at must NOT be populated. Pending
// tasks never auto-fire by design; filling next_run_at would turn a
// rehabbed dep-removed task into an auto-firing one.
func TestSetDependency_ClearPath_LeavesNextRunAtEmpty(t *testing.T) {
	s := openRepoTestStore(t)
	parent, _ := s.Create("", "parent", "", "once", "claude-code", "node", "t", "")
	child, _ := s.Create("", "child", "", "once", "claude-code", "node", "t", parent.ID)

	if got := readStatus(t, s, child.ID); got != "waiting" {
		t.Fatalf("setup: child = %q, want waiting", got)
	}
	if err := s.SetDependency(child.ID, ""); err != nil {
		t.Fatalf("SetDependency clear: %v", err)
	}
	if got := readStatus(t, s, child.ID); got != "pending" {
		t.Fatalf("status = %q, want pending after clear", got)
	}
	if got := readNextRunAt(t, s, child.ID); got != "" {
		t.Errorf("next_run_at = %q, want empty (pending tasks must not auto-fire)", got)
	}
}

// TestFlipManifestBlockedTasks_ScheduledSetsNextRunAt — regression for
// #114 / root-cause-of-#103. When the manifest-close propagation
// walker flips waiting tasks to scheduled, the UPDATE must populate
// next_run_at so the scheduler actually picks them up.
func TestFlipManifestBlockedTasks_ScheduledSetsNextRunAt(t *testing.T) {
	s := openRepoTestStore(t)
	blocked := seedWaitingBlockedByManifest(t, s, "mf-prop", "the-task", "mf-prop")

	n, err := s.FlipManifestBlockedTasks(context.Background(), "mf-prop", StatusScheduled)
	if err != nil {
		t.Fatalf("FlipManifestBlockedTasks: %v", err)
	}
	if n != 1 {
		t.Fatalf("flipped %d rows, want 1", n)
	}
	if got := readStatus(t, s, blocked); got != "scheduled" {
		t.Fatalf("status = %q, want scheduled", got)
	}
	if got := readNextRunAt(t, s, blocked); got == "" {
		t.Fatalf("next_run_at is empty after scheduled flip; this is the #103 root cause")
	}
}

// TestFlipManifestBlockedTasks_PendingLeavesNextRunAtEmpty — the
// Option B rehab path (dep removed → tasks flip to pending) must
// NOT set next_run_at. Symmetric to TestSetDependency_ClearPath.
func TestFlipManifestBlockedTasks_PendingLeavesNextRunAtEmpty(t *testing.T) {
	s := openRepoTestStore(t)
	blocked := seedWaitingBlockedByManifest(t, s, "mf-reh", "the-task", "mf-reh")

	n, err := s.FlipManifestBlockedTasks(context.Background(), "mf-reh", StatusPending)
	if err != nil {
		t.Fatalf("FlipManifestBlockedTasks: %v", err)
	}
	if n != 1 {
		t.Fatalf("flipped %d rows, want 1", n)
	}
	if got := readStatus(t, s, blocked); got != "pending" {
		t.Fatalf("status = %q, want pending", got)
	}
	if got := readNextRunAt(t, s, blocked); got != "" {
		t.Errorf("next_run_at = %q, want empty on pending rehab path", got)
	}
}

// TestFlipProductBlockedTasks_ScheduledSetsNextRunAt — same regression
// check at the product tier. The product-close propagation walker
// also needs next_run_at populated when flipping to scheduled.
func TestFlipProductBlockedTasks_ScheduledSetsNextRunAt(t *testing.T) {
	s := openRepoTestStore(t)
	seedManifestForProduct(t, s, "mf-P", "prod-P")
	tsk, _ := s.Create("mf-P", "blocked", "", "once", "claude-code", "node", "t", "")
	if _, err := s.db.Exec(
		`UPDATE tasks SET status='waiting', block_reason=? WHERE id=?`,
		"product not satisfied — blocked by: other", tsk.ID); err != nil {
		t.Fatalf("force waiting: %v", err)
	}

	n, err := s.FlipProductBlockedTasks(context.Background(), "prod-P", StatusScheduled)
	if err != nil {
		t.Fatalf("FlipProductBlockedTasks: %v", err)
	}
	if n != 1 {
		t.Fatalf("flipped %d rows, want 1", n)
	}
	if got := readStatus(t, s, tsk.ID); got != "scheduled" {
		t.Fatalf("status = %q, want scheduled", got)
	}
	if got := readNextRunAt(t, s, tsk.ID); got == "" {
		t.Fatalf("next_run_at is empty after product-flip; scheduler would never pick this up")
	}
}
