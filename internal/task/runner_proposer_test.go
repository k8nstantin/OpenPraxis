package task

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/k8nstantin/OpenPraxis/internal/entity"
	"github.com/k8nstantin/OpenPraxis/internal/execution"
	"github.com/k8nstantin/OpenPraxis/internal/relationships"
	"github.com/k8nstantin/OpenPraxis/internal/schedule"
	"github.com/k8nstantin/OpenPraxis/internal/settings"
)

// proposerHarness is a runner harness extended with the four stores the
// proposer trigger path depends on (entity, schedule, relationships,
// settings). Returns the runner with everything wired plus the bare stores
// callers need to assert against (entity & schedule & settings).
type proposerHarness struct {
	r        *Runner
	entities *entity.Store
	schedRow *schedule.Store
	rels     *relationships.Store
	settings *settings.Store
}

func newProposerHarness(t *testing.T) *proposerHarness {
	t.Helper()
	r, settingsStore, _, _ := newRunnerHarness(t)

	// The settings.Store inside the harness is constructed off the same
	// *sql.DB as the task store, so we can reuse its DB for the entity,
	// schedule, and relationships stores. There is no exported accessor
	// for the *sql.DB on settings.Store; fortunately Runner already
	// holds the task store whose .DB() returns the underlying handle.
	if r.store == nil {
		t.Fatalf("runner harness left r.store nil; cannot reuse DB")
	}
	db := r.store.DB()

	entities, err := entity.NewStore(db)
	if err != nil {
		t.Fatalf("entity.NewStore: %v", err)
	}
	schedStore, err := schedule.New(db)
	if err != nil {
		t.Fatalf("schedule.New: %v", err)
	}
	rels, err := relationships.New(db)
	if err != nil {
		t.Fatalf("relationships.New: %v", err)
	}

	r.SetEntityStore(entities)
	r.SetScheduleStore(schedStore)
	r.SetRelationships(rels)

	return &proposerHarness{
		r:        r,
		entities: entities,
		schedRow: schedStore,
		rels:     rels,
		settings: settingsStore,
	}
}

// seedManifestOwnsTask wires an EdgeOwns(manifest → task) edge so the
// proposer trigger's ListIncoming walk can find the parent manifest.
func seedManifestOwnsTask(t *testing.T, rels *relationships.Store, manifestID, taskID string) {
	t.Helper()
	if err := rels.Create(context.Background(), relationships.Edge{
		SrcKind: relationships.KindManifest, SrcID: manifestID,
		DstKind: relationships.KindTask, DstID: taskID,
		Kind: relationships.EdgeOwns, CreatedBy: "test", Reason: "test fixture",
	}); err != nil {
		t.Fatalf("seed owns edge: %v", err)
	}
}

// readStreak returns the persisted failure-streak counter at manifest
// scope, or 0 when the row does not yet exist.
func readStreak(t *testing.T, store *settings.Store, manifestID string) int {
	t.Helper()
	entry, err := store.Get(context.Background(), settings.ScopeManifest, manifestID, proposerStreakKey)
	if err != nil {
		return 0
	}
	if entry.Value == "" {
		return 0
	}
	var n int
	if uerr := json.Unmarshal([]byte(entry.Value), &n); uerr != nil {
		t.Fatalf("decode streak: %v", uerr)
	}
	return n
}

// listCurrentProposerSchedules returns every active schedule row for the
// given proposer task UID. The proposer trigger inserts exactly one row.
func listCurrentProposerSchedules(t *testing.T, store *schedule.Store, taskUID string) []*schedule.Schedule {
	t.Helper()
	got, err := store.ListCurrent(context.Background(), schedule.KindTask, taskUID)
	if err != nil {
		t.Fatalf("ListCurrent: %v", err)
	}
	return got
}

// listProposerEntities returns every active proposer task entity in the
// store. Title-match is the cheapest signal here — checkProposerTriggers
// is the only writer of proposerTaskTitle.
func listProposerEntities(t *testing.T, entities *entity.Store) []*entity.Entity {
	t.Helper()
	rows, err := entities.List(entity.TypeTask, entity.StatusDraft, 0)
	if err != nil {
		t.Fatalf("entity.List: %v", err)
	}
	out := make([]*entity.Entity, 0, len(rows))
	for _, e := range rows {
		if e.Title == proposerTaskTitle {
			out = append(out, e)
		}
	}
	return out
}

