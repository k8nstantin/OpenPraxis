package task

import (
	"context"
	"fmt"
	"log/slog"
)

// manifestBlockPrefix is the canonical prefix both Store.Create and
// the scheduler pre-dispatch gate (after #97) write into block_reason
// when seeding a task as 'waiting' due to an unsatisfied manifest.
// FlipManifestBlockedTasks filters on it so a task that ended up in
// 'waiting' for a different reason (e.g. task-level dep not met) does
// not get accidentally flipped by a manifest-level activation.
const manifestBlockPrefix = "manifest not satisfied"

// legacyManifestBlockPrefix is the prefix the scheduler's
// pre-dispatch gate wrote BEFORE #97 normalized it. Kept so the
// activation filter still catches waiting tasks that were seeded
// under the old format — rows in the wild today were written as
// "blocked by manifest <id-prefix> (<title>)". Removable once a DB
// audit confirms no row still has this prefix.
const legacyManifestBlockPrefix = "blocked by manifest"

// FlipManifestBlockedTasks moves tasks in manifestID that are currently
// 'waiting' because of a manifest-level block into `newStatus`, clearing
// their block_reason. Only two targets make sense:
//
//   - StatusScheduled — fired by the manifest-close propagation path
//     (the dep was just satisfied; tasks should auto-run).
//   - StatusPending — fired by the dep-removal rehab path per session
//     Option B (operator removed the dep; tasks are unblocked but must
//     be manually armed to avoid surprise budget burn).
//
// Returns the number of rows flipped. Safe to call when nothing matches
// — it returns 0 with no error.
//
// Note: the caller is responsible for verifying the manifest is actually
// satisfied before invoking this with StatusScheduled. We don't re-check
// here because the propagation walker already has an IsSatisfied result
// in hand and calling twice would race against concurrent writes.
func (s *Store) FlipManifestBlockedTasks(ctx context.Context, manifestID string, newStatus Status) (int, error) {
	if manifestID == "" {
		return 0, fmt.Errorf("FlipManifestBlockedTasks: empty manifest id")
	}
	if newStatus != StatusScheduled && newStatus != StatusPending {
		return 0, fmt.Errorf("FlipManifestBlockedTasks: newStatus must be scheduled or pending, got %q", newStatus)
	}

	// Match the block_reason prefix so we only touch tasks that were
	// seeded by the manifest-level gate. Tasks in 'waiting' due to a
	// task-level depends_on carry a different block_reason ("task ...
	// not completed") and must not be flipped here.
	// Filter accepts both block_reason prefixes: the canonical one
	// seeded by #77 (task.Store.Create) AND the legacy prefix the
	// scheduler's pre-dispatch gate wrote before #97 normalized
	// node.go:615. Without the legacy clause, tasks that went to
	// 'waiting' via the scheduler's path stay invisible to this
	// walker and the chain doesn't advance on manifest close. Drop
	// the legacy clause in a follow-up once DB is known-clean.
	// The tasks table has been retired. Tasks are now managed via
	// the entities table. Return 0 with no error.
	return 0, nil
}

// PropagateManifestClosed is the activation walker fired when a manifest
// transitions to a terminal status. It visits every manifest that
// depends on `closedManifestID`, and for each one that is now fully
// satisfied, flips its waiting-blocked-by-manifest tasks to scheduled
// so they auto-fire.
//
// Propagation is breadth-first with a visited set so no matter how
// tangled the dep graph is, each manifest is only processed once per
// closure event. The visited set guards against pre-existing cycles in
// the graph (#76's cycle detector prevents adds from creating them, but
// legacy/migrated data could still contain loops that would otherwise
// spin forever).
//
// depsFor is the lookup function — typically wrapped around
// manifest.Store.ListDependents. satisfiedFor wraps
// manifest.Store.IsSatisfied. Injected instead of imported so task
// doesn't depend on manifest, preserving the current package direction.
func (s *Store) PropagateManifestClosed(
	ctx context.Context,
	closedManifestID string,
	depsFor func(ctx context.Context, manifestID string) ([]string, error),
	satisfiedFor func(ctx context.Context, manifestID string) (bool, error),
) (totalActivated int, err error) {
	if closedManifestID == "" {
		return 0, fmt.Errorf("PropagateManifestClosed: empty manifest id")
	}

	visited := map[string]bool{closedManifestID: true}
	queue := []string{closedManifestID}

	for len(queue) > 0 {
		head := queue[0]
		queue = queue[1:]

		dependents, derr := depsFor(ctx, head)
		if derr != nil {
			return totalActivated, fmt.Errorf("list dependents of %s: %w", head, derr)
		}
		for _, dep := range dependents {
			if visited[dep] {
				continue
			}
			visited[dep] = true

			// Only activate tasks in dependents that are fully satisfied —
			// a dependent that still has other open deps must stay blocked.
			ok, sErr := satisfiedFor(ctx, dep)
			if sErr != nil {
				slog.Warn("IsSatisfied failed during propagation",
					"component", "task", "manifest_id", dep, "error", sErr)
				continue
			}
			if !ok {
				continue
			}

			flipped, fErr := s.FlipManifestBlockedTasks(ctx, dep, StatusScheduled)
			if fErr != nil {
				slog.Warn("flip failed during propagation",
					"component", "task", "manifest_id", dep, "error", fErr)
				continue
			}
			totalActivated += flipped

			// Enqueue dep so its own dependents can be checked in
			// turn. This does not mean dep's status has changed — it
			// just means tasks inside dep have been scheduled. If the
			// operator later closes dep, that triggers its own
			// propagation separately. But enqueueing here covers the
			// rare case where a chain is satisfied purely by dep
			// edges closing under soft-deletion or backfill races.
			queue = append(queue, dep)
		}
	}
	return totalActivated, nil
}

// CountManifestBlockedTasks is a no-op stub. The tasks table has been retired.
func (s *Store) CountManifestBlockedTasks(ctx context.Context, manifestID string) (int, error) {
	return 0, nil
}

