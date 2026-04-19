package manifest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// ErrCycle is returned by AddDep when the requested edge would introduce a
// cycle into the manifest dependency graph. Callers translate this into a
// 409 Conflict at the HTTP layer and into an MCP error text at the tool
// layer — the operator sees the exact rejected pair.
var ErrCycle = errors.New("manifest_dependencies: cycle detected")

// ErrSelfLoop is returned when a manifest is asked to depend on itself.
// Enforced at the DB layer via CHECK, but we also guard before the insert
// so the error carries a clearer message than the raw sqlite constraint
// text.
var ErrSelfLoop = errors.New("manifest_dependencies: a manifest cannot depend on itself")

// Dep is the denormalized row the UI + MCP callers want when listing
// deps or dependents: id + marker + title + current status are enough to
// render a row without a second lookup per entry.
type Dep struct {
	ID         string    `json:"id"`
	Marker     string    `json:"marker"`
	Title      string    `json:"title"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	CreatedBy  string    `json:"created_by"`
}

// terminalManifestStatuses are the statuses that mean "this manifest is
// done enough that its dependents can stop waiting on it." Per the session
// decision on issue #74 we use the existing status taxonomy — `closed`
// and `archive` count as terminal; `draft` and `open` do not.
//
// Kept as a var (not a const) because the scheduler enforcement path
// passes this set into a SQL IN clause and having it in one place means
// the predicate can't drift between the store and the scheduler.
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
// Exported for use by scheduler/runner code that needs to build SQL IN
// predicates from the same source of truth.
func TerminalManifestStatuses() []string {
	out := make([]string, len(terminalManifestStatuses))
	copy(out, terminalManifestStatuses)
	return out
}

// initDependenciesSchema creates the manifest_dependencies join table plus
// its indexes. Idempotent. Called from Store.init() after the main
// manifests table exists, so FK-like semantics can rely on the parent
// tables being present.
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
// manifests.depends_on column into manifest_dependencies. Idempotent —
// re-running is a no-op because PRIMARY KEY dedup catches existing rows.
// Safe to run on every startup until the legacy column is dropped in a
// follow-up PR.
//
// Returns the number of new rows inserted. Does NOT clear the legacy
// column; dual-read + dual-write keep the old path working while the
// migration settles.
func (s *Store) BackfillLegacyDependsOn(ctx context.Context) (int, error) {
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

	now := time.Now().UTC().Unix()
	inserted := 0
	for _, e := range entries {
		for _, raw := range strings.Split(e.depList, ",") {
			depID := strings.TrimSpace(raw)
			if depID == "" || depID == e.manifestID {
				continue
			}
			res, err := s.db.ExecContext(ctx,
				`INSERT OR IGNORE INTO manifest_dependencies (manifest_id, depends_on_manifest_id, created_at, created_by)
				 VALUES (?, ?, ?, 'legacy-backfill')`,
				e.manifestID, depID, now)
			if err != nil {
				slog.Warn("backfill manifest dep failed",
					"component", "manifest", "manifest_id", e.manifestID, "depends_on", depID, "error", err)
				continue
			}
			n, _ := res.RowsAffected()
			inserted += int(n)
		}
	}
	if inserted > 0 {
		slog.Info("backfilled manifest dependencies from legacy column",
			"component", "manifest", "rows", inserted)
	}
	return inserted, nil
}

// AddDep adds a manifest→depends_on_manifest edge after cycle detection.
//
//   - rejects self-loops (ErrSelfLoop)
//   - rejects edges that would close a cycle (ErrCycle), computed via DFS
//     from depends_on_manifest_id back to manifest_id over the current
//     edge set
//   - idempotent on duplicate edges (INSERT OR IGNORE)
//
// On success, also appends to the legacy comma-separated column so any
// un-migrated reader sees the new edge until the column is retired.
func (s *Store) AddDep(ctx context.Context, manifestID, dependsOnID, createdBy string) error {
	if manifestID == "" || dependsOnID == "" {
		return fmt.Errorf("manifest_dependencies: empty manifest id")
	}
	if manifestID == dependsOnID {
		return ErrSelfLoop
	}

	// Cycle check: would inserting (manifestID → dependsOnID) create a
	// cycle? That happens iff dependsOnID can already reach manifestID
	// by following existing edges. DFS from dependsOnID; bail on first
	// hit.
	reaches, err := s.pathExists(ctx, dependsOnID, manifestID)
	if err != nil {
		return fmt.Errorf("cycle check: %w", err)
	}
	if reaches {
		return fmt.Errorf("%w: %s → %s would close a cycle",
			ErrCycle, manifestID, dependsOnID)
	}

	now := time.Now().UTC().Unix()
	if _, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO manifest_dependencies (manifest_id, depends_on_manifest_id, created_at, created_by)
		 VALUES (?, ?, ?, ?)`,
		manifestID, dependsOnID, now, createdBy); err != nil {
		return fmt.Errorf("insert manifest dep: %w", err)
	}

	// Dual-write: append to legacy comma-separated column so readers
	// that still query `manifests.depends_on` see the edge.
	if err := s.syncLegacyDependsOn(ctx, manifestID); err != nil {
		slog.Warn("sync legacy depends_on failed", "component", "manifest",
			"manifest_id", manifestID, "error", err)
	}
	return nil
}

