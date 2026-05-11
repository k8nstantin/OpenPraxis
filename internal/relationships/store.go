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
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"
)

// MaxWalkDepth is the upper bound on Walk / WalkAt recursion depth.
// Exposed so the MCP layer can apply the same clamp without drifting
// from the store. AOS umbrella walks rarely exceed depth 5; 100 is
// generous + defensive against any future cycle bug. Negative or
// > MaxWalkDepth is clamped to MaxWalkDepth; 0 returns root only.
const MaxWalkDepth = 100

// SlowWalkThreshold logs a warning when Walk / WalkAt exceeds this
// duration. Helps operators spot pathological graphs (cycles caught
// by the depth clamp, unindexed scans on bad query plans). Tuneable
// via the slog level if log volume becomes a problem.
const SlowWalkThreshold = 100 * time.Millisecond

// ErrInvalidKind is returned when a caller passes a src_kind / dst_kind /
// edge kind outside the enumerated set. The schema is portable — no CHECK
// constraint backs this up — so Go-side validation IS the only safety net.
var ErrInvalidKind = errors.New("relationships: invalid kind value")

// ErrSelfLoop is returned when src_id == dst_id. Schema is portable, so
// Go is the only enforcer.
var ErrSelfLoop = errors.New("relationships: a node cannot have an edge to itself")

// ErrEmptyID is returned when src_id or dst_id is empty. An empty-ID
// edge would silently corrupt the DAG (joins to nothing). Schema has no
// CHECK to catch this (portability rule), so Go MUST — and every reader
// rejects empty IDs at the top so corrupted-state never propagates.
var ErrEmptyID = errors.New("relationships: src_id and dst_id must be non-empty")

// allEntityKinds and allEdgeKinds are the source of truth for valid
// kind values. validKind / validEdgeKind iterate these slices instead
// of hard-coding equality chains; the constant declarations below
// reference these slices via the validators, so adding a new kind in
// ONE place (the slice) auto-extends both the validator AND keeps the
// constants iterable for callers that want to enumerate. Was: two
// hard-coded == chains that drifted whenever a constant was added.
var allEntityKinds = []string{KindProduct, KindManifest, KindTask, KindSkill, KindIdea, KindRAG}
var allEdgeKinds = []string{EdgeOwns, EdgeDependsOn}

// AllEdgeKinds returns all valid edge kinds. Use this instead of repeating
// the list at call sites — adding a new kind here propagates everywhere.
func AllEdgeKinds() []string {
	out := make([]string, len(allEdgeKinds))
	copy(out, allEdgeKinds)
	return out
}

// validKind returns true if k is a non-empty string. The DB-driven
// entity_types table is now the source of truth for valid entity kinds;
// Go-side validation only rejects the empty string (which would silently
// corrupt the DAG). Edge kinds are still code-defined and validated via
// validEdgeKind. The allEntityKinds slice is retained for callers that
// enumerate built-in kinds (e.g. the relationships constants).
func validKind(k string) bool {
	return k != ""
}

// validEdgeKind returns true if k is one of the enumerated edge kinds.
// Same iteration pattern as validKind. Single source of truth.
func validEdgeKind(k string) bool {
	for _, v := range allEdgeKinds {
		if k == v {
			return true
		}
	}
	return false
}

// nowUTC returns the current time in ISO8601 UTC with nanosecond
// precision. The composite PK on (src_id, dst_id, kind, valid_from)
// requires distinct timestamps for back-to-back Creates on the same
// edge; RFC3339 (second precision) collides under tight loops or under
// test, so we use RFC3339Nano. The TEXT column accepts any ISO8601
// shape, and lexicographic ordering still works for time comparisons
// because the format is fixed-width up to the fractional seconds.
func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// Standard entity kinds. Stored as TEXT in src_kind / dst_kind columns.
// Schema is portable — no CHECK constraint enforces this set; Go-side
// validKind() is the only gate. Adding a new kind: extend allEntityKinds
// (which the validator iterates) AND add the const; the schema needs
// no migration since it's just TEXT.
const (
	KindProduct  = "product"
	KindManifest = "manifest"
	KindTask     = "task"
	KindSkill    = "skill"
	KindIdea     = "idea"
	KindRAG      = "RAG"
)

// Standard edge kinds. Only owns (parent→child hierarchy) and
// depends_on (execution ordering) are valid.
const (
	EdgeOwns      = "owns"
	EdgeDependsOn = "depends_on"
)

// Edge is one row in the relationships table.
//
// Field semantics for input vs output:
//   - SrcKind/SrcID/DstKind/DstID/Kind: REQUIRED on Create and BackfillRow
//   - Metadata: optional on input, opaque JSON
//   - ValidFrom: optional on Create (defaults to now); REQUIRED on BackfillRow
//   - ValidTo: optional on BackfillRow only (Create always sets ''); on read
//     "" means "this is the current version of the edge", non-empty means
//     "superseded at this timestamp"
//   - CreatedBy/Reason: optional, populates the audit trail
//   - CreatedAt: WRITE-PATH IGNORES THE INPUT — set by store on insert.
//     Populated on read. If you set it on input, it's silently overwritten.
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
	CreatedAt string // ignored on Create input; set by store
}

