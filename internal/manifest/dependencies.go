package manifest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/relationships"
)

// ErrCycle is returned by AddDep when the requested edge would introduce a
// cycle into the manifest dependency graph. Callers translate this into a
// 409 Conflict at the HTTP layer and into an MCP error text at the tool
// layer — the operator sees the exact rejected pair.
var ErrCycle = errors.New("manifest_dependencies: cycle detected")

// ErrSelfLoop is returned when a manifest is asked to depend on itself.
var ErrSelfLoop = errors.New("manifest_dependencies: a manifest cannot depend on itself")

// Dep is the denormalized row the UI + MCP callers want when listing
// deps or dependents: id + marker + title + current status are enough to
// render a row without a second lookup per entry.
type Dep struct {
	ID        string    `json:"id"`
	Marker    string    `json:"marker"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	CreatedBy string    `json:"created_by"`
}

// terminalManifestStatuses are the statuses that mean "this manifest is
// done enough that its dependents can stop waiting on it." Per the session
// decision on issue #74 we use the existing status taxonomy — `closed`
// and `archive` count as terminal; `draft` and `open` do not.
var terminalManifestStatuses = []string{"closed", "archive"}

// IsTerminalStatus reports whether a manifest.status value counts as
// satisfying dependents.
func IsTerminalStatus(status string) bool {
	for _, s := range terminalManifestStatuses {
		if s == status {
			return true
		}
	}
	return false
}

// TerminalManifestStatuses returns a copy of the terminal-status set.
func TerminalManifestStatuses() []string {
	out := make([]string, len(terminalManifestStatuses))
	copy(out, terminalManifestStatuses)
	return out
}

// SetRelationshipsBackend wires the unified relationships SCD-2 store
// as the source of truth for manifest→manifest dependency edges. After
// PR/M3 cutover, every dep read + write routes through this; the
// legacy manifest_dependencies table is dormant historical safety.
func (s *Store) SetRelationshipsBackend(r *relationships.Store) {
	s.rels = r
}

// initDependenciesSchema creates the legacy manifest_dependencies join
// table so existing rows survive the boot. Idempotent. After PR/M3 the
// store no longer reads or writes this table — relationships is the
// source of truth.
func (s *Store) initDependenciesSchema() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS manifest_dependencies (
		manifest_id            TEXT NOT NULL,
		depends_on_manifest_id TEXT NOT NULL,
		created_at             INTEGER NOT NULL,
		created_by             TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (manifest_id, depends_on_manifest_id),
		CHECK (manifest_id != depends_on_manifest_id)
	)`)
	if err != nil {
		return fmt.Errorf("create manifest_dependencies table: %w", err)
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_manifest_deps_src ON manifest_dependencies(manifest_id)`); err != nil {
		return fmt.Errorf("create manifest_deps src index: %w", err)
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_manifest_deps_dst ON manifest_dependencies(depends_on_manifest_id)`); err != nil {
		return fmt.Errorf("create manifest_deps dst index: %w", err)
	}
	return nil
}

