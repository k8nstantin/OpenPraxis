package entity

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// MigrateFromLegacy reads every non-deleted row from the four legacy tables
// (products, manifests, tasks, ideas) and inserts a current SCD-2 entity row
// for each one. Each insert is guarded by a NOT EXISTS check so the function
// is safe to call on every boot — rows already migrated are skipped.
//
// It also seeds nodes_entities: any product or task row whose source_node
// column is non-empty gets a corresponding nodes_entities row.
//
// Returns the total number of rows inserted across all tables.
func (s *Store) MigrateFromLegacy(ctx context.Context) (migrated int, err error) {
	n, err := s.migrateProducts(ctx)
	if err != nil {
		return migrated, fmt.Errorf("entity: migrate products: %w", err)
	}
	migrated += n

	n, err = s.migrateManifests(ctx)
	if err != nil {
		return migrated, fmt.Errorf("entity: migrate manifests: %w", err)
	}
	migrated += n

	n, err = s.migrateTasks(ctx)
	if err != nil {
		return migrated, fmt.Errorf("entity: migrate tasks: %w", err)
	}
	migrated += n

	n, err = s.migrateIdeas(ctx)
	if err != nil {
		return migrated, fmt.Errorf("entity: migrate ideas: %w", err)
	}
	migrated += n

	return migrated, nil
}

func (s *Store) migrateProducts(ctx context.Context) (int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, title, status, tags, source_node, created_at
		FROM products WHERE deleted_at = ''`)
	if err != nil {
		if isNoSuchTable(err) {
			return 0, nil
		}
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id, title, status, tagsStr, sourceNode, createdAt string
		if err := rows.Scan(&id, &title, &status, &tagsStr, &sourceNode, &createdAt); err != nil {
			continue
		}
		mappedStatus := mapProductStatus(status)
		validFrom := coalesceTimestamp(createdAt)

		inserted, err := s.insertLegacyEntity(ctx, id, TypeProduct, title, mappedStatus, tagsStr, validFrom)
		if err != nil {
			return count, err
		}
		if inserted {
			count++
		}

		if sourceNode != "" {
			_ = s.AddNodeEntity(ctx, sourceNode, id, "migration", "migrated from legacy table")
		}
	}
	return count, rows.Err()
}

func (s *Store) migrateManifests(ctx context.Context) (int, error) {
	// manifests uses a soft-delete column added via ALTER TABLE; older DBs
	// may not have it, but we query it anyway — the column has a DEFAULT ''.
	rows, err := s.db.QueryContext(ctx, `SELECT id, title, status, tags, created_at
		FROM manifests WHERE deleted_at = ''`)
	if err != nil {
		if isNoSuchTable(err) {
			return 0, nil
		}
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id, title, status, tagsStr, createdAt string
		if err := rows.Scan(&id, &title, &status, &tagsStr, &createdAt); err != nil {
			continue
		}
		mappedStatus := mapManifestStatus(status)
		validFrom := coalesceTimestamp(createdAt)

		inserted, err := s.insertLegacyEntity(ctx, id, TypeManifest, title, mappedStatus, tagsStr, validFrom)
		if err != nil {
			return count, err
		}
		if inserted {
			count++
		}
	}
	return count, rows.Err()
}

func (s *Store) migrateTasks(ctx context.Context) (int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, title, status, source_node, created_at
		FROM tasks WHERE deleted_at = ''`)
	if err != nil {
		if isNoSuchTable(err) {
			return 0, nil
		}
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id, title, status, sourceNode, createdAt string
		if err := rows.Scan(&id, &title, &status, &sourceNode, &createdAt); err != nil {
			continue
		}
		mappedStatus := mapTaskStatus(status)
		validFrom := coalesceTimestamp(createdAt)

		// Tasks don't carry a tags column in the legacy schema.
		inserted, err := s.insertLegacyEntity(ctx, id, TypeTask, title, mappedStatus, "[]", validFrom)
		if err != nil {
			return count, err
		}
		if inserted {
			count++
		}

		if sourceNode != "" {
			_ = s.AddNodeEntity(ctx, sourceNode, id, "migration", "migrated from legacy table")
		}
	}
	return count, rows.Err()
}

