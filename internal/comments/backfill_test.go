package comments

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func openBackfillTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("wal: %v", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		t.Fatalf("busy_timeout: %v", err)
	}
	if err := InitSchema(db); err != nil {
		t.Fatalf("init comments schema: %v", err)
	}
	createEntityTables(t, db)
	return db
}

func createEntityTables(t *testing.T, db *sql.DB) {
	t.Helper()
	schemas := []string{
		`CREATE TABLE products (
			id TEXT PRIMARY KEY, title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			source_node TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT '',
			deleted_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE manifests (
			id TEXT PRIMARY KEY, title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL DEFAULT '',
			source_node TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT '',
			deleted_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE tasks (
			id TEXT PRIMARY KEY, title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			source_node TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT '',
			deleted_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE ideas (
			id TEXT PRIMARY KEY, title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			source_node TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT '',
			deleted_at TEXT NOT NULL DEFAULT ''
		)`,
	}
	for _, s := range schemas {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("create: %v", err)
		}
	}
}

func TestBackfillDescriptionRevisions(t *testing.T) {
	db := openBackfillTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	mustExec(t, db, `INSERT INTO products (id, title, description, source_node, updated_at) VALUES
		('p-with-body', 'P1', 'prod body', 'node-a', ?),
		('p-empty',     'P2', '',          'node-a', ?)`, now, now)
	mustExec(t, db, `INSERT INTO manifests (id, title, description, content, source_node, updated_at) VALUES
		('m-content-only', 'M1', '',        'manifest content', 'node-b', ?),
		('m-desc-only',    'M2', 'desc only','',                 '',       ?),
		('m-empty',        'M3', '',        '',                  'node-b', ?)`, now, now, now)
	mustExec(t, db, `INSERT INTO tasks (id, title, description, source_node, updated_at) VALUES
		('t-with-body', 'T1', 'task desc', 'node-c', ?),
		('t-empty',     'T2', '',          'node-c', ?)`, now, now)

	// Pre-seed one existing description_revision so idempotency is exercised
	// from the very first invocation.
	mustExec(t, db, `INSERT INTO comments (id, target_type, target_id, author, type, body, created_at)
		VALUES ('pre-existing', 'product', 'p-with-body', 'prior', 'description_revision', 'already there', 1)`)

	// Dry run should not write.
	rep, err := BackfillDescriptionRevisions(ctx, db, false)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !rep.DryRun {
		t.Fatalf("DryRun flag not set")
	}
	if rep.ProductsSeeded != 0 || rep.ManifestsSeeded != 2 || rep.TasksSeeded != 1 {
		t.Fatalf("dry-run counts = %+v", rep)
	}
	if rep.ProductsSkipped != 1 { // pre-seeded p-with-body
		t.Fatalf("expected 1 product skipped, got %+v", rep)
	}
	var got int
	if err := db.QueryRow(`SELECT COUNT(*) FROM comments WHERE type='description_revision'`).Scan(&got); err != nil {
		t.Fatalf("count: %v", err)
	}
	if got != 1 { // only the pre-seeded row
		t.Fatalf("dry run wrote rows: got %d want 1", got)
	}

	// Apply.
	rep, err = BackfillDescriptionRevisions(ctx, db, true)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if rep.ProductsSeeded != 0 || rep.ManifestsSeeded != 2 || rep.TasksSeeded != 1 {
		t.Fatalf("apply counts = %+v", rep)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM comments WHERE type='description_revision'`).Scan(&got); err != nil {
		t.Fatalf("count: %v", err)
	}
	if got != 4 { // pre-existing + 2 manifests + 1 task
		t.Fatalf("apply wrote wrong count: got %d want 4", got)
	}

	// Re-apply must be a no-op.
	rep, err = BackfillDescriptionRevisions(ctx, db, true)
	if err != nil {
		t.Fatalf("reapply: %v", err)
	}
	if rep.Total() != 0 {
		t.Fatalf("re-apply seeded rows: %+v", rep)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM comments WHERE type='description_revision'`).Scan(&got); err != nil {
		t.Fatalf("count: %v", err)
	}
	if got != 4 {
		t.Fatalf("re-apply changed count: got %d want 4", got)
	}

	// Author fallback: m-desc-only had empty source_node → system-backfill.
	var author, body string
	if err := db.QueryRow(
		`SELECT author, body FROM comments WHERE target_type='manifest' AND target_id='m-desc-only' AND type='description_revision'`,
	).Scan(&author, &body); err != nil {
		t.Fatalf("fetch m-desc-only: %v", err)
	}
	if author != "system-backfill" {
		t.Fatalf("author fallback = %q want system-backfill", author)
	}
	if body != "desc only" {
		t.Fatalf("manifest body fallback = %q want 'desc only'", body)
	}

	// Manifest content preferred over description when both set is covered
	// by m-content-only (content is the body).
	if err := db.QueryRow(
		`SELECT body FROM comments WHERE target_type='manifest' AND target_id='m-content-only' AND type='description_revision'`,
	).Scan(&body); err != nil {
		t.Fatalf("fetch m-content-only: %v", err)
	}
	if body != "manifest content" {
		t.Fatalf("manifest content body = %q", body)
	}
}

func mustExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}
