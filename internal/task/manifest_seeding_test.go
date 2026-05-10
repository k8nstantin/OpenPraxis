package task

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeChecker is a hand-rolled test double for ManifestReadinessChecker.
// Each manifestID maps to a (satisfied, unsatisfied) response; lookups
// for unknown ids return satisfied=true so tests that don't care about
// the manifest layer don't have to configure each call.
type fakeChecker struct {
	responses map[string]fakeCheckResult
	errFor    string // manifestID that should return a lookup error
}

type fakeCheckResult struct {
	satisfied   bool
	unsatisfied []string
}

func (f *fakeChecker) IsSatisfied(_ context.Context, manifestID string) (bool, []string, error) {
	if f.errFor == manifestID {
		return false, nil, errors.New("fake lookup failure")
	}
	if r, ok := f.responses[manifestID]; ok {
		return r.satisfied, r.unsatisfied, nil
	}
	return true, nil, nil
}

// readBlockReason pulls the block_reason column directly. scanTask
// strips it via taskColumns — fine for most paths but tests need the
// raw value.
func readBlockReason(t *testing.T, s *Store, id string) string {
	t.Helper()
	var br string
	if err := s.db.QueryRow(`SELECT block_reason FROM tasks WHERE id = ?`, id).Scan(&br); err != nil {
		t.Fatalf("read block_reason for %s: %v", id, err)
	}
	return br
}

// TestCreate_ManifestUnsatisfied_SeedsWaiting — the headline behavior
// this PR introduces: a task created against an unsatisfied manifest
// lands in 'waiting' with block_reason naming the unsatisfied dep(s),
// not in 'pending'. Matches session Option B / issue #75 scope.
func TestCreate_ManifestUnsatisfied_SeedsWaiting(t *testing.T) {
	t.Skip("task store migrated to entities")
	s := openRepoTestStore(t)
	s.SetManifestChecker(&fakeChecker{
		responses: map[string]fakeCheckResult{
			"mf-blocked": {satisfied: false, unsatisfied: []string{"mf-dep-1", "mf-dep-2"}},
		},
	})

	task, err := s.Create("mf-blocked", "t", "", "once", "claude-code", "node", "user", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := readStatus(t, s, task.ID); got != "waiting" {
		t.Fatalf("status = %q, want waiting", got)
	}
	br := readBlockReason(t, s, task.ID)
	if !strings.Contains(br, "manifest not satisfied") {
		t.Errorf("block_reason = %q, want it to say 'manifest not satisfied'", br)
	}
	for _, want := range []string{"mf-dep-1", "mf-dep-2"} {
		if !strings.Contains(br, want) {
			t.Errorf("block_reason = %q, want id-prefix %q present", br, want)
		}
	}
}

// TestCreate_ManifestSatisfied_NoDep_LandsPending — when the manifest
// is satisfied AND there's no task-level dep, behavior matches the
// pre-#75 no-dep case: plain 'pending', empty block_reason.
func TestCreate_ManifestSatisfied_NoDep_LandsPending(t *testing.T) {
	t.Skip("task store migrated to entities")
	s := openRepoTestStore(t)
	s.SetManifestChecker(&fakeChecker{
		responses: map[string]fakeCheckResult{"mf-ready": {satisfied: true}},
	})

	task, err := s.Create("mf-ready", "t", "", "once", "claude-code", "node", "user", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := readStatus(t, s, task.ID); got != "pending" {
		t.Fatalf("status = %q, want pending", got)
	}
	if br := readBlockReason(t, s, task.ID); br != "" {
		t.Errorf("block_reason = %q, want empty", br)
	}
}

// TestCreate_TaskDepTrumpsManifestCheck — when the task has a
// task-level dep AND the manifest is also unsatisfied, the block_reason
// must call out the task-level blocker (the closer one), not the
// manifest. Once the task dep completes, the manifest check re-runs at
// activation time.
func TestCreate_TaskDepTrumpsManifestCheck(t *testing.T) {
	t.Skip("task store migrated to entities")
	s := openRepoTestStore(t)
	s.SetManifestChecker(&fakeChecker{
		responses: map[string]fakeCheckResult{"mf-blocked": {satisfied: false, unsatisfied: []string{"mf-dep"}}},
	})

	parent, err := s.Create("", "parent", "", "once", "claude-code", "node", "user", "")
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	child, err := s.Create("mf-blocked", "child", "", "once", "claude-code", "node", "user", parent.ID)
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	if got := readStatus(t, s, child.ID); got != "waiting" {
		t.Fatalf("status = %q, want waiting", got)
	}
	br := readBlockReason(t, s, child.ID)
	if !strings.Contains(br, "task ") {
		t.Errorf("block_reason = %q, want task-level blocker named first", br)
	}
	if strings.Contains(br, "manifest not satisfied") {
		t.Errorf("block_reason = %q, shouldn't mention manifest when task dep is the closer blocker", br)
	}
}

// TestCreate_NilChecker_PreservesOldBehavior — for unit tests that
// don't wire a checker (the majority) the manifest gate is skipped and
// the no-dep path still lands pending. This makes the feature
// opt-in at the test layer.
func TestCreate_NilChecker_PreservesOldBehavior(t *testing.T) {
	t.Skip("task store migrated to entities")
	s := openRepoTestStore(t)
	// No SetManifestChecker call.

	task, err := s.Create("mf-anything", "t", "", "once", "claude-code", "node", "user", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := readStatus(t, s, task.ID); got != "pending" {
		t.Fatalf("nil-checker status = %q, want pending", got)
	}
}

// TestCreate_CheckerError_DegradesToPending — a transient error from
// the checker (e.g. DB contention) must NOT fail the create. The task
// lands pending; operator can later re-run activation or the seeding
// retries naturally on the next create attempt.
func TestCreate_CheckerError_DegradesToPending(t *testing.T) {
	t.Skip("task store migrated to entities")
	s := openRepoTestStore(t)
	s.SetManifestChecker(&fakeChecker{errFor: "mf-broken"})

	task, err := s.Create("mf-broken", "t", "", "once", "claude-code", "node", "user", "")
	if err != nil {
		t.Fatalf("Create should not fail on checker error: %v", err)
	}
	if got := readStatus(t, s, task.ID); got != "pending" {
		t.Fatalf("status = %q, want pending (checker error fallback)", got)
	}
}

// TestCreate_StandaloneTask_SkipsManifestCheck — tasks with empty
// manifest_id can't be blocked by a manifest they don't have. The
// checker should not be consulted.
func TestCreate_StandaloneTask_SkipsManifestCheck(t *testing.T) {
	t.Skip("task store migrated to entities")
	s := openRepoTestStore(t)
	checker := &fakeChecker{
		responses: map[string]fakeCheckResult{"": {satisfied: false, unsatisfied: []string{"x"}}},
	}
	s.SetManifestChecker(checker)

	task, err := s.Create("", "standalone", "", "once", "claude-code", "node", "user", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := readStatus(t, s, task.ID); got != "pending" {
		t.Fatalf("standalone task status = %q, want pending", got)
	}
}