// TestCheckProposerTriggers_NilGuards — the path is safe to invoke on a
// runner missing any of its store dependencies. Production wires them at
// boot, but tests + early-init code paths should not panic.
func TestCheckProposerTriggers_NilGuards(t *testing.T) {
	r, _, _, _ := newRunnerHarness(t)
	// Deliberately skip SetEntityStore / SetScheduleStore / SetRelationships.
	// First panic-free call asserts the guards short-circuit cleanly.
	r.checkProposerTriggers(
		context.Background(),
		&Task{ID: "t-nil"},
		execution.TerminalReasonMaxTurns,
		nil,
		runtimeKnobs{ProposerEnabled: true, ProposerTriggerFailureStreak: 1},
	)
}

// TestCheckProposerTriggers_FailureStreakFires — three consecutive
// non-transient failures (max_turns at threshold=3) mint a proposer task
// under the manifest and queue exactly one schedule row for it.
func TestCheckProposerTriggers_FailureStreakFires(t *testing.T) {
	h := newProposerHarness(t)
	const manifestID = "m-streak"
	const taskID = "t-streak"
	seedManifestOwnsTask(t, h.rels, manifestID, taskID)

	knobs := runtimeKnobs{
		ProposerEnabled:              true,
		ProposerTriggerFailureStreak: 3,
		ProposerTriggerCostUSD:       0,
	}

	for i := 0; i < 2; i++ {
		h.r.checkProposerTriggers(context.Background(),
			&Task{ID: taskID}, execution.TerminalReasonMaxTurns, nil, knobs)
		if got := readStreak(t, h.settings, manifestID); got != i+1 {
			t.Fatalf("after %d max_turns, streak = %d, want %d", i+1, got, i+1)
		}
		if rows := listProposerEntities(t, h.entities); len(rows) != 0 {
			t.Fatalf("after %d failures, proposer entities = %d, want 0", i+1, len(rows))
		}
	}

	// Third failure should fire — and reset the streak so the next run
	// doesn't immediately re-fire.
	h.r.checkProposerTriggers(context.Background(),
		&Task{ID: taskID}, execution.TerminalReasonMaxTurns, nil, knobs)
	if got := readStreak(t, h.settings, manifestID); got != 0 {
		t.Fatalf("post-fire streak = %d, want 0 (reset)", got)
	}

	proposers := listProposerEntities(t, h.entities)
	if len(proposers) != 1 {
		t.Fatalf("proposer entities = %d, want 1", len(proposers))
	}
	if proposers[0].Type != entity.TypeTask {
		t.Fatalf("proposer.Type = %q, want %q", proposers[0].Type, entity.TypeTask)
	}

	scheds := listCurrentProposerSchedules(t, h.schedRow, proposers[0].EntityUID)
	if len(scheds) != 1 {
		t.Fatalf("proposer schedules = %d, want 1", len(scheds))
	}
	if scheds[0].EntityKind != schedule.KindTask {
		t.Fatalf("schedule.EntityKind = %q, want %q", scheds[0].EntityKind, schedule.KindTask)
	}

	// The owns edge should connect the manifest to the new proposer.
	edges, err := h.rels.ListIncoming(context.Background(), proposers[0].EntityUID, relationships.EdgeOwns)
	if err != nil {
		t.Fatalf("ListIncoming: %v", err)
	}
	found := false
	for _, e := range edges {
		if e.SrcKind == relationships.KindManifest && e.SrcID == manifestID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("owns edge from manifest %q to proposer not found", manifestID)
	}
}

