package templates

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Seed inserts the seven system-scope default rows and the per-agent
// markdown-heading overrides (codex, cursor) on first call.
//
// Idempotency is gated per (scope, scope_id) bucket: a bucket is seeded
// only if it currently holds zero active rows. That lets Seed() be safely
// re-run after either the system tier or any one agent tier has already
// been populated, without duplicating rows.
//
// peerID is the source_node stamp for audit; changed_by is set to
// "system-seed" per the manifest.
func Seed(ctx context.Context, store *Store, peerID string) error {
	if store == nil || store.DB() == nil {
		return fmt.Errorf("templates.Seed: nil store")
	}

	if err := seedBucket(ctx, store, ScopeSystem, "", SystemDefaults(), SystemDefaultTitles(), peerID); err != nil {
		return err
	}
	for _, agent := range SeededAgents {
		if err := seedBucket(ctx, store, ScopeAgent, agent, AgentDefaults(agent), AgentDefaultTitles(agent), peerID); err != nil {
			return err
		}
	}
	return nil
}

// seedBucket inserts one row per Section for (scope, scopeID) unless any
// row already exists in that bucket. Missing bodies/titles fall back to
// the section name.
func seedBucket(ctx context.Context, store *Store, scope, scopeID string, bodies, titles map[string]string, peerID string) error {
	if len(bodies) == 0 {
		return nil
	}

	var count int
	if err := store.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM prompt_templates WHERE scope = ? AND scope_id = ?`,
		scope, scopeID,
	).Scan(&count); err != nil {
		return fmt.Errorf("templates.Seed check (%s/%s): %w", scope, scopeID, err)
	}
	if count > 0 {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)

	tx, err := store.DB().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("templates.Seed begin (%s/%s): %w", scope, scopeID, err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := insertBucket(ctx, tx, scope, scopeID, bodies, titles, peerID, now); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("templates.Seed commit (%s/%s): %w", scope, scopeID, err)
	}
	return nil
}

func insertBucket(ctx context.Context, tx *sql.Tx, scope, scopeID string, bodies, titles map[string]string, peerID, now string) error {
	insert := `INSERT INTO prompt_templates
		(template_uid, title, scope, scope_id, section, body, status, tags,
		 source_node, valid_from, valid_to, changed_by, reason, created_at, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, 'open', '[]', ?, ?, '', 'system-seed', ?, ?, '')`

	reason := "Initial seed from buildPrompt() defaults"
	if scope == ScopeAgent {
		reason = fmt.Sprintf("Initial seed: %s markdown-heading frame", scopeID)
	}

	for _, section := range Sections {
		body, ok := bodies[section]
		if !ok {
			continue
		}
		title := titles[section]
		if title == "" {
			title = section
		}
		uid, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("templates.Seed uuid (%s/%s/%s): %w", scope, scopeID, section, err)
		}
		if _, err := tx.ExecContext(ctx, insert,
			uid.String(), title, scope, scopeID, section, body, peerID, now, reason, now,
		); err != nil {
			return fmt.Errorf("templates.Seed insert %s/%s/%s: %w", scope, scopeID, section, err)
		}
	}
	return nil
}
