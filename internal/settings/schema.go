package settings

import (
	"database/sql"
	"fmt"
)

// InitSchema creates the settings table and its index on the given DB.
// Safe to call repeatedly; uses IF NOT EXISTS semantics.
//
// The caller is responsible for opening the DB with WAL mode and
// busy_timeout=5000 (see visceral rule #10 — SQLite must always use
// WAL + busy_timeout=5000 on every db.Open).
func InitSchema(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS settings (
		scope_type TEXT NOT NULL CHECK (scope_type IN ('system','product','manifest','task')),
		scope_id   TEXT NOT NULL,
		key        TEXT NOT NULL,
		value      TEXT NOT NULL,
		updated_at INTEGER NOT NULL,
		updated_by TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (scope_type, scope_id, key)
	) WITHOUT ROWID`)
	if err != nil {
		return fmt.Errorf("create settings table: %w", err)
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_settings_scope ON settings(scope_type, scope_id)`)
	if err != nil {
		return fmt.Errorf("create settings scope index: %w", err)
	}

	return nil
}