// BackfillLegacyDependsOn copies edges out of the legacy comma-separated
// manifests.depends_on column into the unified relationships store as
// EdgeDependsOn rows. Idempotent — Get probe per row. Not called from
// the boot path after PR/M3; retained for forensic / emergency reseed
// from the legacy column.
//
// Returns the number of new rows inserted.
func (s *Store) BackfillLegacyDependsOn(ctx context.Context) (int, error) {
	if s.rels == nil {
		// Nothing to do until the backend is wired; treat as zero-row
		// migrate so callers can run this idempotently.
		return 0, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, depends_on FROM manifests WHERE depends_on != '' AND deleted_at = ''`)
	if err != nil {
		return 0, fmt.Errorf("scan legacy depends_on: %w", err)
	}
	defer rows.Close()

	type entry struct{ manifestID, depList string }
	var entries []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.manifestID, &e.depList); err != nil {
			return 0, err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	inserted := 0
	for _, e := range entries {
		for _, raw := range strings.Split(e.depList, ",") {
			depID := strings.TrimSpace(raw)
			if depID == "" || depID == e.manifestID {
				continue
			}
			if _, found, err := s.rels.Get(ctx, e.manifestID, depID, relationships.EdgeDependsOn); err != nil {
				slog.Warn("backfill manifest dep get failed",
					"manifest_id", e.manifestID, "depends_on", depID, "error", err)
				continue
			} else if found {
				continue
			}
			if err := s.rels.Create(ctx, relationships.Edge{
				SrcKind:   relationships.KindManifest,
				SrcID:     e.manifestID,
				DstKind:   relationships.KindManifest,
				DstID:     depID,
				Kind:      relationships.EdgeDependsOn,
				CreatedBy: "legacy-backfill",
				Reason:    "backfill from manifests.depends_on column",
			}); err != nil {
				slog.Warn("backfill manifest dep create failed",
					"manifest_id", e.manifestID, "depends_on", depID, "error", err)
				continue
			}
			inserted++
		}
	}
	if inserted > 0 {
		slog.Info("backfilled manifest dependencies from legacy column",
			"component", "manifest", "rows", inserted)
	}
	return inserted, nil
}

// AddDep adds a manifest→depends_on_manifest edge in the relationships
// store after cycle detection. Direction: src=depender, dst=dependee.
//
//   - rejects self-loops (ErrSelfLoop)
//   - rejects edges that would close a cycle (ErrCycle)
//   - idempotent on duplicate edges (Get probe)
//
// Also keeps the legacy comma-separated `manifests.depends_on` column
// in sync so any code path that still reads it (CheckManifestDeps,
// ParseDependsOn, propagation walkers) sees the new edge. The cache
// column is OUT OF SCOPE for the relationships migration — kept as a
// denormalised lookup that's regenerated from the relationships graph.
func (s *Store) AddDep(ctx context.Context, manifestID, dependsOnID, createdBy string) error {
	if manifestID == "" || dependsOnID == "" {
		return fmt.Errorf("manifest_dependencies: empty manifest id")
	}
	if manifestID == dependsOnID {
		return ErrSelfLoop
	}
	if s.rels == nil {
		return fmt.Errorf("manifest_dependencies: relationships backend not wired")
	}

	// Cycle check via DFS over the relationships graph: would inserting
	// (manifestID → dependsOnID) create a cycle? That happens iff
	// dependsOnID can already reach manifestID. DFS from dependsOnID;
	// bail on first hit.
	reaches, err := s.pathExists(ctx, dependsOnID, manifestID)
	if err != nil {
		return fmt.Errorf("cycle check: %w", err)
	}
	if reaches {
		return fmt.Errorf("%w: %s → %s would close a cycle",
			ErrCycle, manifestID, dependsOnID)
	}

	if _, found, err := s.rels.Get(ctx, manifestID, dependsOnID, relationships.EdgeDependsOn); err != nil {
		return err
	} else if found {
		return nil
	}

	if err := s.rels.Create(ctx, relationships.Edge{
		SrcKind:   relationships.KindManifest,
		SrcID:     manifestID,
		DstKind:   relationships.KindManifest,
		DstID:     dependsOnID,
		Kind:      relationships.EdgeDependsOn,
		CreatedBy: createdBy,
		Reason:    "manifest dep added",
	}); err != nil {
		return err
	}

	if err := s.syncLegacyDependsOn(ctx, manifestID); err != nil {
		slog.Warn("sync legacy depends_on failed", "component", "manifest",
			"manifest_id", manifestID, "error", err)
	}
	return nil
}

// RemoveDep is idempotent — removing an edge that doesn't exist returns
// nil, not an error. The legacy `manifests.depends_on` cache column is
// re-synced from the relationships graph after the close so it stays
// canonical.
//
// Fires the onDepRemoved handler (if wired) after a successful close +
// legacy sync. The handler rehabs any now-unblocked waiting tasks per
// Option B.
func (s *Store) RemoveDep(ctx context.Context, manifestID, dependsOnID string) error {
	if s.rels == nil {
		return fmt.Errorf("manifest_dependencies: relationships backend not wired")
	}
	if err := s.rels.Remove(ctx, manifestID, dependsOnID,
		relationships.EdgeDependsOn, "http-api", "removed via manifest_dependencies API"); err != nil {
		return fmt.Errorf("remove manifest dep: %w", err)
	}
	if err := s.syncLegacyDependsOn(ctx, manifestID); err != nil {
		slog.Warn("sync legacy depends_on failed", "component", "manifest",
			"manifest_id", manifestID, "error", err)
	}
	if s.onDepRemoved != nil {
		s.onDepRemoved(ctx, manifestID)
	}
	return nil
}

// ListDeps returns the out-edges: manifests this one depends on.
// Joined against the manifests table so each row carries the
// denormalized status + title the UI renders.
func (s *Store) ListDeps(ctx context.Context, manifestID string) ([]Dep, error) {
	if s.rels == nil {
		return nil, fmt.Errorf("manifest_dependencies: relationships backend not wired")
	}
	edges, err := s.rels.ListOutgoing(ctx, manifestID, relationships.EdgeDependsOn)
	if err != nil {
		return nil, err
	}
	return s.enrichEdgesAsDeps(ctx, edges, true)
}

// ListDependents returns the in-edges: manifests that depend on this
// one. Same shape as ListDeps; used by the watcher audit callback to
// walk impacted manifests when a parent closes.
func (s *Store) ListDependents(ctx context.Context, manifestID string) ([]Dep, error) {
	if s.rels == nil {
		return nil, fmt.Errorf("manifest_dependencies: relationships backend not wired")
	}
	edges, err := s.rels.ListIncoming(ctx, manifestID, relationships.EdgeDependsOn)
	if err != nil {
		return nil, err
	}
	return s.enrichEdgesAsDeps(ctx, edges, false)
}

// IsSatisfied reports whether every manifest this one depends on is in
// a terminal status. The second return is the list of unsatisfied dep
// manifest IDs.
func (s *Store) IsSatisfied(ctx context.Context, manifestID string) (bool, []string, error) {
	if s.rels == nil {
		return false, nil, fmt.Errorf("manifest_dependencies: relationships backend not wired")
	}
	edges, err := s.rels.ListOutgoing(ctx, manifestID, relationships.EdgeDependsOn)
	if err != nil {
		return false, nil, err
	}
	var unsatisfied []string
	for _, e := range edges {
		if e.DstKind != relationships.KindManifest {
			continue
		}
		var status, deletedAt string
		if err := s.db.QueryRowContext(ctx,
			`SELECT status, deleted_at FROM manifests WHERE id = ?`, e.DstID).Scan(&status, &deletedAt); err != nil {
			continue // missing dep — don't block on phantom
		}
		if deletedAt != "" {
			continue
		}
		if !IsTerminalStatus(status) {
			unsatisfied = append(unsatisfied, e.DstID)
		}
	}
	return len(unsatisfied) == 0, unsatisfied, nil
}

// pathExists does a DFS from src over the relationships dep graph,
// returning true if dst is reachable. Cycle detection on AddDep uses
// it in reverse: "does B already reach A? if so A→B would close a
// cycle."
func (s *Store) pathExists(ctx context.Context, src, dst string) (bool, error) {
	if s.rels == nil {
		return false, fmt.Errorf("manifest_dependencies: relationships backend not wired")
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
			if e.DstKind != relationships.KindManifest {
				continue
			}
			if !visited[e.DstID] {
				stack = append(stack, e.DstID)
			}
		}
	}
	return false, nil
}

// enrichEdgesAsDeps joins relationships rows against the manifests
// table to produce the Dep shape the UI renders. outgoing controls
// which side of the edge is the "other" manifest to fetch — true for
// ListDeps (dst is the dep), false for ListDependents (src is the
// dependent).
func (s *Store) enrichEdgesAsDeps(ctx context.Context, edges []relationships.Edge, outgoing bool) ([]Dep, error) {
	if len(edges) == 0 {
		return nil, nil
	}
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
		if otherKind != relationships.KindManifest {
			continue
		}
		lookups = append(lookups, lookup{other: other, edge: e})
		ids = append(ids, other)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, status FROM manifests
		 WHERE id IN (`+placeholders+`) AND deleted_at = ''`,
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
	out := make([]Dep, 0, len(lookups))
	for _, l := range lookups {
		t, ok := titles[l.other]
		if !ok {
			continue
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
		if l.edge.ValidFrom != "" {
			if pt, err := time.Parse(time.RFC3339Nano, l.edge.ValidFrom); err == nil {
				d.CreatedAt = pt.UTC()
			}
		}
		out = append(out, d)
	}
	return out, nil
}

// syncLegacyDependsOn regenerates the comma-separated
// manifests.depends_on column from the current relationships rows for
// manifestID. Keeps legacy readers (CheckManifestDeps,
// ParseDependsOn, propagation walkers) seeing a consistent view. The
// cache column is OUT OF SCOPE for the relationships migration but
// must stay aligned with the graph after every Add/RemoveDep.
func (s *Store) syncLegacyDependsOn(ctx context.Context, manifestID string) error {
	if s.rels == nil {
		return nil
	}
	edges, err := s.rels.ListOutgoing(ctx, manifestID, relationships.EdgeDependsOn)
	if err != nil {
		return err
	}
	// Sort by valid_from so the cache order matches insertion order.
	sortedIDs := make([]string, 0, len(edges))
	for _, e := range edges {
		if e.DstKind != relationships.KindManifest {
			continue
		}
		sortedIDs = append(sortedIDs, e.DstID)
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE manifests SET depends_on = ?, updated_at = ? WHERE id = ?`,
		strings.Join(sortedIDs, ","), time.Now().UTC().Format(time.RFC3339), manifestID)
	return err
}