func (s *Store) migrateIdeas(ctx context.Context) (int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, title, status, tags, created_at
		FROM ideas WHERE deleted_at = ''`)
	if err != nil {
		if isNoSuchTable(err) {
			return 0, nil
		}
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id, title, status, tagsStr, createdAt string
		if err := rows.Scan(&id, &title, &status, &tagsStr, &createdAt); err != nil {
			continue
		}
		mappedStatus := mapIdeaStatus(status)
		validFrom := coalesceTimestamp(createdAt)

		inserted, err := s.insertLegacyEntity(ctx, id, TypeIdea, title, mappedStatus, tagsStr, validFrom)
		if err != nil {
			return count, err
		}
		if inserted {
			count++
		}
	}
	return count, rows.Err()
}

// insertLegacyEntity writes one entity row when no current row exists yet for
// entityUID. Returns true when a new row was inserted.
func (s *Store) insertLegacyEntity(ctx context.Context, entityUID, entityType, title, status, tagsJSON, validFrom string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM entities
		WHERE entity_uid = ? AND valid_to = ''`, entityUID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("entity: migrate check: %w", err)
	}
	if exists > 0 {
		return false, nil
	}

	_, err = s.db.ExecContext(ctx, `INSERT INTO entities
		(entity_uid, type, title, status, tags, valid_from, valid_to, changed_by, change_reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?, '', 'migration', 'migrated from legacy table', ?)`,
		entityUID, entityType, title, status, tagsJSON, validFrom, validFrom)
	if err != nil {
		return false, fmt.Errorf("entity: migrate insert: %w", err)
	}
	return true, nil
}

// mapProductStatus converts legacy product status values to entity status constants.
func mapProductStatus(s string) string {
	switch s {
	case "draft":
		return StatusDraft
	case "open":
		return StatusActive
	case "closed":
		return StatusClosed
	case "archive":
		return StatusArchived
	default:
		return StatusDraft
	}
}

// mapManifestStatus converts legacy manifest status values to entity status constants.
func mapManifestStatus(s string) string {
	switch s {
	case "draft":
		return StatusDraft
	case "open":
		return StatusActive
	case "closed":
		return StatusClosed
	case "archive":
		return StatusArchived
	default:
		return StatusDraft
	}
}

// mapTaskStatus converts legacy task status values to entity status constants.
func mapTaskStatus(s string) string {
	switch s {
	case "pending", "waiting", "scheduled", "paused":
		return StatusDraft
	case "running":
		return StatusActive
	case "completed":
		return StatusClosed
	case "failed", "cancelled":
		return StatusArchived
	default:
		return StatusDraft
	}
}

// mapIdeaStatus converts legacy idea status values to entity status constants.
func mapIdeaStatus(s string) string {
	switch s {
	case "draft":
		return StatusDraft
	case "open":
		return StatusActive
	case "closed":
		return StatusClosed
	case "archive":
		return StatusArchived
	default:
		return StatusDraft
	}
}

// coalesceTimestamp returns ts if it parses as RFC3339; otherwise returns
// the current UTC time formatted as RFC3339Nano. Prevents inserting a blank
// valid_from for legacy rows whose created_at was never populated.
func coalesceTimestamp(ts string) string {
	if ts != "" {
		if _, err := time.Parse(time.RFC3339, ts); err == nil {
			return ts
		}
		if _, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			return ts
		}
	}
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// isNoSuchTable returns true when err is the SQLite "no such table" error.
// Avoids importing mattn/go-sqlite3 just for the error type check.
func isNoSuchTable(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "no such table")
}
