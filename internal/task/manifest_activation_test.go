package task

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// seedWaitingBlockedByManifest writes a task row directly in 'waiting'
// with a manifest-level block_reason, bypassing Create's state-machine
// logic. Tests of the activation path need to assert behavior
// independently of the seeding path tested elsewhere.
func seedWaitingBlockedByManifest(t *testing.T, s *Store, manifestID, taskTitle, blockers string) string {
	t.Helper()
	task, err := s.Create(manifestID, taskTitle, "", "once", "claude-code", "node", "test", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Force the row into waiting + manifest block_reason regardless of
	// what the seeding path decided — tests here care about the flip
	// step, not the seed step.
	if _, err := s.db.Exec(
		`UPDATE tasks SET status = 'waiting', block_reason = ? WHERE id = ?`,
		"manifest not satisfied — blocked by: "+blockers, task.ID); err != nil {
		t.Fatalf("force waiting: %v", err)
	}
	return task.ID
}

// TestFlipManifestBlockedTasks_AcceptsLegacyPrefix — #97 regression
// test. Tasks seeded by the scheduler's pre-dispatch gate before
// that PR normalized node.go:615 carry the old prefix
// "blocked by manifest ...". The activation walker must still find
// and flip them during the compatibility window.
func TestFlipManifestBlockedTasks_AcceptsLegacyPrefix(t *testing.T) {
	s := openRepoTestStore(t)
	ctx := context.Background()

	tsk, err := s.Create("mf-legacy", "legacy waiter", "", "once", "claude-code", "node", "t", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Force legacy block_reason + status directly — bypasses #77's
	// seeding logic to simulate a row written by the old scheduler
	// path.
	if _, err := s.db.Exec(
		`UPDATE tasks SET status='waiting', block_reason='blocked by manifest mf-legacy (Some Manifest)' WHERE id=?`,
		tsk.ID); err != nil {
		t.Fatalf("force legacy: %v", err)
	}

	n, err := s.FlipManifestBlockedTasks(ctx, "mf-legacy", StatusScheduled)
	if err != nil {
		t.Fatalf("FlipManifestBlockedTasks: %v", err)
	}
	if n != 1 {
		t.Fatalf("flipped = %d, want 1 (legacy prefix must match)", n)
	}
	if got := readStatus(t, s, tsk.ID); got != "scheduled" {
		t.Errorf("status = %q, want scheduled", got)
	}
}

// TestFlipManifestBlockedTasks_ScheduledOnPropagation — the core
// behavior for the close path: every waiting-blocked-by-manifest task
// in the given manifest flips to scheduled + block_reason clears.
func TestFlipManifestBlockedTasks_ScheduledOnPropagation(t *testing.T) {
	s := openRepoTestStore(t)
	ctx := context.Background()

	a := seedWaitingBlockedByManifest(t, s, "mf-1", "A", "mf-dep")
	b := seedWaitingBlockedByManifest(t, s, "mf-1", "B", "mf-dep")

	n, err := s.FlipManifestBlockedTasks(ctx, "mf-1", StatusScheduled)
	if err != nil {
		t.Fatalf("FlipManifestBlockedTasks: %v", err)
	}
	if n != 2 {
		t.Errorf("flipped = %d, want 2", n)
	}
	for _, id := range []string{a, b} {
		if got := readStatus(t, s, id); got != "scheduled" {
			t.Errorf("task %s status = %q, want scheduled", id, got)
		}
		if got := readBlockReason(t, s, id); got != "" {
			t.Errorf("task %s block_reason = %q, want empty", id, got)
		}
	}
}

// TestFlipManifestBlockedTasks_PendingOnRemoval — the rehab path
// (Option B) uses the same function with StatusPending as the target.
// Tasks land pending, not scheduled, so the operator explicitly arms
// them before they burn budget.
func TestFlipManifestBlockedTasks_PendingOnRemoval(t *testing.T) {
	s := openRepoTestStore(t)
	ctx := context.Background()

	id := seedWaitingBlockedByManifest(t, s, "mf-1", "A", "mf-dep")

	n, err := s.FlipManifestBlockedTasks(ctx, "mf-1", StatusPending)
	if err != nil {
		t.Fatalf("FlipManifestBlockedTasks: %v", err)
	}
	if n != 1 {
		t.Fatalf("flipped = %d, want 1", n)
	}
	if got := readStatus(t, s, id); got != "pending" {
		t.Errorf("status = %q, want pending", got)
	}
}

// TestFlipManifestBlockedTasks_SkipsTaskLevelBlocks — a task in
// 'waiting' because its task-level depends_on isn't met has a DIFFERENT
// block_reason prefix. The manifest-activation path must not sweep
// those up, or closing a manifest would auto-fire tasks that still
// have an open task-level blocker.
func TestFlipManifestBlockedTasks_SkipsTaskLevelBlocks(t *testing.T) {
	s := openRepoTestStore(t)
	ctx := context.Background()

	manifestBlocked := seedWaitingBlockedByManifest(t, s, "mf-1", "A", "mf-dep")
	// Hand-craft a task-level block_reason on a separate task.
	taskBlocked, _ := s.Create("mf-1", "B", "", "once", "claude-code", "node", "t", "")
	if _, err := s.db.Exec(
		`UPDATE tasks SET status = 'waiting', block_reason = 'task xyz not completed' WHERE id = ?`,
		taskBlocked.ID); err != nil {
		t.Fatalf("force task-level waiting: %v", err)
	}

	n, err := s.FlipManifestBlockedTasks(ctx, "mf-1", StatusScheduled)
	if err != nil {
		t.Fatalf("FlipManifestBlockedTasks: %v", err)
	}
	if n != 1 {
		t.Fatalf("flipped = %d, want 1 (only the manifest-blocked one)", n)
	}
	if got := readStatus(t, s, manifestBlocked); got != "scheduled" {
		t.Errorf("manifest-blocked status = %q, want scheduled", got)
	}
	if got := readStatus(t, s, taskBlocked.ID); got != "waiting" {
		t.Errorf("task-level-blocked status = %q, want waiting (must NOT be flipped)", got)
	}
	if br := readBlockReason(t, s, taskBlocked.ID); !strings.Contains(br, "task xyz") {
		t.Errorf("task-level block_reason lost: %q", br)
	}
}

// TestFlipManifestBlockedTasks_RejectsInvalidTarget — the state machine
// forbids flipping waiting to anything other than scheduled or pending
// via this path. A caller bug that passed, say, 'running' must be
// rejected up front, not produce an illegal SQL state.
func TestFlipManifestBlockedTasks_RejectsInvalidTarget(t *testing.T) {
	s := openRepoTestStore(t)
	ctx := context.Background()

	_, err := s.FlipManifestBlockedTasks(ctx, "mf-1", StatusRunning)
	if err == nil {
		t.Fatal("FlipManifestBlockedTasks(running) should fail")
	}
	if !strings.Contains(err.Error(), "scheduled or pending") {
		t.Errorf("error should name the valid targets: %v", err)
	}
}

// TestPropagateManifestClosed_ActivatesDownstream — end-to-end walk of
// the activation graph. mfA depends on mfB. A task in mfA is waiting
// for mfB to close. When PropagateManifestClosed(mfB) runs with a
// dependency lookup that says "mfA depends on mfB" and IsSatisfied(mfA)
// returns true, the task in mfA flips to scheduled.
func TestPropagateManifestClosed_ActivatesDownstream(t *testing.T) {
	s := openRepoTestStore(t)
	ctx := context.Background()

	waiting := seedWaitingBlockedByManifest(t, s, "mfA", "downstream", "mfB")

	depsFor := func(_ context.Context, id string) ([]string, error) {
		if id == "mfB" {
			return []string{"mfA"}, nil
		}
		return nil, nil
	}
	satisfiedFor := func(_ context.Context, id string) (bool, error) {
		return true, nil // mfA is now satisfied because mfB just closed
	}

	activated, err := s.PropagateManifestClosed(ctx, "mfB", depsFor, satisfiedFor)
	if err != nil {
		t.Fatalf("PropagateManifestClosed: %v", err)
	}
	if activated != 1 {
		t.Fatalf("activated = %d, want 1", activated)
	}
	if got := readStatus(t, s, waiting); got != "scheduled" {
		t.Errorf("waiting task status = %q, want scheduled", got)
	}
}

// TestPropagateManifestClosed_LeavesUnsatisfiedAlone — if mfA depends
// on both mfB and mfC, closing mfB alone does NOT activate mfA's
// tasks, because mfC is still open. The satisfied lookup is the gate.
func TestPropagateManifestClosed_LeavesUnsatisfiedAlone(t *testing.T) {
	s := openRepoTestStore(t)
	ctx := context.Background()

	stillWaiting := seedWaitingBlockedByManifest(t, s, "mfA", "still-blocked", "mfB, mfC")

	depsFor := func(_ context.Context, id string) ([]string, error) {
		if id == "mfB" {
			return []string{"mfA"}, nil
		}
		return nil, nil
	}
	satisfiedFor := func(_ context.Context, id string) (bool, error) {
		return false, nil // mfC still open → mfA not satisfied
	}

	activated, err := s.PropagateManifestClosed(ctx, "mfB", depsFor, satisfiedFor)
	if err != nil {
		t.Fatalf("PropagateManifestClosed: %v", err)
	}
	if activated != 0 {
		t.Fatalf("activated = %d, want 0 (mfA still has open deps)", activated)
	}
	if got := readStatus(t, s, stillWaiting); got != "waiting" {
		t.Errorf("status = %q, want waiting (mfA not yet satisfied)", got)
	}
}

// TestPropagateManifestClosed_CycleSafe — if the dep graph somehow
// contains a cycle (shouldn't — #76's detector prevents adds — but
// legacy/migrated data might), the BFS visited-set must guarantee
// termination. We run the walk on a bounded timeout; if the visited
// set protection regresses, the test fails rather than hanging the
// whole `go test` process.
func TestPropagateManifestClosed_CycleSafe(t *testing.T) {
	s := openRepoTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// mfA ↔ mfB — cyclical dep graph.
	depsFor := func(_ context.Context, id string) ([]string, error) {
		switch id {
		case "mfA":
			return []string{"mfB"}, nil
		case "mfB":
			return []string{"mfA"}, nil
		}
		return nil, nil
	}
	satisfiedFor := func(_ context.Context, _ string) (bool, error) { return true, nil }

	done := make(chan struct{})
	go func() {
		_, _ = s.PropagateManifestClosed(ctx, "mfA", depsFor, satisfiedFor)
		close(done)
	}()
	select {
	case <-done:
		// reached — visited set terminated the walk
	case <-ctx.Done():
		t.Fatal("PropagateManifestClosed did not terminate on cyclical graph within 2s")
	}
}

// TestPropagateManifestClosed_PropagatesErrors — if depsFor errors on
// the root manifest, the walker surfaces that error rather than
// silently succeeding with 0 activations. The operator needs to see
// the failure to investigate.
func TestPropagateManifestClosed_PropagatesErrors(t *testing.T) {
	s := openRepoTestStore(t)
	ctx := context.Background()

	boom := errors.New("synthetic")
	depsFor := func(_ context.Context, _ string) ([]string, error) { return nil, boom }
	satisfiedFor := func(_ context.Context, _ string) (bool, error) { return true, nil }

	_, err := s.PropagateManifestClosed(ctx, "mfB", depsFor, satisfiedFor)
	if err == nil {
		t.Fatal("PropagateManifestClosed did not surface depsFor error")
	}
	if !errors.Is(err, boom) && !strings.Contains(err.Error(), "synthetic") {
		t.Errorf("wrapped error lost the original cause: %v", err)
	}
}

// TestCountManifestBlockedTasks — dashboards and tests both want a
// cheap count. Verify the same prefix filter used by the flip function
// drives the count so we can't have inconsistent numbers.
func TestCountManifestBlockedTasks(t *testing.T) {
	s := openRepoTestStore(t)
	ctx := context.Background()

	seedWaitingBlockedByManifest(t, s, "mf-1", "A", "mf-dep")
	seedWaitingBlockedByManifest(t, s, "mf-1", "B", "mf-dep")
	// Different manifest — not counted.
	seedWaitingBlockedByManifest(t, s, "mf-2", "C", "mf-dep")

	got, err := s.CountManifestBlockedTasks(ctx, "mf-1")
	if err != nil {
		t.Fatalf("CountManifestBlockedTasks: %v", err)
	}
	if got != 2 {
		t.Errorf("count = %d, want 2", got)
	}
}