// TestCheckProposerTriggers_DeliverableMissingAdvancesStreak — confirms
// deliverable_missing classifies as non-transient (same bucket as
// max_turns). Mixed reasons within a streak window still fire.
func TestCheckProposerTriggers_DeliverableMissingAdvancesStreak(t *testing.T) {
	h := newProposerHarness(t)
	const manifestID = "m-mix"
	const taskID = "t-mix"
	seedManifestOwnsTask(t, h.rels, manifestID, taskID)

	knobs := runtimeKnobs{ProposerEnabled: true, ProposerTriggerFailureStreak: 2}

	h.r.checkProposerTriggers(context.Background(),
		&Task{ID: taskID}, execution.TerminalReasonDeliverableMissing, nil, knobs)
	if got := readStreak(t, h.settings, manifestID); got != 1 {
		t.Fatalf("after deliverable_missing, streak = %d, want 1", got)
	}

	h.r.checkProposerTriggers(context.Background(),
		&Task{ID: taskID}, execution.TerminalReasonMaxTurns, nil, knobs)
	if got := readStreak(t, h.settings, manifestID); got != 0 {
		t.Fatalf("post-fire streak = %d, want 0", got)
	}
	if rows := listProposerEntities(t, h.entities); len(rows) != 1 {
		t.Fatalf("proposer entities = %d, want 1", len(rows))
	}
}

// TestCheckProposerTriggers_TransientResetsStreak — a transient failure
// (timeout / process_error / build_fail) zeroes the streak counter so it
// doesn't accumulate across unrelated infrastructure flakes.
func TestCheckProposerTriggers_TransientResetsStreak(t *testing.T) {
	cases := []string{
		execution.TerminalReasonTimeout,
		execution.TerminalReasonProcessError,
		execution.TerminalReasonBuildFail,
	}
	for _, reason := range cases {
		t.Run(reason, func(t *testing.T) {
			h := newProposerHarness(t)
			manifestID := "m-trans-" + reason
			taskID := "t-trans-" + reason
			seedManifestOwnsTask(t, h.rels, manifestID, taskID)

			knobs := runtimeKnobs{ProposerEnabled: true, ProposerTriggerFailureStreak: 3}

			// Build streak to 2 with non-transient.
			h.r.checkProposerTriggers(context.Background(),
				&Task{ID: taskID}, execution.TerminalReasonMaxTurns, nil, knobs)
			h.r.checkProposerTriggers(context.Background(),
				&Task{ID: taskID}, execution.TerminalReasonMaxTurns, nil, knobs)
			if got := readStreak(t, h.settings, manifestID); got != 2 {
				t.Fatalf("pre-reset streak = %d, want 2", got)
			}

			// Transient failure — streak resets, no proposer fires.
			h.r.checkProposerTriggers(context.Background(),
				&Task{ID: taskID}, reason, nil, knobs)
			if got := readStreak(t, h.settings, manifestID); got != 0 {
				t.Fatalf("post-transient streak = %d, want 0", got)
			}
			if rows := listProposerEntities(t, h.entities); len(rows) != 0 {
				t.Fatalf("proposer entities = %d, want 0", len(rows))
			}
		})
	}
}

// TestCheckProposerTriggers_CostThresholdFires — a single run whose
// EstimatedCostUSD exceeds proposer_trigger_cost_usd fires the proposer
// regardless of the streak counter.
func TestCheckProposerTriggers_CostThresholdFires(t *testing.T) {
	h := newProposerHarness(t)
	const manifestID = "m-cost"
	const taskID = "t-cost"
	seedManifestOwnsTask(t, h.rels, manifestID, taskID)

	row := &execution.Row{EstimatedCostUSD: 10.0}
	knobs := runtimeKnobs{
		ProposerEnabled:              true,
		ProposerTriggerFailureStreak: 0,
		ProposerTriggerCostUSD:       5.0,
	}

	h.r.checkProposerTriggers(context.Background(),
		&Task{ID: taskID}, execution.TerminalReasonSuccess, row, knobs)

	if rows := listProposerEntities(t, h.entities); len(rows) != 1 {
		t.Fatalf("proposer entities = %d, want 1", len(rows))
	}
}

