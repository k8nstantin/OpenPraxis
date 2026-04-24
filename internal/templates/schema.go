package templates

import (
	"database/sql"
	"fmt"
)

// InitSchema creates the prompt_templates table plus its two indexes.
// SCD-Type-2 layout: template_uid is the stable logical id; each mutation
// inserts a new row with valid_from=now() and closes the previous row by
// stamping its valid_to. The active set is the subset where valid_to=''.
//
// Caller is responsible for opening the DB with WAL + busy_timeout=5000
// (visceral rule #10).
func InitSchema(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS prompt_templates (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		template_uid TEXT NOT NULL,
		title        TEXT NOT NULL,
		scope        TEXT NOT NULL,
		scope_id     TEXT NOT NULL DEFAULT '',
		section      TEXT NOT NULL,
		body         TEXT NOT NULL DEFAULT '',
		status       TEXT NOT NULL DEFAULT 'open',
		tags         TEXT NOT NULL DEFAULT '[]',
		source_node  TEXT NOT NULL DEFAULT '',
		valid_from   TEXT NOT NULL,
		valid_to     TEXT NOT NULL DEFAULT '',
		changed_by   TEXT NOT NULL DEFAULT '',
		reason       TEXT NOT NULL DEFAULT '',
		created_at   TEXT NOT NULL,
		deleted_at   TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		return fmt.Errorf("create prompt_templates table: %w", err)
	}

	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_prompt_templates_current
		ON prompt_templates(scope, scope_id, section)
		WHERE valid_to = ''`); err != nil {
		return fmt.Errorf("create current index: %w", err)
	}

	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_prompt_templates_history
		ON prompt_templates(template_uid, valid_from DESC)`); err != nil {
		return fmt.Errorf("create history index: %w", err)
	}

	// Partial UNIQUE index: at most one active row per logical template.
	// Closes the read-modify-write window on SCD-2 update where two concurrent
	// PUTs against the same uid could both leave valid_to='' active rows.
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_prompt_templates_one_active
		ON prompt_templates(template_uid)
		WHERE valid_to = ''`); err != nil {
		return fmt.Errorf("create one-active unique index: %w", err)
	}

	return nil
}
