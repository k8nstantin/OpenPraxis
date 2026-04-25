// Package relationships is the unified edge store for OpenPraxis. Every
// edge between entities (product / manifest / task) — ownership AND
// dependencies AND secondary links — lives as one SCD-2 row in the
// relationships table.
//
// Rows are never DELETEd. A "remove" closes the row by setting valid_to
// to the close timestamp; the row stays for audit. A "change" closes the
// prior current row and inserts a new one in one transaction.
//
// Convention matches internal/task/store.go's task_dependency table:
// TEXT for timestamps, empty-string '' (not NULL) for "still current",
// partial indexes on valid_to = '' for the hot read path.
//
// The current SetDependency / AddDep code in task/manifest/product
// stores stays live during PR/M2's dual-write phase. PR/M3 cuts reads
// over to this store and drops the legacy tables.
package relationships

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrInvalidKind is returned when a caller passes a src_kind / dst_kind /
// edge kind outside the enumerated set. The DB-level CHECK constraint
// would catch this too, but we validate in Go first so the error has a
// clearer attribution than "CHECK constraint failed".
var ErrInvalidKind = errors.New("relationships: invalid kind value")

// ErrSelfLoop is returned when src_id == dst_id. Schema-level CHECK
// enforces this too; we duplicate the check in Go for the same clearer-
// error reason.
var ErrSelfLoop = errors.New("relationships: a node cannot have an edge to itself")

// validKinds returns true if k is one of the enumerated entity kinds.
// Lifted to a free function so future callers (e.g. MCP tool input
// validators) share the exact same allow-list as the store.
func validKind(k string) bool {
	return k == KindProduct || k == KindManifest || k == KindTask
}

// validEdgeKind returns true if k is one of the enumerated edge kinds.
// Same role as validKind for src/dst types — single source of truth so
// the schema CHECK + Go-side check + MCP tool validator never drift.
func validEdgeKind(k string) bool {
	return k == EdgeOwns || k == EdgeDependsOn || k == EdgeReviews || k == EdgeLinksTo
}

// nowUTC returns the current time in ISO8601 UTC. Centralised so every
// timestamp written by this package matches the format expected by the
// existing TEXT columns and the SCD-2 conventions in task_dependency.
func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// Standard entity kinds. Stored as TEXT in src_kind / dst_kind columns
// and gated by CHECK constraints at the schema level.
const (
	KindProduct  = "product"
	KindManifest = "manifest"
	KindTask     = "task"
)

// Standard edge kinds. The schema's CHECK constraint pins the allowed
// set; adding a new kind requires a migration.
const (
	EdgeOwns      = "owns"
	EdgeDependsOn = "depends_on"
	EdgeReviews   = "reviews"
	EdgeLinksTo   = "links_to"
)

// Edge is one row in the relationships table. ValidTo == "" means
// "this is the current version of the edge"; non-empty means "superseded
// at this timestamp." Metadata is an opaque JSON string for edge-kind-
// specific extras (left empty for the common case).
type Edge struct {
	SrcKind   string
	SrcID     string
	DstKind   string
	DstID     string
	Kind      string
	Metadata  string
	ValidFrom string
	ValidTo   string
	CreatedBy string
	Reason    string
	CreatedAt string
}

// Store owns the relationships table.
type Store struct {
	db *sql.DB
}

