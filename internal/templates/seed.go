package templates

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Seed inserts the seven system-scope default rows on first call.
// Idempotent: detects prior seeding by counting existing system-scope
// rows and no-ops when any are present. Acceptance #1 in the manifest
// requires SELECT COUNT(*) FROM prompt_templates == 7 on a fresh DB,
// so we do NOT insert a separate marker row.
//
// peerID is the source_node stamp for audit; changed_by is set to
// "system-seed" per the manifest.
func Seed(ctx context.Context, store *Store, peerID string) error {
	if store == nil || store.DB() == nil {
		return fmt.Errorf("templates.Seed: nil store")
	}

	var count int
	if err := store.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM prompt_templates WHERE scope = 'system'`,
	).Scan(&count); err != nil {
		return fmt.Errorf("templates.Seed check: %w", err)
	}
	if count > 0 {
		return nil
	}

	defaults := SystemDefaults()
	titles := SystemDefaultTitles()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	tx, err := store.DB().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("templates.Seed begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	insert := `INSERT INTO prompt_templates
		(template_uid, title, scope, scope_id, section, body, status, tags,
		 source_node, valid_from, valid_to, changed_by, reason, created_at, deleted_at)
		VALUES (?, ?, 'system', '', ?, ?, 'open', '[]', ?, ?, '', 'system-seed',
		        'Initial seed from buildPrompt() defaults', ?, '')`

	for _, section := range Sections {
		body := defaults[section]
		title := titles[section]
		if title == "" {
			title = section
		}
		uid, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("templates.Seed uuid: %w", err)
		}
		if _, err := tx.ExecContext(ctx, insert,
			uid.String(), title, section, body, peerID, now, now,
		); err != nil {
			return fmt.Errorf("templates.Seed insert %s: %w", section, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("templates.Seed commit: %w", err)
	}
	return nil
}
