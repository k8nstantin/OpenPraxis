package product

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/relationships"
)

// ErrCycle is returned by AddDep when the requested edge would close a
// cycle in the product dependency graph. Handlers translate to HTTP 409
// / MCP error text verbatim so the operator sees the rejected pair.
var ErrCycle = errors.New("product_dependencies: cycle detected")

// ErrSelfLoop is returned when a product is asked to depend on itself.
var ErrSelfLoop = errors.New("product_dependencies: a product cannot depend on itself")

// Dep is the denormalized row UI + MCP callers want when listing deps
// or dependents: id + marker + title + current status is enough to
// render without a second lookup per row. Matches the Dep shape in
// manifest.Dep so consumers can reuse render helpers across tiers.
type Dep struct {
	ID        string    `json:"id"`
	Marker    string    `json:"marker"`
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

// SetRelationshipsBackend wires the unified relationships SCD-2 store
// as the source of truth for product→product dependency edges. Once
// set, AddDep / RemoveDep / ListDeps / ListDependents / IsSatisfied
// route through relationships instead of the legacy
// product_dependencies table. The legacy table is left in place as
// historical safety but is never read or written after the cutover.
func (s *Store) SetRelationshipsBackend(r *relationships.Store) {
	s.rels = r
}

// initDependenciesSchema creates the legacy product_dependencies join
// table so existing rows survive the boot. Idempotent. After PR/M3 the
// store no longer reads or writes this table — relationships is the
// source of truth. Schema kept so historical queries against the
// dormant table still work for forensic purposes.
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

// AddDep adds a product→depends_on_product edge in the relationships
// store after cycle detection. Direction: src=depender, dst=dependee.
//
//   - rejects self-loops (ErrSelfLoop)
//   - rejects edges that would close a cycle (ErrCycle), via DFS over
//     the relationships graph
//   - idempotent on duplicate edges (Get probe before Create)
func (s *Store) AddDep(ctx context.Context, productID, dependsOnID, createdBy string) error {
	if productID == "" || dependsOnID == "" {
		return fmt.Errorf("product_dependencies: empty product id")
	}
	if productID == dependsOnID {
		return ErrSelfLoop
	}
	if s.rels == nil {
		return fmt.Errorf("product_dependencies: relationships backend not wired")
	}

	reaches, err := s.pathExists(ctx, dependsOnID, productID)
	if err != nil {
		return fmt.Errorf("cycle check: %w", err)
	}
	if reaches {
		return fmt.Errorf("%w: %s → %s would close a cycle",
			ErrCycle, productID, dependsOnID)
	}

	// Idempotency: if the current edge already exists, skip the Create
	// (which would otherwise close + re-open it and grow history for no
	// state change).
	if _, found, err := s.rels.Get(ctx, productID, dependsOnID, relationships.EdgeDependsOn); err != nil {
		return err
	} else if found {
		return nil
	}

	return s.rels.Create(ctx, relationships.Edge{
		SrcKind:   relationships.KindProduct,
		SrcID:     productID,
		DstKind:   relationships.KindProduct,
		DstID:     dependsOnID,
		Kind:      relationships.EdgeDependsOn,
		CreatedBy: createdBy,
		Reason:    "product dep added",
	})
}

// RemoveDep is idempotent — removing an edge that doesn't exist
// returns nil. Closes the relationships row by stamping valid_to.
//
// Fires the onDepRemoved handler (if wired) after a successful close.
// The handler rehabs now-unblocked waiting tasks per Option B.
func (s *Store) RemoveDep(ctx context.Context, productID, dependsOnID string) error {
	if s.rels == nil {
		return fmt.Errorf("product_dependencies: relationships backend not wired")
	}
	if err := s.rels.Remove(ctx, productID, dependsOnID,
		relationships.EdgeDependsOn, "http-api", "removed via product_dependencies API"); err != nil {
		return fmt.Errorf("remove product dep: %w", err)
	}
	if s.onDepRemoved != nil {
		s.onDepRemoved(ctx, productID)
	}
	return nil
}

// ListDeps returns out-edges: products this one depends on. Joined
// against products so each row carries denormalized status + title
// for direct rendering.
func (s *Store) ListDeps(ctx context.Context, productID string) ([]Dep, error) {
	if s.rels == nil {
		return nil, fmt.Errorf("product_dependencies: relationships backend not wired")
	}
	edges, err := s.rels.ListOutgoing(ctx, productID, relationships.EdgeDependsOn)
	if err != nil {
		return nil, err
	}
	return s.enrichEdgesAsDeps(ctx, edges, true)
}

// ListDependents returns in-edges: products that depend on this one.
// Used by activation walkers when a product becomes terminal.
func (s *Store) ListDependents(ctx context.Context, productID string) ([]Dep, error) {
	if s.rels == nil {
		return nil, fmt.Errorf("product_dependencies: relationships backend not wired")
	}
	edges, err := s.rels.ListIncoming(ctx, productID, relationships.EdgeDependsOn)
	if err != nil {
		return nil, err
	}
	return s.enrichEdgesAsDeps(ctx, edges, false)
}

// IsSatisfied reports whether every product this one depends on is in
// a terminal status. Second return is the list of unsatisfied dep ids
// so callers can build human-readable block reasons.
func (s *Store) IsSatisfied(ctx context.Context, productID string) (bool, []string, error) {
	if s.rels == nil {
		return false, nil, fmt.Errorf("product_dependencies: relationships backend not wired")
	}
	edges, err := s.rels.ListOutgoing(ctx, productID, relationships.EdgeDependsOn)
	if err != nil {
		return false, nil, err
	}
	var unsatisfied []string
	for _, e := range edges {
		if e.DstKind != relationships.KindProduct {
			continue
		}
		var status, deletedAt string
		err := s.db.QueryRowContext(ctx,
			`SELECT status, deleted_at FROM products WHERE id = ?`, e.DstID).Scan(&status, &deletedAt)
		if err != nil || deletedAt != "" {
			continue // missing / deleted dep — don't block on a phantom
		}
		if !IsTerminalStatus(status) {
			unsatisfied = append(unsatisfied, e.DstID)
		}
	}
	return len(unsatisfied) == 0, unsatisfied, nil
}

// pathExists does a DFS from src over the relationships dep graph,
// returning true if dst is reachable. Cycle detection on AddDep uses
// this in reverse: "does B already reach A? if so A→B would cycle."
func (s *Store) pathExists(ctx context.Context, src, dst string) (bool, error) {
	if s.rels == nil {
		return false, fmt.Errorf("product_dependencies: relationships backend not wired")
	}
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

		edges, err := s.rels.ListOutgoing(ctx, node, relationships.EdgeDependsOn)
		if err != nil {
			return false, err
		}
		for _, e := range edges {
			if e.DstKind != relationships.KindProduct {
				continue
			}
			if !visited[e.DstID] {
				stack = append(stack, e.DstID)
			}
		}
	}
	return false, nil
}