// New opens the store and runs the idempotent schema migration. Pass
// the same *sql.DB used by the rest of the OpenPraxis stores so this
// table lives in the canonical openpraxis.db.
func New(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.init(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) init() error {
	// Schema invariants:
	//   - Composite PK ensures no two rows share (src_id, dst_id, kind, valid_from);
	//     a simultaneous duplicate Create at the same timestamp will fail at
	//     INSERT and the caller can retry.
	//   - CHECK constraints enforce kind enums at the DB level so a corrupt
	//     write is impossible even if Go-side validation has a bug.
	//   - src_id <> dst_id forbids self-loops at the relationship level
	//     (a node depending on itself is meaningless).
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS relationships (
		src_kind   TEXT NOT NULL,
		src_id     TEXT NOT NULL,
		dst_kind   TEXT NOT NULL,
		dst_id     TEXT NOT NULL,
		kind       TEXT NOT NULL,
		metadata   TEXT NOT NULL DEFAULT '',
		valid_from TEXT NOT NULL,
		valid_to   TEXT NOT NULL DEFAULT '',
		created_by TEXT NOT NULL DEFAULT '',
		reason     TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		PRIMARY KEY (src_id, dst_id, kind, valid_from),
		CHECK (src_id <> dst_id),
		CHECK (src_kind IN ('product','manifest','task')),
		CHECK (dst_kind IN ('product','manifest','task')),
		CHECK (kind IN ('owns','depends_on','reviews','links_to'))
	)`)
	if err != nil {
		return fmt.Errorf("create relationships table: %w", err)
	}

	// Hot path: "what edges leave / enter this node right now?" — driven
	// by the dashboard, the scheduler, and every DAG walk. Partial index
	// on valid_to = '' keeps these queries O(log n) on current state
	// regardless of how much history accumulates.
	if _, err := s.db.Exec(
		`CREATE INDEX IF NOT EXISTS idx_rel_src_current ON relationships(src_id, kind) WHERE valid_to = ''`,
	); err != nil {
		return fmt.Errorf("create idx_rel_src_current: %w", err)
	}
	if _, err := s.db.Exec(
		`CREATE INDEX IF NOT EXISTS idx_rel_dst_current ON relationships(dst_id, kind) WHERE valid_to = ''`,
	); err != nil {
		return fmt.Errorf("create idx_rel_dst_current: %w", err)
	}

	// History / time-travel: "what did this edge look like at time t?"
	// Full (non-partial) index — touches both current + closed rows.
	if _, err := s.db.Exec(
		`CREATE INDEX IF NOT EXISTS idx_rel_history ON relationships(src_id, dst_id, valid_from DESC)`,
	); err != nil {
		return fmt.Errorf("create idx_rel_history: %w", err)
	}

	return nil
}

// Create opens or replaces the current edge for (e.SrcID, e.DstID, e.Kind).
// The full SCD-2 mutation:
//
//  1. If a current row exists for this edge tuple, UPDATE its valid_to to
//     the close timestamp. The closed row stays in the table — never
//     DELETEd — so the audit trail is intact.
//  2. INSERT a fresh row with valid_to='' (the new "current"). The new
//     row's created_by + reason describe THIS mutation; the prior row's
//     attribution is preserved as it was.
//
// Both steps run in one transaction. If either fails, neither lands.
//
// Timestamps:
//   - e.ValidFrom: optional. Empty → use server's now() in UTC. Non-empty
//     → caller controls (used by PR/M2 backfill to preserve historical
//     valid_from from legacy tables). Caller is responsible for ensuring
//     monotonicity if they pass an explicit value.
//   - e.CreatedAt: always overwritten with now() — this is when the row
//     was inserted, not when the edge logically became current. Useful
//     for "show me edges actually written today" queries even if their
//     ValidFrom is backfilled to the past.
//
// Validation: src_kind / dst_kind / kind must be in the enumerated sets.
// SrcID == DstID is rejected. The DB-level CHECK constraints would catch
// these too, but we check in Go first for cleaner error messages.
//
// Idempotency note: Create is NOT a no-op when called with identical
// inputs — it WILL produce two rows in history (the second one closes
// the first immediately on the next call). That matches SCD-2 semantics:
// every Create is a state mutation. Callers wanting "no-op when
// unchanged" should ListOutgoing first and skip the Create if the
// current row already matches.
//
// Concurrent writes: two Create calls at the exact same nanosecond for
// the same (src_id, dst_id, kind, valid_from) hit the composite PK
// constraint and one loses with ErrConstraint. Caller can retry; the
// retry's nowUTC() will differ.
func (s *Store) Create(ctx context.Context, e Edge) error {
	// Defense-in-depth validation. Each check returns ErrInvalidKind /
	// ErrSelfLoop with a wrapped reason rather than waiting for SQLite
	// to reject the INSERT with a generic CHECK error.
	if !validKind(e.SrcKind) {
		return fmt.Errorf("%w: src_kind=%q", ErrInvalidKind, e.SrcKind)
	}
	if !validKind(e.DstKind) {
		return fmt.Errorf("%w: dst_kind=%q", ErrInvalidKind, e.DstKind)
	}
	if !validEdgeKind(e.Kind) {
		return fmt.Errorf("%w: kind=%q", ErrInvalidKind, e.Kind)
	}
	if e.SrcID == e.DstID {
		return fmt.Errorf("%w: %s", ErrSelfLoop, e.SrcID)
	}

	// Resolve timestamps once so the close + insert share the same
	// "now". Using one timestamp means the prior row's valid_to and
	// the new row's valid_from align exactly — the audit trail has no
	// gap between versions.
	now := nowUTC()
	validFrom := e.ValidFrom
	if validFrom == "" {
		validFrom = now
	}

	// Run close + insert in one transaction. If the INSERT fails (PK
	// collision, CHECK constraint, etc.) the close also rolls back so
	// the prior row stays current — no orphaned "everyone's closed"
	// state.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // safe; commit promotes to no-op

	// Step 1: close the prior current row (if any). The WHERE clause
	// includes valid_to='' so this only touches the one current row.
	// If no current row exists, this is a no-op (RowsAffected == 0)
	// and Create proceeds straight to the INSERT — opening a fresh
	// edge with no prior history.
	if _, err := tx.ExecContext(ctx,
		`UPDATE relationships
		   SET valid_to = ?
		 WHERE src_id = ? AND dst_id = ? AND kind = ? AND valid_to = ''`,
		now, e.SrcID, e.DstID, e.Kind); err != nil {
		return fmt.Errorf("close prior edge: %w", err)
	}

	// Step 2: insert the new current row.
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO relationships
		   (src_kind, src_id, dst_kind, dst_id, kind, metadata,
		    valid_from, valid_to, created_by, reason, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?)`,
		e.SrcKind, e.SrcID, e.DstKind, e.DstID, e.Kind, e.Metadata,
		validFrom, e.CreatedBy, e.Reason, now); err != nil {
		return fmt.Errorf("insert new edge: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// Remove closes the current edge for (srcID, dstID, kind) by setting its
// valid_to to now(). Does NOT insert a replacement row — the edge stops
// existing. To re-add it later, call Create; that opens a fresh history
// thread.
//
// Idempotent: if no current row exists for this tuple, returns nil
// without error (the post-state matches the request).
//
// Attribution of the close: by + reason are written into the closing
// row's created_by + reason columns, OVERWRITING whatever was there from
// the original Create. This deliberately conflates "who made the row"
// and "who closed the row" into the same two columns — the simpler
// alternative to a separate audit_event table. The valid_to column
// disambiguates: if valid_to != '', created_by/reason describe the
// CLOSE; if valid_to == '', they describe the CREATE. Document this in
// the operator playbook for PR/M3.
//
// Use case: a manifest's depends_on edge gets removed when the operator
// decides M5 no longer needs M2 as a prereq. The closing row then says
// `valid_to=2026-04-25T...`, `created_by=alice`, `reason="M5 standalone now"`.
func (s *Store) Remove(ctx context.Context, srcID, dstID, kind, by, reason string) error {
	if !validEdgeKind(kind) {
		return fmt.Errorf("%w: kind=%q", ErrInvalidKind, kind)
	}
	now := nowUTC()

	// Single UPDATE; partial index on valid_to='' makes the lookup O(log n).
	// RowsAffected==0 is a valid outcome (idempotent semantics) so we
	// don't surface an error in that case.
	_, err := s.db.ExecContext(ctx,
		`UPDATE relationships
		   SET valid_to = ?, created_by = ?, reason = ?
		 WHERE src_id = ? AND dst_id = ? AND kind = ? AND valid_to = ''`,
		now, by, reason, srcID, dstID, kind)
	if err != nil {
		return fmt.Errorf("close edge: %w", err)
	}
	return nil
}

// rowColumns is the standard SELECT projection for relationships rows.
// Centralised so every reader (List/History/Walk) uses identical
// column ordering — a misaligned scan is a class of bug we eliminate
// at the type level.
const rowColumns = `src_kind, src_id, dst_kind, dst_id, kind, metadata,
	valid_from, valid_to, created_by, reason, created_at`

// scanEdge scans one row from the standard rowColumns projection into
// an Edge struct. Used by every reader's row loop. Keeping this in one
// place means changing the column order is a single-file edit instead
// of touching every Scan call site.
func scanEdge(rows *sql.Rows) (Edge, error) {
	var e Edge
	err := rows.Scan(
		&e.SrcKind, &e.SrcID, &e.DstKind, &e.DstID, &e.Kind, &e.Metadata,
		&e.ValidFrom, &e.ValidTo, &e.CreatedBy, &e.Reason, &e.CreatedAt,
	)
	return e, err
}

// ListOutgoing returns all CURRENT edges leaving srcID, optionally
// filtered to a specific edge kind. "Current" means valid_to == ''.
//
// edgeKind == "" returns every kind (owns + depends_on + reviews +
// links_to). Pass a specific kind to restrict (e.g. EdgeDependsOn for
// just the dep graph).
//
// Hits the partial index idx_rel_src_current — O(log n + matches) on
// current state regardless of how much history accumulates.
//
// Returned edges are unordered. Callers that need ordering should sort
// in Go (typically by ValidFrom or DstID).
func (s *Store) ListOutgoing(ctx context.Context, srcID, edgeKind string) ([]Edge, error) {
	// Build query + args dynamically so the WHERE is precisely "what
	// the partial index covers" — no wasted predicate that would force
	// a wider scan.
	q := `SELECT ` + rowColumns + ` FROM relationships
		 WHERE src_id = ? AND valid_to = ''`
	args := []any{srcID}
	if edgeKind != "" {
		if !validEdgeKind(edgeKind) {
			return nil, fmt.Errorf("%w: kind=%q", ErrInvalidKind, edgeKind)
		}
		q += ` AND kind = ?`
		args = append(args, edgeKind)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query outgoing: %w", err)
	}
	defer rows.Close()

	// Pre-allocate a small slice; most nodes have a handful of edges.
	// If a node has hundreds, append's growth handles it.
	out := make([]Edge, 0, 8)
	for rows.Next() {
		e, err := scanEdge(rows)
		if err != nil {
			return nil, fmt.Errorf("scan outgoing: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListIncoming returns all CURRENT edges arriving at dstID, optionally
// filtered to a specific edge kind. Mirror of ListOutgoing but driven
// by the dst-side partial index idx_rel_dst_current.
//
// Use case: "who depends on this manifest?" — reverse-lookup that
// today requires a full scan of manifest_dependencies in the reverse
// direction. With the dst index it's O(log n + matches).
func (s *Store) ListIncoming(ctx context.Context, dstID, edgeKind string) ([]Edge, error) {
	q := `SELECT ` + rowColumns + ` FROM relationships
		 WHERE dst_id = ? AND valid_to = ''`
	args := []any{dstID}
	if edgeKind != "" {
		if !validEdgeKind(edgeKind) {
			return nil, fmt.Errorf("%w: kind=%q", ErrInvalidKind, edgeKind)
		}
		q += ` AND kind = ?`
		args = append(args, edgeKind)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query incoming: %w", err)
	}
	defer rows.Close()

	out := make([]Edge, 0, 8)
	for rows.Next() {
		e, err := scanEdge(rows)
		if err != nil {
			return nil, fmt.Errorf("scan incoming: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// History returns every version of one specific edge (one src_id +
// dst_id + kind tuple), oldest first. Includes both the closed
// historical rows AND the current row (if any) at the tail.
//
// Use case: "show me the audit trail of M5's depends_on M2 edge over
// time" — when did it open, who opened it, when was it closed, who
// closed it, did it re-open later, etc. Drives the dashboard's
// per-edge history panel.
//
// Order: ASC by valid_from. Earliest version first; current row (if
// present) last. This is the natural "story" order — readers walk it
// chronologically.
//
// Hits idx_rel_history (src_id, dst_id, valid_from DESC) which the
// query optimizer reverses to ASC at minimal cost.
func (s *Store) History(ctx context.Context, srcID, dstID, kind string) ([]Edge, error) {
	if !validEdgeKind(kind) {
		return nil, fmt.Errorf("%w: kind=%q", ErrInvalidKind, kind)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT `+rowColumns+` FROM relationships
		 WHERE src_id = ? AND dst_id = ? AND kind = ?
		 ORDER BY valid_from ASC`,
		srcID, dstID, kind)
	if err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}
	defer rows.Close()

	// Edges typically have <10 versions over their lifetime. 8 is a
	// reasonable default capacity; growth handles outliers.
	out := make([]Edge, 0, 8)
	for rows.Next() {
		e, err := scanEdge(rows)
		if err != nil {
			return nil, fmt.Errorf("scan history: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