// RemoveDep is idempotent — removing an edge that doesn't exist returns
// nil, not an error. The legacy column is re-synced from the join table
// after the delete so the comma-separated list stays canonical-ish.
//
// Fires the onDepRemoved handler (if wired) after a successful delete +
// legacy sync. The handler rehabs any now-unblocked waiting tasks per
// Option B — see SetDepRemovedHandler. Firing unconditionally (not only
// when the edge actually existed) keeps the contract simple: "after
// this call, the edge is gone and any rehab that should have happened
// has happened." A no-op delete followed by a no-op rehab is fine.
func (s *Store) RemoveDep(ctx context.Context, manifestID, dependsOnID string) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM manifest_dependencies WHERE manifest_id = ? AND depends_on_manifest_id = ?`,
		manifestID, dependsOnID); err != nil {
		return fmt.Errorf("delete manifest dep: %w", err)
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
// Joined against the manifests table so each row carries the denormalized
// status + title the UI renders. Results sorted by created_at so the
// UI order matches the insertion order.
func (s *Store) ListDeps(ctx context.Context, manifestID string) ([]Dep, error) {
	return s.queryDeps(ctx,
		`SELECT m.id, m.title, m.status, d.created_at, d.created_by
		 FROM manifest_dependencies d
		 JOIN manifests m ON m.id = d.depends_on_manifest_id
		 WHERE d.manifest_id = ? AND m.deleted_at = ''
		 ORDER BY d.created_at ASC`,
		manifestID)
}

// ListDependents returns the in-edges: manifests that depend on this one.
// Same shape as ListDeps; used by the watcher audit callback to walk
// impacted manifests when a parent closes.
func (s *Store) ListDependents(ctx context.Context, manifestID string) ([]Dep, error) {
	return s.queryDeps(ctx,
		`SELECT m.id, m.title, m.status, d.created_at, d.created_by
		 FROM manifest_dependencies d
		 JOIN manifests m ON m.id = d.manifest_id
		 WHERE d.depends_on_manifest_id = ? AND m.deleted_at = ''
		 ORDER BY d.created_at ASC`,
		manifestID)
}

// IsSatisfied reports whether every manifest this one depends on is in a
// terminal status. The second return is the list of unsatisfied dep
// manifest IDs — populated so the caller can build a human-readable
// block_reason for the task row without a second round-trip.
func (s *Store) IsSatisfied(ctx context.Context, manifestID string) (bool, []string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT m.id, m.status
		 FROM manifest_dependencies d
		 JOIN manifests m ON m.id = d.depends_on_manifest_id
		 WHERE d.manifest_id = ? AND m.deleted_at = ''`, manifestID)
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
	if err := rows.Err(); err != nil {
		return false, nil, err
	}
	return len(unsatisfied) == 0, unsatisfied, nil
}

// pathExists does a DFS from src, returning true if dst is reachable by
// walking manifest_dependencies edges (src → ... → dst). Cycle detection
// uses it in reverse: "does B already reach A? if so A→B would close a
// cycle."
//
// The visited set caps traversal so pre-existing cycles in the DB (which
// shouldn't happen but mustn't hang the process) terminate. Manifests are
// counted in the dozens per node so O(V+E) traversal is fine; if this
// ever shows up in a profile, we switch to storing transitive closure on
// each row.
func (s *Store) pathExists(ctx context.Context, src, dst string) (bool, error) {
	visited := map[string]bool{}
	stack := []string{src}
	for len(stack) > 0 {
		// Pop.
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
			`SELECT depends_on_manifest_id FROM manifest_dependencies WHERE manifest_id = ?`, node)
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

// queryDeps is the shared scan path for ListDeps + ListDependents. Marker
// is derived here so callers don't have to duplicate the 12-char prefix
// rule.
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
		if len(d.ID) >= 12 {
			d.Marker = d.ID[:12]
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// syncLegacyDependsOn regenerates the comma-separated manifests.depends_on
// column from the current join-table rows for manifestID. Keeps legacy
// readers seeing a consistent view until the column is dropped. Called
// after Add/RemoveDep.
func (s *Store) syncLegacyDependsOn(ctx context.Context, manifestID string) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT depends_on_manifest_id FROM manifest_dependencies WHERE manifest_id = ? ORDER BY created_at ASC`,
		manifestID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE manifests SET depends_on = ?, updated_at = ? WHERE id = ?`,
		strings.Join(ids, ","), time.Now().UTC().Format(time.RFC3339), manifestID)
	return err
}