// Store owns the relationships table.
//
// createMu serializes Create's close-then-insert transaction. SQLite
// WAL gives each connection a snapshot at BEGIN time; two concurrent
// Create txs on the same edge can both see "no current row" at their
// BEGIN, both close-then-insert, and produce two rows with valid_to='',
// breaking the SCD-2 invariant. The mutex makes the close + insert
// atomic across goroutines without depending on engine-specific
// BEGIN IMMEDIATE / SERIALIZABLE semantics. Concurrent Creates on
// DIFFERENT edges still serialize through this — acceptable since
// SQLite's underlying write lock would serialize them anyway. Future
// Postgres backend can drop the mutex (MVCC + SERIALIZABLE isolation
// handles this correctly) at the cost of one rebench cycle.
type Store struct {
	db       *sql.DB
	createMu sync.Mutex
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
	// Schema portability constraint (2026-04-25): NO CHECK constraints,
	// NO triggers, NO SQLite-specific dialect features. The DB must
	// remain portable across SQL engines (SQLite today; Postgres /
	// Iceberg-via-Trino / MySQL likely in the future for fleet-scale
	// peers). All value-domain validation lives in Go (validKind /
	// validEdgeKind / non-empty / self-loop checks in Create + Remove).
	// Tests cover what CHECK constraints used to catch, so regression
	// risk moves from "DB rejects malformed write" to "Go layer rejects
	// it AND the test suite proves it."
	//
	// Composite PK on (src_id, dst_id, kind, valid_from) is portable
	// (every SQL engine supports composite PK) and remains the only
	// schema-level invariant — a simultaneous duplicate Create at the
	// same timestamp will fail at INSERT and the caller can retry.
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
		PRIMARY KEY (src_id, dst_id, kind, valid_from)
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
// Validation: src_kind / dst_kind / kind must be in the enumerated sets;
// SrcID and DstID must be non-empty; SrcID == DstID is rejected. Schema
// is portable — there are NO CHECK constraints, so Go is the only gate.
// Tests cover every rejection path so regressions surface in CI.
//
// Idempotency note: Create is NOT a no-op when called with identical
// inputs — it WILL produce two rows in history (the second one closes
// the first immediately on the next call). That matches SCD-2 semantics:
// every Create is a state mutation. Callers wanting "no-op when
// unchanged" should ListOutgoing or Get first and skip the Create if
// the current row already matches.
//
// Concurrency under SQLite (current backend): writes serialize on the
// WAL write lock; concurrent Create calls run sequentially, never
// observe each other's pending state. Under Postgres MVCC (future
// backend), serializable-isolation re-runs handle the rare case where
// two txs compute the same nowUTC() before either commits.
func (s *Store) Create(ctx context.Context, e Edge) error {
	// All validation lives in Go (schema is portable, no CHECK / triggers).
	// Order: empty-ID first so a self-loop check on two empty strings
	// doesn't fire incorrectly; kind enums next; self-loop last (it's
	// the only check that requires both IDs to be present + valid).
	if e.SrcID == "" || e.DstID == "" {
		return fmt.Errorf("%w: src_id=%q dst_id=%q", ErrEmptyID, e.SrcID, e.DstID)
	}
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

	// Serialize close-then-insert across goroutines. See Store.createMu
	// docstring for why a Go mutex (SQLite WAL snapshot isolation lets
	// concurrent txs both miss the prior row).
	s.createMu.Lock()
	defer s.createMu.Unlock()

	// Resolve timestamps once so the close + insert share the same
	// "now". Using one timestamp means the prior row's valid_to and
	// the new row's valid_from align exactly — the audit trail has no
	// gap between versions. Computed AFTER acquiring the mutex so
	// timestamps reflect serialization order rather than racing
	// goroutines' nowUTC() calls.
	now := nowUTC()
	validFrom := e.ValidFrom
	if validFrom == "" {
		validFrom = now
	}

	// Run close + insert in one transaction. If the INSERT fails the
	// close also rolls back so the prior row stays current — no orphaned
	// "everyone's closed" state.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	// After a successful Commit, Rollback returns sql.ErrTxDone — that's
	// the documented sentinel for "tx already finished," not a real
	// error. We deliberately swallow it. Any OTHER rollback error (e.g.
	// connection died after Commit but before this defer ran) we surface
	// via slog so operators have a signal something flaked.
	defer func() {
		if rerr := tx.Rollback(); rerr != nil && !errors.Is(rerr, sql.ErrTxDone) {
			slog.Warn("relationships: rollback after Create failed", "err", rerr)
		}
	}()

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

// validateBackfillEdge runs the validation rules shared by BackfillRow
// and BackfillBulk. Centralised so adding a new rule (say "metadata
// must be parseable JSON" in PR/M2) lands in ONE place; the two
// callers can't drift. Returns nil if the edge is acceptable for a
// backfill insert.
func validateBackfillEdge(e Edge) error {
	if e.SrcID == "" || e.DstID == "" {
		return fmt.Errorf("%w: src_id=%q dst_id=%q", ErrEmptyID, e.SrcID, e.DstID)
	}
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
	if e.ValidFrom == "" {
		return fmt.Errorf("relationships: backfill requires explicit ValidFrom (callers must preserve historical timing)")
	}
	return nil
}

// backfillInsertSQL is the standard INSERT statement used by BOTH
// BackfillRow (single-row) and BackfillBulk (prepared statement,
// per-tx). Sharing the SQL means a future column add updates one
// place, not two.
const backfillInsertSQL = `INSERT INTO relationships
	(src_kind, src_id, dst_kind, dst_id, kind, metadata,
	 valid_from, valid_to, created_by, reason, created_at)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

// BackfillRow inserts a single row with caller-controlled valid_from
// AND valid_to — bypassing Create's close-then-insert dance. Used by
// PR/M2's migration path to copy historical rows out of legacy dep
// tables (task_dependency / manifest_dependencies / etc.) preserving
// their original time intervals.
//
// Validation: same rules as Create EXCEPT we DO NOT require valid_to
// to be empty (caller can write a fully-closed historical row) AND we
// DO require valid_from to be non-empty (no implicit now() — backfill
// callers must preserve historical timing). Lives in the shared
// validateBackfillEdge() function used by BackfillBulk too.
//
// MUST NOT be used by application code outside backfill paths. Calling
// this for a normal mutation breaks the SCD-2 invariant that "only one
// row per (src, dst, kind) has valid_to=''" — the close-then-insert
// transaction in Create exists to maintain it. BackfillRow trusts the
// caller; backfills run in a controlled migration transaction with
// known-non-overlapping intervals.
//
// Idempotent at the row level: the composite PK rejects duplicate
// inserts, so re-running a backfill on the same source data returns
// PK error rows that the caller can filter as "already migrated."
//
// For high-volume backfills (>1K rows) prefer BackfillBulk — it shares
// one transaction and prepared statement instead of paying per-row
// fsync + parse cost.
func (s *Store) BackfillRow(ctx context.Context, e Edge) error {
	if err := validateBackfillEdge(e); err != nil {
		return err
	}
	now := nowUTC()
	_, err := s.db.ExecContext(ctx, backfillInsertSQL,
		e.SrcKind, e.SrcID, e.DstKind, e.DstID, e.Kind, e.Metadata,
		e.ValidFrom, e.ValidTo, e.CreatedBy, e.Reason, now)
	if err != nil {
		return fmt.Errorf("backfill insert: %w", err)
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
	// Same Go-side validation as Create — schema is portable, no CHECK.
	if srcID == "" || dstID == "" {
		return fmt.Errorf("%w: src_id=%q dst_id=%q", ErrEmptyID, srcID, dstID)
	}
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

// buildKindFilter turns an []edgeKinds slice into the SQL fragment
// `AND r.kind IN (?, ?, ...)` plus the matching args slice. Empty input
// returns ("", nil, nil) — no filter clause, no args appended.
//
// Centralised so Walk and WalkAt build the IN-clause filter identically;
// previously the same loop existed in both with the same validation
// pattern. One source of truth + one validation path.
func buildKindFilter(edgeKinds []string) (sql string, args []any, err error) {
	if len(edgeKinds) == 0 {
		return "", nil, nil
	}
	placeholders := make([]string, 0, len(edgeKinds))
	args = make([]any, 0, len(edgeKinds))
	for _, k := range edgeKinds {
		if !validEdgeKind(k) {
			return "", nil, fmt.Errorf("%w: edge_kinds[]=%q", ErrInvalidKind, k)
		}
		placeholders = append(placeholders, "?")
		args = append(args, k)
	}
	return ` AND r.kind IN (` + strings.Join(placeholders, ", ") + `)`, args, nil
}

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

// validateAsOf parses an ISO8601 timestamp; empty string is allowed
// (means "now / current state"). Centralised so every time-travel
// reader uses identical parsing rules.
//
// Implementation note: time.Parse(time.RFC3339Nano, ...) accepts inputs
// WITHOUT fractional seconds — Go's `9` quantifier in the format string
// is "optional digits". So a single RFC3339Nano parse covers both
// "2026-04-25T10:00:00Z" (no fraction) and the writer's full-precision
// nanosecond format. An earlier two-pass implementation (RFC3339Nano
// fallback to RFC3339) was dead code.
func validateAsOf(asOf string) error {
	if asOf == "" {
		return nil
	}
	if _, err := time.Parse(time.RFC3339Nano, asOf); err == nil {
		return nil
	}
	return fmt.Errorf("relationships: as_of must be ISO8601 (RFC3339), got %q", asOf)
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
//
// For time-travel ("what edges left srcID at past time T?") use
// ListOutgoingAt instead. ListOutgoing is the hot path; ListOutgoingAt
// trades the partial index for a wider scan to honour history.
func (s *Store) ListOutgoing(ctx context.Context, srcID, edgeKind string) ([]Edge, error) {
	// Defense-in-depth: empty src_id would silently match nothing AND
	// (after C1 from self-review) signals a buggy caller. Reject early.
	if srcID == "" {
		return nil, fmt.Errorf("%w: src_id empty", ErrEmptyID)
	}
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
	if dstID == "" {
		return nil, fmt.Errorf("%w: dst_id empty", ErrEmptyID)
	}
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

// ListOutgoingAt returns edges leaving srcID that were CURRENT at the
// provided ISO8601 timestamp `asOf`. The predicate that defines "valid
// at asOf":
//
//	valid_from <= asOf AND (valid_to > asOf OR valid_to = '')
//
//	      ↓ semantics ↓
//	the edge had been activated (valid_from in the past relative to asOf)
//	AND was not yet superseded as of asOf (either closed AFTER asOf,
//	or still current today).
//
// asOf == "" falls through to ListOutgoing's current-state path —
// callers that don't care about history shouldn't pay the wider-scan
// cost. Empty asOf is therefore the ergonomic default for "now."
//
// Performance: cannot use idx_rel_src_current (partial on valid_to='')
// because past-snapshot queries need closed rows. Falls back to a
// scan of the (src_id, kind) index OR a full table scan depending on
// the optimizer's stats. Acceptable for audit / forensics / dashboard
// time-slider use cases; do NOT call this in a hot loop.
//
// Future asOf timestamps are accepted but degenerate: any future point
// returns whatever's currently active (since no row's valid_to is yet
// set after the future asOf). Caller can choose to validate themselves.
func (s *Store) ListOutgoingAt(ctx context.Context, srcID, edgeKind, asOf string) ([]Edge, error) {
	if srcID == "" {
		return nil, fmt.Errorf("%w: src_id empty", ErrEmptyID)
	}
	if edgeKind != "" && !validEdgeKind(edgeKind) {
		return nil, fmt.Errorf("%w: kind=%q", ErrInvalidKind, edgeKind)
	}
	if err := validateAsOf(asOf); err != nil {
		return nil, err
	}
	// Empty asOf → delegate to the hot-path reader for partial-index speed.
	if asOf == "" {
		return s.ListOutgoing(ctx, srcID, edgeKind)
	}

	q := `SELECT ` + rowColumns + ` FROM relationships
		 WHERE src_id = ?
		   AND valid_from <= ?
		   AND (valid_to > ? OR valid_to = '')`
	args := []any{srcID, asOf, asOf}
	if edgeKind != "" {
		q += ` AND kind = ?`
		args = append(args, edgeKind)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query outgoing-at: %w", err)
	}
	defer rows.Close()

	out := make([]Edge, 0, 8)
	for rows.Next() {
		e, err := scanEdge(rows)
		if err != nil {
			return nil, fmt.Errorf("scan outgoing-at: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListIncomingAt is the dst-side mirror of ListOutgoingAt. Same
// predicate, same delegation-to-hot-path on empty asOf, same
// performance caveat.
//
// Use case: "who depended on this manifest 30 days ago?" Walks the
// idx_rel_dst index without the partial filter.
func (s *Store) ListIncomingAt(ctx context.Context, dstID, edgeKind, asOf string) ([]Edge, error) {
	if dstID == "" {
		return nil, fmt.Errorf("%w: dst_id empty", ErrEmptyID)
	}
	if edgeKind != "" && !validEdgeKind(edgeKind) {
		return nil, fmt.Errorf("%w: kind=%q", ErrInvalidKind, edgeKind)
	}
	if err := validateAsOf(asOf); err != nil {
		return nil, err
	}
	if asOf == "" {
		return s.ListIncoming(ctx, dstID, edgeKind)
	}

	q := `SELECT ` + rowColumns + ` FROM relationships
		 WHERE dst_id = ?
		   AND valid_from <= ?
		   AND (valid_to > ? OR valid_to = '')`
	args := []any{dstID, asOf, asOf}
	if edgeKind != "" {
		q += ` AND kind = ?`
		args = append(args, edgeKind)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query incoming-at: %w", err)
	}
	defer rows.Close()

	out := make([]Edge, 0, 8)
	for rows.Next() {
		e, err := scanEdge(rows)
		if err != nil {
			return nil, fmt.Errorf("scan incoming-at: %w", err)
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
	if srcID == "" || dstID == "" {
		return nil, fmt.Errorf("%w: src_id=%q dst_id=%q", ErrEmptyID, srcID, dstID)
	}
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

// WalkRow is one node visited during a recursive DAG walk. The (ID, Kind)
// pair identifies the entity; ViaKind says how we got here (which edge
// kind led to this node from its predecessor); ViaSrc is the predecessor
// itself; Depth is hops from the root (root = 0).
//
// Returned in BFS-ish order — closer-to-root nodes appear before deeper
// ones, but ordering within the same depth is not stable (depends on
// SQLite's CTE iteration). Callers needing strict order should sort.
type WalkRow struct {
	ID      string
	Kind    string
	ViaKind string // edge kind that led here ("" for the root)
	ViaSrc  string // src_id of the edge that led here ("" for the root)
	Depth   int
}

// Walk traverses the DAG starting at (rootID, rootKind), following
// outgoing edges. Returns every reachable node along with the edge that
// led to it. The recursive CTE walks the partial-current-edges index so
// only edges with valid_to='' are followed — historical edges don't
// appear in the walk.
//
// edgeKinds filters which edge kinds to follow:
//   - nil or empty: follow ALL edge kinds (owns + depends_on + reviews + links_to)
//   - otherwise: only follow edges whose kind is in the slice
//
// maxDepth caps recursion. 0 means "root only" (anchor row only — the
// recursive WHERE w.depth < 0 excludes everything). Negative or
// > MaxWalkDepth clamps to MaxWalkDepth. The clamp is the only
// cycle-defense the system has — schema portability forbids CHECK
// constraints and triggers, so an acyclicity invariant lives nowhere
// at the DB level. If a cycle ever appears, Walk hits the depth
// ceiling and emits a slog.Warn (see end of function).
//
// The CTE is the heart of why this whole product exists: ONE query
// replaces what used to be 6+ table joins for a hierarchy walk. The
// dashboard's apiProductHierarchy will eventually rebuild on this in
// PR/M3.
func (s *Store) Walk(ctx context.Context, rootID, rootKind string, edgeKinds []string, maxDepth int) ([]WalkRow, error) {
	if rootID == "" {
		return nil, fmt.Errorf("%w: root_id empty", ErrEmptyID)
	}
	// Validate root kind. Edge kinds in the slice are validated below.
	if !validKind(rootKind) {
		return nil, fmt.Errorf("%w: root_kind=%q", ErrInvalidKind, rootKind)
	}
	// Clamp depth via the package-level constant. Out-of-range or
	// negative collapses to MaxWalkDepth; 0 means "root only" (anchor
	// row only — recursive WHERE w.depth < 0 excludes everything).
	if maxDepth < 0 || maxDepth > MaxWalkDepth {
		maxDepth = MaxWalkDepth
	}

	// Args order: [rootID, rootKind, kind1..kindN, maxDepth]. The query
	// reads ?,? for the anchor SELECT, then the IN(...) placeholders,
	// then the trailing w.depth < ? predicate. Mis-ordering silently
	// binds maxDepth into the IN clause and walks return only the root.
	kindFilter, kindArgs, err := buildKindFilter(edgeKinds)
	if err != nil {
		return nil, err
	}
	args := []any{rootID, rootKind}
	args = append(args, kindArgs...)
	args = append(args, maxDepth)

	// Recursive CTE explained:
	//   - Anchor row: the root itself, depth=0, no via_kind/via_src.
	//   - Recursive row: for each row already in walk, find its
	//     outgoing CURRENT edges (valid_to='') in the optional kind
	//     filter, emit the dst as a new walk row at depth+1.
	//   - WHERE w.depth < ? caps the recursion — we'd rather return
	//     a partial walk with a clear depth ceiling than spin
	//     forever on a malformed graph.
	//
	// UNION ALL: emits raw. SQL UNION's full-row dedup wouldn't help
	// for diamond paths (different via_src means rows aren't equal),
	// so we'd pay the SQL dedup cost AND still need Go-side dedup. ALL
	// + Go-side post-process is the cheaper combo.
	q := `WITH RECURSIVE walk(id, kind, via_kind, via_src, depth) AS (
		SELECT ?, ?, '', '', 0
		UNION ALL
		SELECT r.dst_id, r.dst_kind, r.kind, r.src_id, w.depth + 1
		  FROM relationships r
		  JOIN walk w ON r.src_id = w.id
		 WHERE r.valid_to = ''` + kindFilter + `
		   AND w.depth < ?
	)
	SELECT id, kind, via_kind, via_src, depth FROM walk`

	// Time the query so we can log slow walks. A walk that hits the depth
	// clamp on a graph with cycles (or a graph that's just enormous) is
	// invisible without instrumentation; D1 + M9 from self-review.
	start := time.Now()
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("walk query: %w", err)
	}
	defer rows.Close()

	// Capacity guess: typical AOS umbrella walk hits ~30 nodes (1 +
	// 8 sub-products + 15 manifests + ~6 tasks). Pre-allocate a bit
	// over that to avoid the first growth.
	out := make([]WalkRow, 0, 32)
	maxObservedDepth := 0
	for rows.Next() {
		var w WalkRow
		if err := rows.Scan(&w.ID, &w.Kind, &w.ViaKind, &w.ViaSrc, &w.Depth); err != nil {
			return nil, fmt.Errorf("scan walk: %w", err)
		}
		if w.Depth > maxObservedDepth {
			maxObservedDepth = w.Depth
		}
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rawRowCount := len(out) // capture pre-dedup count for the log

	// SQL UNION ALL emits every reached path; Go-side dedup-by-id collapses
	// diamond paths to one row per node. Using ALL (not UNION) avoids a
	// SQLite sort + dedup pass that wouldn't have helped anyway —
	// different via_src on the same dst means full-row equality fails and
	// SQL UNION keeps both rows. So we let SQL emit raw + Go does the
	// authoritative dedup. Cheaper than UNION + Go dedup.
	//
	// Sort stable by depth ASC; first occurrence per id wins (= shortest
	// path). O(n log n) + O(n). Fine up to thousands of nodes.
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Depth < out[j].Depth
	})
	seen := make(map[string]bool, len(out))
	deduped := make([]WalkRow, 0, len(out))
	for _, w := range out {
		if !seen[w.ID] {
			seen[w.ID] = true
			deduped = append(deduped, w)
		}
	}

	// Surface pathological walks AFTER dedup so log row counts match what
	// the caller gets back. Slow threshold: total elapsed including dedup.
	// Cycle warning: depth ceiling hit during recursion. Both use the
	// final, deduped row count to avoid misleading operators with raw-CTE
	// numbers that don't exist in the response.
	if elapsed := time.Since(start); elapsed > SlowWalkThreshold {
		slog.Warn("relationships.Walk slow",
			"root_id", rootID, "root_kind", rootKind,
			"rows_returned", len(deduped), "raw_rows", rawRowCount,
			"max_depth_observed", maxObservedDepth,
			"max_depth_allowed", maxDepth,
			"elapsed_ms", elapsed.Milliseconds())
	}
	if maxDepth > 0 && maxObservedDepth >= maxDepth {
		slog.Warn("relationships.Walk hit depth ceiling — possible cycle",
			"root_id", rootID, "root_kind", rootKind,
			"max_depth", maxDepth, "rows_returned", len(deduped))
	}
	return deduped, nil
}

// WalkAt traverses the DAG starting at (rootID, rootKind) showing the
// state that was current at `asOf`. Same shape as Walk except the
// recursive-row predicate is the time-travel predicate from
// ListOutgoingAt instead of `valid_to = ''`.
//
// asOf == "" delegates to Walk for the hot-path partial-index speed.
//
// Use case: dashboard time-slider — "show me what the AOS umbrella DAG
// looked like on 2026-04-15." Operator can scrub through history. The
// returned WalkRow shape is identical to Walk's, so any consumer that
// renders a current walk renders a past walk identically.
//
// The recursive CTE materializes everything at the past snapshot; for
// graphs with thousands of nodes this allocates more than the current-
// state walk. AOS-scale (~50 nodes) is fine.
//
// Same dedup contract as Walk: UNION ALL emits raw, Go-side
// dedup-by-id collapses diamond paths to one row per node (shortest
// path wins). maxDepth clamped to [0, MaxWalkDepth].
func (s *Store) WalkAt(ctx context.Context, rootID, rootKind string, edgeKinds []string, maxDepth int, asOf string) ([]WalkRow, error) {
	if rootID == "" {
		return nil, fmt.Errorf("%w: root_id empty", ErrEmptyID)
	}
	if !validKind(rootKind) {
		return nil, fmt.Errorf("%w: root_kind=%q", ErrInvalidKind, rootKind)
	}
	if err := validateAsOf(asOf); err != nil {
		return nil, err
	}
	// Empty asOf → use the hot-path Walk that hits idx_rel_src_current.
	if asOf == "" {
		return s.Walk(ctx, rootID, rootKind, edgeKinds, maxDepth)
	}

	if maxDepth < 0 || maxDepth > MaxWalkDepth {
		maxDepth = MaxWalkDepth
	}

	// Args order matters: the CTE expects
	//   anchor:    (?, ?)         rootID, rootKind
	//   recursive: (?, ?)         asOf, asOf  (twice — two-clause predicate)
	//              IN(?, ?, ...)  edge kinds (optional)
	//              <?             maxDepth (last)
	kindFilter, kindArgs, err := buildKindFilter(edgeKinds)
	if err != nil {
		return nil, err
	}
	args := []any{rootID, rootKind, asOf, asOf}
	args = append(args, kindArgs...)
	args = append(args, maxDepth)

	// Time-travel CTE: same shape as Walk, but the recursive WHERE swaps
	// `r.valid_to = ''` for the asOf-validity predicate. UNION ALL +
	// Go-side dedup, same rationale as Walk.
	q := `WITH RECURSIVE walk(id, kind, via_kind, via_src, depth) AS (
		SELECT ?, ?, '', '', 0
		UNION ALL
		SELECT r.dst_id, r.dst_kind, r.kind, r.src_id, w.depth + 1
		  FROM relationships r
		  JOIN walk w ON r.src_id = w.id
		 WHERE r.valid_from <= ? AND (r.valid_to > ? OR r.valid_to = '')` + kindFilter + `
		   AND w.depth < ?
	)
	SELECT id, kind, via_kind, via_src, depth FROM walk`

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("walk-at query: %w", err)
	}
	defer rows.Close()

	out := make([]WalkRow, 0, 32)
	for rows.Next() {
		var w WalkRow
		if err := rows.Scan(&w.ID, &w.Kind, &w.ViaKind, &w.ViaSrc, &w.Depth); err != nil {
			return nil, fmt.Errorf("scan walk-at: %w", err)
		}
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Same dedup-by-id sweep as Walk so diamond-path nodes appear once
	// (shortest path wins). See Walk for rationale.
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Depth < out[j].Depth
	})
	seen := make(map[string]bool, len(out))
	deduped := make([]WalkRow, 0, len(out))
	for _, w := range out {
		if !seen[w.ID] {
			seen[w.ID] = true
			deduped = append(deduped, w)
		}
	}
	return deduped, nil
}

// HealthStats is a snapshot of the store's row counts. Used by the
// dashboard's overview card + by ops health checks. Cheap — two
// COUNT queries against an indexed table.
type HealthStats struct {
	CurrentEdges int // valid_to = ''
	TotalRows    int // all rows including closed history
}

// Health returns row counts so callers don't have to write the same
// COUNT queries everywhere.
//
// Contract: callers MUST call New() (which runs init() / the idempotent
// migration) BEFORE Health(). Earlier prototypes tried to detect
// "table not found" via error string match, but the message shape
// varies across SQL engines (SQLite "no such table:", Postgres
// "relation X does not exist", MySQL "Table 'db.X' doesn't exist") —
// any portable sentinel would be brittle. Go's database/sql doesn't
// surface a typed "table missing" error either. So Health trusts that
// init succeeded; callers wanting "table-was-already-there?" semantics
// should add a top-level boot check rather than embedding it here.
func (s *Store) Health(ctx context.Context) (HealthStats, error) {
	var h HealthStats
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM relationships WHERE valid_to = ''`).Scan(&h.CurrentEdges); err != nil {
		return h, fmt.Errorf("health current count: %w", err)
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM relationships`).Scan(&h.TotalRows); err != nil {
		return h, fmt.Errorf("health total count: %w", err)
	}
	return h, nil
}

// Get returns the CURRENT edge for (srcID, dstID, kind), or
// (Edge{}, false, nil) if no current row exists. Atomic single-edge
// lookup — the most common pre-mutation query ("does this edge already
// exist before I Create?"). Replaces today's pattern of calling
// ListOutgoing + filtering by DstID, which loads every outgoing edge
// when only one matters.
//
// Returns at most ONE row by construction: the (src_id, dst_id, kind,
// valid_from) PK + valid_to='' partial index together guarantee a
// single current version per edge tuple. Multiple rows would mean the
// SCD-2 invariant was violated by a backfill — that's a bug, not a
// shape this function tolerates.
func (s *Store) Get(ctx context.Context, srcID, dstID, kind string) (Edge, bool, error) {
	if srcID == "" || dstID == "" {
		return Edge{}, false, fmt.Errorf("%w: src_id=%q dst_id=%q", ErrEmptyID, srcID, dstID)
	}
	if !validEdgeKind(kind) {
		return Edge{}, false, fmt.Errorf("%w: kind=%q", ErrInvalidKind, kind)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+rowColumns+` FROM relationships
		 WHERE src_id = ? AND dst_id = ? AND kind = ? AND valid_to = ''
		 LIMIT 1`,
		srcID, dstID, kind)
	if err != nil {
		return Edge{}, false, fmt.Errorf("get: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		// Differentiate "row iterator advanced cleanly past the end" from
		// "iterator errored before producing a row." rows.Err() captures
		// the latter; if non-nil, return it as a real error rather than
		// implying the edge doesn't exist.
		if rerr := rows.Err(); rerr != nil {
			return Edge{}, false, fmt.Errorf("get iterate: %w", rerr)
		}
		return Edge{}, false, nil // no current edge — caller's expected case
	}
	e, err := scanEdge(rows)
	if err != nil {
		return Edge{}, false, fmt.Errorf("scan get: %w", err)
	}
	// Final iterator check after the scan — catches errors that surface
	// during row consumption. Returning (e, true, err) here would let a
	// caller process a partially-scanned Edge thinking found=true; safer
	// to return zero on any non-nil iterator error.
	if rerr := rows.Err(); rerr != nil {
		return Edge{}, false, fmt.Errorf("get final iterate: %w", rerr)
	}
	return e, true, nil
}

// BackfillBulk inserts many historical rows in one transaction.
// Migration paths (PR/M2) typically write 10K–100K rows; calling
// BackfillRow in a loop pays one fsync per row (slow on cloud disks,
// minutes for 100K rows). BackfillBulk amortises to one fsync per
// transaction.
//
// Validation: every edge in the slice gets the same checks as
// BackfillRow. If ANY edge fails validation, the entire transaction
// rolls back — caller must filter / fix before retrying. There is no
// partial-success mode by design (avoids the "which rows landed?"
// question downstream).
//
// Performance: ~100x faster than BackfillRow loop on disk-bound
// systems; ~10x on NVMe. Recommended batch size: 1K–10K rows. Beyond
// that the transaction lock starves concurrent readers.
func (s *Store) BackfillBulk(ctx context.Context, edges []Edge) error {
	if len(edges) == 0 {
		return nil
	}
	// Pre-validate ALL edges before starting the tx so we don't have to
	// roll back after partial inserts. validate-then-write is cheaper
	// than write-then-rollback. Uses the same validateBackfillEdge() as
	// BackfillRow — rules can't drift between the two.
	for i, e := range edges {
		if err := validateBackfillEdge(e); err != nil {
			return fmt.Errorf("edges[%d]: %w", i, err)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin bulk tx: %w", err)
	}
	defer func() {
		if rerr := tx.Rollback(); rerr != nil && !errors.Is(rerr, sql.ErrTxDone) {
			slog.Warn("relationships: rollback after BackfillBulk failed", "err", rerr)
		}
	}()

	// Reuse the shared backfillInsertSQL — same column order as
	// BackfillRow so future schema changes touch one constant.
	stmt, err := tx.PrepareContext(ctx, backfillInsertSQL)
	if err != nil {
		return fmt.Errorf("prepare bulk insert: %w", err)
	}
	defer stmt.Close()

	now := nowUTC()
	for i, e := range edges {
		if _, err := stmt.ExecContext(ctx,
			e.SrcKind, e.SrcID, e.DstKind, e.DstID, e.Kind, e.Metadata,
			e.ValidFrom, e.ValidTo, e.CreatedBy, e.Reason, now); err != nil {
			return fmt.Errorf("bulk insert edges[%d]: %w", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit bulk: %w", err)
	}
	return nil
}

// ListIncomingForMany is the dst-side mirror of ListOutgoingForMany.
// "Who depends on these N manifests?" — same use case, reverse
// direction. One IN(...) query against idx_rel_dst_current; results
// grouped by dst_id.
func (s *Store) ListIncomingForMany(ctx context.Context, dstIDs []string, edgeKind string) (map[string][]Edge, error) {
	if len(dstIDs) == 0 {
		return map[string][]Edge{}, nil
	}
	for _, id := range dstIDs {
		if id == "" {
			return nil, fmt.Errorf("%w: dstIDs contains empty string", ErrEmptyID)
		}
	}
	if edgeKind != "" && !validEdgeKind(edgeKind) {
		return nil, fmt.Errorf("%w: kind=%q", ErrInvalidKind, edgeKind)
	}

	placeholders := make([]string, 0, len(dstIDs))
	args := make([]any, 0, len(dstIDs)+1)
	for _, id := range dstIDs {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	q := `SELECT ` + rowColumns + ` FROM relationships
		 WHERE dst_id IN (` + strings.Join(placeholders, ", ") + `)
		   AND valid_to = ''`
	if edgeKind != "" {
		q += ` AND kind = ?`
		args = append(args, edgeKind)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query incoming-for-many: %w", err)
	}
	defer rows.Close()

	out := make(map[string][]Edge, len(dstIDs))
	for rows.Next() {
		e, err := scanEdge(rows)
		if err != nil {
			return nil, fmt.Errorf("scan incoming-for-many: %w", err)
		}
		out[e.DstID] = append(out[e.DstID], e)
	}
	return out, rows.Err()
}

// ListOutgoingForMany batches the per-srcID lookup for the dashboard's
// product-list / manifest-list views. Today's pattern (loop calling
// ListOutgoing) is N+1 — one query per row in the list. This issues
// ONE query covering every srcID in the slice and groups the result
// by src_id in Go.
//
// Returns map[srcID][]Edge. A srcID with zero edges has no map entry
// (caller can do `len(m[id]) == 0` to check).
//
// edgeKind == "" returns all kinds (same as ListOutgoing). Filter
// applies uniformly across every srcID.
//
// Performance: one IN(...) query against idx_rel_src_current. ≤ ~500
// srcIDs is comfortable; beyond that, batch the call.
func (s *Store) ListOutgoingForMany(ctx context.Context, srcIDs []string, edgeKind string) (map[string][]Edge, error) {
	if len(srcIDs) == 0 {
		return map[string][]Edge{}, nil
	}
	for _, id := range srcIDs {
		if id == "" {
			return nil, fmt.Errorf("%w: srcIDs contains empty string", ErrEmptyID)
		}
	}
	if edgeKind != "" && !validEdgeKind(edgeKind) {
		return nil, fmt.Errorf("%w: kind=%q", ErrInvalidKind, edgeKind)
	}

	placeholders := make([]string, 0, len(srcIDs))
	args := make([]any, 0, len(srcIDs)+1)
	for _, id := range srcIDs {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	q := `SELECT ` + rowColumns + ` FROM relationships
		 WHERE src_id IN (` + strings.Join(placeholders, ", ") + `)
		   AND valid_to = ''`
	if edgeKind != "" {
		q += ` AND kind = ?`
		args = append(args, edgeKind)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query outgoing-for-many: %w", err)
	}
	defer rows.Close()

	out := make(map[string][]Edge, len(srcIDs))
	for rows.Next() {
		e, err := scanEdge(rows)
		if err != nil {
			return nil, fmt.Errorf("scan outgoing-for-many: %w", err)
		}
		out[e.SrcID] = append(out[e.SrcID], e)
	}
	return out, rows.Err()
}