// enrichEdgesAsDeps joins relationships rows against the products
// table so each Dep carries the denormalised title/status the UI
// renders. outgoing controls which side of the edge is the "other"
// product to fetch — true for ListDeps (dst is the dep), false for
// ListDependents (src is the dependent).
func (s *Store) enrichEdgesAsDeps(ctx context.Context, edges []relationships.Edge, outgoing bool) ([]Dep, error) {
	if len(edges) == 0 {
		return nil, nil
	}
	// Collect the IDs we need to fetch + remember the source edge so
	// the per-row created_at / created_by can be attached.
	type lookup struct {
		other string
		edge  relationships.Edge
	}
	lookups := make([]lookup, 0, len(edges))
	ids := make([]string, 0, len(edges))
	for _, e := range edges {
		var other, otherKind string
		if outgoing {
			other, otherKind = e.DstID, e.DstKind
		} else {
			other, otherKind = e.SrcID, e.SrcKind
		}
		if otherKind != relationships.KindProduct {
			continue
		}
		lookups = append(lookups, lookup{other: other, edge: e})
		ids = append(ids, other)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	// One IN(...) lookup against products. Empty-id phantoms / soft-
	// deleted rows are filtered.
	placeholders := make([]byte, 0, 2*len(ids))
	args := make([]any, 0, len(ids))
	for i, id := range ids {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, status FROM products
		 WHERE id IN (`+string(placeholders)+`) AND deleted_at = ''`,
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type info struct{ title, status string }
	titles := make(map[string]info, len(ids))
	for rows.Next() {
		var id, title, status string
		if err := rows.Scan(&id, &title, &status); err != nil {
			return nil, err
		}
		titles[id] = info{title: title, status: status}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Build the result preserving the input edge order.
	out := make([]Dep, 0, len(lookups))
	for _, l := range lookups {
		t, ok := titles[l.other]
		if !ok {
			continue // soft-deleted or missing
		}
		d := Dep{
			ID:        l.other,
			Title:     t.title,
			Status:    t.status,
			CreatedBy: l.edge.CreatedBy,
		}
		if len(d.ID) >= 12 {
			d.Marker = d.ID[:12]
		}
		// valid_from is the canonical creation timestamp in the new
		// store; parse to time.Time matching the legacy Dep shape.
		if l.edge.ValidFrom != "" {
			if t, err := time.Parse(time.RFC3339Nano, l.edge.ValidFrom); err == nil {
				d.CreatedAt = t.UTC()
			}
		}
		out = append(out, d)
	}
	return out, nil
}

// logSchemaReady emits a one-line info log on startup so operators see
// which tier's dep schema is active. Not called from hot paths.
func (s *Store) logSchemaReady() {
	slog.Debug("product_dependencies schema ready (relationships backend)", "component", "product")
}
