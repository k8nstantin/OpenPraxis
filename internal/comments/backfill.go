package comments

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// BackfillReport summarizes a single invocation of
// BackfillDescriptionRevisions. Counts are per-target-type.
type BackfillReport struct {
	DryRun            bool
	ProductsSeeded    int
	ProductsSkipped   int
	ManifestsSeeded   int
	ManifestsSkipped  int
	TasksSeeded       int
	TasksSkipped      int
	IdeasSeeded       int
	IdeasSkipped      int
}

// Total returns the total number of rows that would be (or were) inserted.
func (r BackfillReport) Total() int {
	return r.ProductsSeeded + r.ManifestsSeeded + r.TasksSeeded + r.IdeasSeeded
}

// BackfillDescriptionRevisions seeds one prompt comment per
// existing Product, Manifest, and Task whose body text is non-empty and that
// does not already carry a prompt row. When apply is false the
// function only counts what it would insert (dry run). Re-running after a
// successful apply is a no-op because of the NOT EXISTS guard.
//
// Body selection per entity:
//   - products  → description
//   - manifests → content (falls back to description if content is empty)
//   - tasks     → description
//
// Author is the entity's source_node, falling back to "system-backfill" when
// that column is empty. created_at is taken from the entity's updated_at
// (RFC3339 string) and stored as unix seconds to match the comments schema.
func BackfillDescriptionRevisions(ctx context.Context, db *sql.DB, apply bool) (BackfillReport, error) {
	rep := BackfillReport{DryRun: !apply}

	type row struct {
		id         string
		author     string
		body       string
		updatedStr string
	}

	// Products
	prodRows, err := queryBackfillProducts(ctx, db)
	if err != nil {
		return rep, fmt.Errorf("query products: %w", err)
	}
	for _, r := range prodRows {
		ok, err := seedRevision(ctx, db, apply, "product", r.id, r.author, r.body, r.updatedStr)
		if err != nil {
			return rep, err
		}
		if ok {
			rep.ProductsSeeded++
		} else {
			rep.ProductsSkipped++
		}
	}

	// Manifests
	manRows, err := queryBackfillManifests(ctx, db)
	if err != nil {
		return rep, fmt.Errorf("query manifests: %w", err)
	}
	for _, r := range manRows {
		ok, err := seedRevision(ctx, db, apply, "manifest", r.id, r.author, r.body, r.updatedStr)
		if err != nil {
			return rep, err
		}
		if ok {
			rep.ManifestsSeeded++
		} else {
			rep.ManifestsSkipped++
		}
	}

	// Tasks
	taskRows, err := queryBackfillTasks(ctx, db)
	if err != nil {
		return rep, fmt.Errorf("query tasks: %w", err)
	}
	for _, r := range taskRows {
		ok, err := seedRevision(ctx, db, apply, "task", r.id, r.author, r.body, r.updatedStr)
		if err != nil {
			return rep, err
		}
		if ok {
			rep.TasksSeeded++
		} else {
			rep.TasksSkipped++
		}
	}

	// Ideas (added when idea was promoted to a full DV target type —
	// missing from the original DV/M1 backfill because ideas weren't
	// in scope at that time).
	ideaRows, err := queryBackfillIdeas(ctx, db)
	if err != nil {
		return rep, fmt.Errorf("query ideas: %w", err)
	}
	for _, r := range ideaRows {
		ok, err := seedRevision(ctx, db, apply, "idea", r.id, r.author, r.body, r.updatedStr)
		if err != nil {
			return rep, err
		}
		if ok {
			rep.IdeasSeeded++
		} else {
			rep.IdeasSkipped++
		}
	}

	return rep, nil
}

type backfillRow struct {
	id         string
	author     string
	body       string
	updatedStr string
}

func queryBackfillProducts(ctx context.Context, db *sql.DB) ([]backfillRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, COALESCE(source_node, ''), COALESCE(description, ''), COALESCE(updated_at, '')
		FROM products
		WHERE COALESCE(deleted_at, '') = ''
		  AND COALESCE(description, '') <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBackfillRows(rows)
}

func queryBackfillManifests(ctx context.Context, db *sql.DB) ([]backfillRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, COALESCE(source_node, ''),
		       CASE WHEN COALESCE(content, '') <> '' THEN content ELSE COALESCE(description, '') END,
		       COALESCE(updated_at, '')
		FROM manifests
		WHERE COALESCE(deleted_at, '') = ''
		  AND (COALESCE(content, '') <> '' OR COALESCE(description, '') <> '')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBackfillRows(rows)
}

func queryBackfillTasks(ctx context.Context, db *sql.DB) ([]backfillRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, COALESCE(source_node, ''), COALESCE(description, ''), COALESCE(updated_at, '')
		FROM tasks
		WHERE COALESCE(deleted_at, '') = ''
		  AND COALESCE(description, '') <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBackfillRows(rows)
}

func queryBackfillIdeas(ctx context.Context, db *sql.DB) ([]backfillRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, COALESCE(source_node, ''), COALESCE(description, ''), COALESCE(updated_at, '')
		FROM ideas
		WHERE COALESCE(deleted_at, '') = ''
		  AND COALESCE(description, '') <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBackfillRows(rows)
}

func scanBackfillRows(rows *sql.Rows) ([]backfillRow, error) {
	var out []backfillRow
	for rows.Next() {
		var r backfillRow
		if err := rows.Scan(&r.id, &r.author, &r.body, &r.updatedStr); err != nil {
			return nil, err
		}
		if r.author == "" {
			r.author = "system-backfill"
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// seedRevision returns (inserted, err). inserted=false means the entity
// already carries a prompt row and was skipped.
func seedRevision(ctx context.Context, db *sql.DB, apply bool,
	targetType, targetID, author, body, updatedStr string) (bool, error) {

	var existing int
	err := db.QueryRowContext(ctx,
		`SELECT 1 FROM comments WHERE type = ? AND target_type = ? AND target_id = ? LIMIT 1`,
		string(TypeDescriptionRevision), targetType, targetID,
	).Scan(&existing)
	if err != nil && err != sql.ErrNoRows {
		return false, fmt.Errorf("check existing: %w", err)
	}
	if err == nil {
		return false, nil
	}

	if !apply {
		return true, nil
	}

	createdAt := parseUpdatedAt(updatedStr)

	id, err := uuid.NewV7()
	if err != nil {
		return false, fmt.Errorf("uuid v7: %w", err)
	}

	_, err = db.ExecContext(ctx,
		`INSERT INTO comments (id, target_type, target_id, author, type, body, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id.String(), targetType, targetID, author,
		string(TypeDescriptionRevision), body, createdAt,
	)
	if err != nil {
		return false, fmt.Errorf("insert %s/%s: %w", targetType, targetID, err)
	}
	return true, nil
}

func parseUpdatedAt(s string) int64 {
	if s == "" {
		return time.Now().Unix()
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.Unix()
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Unix()
	}
	return time.Now().Unix()
}
