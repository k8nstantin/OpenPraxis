package product

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// ErrCycle is returned by AddDep when the requested edge would close a
// cycle in the product dependency graph. Handlers translate to HTTP 409
// / MCP error text verbatim so the operator sees the rejected pair.
var ErrCycle = errors.New("product_dependencies: cycle detected")

// ErrSelfLoop is returned when a product is asked to depend on itself.
// Enforced at the DB layer via CHECK but guarded before INSERT so the
// error message is clearer than the raw sqlite constraint text.
var ErrSelfLoop = errors.New("product_dependencies: a product cannot depend on itself")

// Dep is the denormalized row UI + MCP callers want when listing deps
// or dependents: id + marker + title + current status is enough to
// render without a second lookup per row. Matches the Dep shape in
// manifest.Dep so consumers can reuse render helpers across tiers.
type Dep struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	CreatedBy string    `json:"created_by"`
}

// terminalProductStatuses mirrors the manifest convention (#76): a
// product's dep is "satisfied" when it reaches one of these terminal
// states. Kept as a var so scheduler code can build IN clauses from
// the same source of truth and the two tiers don't drift.
var terminalProductStatuses = []string{"closed", "archive"}

// IsTerminalStatus reports whether a product.status value counts as
// satisfying dependents.
func IsTerminalStatus(status string) bool {
	for _, s := range terminalProductStatuses {
		if s == status {
			return true
		}
	}
	return false
}

// TerminalStatuses returns a copy of the terminal-status set for use
// in scheduler / runner SQL predicates. Matches manifest.TerminalManifestStatuses.
func TerminalStatuses() []string {
	out := make([]string, len(terminalProductStatuses))
	copy(out, terminalProductStatuses)
	return out
}

// initDependenciesSchema creates the product_dependencies join table.
// Called from Store.init() after the main products table exists.
// Idempotent.
func (s *Store) initDependenciesSchema() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS product_dependencies (
		product_id            TEXT NOT NULL,
		depends_on_product_id TEXT NOT NULL,
		created_at            INTEGER NOT NULL,
		created_by            TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (product_id, depends_on_product_id),
		CHECK (product_id != depends_on_product_id)
	)`)
	if err != nil {
		return fmt.Errorf("create product_dependencies table: %w", err)
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_product_deps_src ON product_dependencies(product_id)`); err != nil {
		return fmt.Errorf("create product_deps src index: %w", err)
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_product_deps_dst ON product_dependencies(depends_on_product_id)`); err != nil {
		return fmt.Errorf("create product_deps dst index: %w", err)
	}
	return nil
}

// AddDep adds a product→depends_on_product edge after cycle detection.
//
//   - rejects self-loops (ErrSelfLoop)
//   - rejects edges that would close a cycle (ErrCycle), via DFS from
//     depends_on_product_id back to product_id over the current edge set
//   - idempotent on duplicate edges (INSERT OR IGNORE)
//
// On success the row carries created_at + created_by for audit.
func (s *Store) AddDep(ctx context.Context, productID, dependsOnID, createdBy string) error {
	if productID == "" || dependsOnID == "" {
		return fmt.Errorf("product_dependencies: empty product id")
	}
	if productID == dependsOnID {
		return ErrSelfLoop
	}

	reaches, err := s.pathExists(ctx, dependsOnID, productID)
	if err != nil {
		return fmt.Errorf("cycle check: %w", err)
	}
	if reaches {
		return fmt.Errorf("%w: %s → %s would close a cycle",
			ErrCycle, productID, dependsOnID)
	}

	now := time.Now().UTC().Unix()
	if _, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO product_dependencies (product_id, depends_on_product_id, created_at, created_by)
		 VALUES (?, ?, ?, ?)`,
		productID, dependsOnID, now, createdBy); err != nil {
		return fmt.Errorf("insert product dep: %w", err)
	}
	return nil
}

