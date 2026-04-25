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
	"database/sql"
	"fmt"
)

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
