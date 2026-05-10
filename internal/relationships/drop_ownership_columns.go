package relationships

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
)

// DropOwnershipColumns is the M3 schema migration that removes the
// legacy ownership FKs `manifests.project_id` and `tasks.manifest_id`
// from the schema. After M3 the canonical source of truth for ownership
// is the relationships SCD-2 table — every read path consults
// `EdgeOwns(product → manifest)` and `EdgeOwns(manifest → task)` rows.
//
// Run AFTER MigrateLegacyDeps so any rows whose ownership lived only in
// the legacy column have been swept into relationships first. Idempotent:
// if the columns are already gone (post-M3 fresh DBs or a prior boot
// already dropped them) the function is a no-op.
//
// The migration uses the SQLite-portable rename-table pattern instead of
// ALTER TABLE DROP COLUMN: CREATE TABLE *_new without the column, INSERT
// FROM old, DROP old, RENAME new → original. This works on every SQLite
// version we ship against (3.30+) including ones predating the native
// DROP COLUMN support added in 3.35.
//
// Wraps the whole sequence in a single transaction so a failure leaves
// the legacy schema intact rather than half-converted. Indexes that
// existed on the old table are recreated on the renamed one.
func DropOwnershipColumns(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("relationships: nil db handle")
	}
	if err := dropManifestProjectIDColumn(ctx, db); err != nil {
		return fmt.Errorf("drop manifests.project_id: %w", err)
	}
	if err := dropTaskManifestIDColumn(ctx, db); err != nil {
		return fmt.Errorf("drop tasks.manifest_id: %w", err)
	}
	return nil
}

// hasColumn reports whether `table` currently has a column named `col`.
// Uses PRAGMA table_info (SQLite-only). Returns (false, nil) when the
// table doesn't exist — both the "no project_id column" and "no
// manifests table" cases collapse to the same "nothing to drop"
// outcome, which is what the caller wants.
func hasColumn(ctx context.Context, db *sql.DB, table, col string) (bool, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid          int
			name, ctype  string
			notnull, pk  int
			defaultVal   sql.NullString
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &defaultVal, &pk); err != nil {
			return false, err
		}
		if strings.EqualFold(name, col) {
			return true, nil
		}
	}
	return false, rows.Err()
}

func dropManifestProjectIDColumn(ctx context.Context, db *sql.DB) error {
	has, err := hasColumn(ctx, db, "manifests", "project_id")
	if err != nil {
		return err
	}
	if !has {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// Schema mirrors manifest.Store.init() current shape minus project_id.
	// The init() ALTER TABLE chain adds source_node, deleted_at, depends_on
	// on legacy DBs; we declare the union here so post-migration the table
	// has exactly the columns the rest of the code reads.
	if _, err := tx.ExecContext(ctx, `CREATE TABLE manifests_new (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		content TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'draft',
		jira_refs TEXT NOT NULL DEFAULT '[]',
		tags TEXT NOT NULL DEFAULT '[]',
		author TEXT NOT NULL DEFAULT '',
		source_node TEXT NOT NULL DEFAULT '',
		depends_on TEXT NOT NULL DEFAULT '',
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		deleted_at TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		return fmt.Errorf("create manifests_new: %w", err)
	}

	// Pull every column except project_id. COALESCE'd defaults match the
	// init()-time ALTER ADD COLUMN defaults so legacy rows that never had
	// these columns landed empty rather than NULL.
	if _, err := tx.ExecContext(ctx, `INSERT INTO manifests_new
		(id, title, description, content, status, jira_refs, tags, author,
		 source_node, depends_on, version, created_at, updated_at, deleted_at)
		SELECT id, title, description, content, status, jira_refs, tags, author,
		       COALESCE(source_node, ''), COALESCE(depends_on, ''),
		       COALESCE(version, 1), created_at, updated_at,
		       COALESCE(deleted_at, '')
		  FROM manifests`); err != nil {
		return fmt.Errorf("copy rows to manifests_new: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DROP TABLE manifests`); err != nil {
		return fmt.Errorf("drop legacy manifests: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE manifests_new RENAME TO manifests`); err != nil {
		return fmt.Errorf("rename manifests_new: %w", err)
	}

	// Recreate the indexes the original table had. idx_manifests_project
	// is intentionally NOT recreated — it pointed at the column we just
	// dropped.
	if _, err := tx.ExecContext(ctx,
		`CREATE INDEX IF NOT EXISTS idx_manifests_status ON manifests(status)`); err != nil {
		return fmt.Errorf("recreate idx_manifests_status: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`CREATE INDEX IF NOT EXISTS idx_manifests_updated ON manifests(updated_at DESC)`); err != nil {
		return fmt.Errorf("recreate idx_manifests_updated: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	committed = true
	slog.Info("relationships: dropped legacy manifests.project_id column")
	return nil
}

func dropTaskManifestIDColumn(ctx context.Context, db *sql.DB) error {
	// The tasks table has been retired entirely. This migration is a no-op —
	// there is no tasks table to inspect or modify.
	return nil
}