// RemoveDep is idempotent — removing an edge that doesn't exist
// returns nil. Matches the #79 pattern at the manifest tier so the
// operator-surface semantics are consistent across tiers.
//
// Fires the onDepRemoved handler (if wired) after a successful
// delete. The handler rehabs now-unblocked waiting tasks per Option B.
// Firing unconditionally (not just when the edge actually existed)
// keeps the contract simple: "after this call, the edge is gone and
// any rehab that should have happened has happened."
func (s *Store) RemoveDep(ctx context.Context, productID, dependsOnID string) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM product_dependencies WHERE product_id = ? AND depends_on_product_id = ?`,
		productID, dependsOnID); err != nil {
		return fmt.Errorf("delete product dep: %w", err)
	}
	if s.onDepRemoved != nil {
		s.onDepRemoved(ctx, productID)
	}
	return nil
}

// ListDeps returns out-edges: products this one depends on. Joined
// against products so each row carries denormalized status + title
// for direct rendering. Sorted by created_at so the order of addition
// is preserved in the UI.
func (s *Store) ListDeps(ctx context.Context, productID string) ([]Dep, error) {
	return s.queryDeps(ctx,
		`SELECT p.id, p.title, p.status, d.created_at, d.created_by
		 FROM product_dependencies d
		 JOIN products p ON p.id = d.depends_on_product_id
		 WHERE d.product_id = ? AND p.deleted_at = ''
		 ORDER BY d.created_at ASC`,
		productID)
}

// ListDependents returns in-edges: products that depend on this one.
// Used by activation walkers when a product becomes terminal.
func (s *Store) ListDependents(ctx context.Context, productID string) ([]Dep, error) {
	return s.queryDeps(ctx,
		`SELECT p.id, p.title, p.status, d.created_at, d.created_by
		 FROM product_dependencies d
		 JOIN products p ON p.id = d.product_id
		 WHERE d.depends_on_product_id = ? AND p.deleted_at = ''
		 ORDER BY d.created_at ASC`,
		productID)
}

// IsSatisfied reports whether every product this one depends on is in
// a terminal status. Second return is the list of unsatisfied dep ids
// so callers can build human-readable block reasons.
func (s *Store) IsSatisfied(ctx context.Context, productID string) (bool, []string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT p.id, p.status
		 FROM product_dependencies d
		 JOIN products p ON p.id = d.depends_on_product_id
		 WHERE d.product_id = ? AND p.deleted_at = ''`, productID)
	if err != nil {
		return false, nil, err
	}
	defer rows.Close()
	var unsatisfied []string
	for rows.Next() {
		var id, status string
		if err := rows.Scan(&id, &status); err != nil {
			return false, nil, err
		}
		if !IsTerminalStatus(status) {
			unsatisfied = append(unsatisfied, id)
		}
	}
	return len(unsatisfied) == 0, unsatisfied, rows.Err()
}

// pathExists does a DFS from src over product_dependencies edges,
// returning true if dst is reachable. Cycle detection on AddDep uses
// this in reverse: "does B already reach A? if so A→B would cycle."
// Visited-set guards against pre-existing cycles in legacy data.
func (s *Store) pathExists(ctx context.Context, src, dst string) (bool, error) {
	visited := map[string]bool{}
	stack := []string{src}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if node == dst {
			return true, nil
		}
		if visited[node] {
			continue
		}
		visited[node] = true

		rows, err := s.db.QueryContext(ctx,
			`SELECT depends_on_product_id FROM product_dependencies WHERE product_id = ?`, node)
		if err != nil {
			return false, err
		}
		for rows.Next() {
			var next string
			if err := rows.Scan(&next); err != nil {
				rows.Close()
				return false, err
			}
			if !visited[next] {
				stack = append(stack, next)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return false, err
		}
		rows.Close()
	}
	return false, nil
}

// queryDeps is the shared scan for ListDeps + ListDependents. Marker
// is derived here (12-char prefix) so callers don't redo the rule.
func (s *Store) queryDeps(ctx context.Context, query, id string) ([]Dep, error) {
	rows, err := s.db.QueryContext(ctx, query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Dep
	for rows.Next() {
		var d Dep
		var createdAtUnix int64
		if err := rows.Scan(&d.ID, &d.Title, &d.Status, &createdAtUnix, &d.CreatedBy); err != nil {
			return nil, err
		}
		d.CreatedAt = time.Unix(createdAtUnix, 0).UTC()
		out = append(out, d)
	}
	return out, rows.Err()
}

// logSchemaReady emits a one-line info log on startup so operators see
// which tier's dep schema is active. Not called from hot paths.
func (s *Store) logSchemaReady() {
	slog.Debug("product_dependencies schema ready", "component", "product")
}
