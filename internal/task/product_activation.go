package task

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// productBlockPrefix is the literal prefix Store.Create writes into
// block_reason when seeding a task as 'waiting' due to an unsatisfied
// product-level dependency. FlipProductBlockedTasks filters on it so
// tasks waiting for a different reason (task-level or manifest-level
// dep) don't get swept up by product-close propagation.
const productBlockPrefix = "product not satisfied"

// ProductReadinessChecker is the hook task.Store consults to decide
// whether a new task's product has all its product-level dependencies
// satisfied. Mirrors ManifestReadinessChecker from #77. Wired in
// node.go.
type ProductReadinessChecker interface {
	IsSatisfied(ctx context.Context, productID string) (ok bool, unsatisfied []string, err error)
}

// SetProductChecker wires a product readiness checker. Nil-safe.
func (s *Store) SetProductChecker(c ProductReadinessChecker) {
	s.productChecker = c
}

// FlipProductBlockedTasks moves tasks in every manifest belonging to
// productID that are currently 'waiting' with a product-level
// block_reason to `newStatus`, clearing block_reason.
//
// Joins tasks → manifests → products so the filter works even though
// tasks don't carry product_id directly. The block_reason prefix
// filter guarantees we never touch tasks blocked at a different tier.
//
// Valid targets:
//
//   - StatusScheduled — fired by close propagation (dep just satisfied;
//     tasks auto-run).
//   - StatusPending — fired by dep-removal rehab per Option B
//     (operator must arm manually after a scope change).
//
// Caller verifies the product is satisfied before invoking with
// StatusScheduled.
func (s *Store) FlipProductBlockedTasks(ctx context.Context, productID string, newStatus Status) (int, error) {
	if productID == "" {
		return 0, fmt.Errorf("FlipProductBlockedTasks: empty product id")
	}
	if newStatus != StatusScheduled && newStatus != StatusPending {
		return 0, fmt.Errorf("FlipProductBlockedTasks: newStatus must be scheduled or pending, got %q", newStatus)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	// next_run_at must be set when flipping to scheduled; see #114 +
	// the matching fix in FlipManifestBlockedTasks. Pending path
	// keeps it empty (pending tasks never auto-fire).
	nextRunAt := ""
	if newStatus == StatusScheduled {
		nextRunAt = now
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE tasks
		 SET status = ?, block_reason = '', next_run_at = ?, updated_at = ?
		 WHERE id IN (
		   SELECT t.id FROM tasks t
		   JOIN manifests m ON m.id = t.manifest_id
		   WHERE m.project_id = ?
		     AND t.status = 'waiting'
		     AND t.block_reason LIKE ?
		     AND t.deleted_at = ''
		     AND m.deleted_at = ''
		 )`,
		string(newStatus), nextRunAt, now, productID, productBlockPrefix+"%")
	if err != nil {
		return 0, fmt.Errorf("flip product-blocked tasks: %w", err)
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		slog.Info("flipped product-blocked tasks",
			"component", "task", "product_id", productID,
			"new_status", newStatus, "count", n)
	}
	return int(n), nil
}

// PropagateProductClosed is the activation walker fired when a product
// transitions to a terminal status. Mirror of PropagateManifestClosed
// (#78): BFS with visited set, closures injected so task doesn't
// import product.
func (s *Store) PropagateProductClosed(
	ctx context.Context,
	closedProductID string,
	depsFor func(ctx context.Context, productID string) ([]string, error),
	satisfiedFor func(ctx context.Context, productID string) (bool, error),
) (totalActivated int, err error) {
	if closedProductID == "" {
		return 0, fmt.Errorf("PropagateProductClosed: empty product id")
	}

	visited := map[string]bool{closedProductID: true}
	queue := []string{closedProductID}

	for len(queue) > 0 {
		head := queue[0]
		queue = queue[1:]

		dependents, derr := depsFor(ctx, head)
		if derr != nil {
			return totalActivated, fmt.Errorf("list product dependents of %s: %w", head, derr)
		}
		for _, dep := range dependents {
			if visited[dep] {
				continue
			}
			visited[dep] = true

			ok, sErr := satisfiedFor(ctx, dep)
			if sErr != nil {
				slog.Warn("product IsSatisfied failed during propagation",
					"component", "task", "product_id", dep, "error", sErr)
				continue
			}
			if !ok {
				continue
			}

			flipped, fErr := s.FlipProductBlockedTasks(ctx, dep, StatusScheduled)
			if fErr != nil {
				slog.Warn("flip failed during product propagation",
					"component", "task", "product_id", dep, "error", fErr)
				continue
			}
			totalActivated += flipped
			queue = append(queue, dep)
		}
	}
	return totalActivated, nil
}