// TestCheckProposerTriggers_BelowCostThreshold_NoFire — a run at or
// below the cost threshold does not fire on cost; with no streak budget
// the path stays inert.
func TestCheckProposerTriggers_BelowCostThreshold_NoFire(t *testing.T) {
	h := newProposerHarness(t)
	const manifestID = "m-undercost"
	const taskID = "t-undercost"
	seedManifestOwnsTask(t, h.rels, manifestID, taskID)

	row := &execution.Row{EstimatedCostUSD: 5.0}
	knobs := runtimeKnobs{
		ProposerEnabled:        true,
		ProposerTriggerCostUSD: 5.0, // strictly-greater check, so 5 == 5 should not fire
	}

	h.r.checkProposerTriggers(context.Background(),
		&Task{ID: taskID}, execution.TerminalReasonSuccess, row, knobs)
	if rows := listProposerEntities(t, h.entities); len(rows) != 0 {
		t.Fatalf("proposer entities = %d, want 0", len(rows))
	}
}

// TestCheckProposerTriggers_NoManifest_NoFire — a task that lacks an
// owns-edge to a manifest has no scope to score; the path bails without
// minting an entity or queueing a schedule.
func TestCheckProposerTriggers_NoManifest_NoFire(t *testing.T) {
	h := newProposerHarness(t)
	knobs := runtimeKnobs{ProposerEnabled: true, ProposerTriggerFailureStreak: 1}

	h.r.checkProposerTriggers(context.Background(),
		&Task{ID: "t-orphan"}, execution.TerminalReasonMaxTurns, nil, knobs)
	if rows := listProposerEntities(t, h.entities); len(rows) != 0 {
		t.Fatalf("proposer entities = %d, want 0", len(rows))
	}
}

// TestDecodeRuntimeKnobs_ProposerDefaults — catalog defaults flow
// through decodeRuntimeKnobs cleanly: enabled=false (off), streak
// threshold = 3, cost threshold = 0. Drift here would silently flip the
// runtime contract for every task.
func TestDecodeRuntimeKnobs_ProposerDefaults(t *testing.T) {
	r, _, tasks, manifests := newRunnerHarness(t)
	wireTask(tasks, manifests, "t-pd", "m-pd", "prod-pd")

	ctx := context.Background()
	scope, _, err := r.resolveMaxParallel(ctx, "t-pd")
	if err != nil {
		t.Fatalf("resolveMaxParallel: %v", err)
	}
	all, err := r.resolver.ResolveAll(ctx, scope)
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}
	knobs, err := decodeRuntimeKnobs(all)
	if err != nil {
		t.Fatalf("decodeRuntimeKnobs: %v", err)
	}

	if knobs.ProposerEnabled {
		t.Errorf("ProposerEnabled = true, want false (catalog default)")
	}
	if knobs.ProposerTriggerFailureStreak != 3 {
		t.Errorf("ProposerTriggerFailureStreak = %d, want 3", knobs.ProposerTriggerFailureStreak)
	}
	if knobs.ProposerTriggerCostUSD != 0.0 {
		t.Errorf("ProposerTriggerCostUSD = %v, want 0.0", knobs.ProposerTriggerCostUSD)
	}
}

// TestDecodeRuntimeKnobs_ProposerEnabledOverride — flipping
// proposer_enabled at product scope flows through to the runtimeKnobs
// bool, exercising the enum-string → bool normalisation.
func TestDecodeRuntimeKnobs_ProposerEnabledOverride(t *testing.T) {
	r, store, tasks, manifests := newRunnerHarness(t)
	wireTask(tasks, manifests, "t-pe", "m-pe", "prod-pe")
	setProductKnob(t, store, "prod-pe", "proposer_enabled", "true")

	ctx := context.Background()
	scope, _, err := r.resolveMaxParallel(ctx, "t-pe")
	if err != nil {
		t.Fatalf("resolveMaxParallel: %v", err)
	}
	all, err := r.resolver.ResolveAll(ctx, scope)
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}
	knobs, err := decodeRuntimeKnobs(all)
	if err != nil {
		t.Fatalf("decodeRuntimeKnobs: %v", err)
	}
	if !knobs.ProposerEnabled {
		t.Fatalf("ProposerEnabled = false, want true (product override)")
	}
}
