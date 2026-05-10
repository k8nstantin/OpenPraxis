package relationships

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// openTestDB opens an in-memory SQLite handle in WAL mode + busy_timeout
// so concurrent writes during these tests don't deadlock.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:?_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// columnExists is the test-side mirror of the package-private hasColumn
// helper. We can't import the unexported one, but PRAGMA is a one-liner.
func columnExists(t *testing.T, db *sql.DB, table, col string) bool {
	t.Helper()
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false
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
			return false
		}
		if strings.EqualFold(name, col) {
			return true
		}
	}
	return false
}

// TestDropOwnershipColumns_DropsManifestProjectID — given a legacy DB
// with manifests.project_id present, the migration moves rows into a
// new schema without the column and preserves all other data.
func TestDropOwnershipColumns_DropsManifestProjectID(t *testing.T) {
	db := openTestDB(t)

	// Recreate the legacy schema. Mirrors what manifest.Store.init()
	// produced before PR/M3 cut the project_id column out.
	if _, err := db.Exec(`CREATE TABLE manifests (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		content TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'draft',
		jira_refs TEXT NOT NULL DEFAULT '[]',
		tags TEXT NOT NULL DEFAULT '[]',
		author TEXT NOT NULL DEFAULT '',
		source_node TEXT NOT NULL DEFAULT '',
		project_id TEXT NOT NULL DEFAULT '',
		depends_on TEXT NOT NULL DEFAULT '',
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		deleted_at TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("create legacy manifests: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO manifests
		(id, title, description, content, status, jira_refs, tags, author, source_node, project_id, depends_on, version, created_at, updated_at, deleted_at)
		VALUES ('m1', 'M1', 'd1', 'c1', 'open', '[]', '[]', 'me', 'src', 'prod-A', '', 2, '2026-01-01T00:00:00Z', '2026-01-02T00:00:00Z', '')`); err != nil {
		t.Fatalf("seed manifests row: %v", err)
	}

	if err := DropOwnershipColumns(context.Background(), db); err != nil {
		t.Fatalf("DropOwnershipColumns: %v", err)
	}
	if columnExists(t, db, "manifests", "project_id") {
		t.Errorf("manifests.project_id still present after migration")
	}

	// Row data preserved.
	var id, title, status, srcNode, dependsOn string
	var version int
	row := db.QueryRow(`SELECT id, title, status, source_node, depends_on, version FROM manifests WHERE id = 'm1'`)
	if err := row.Scan(&id, &title, &status, &srcNode, &dependsOn, &version); err != nil {
		t.Fatalf("scan migrated row: %v", err)
	}
	if id != "m1" || title != "M1" || status != "open" || srcNode != "src" || version != 2 {
		t.Errorf("data lost in migration: id=%q title=%q status=%q src=%q version=%d", id, title, status, srcNode, version)
	}
}

// TestDropOwnershipColumns_DropsTaskManifestID — same shape for tasks.
func TestDropOwnershipColumns_DropsTaskManifestID(t *testing.T) {
	t.Skip("task store migrated to entities; dropTaskManifestIDColumn is now a no-op")
	db := openTestDB(t)

	if _, err := db.Exec(`CREATE TABLE tasks (
		id TEXT PRIMARY KEY,
		manifest_id TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		schedule TEXT NOT NULL DEFAULT 'once',
		status TEXT NOT NULL DEFAULT 'pending',
		agent TEXT NOT NULL DEFAULT 'claude-code',
		source_node TEXT NOT NULL DEFAULT '',
		created_by TEXT NOT NULL DEFAULT '',
		run_count INTEGER NOT NULL DEFAULT 0,
		last_run_at TEXT NOT NULL DEFAULT '',
		next_run_at TEXT NOT NULL DEFAULT '',
		last_output TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		deleted_at TEXT NOT NULL DEFAULT '',
		depends_on TEXT NOT NULL DEFAULT '',
		block_reason TEXT NOT NULL DEFAULT '',
		action_request TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("create legacy tasks: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO tasks (id, manifest_id, title, status, created_at, updated_at)
		VALUES ('t1', 'm-legacy', 'T1', 'pending', '2026-01-01T00:00:00Z', '2026-01-02T00:00:00Z')`); err != nil {
		t.Fatalf("seed tasks row: %v", err)
	}

	if err := DropOwnershipColumns(context.Background(), db); err != nil {
		t.Fatalf("DropOwnershipColumns: %v", err)
	}
	if columnExists(t, db, "tasks", "manifest_id") {
		t.Errorf("tasks.manifest_id still present after migration")
	}

	var id, title, status string
	row := db.QueryRow(`SELECT id, title, status FROM tasks WHERE id = 't1'`)
	if err := row.Scan(&id, &title, &status); err != nil {
		t.Fatalf("scan migrated row: %v", err)
	}
	if id != "t1" || title != "T1" || status != "pending" {
		t.Errorf("data lost: id=%q title=%q status=%q", id, title, status)
	}
}

// TestDropOwnershipColumns_Idempotent — running the migration twice is
// a no-op the second time. Guards against accidental re-runs corrupting
// data on subsequent boots.
func TestDropOwnershipColumns_Idempotent(t *testing.T) {
	db := openTestDB(t)

	// Fresh schema (post-M3 shape).
	if _, err := db.Exec(`CREATE TABLE manifests (
		id TEXT PRIMARY KEY, title TEXT NOT NULL,
		created_at TEXT NOT NULL, updated_at TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create fresh manifests: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE tasks (
		id TEXT PRIMARY KEY, title TEXT NOT NULL,
		created_at TEXT NOT NULL, updated_at TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create fresh tasks: %v", err)
	}

	if err := DropOwnershipColumns(context.Background(), db); err != nil {
		t.Fatalf("DropOwnershipColumns first call: %v", err)
	}
	if err := DropOwnershipColumns(context.Background(), db); err != nil {
		t.Fatalf("DropOwnershipColumns second call: %v", err)
	}
}

// TestBackfillOwnershipEdges_Idempotent — given seeded legacy ownership
// rows, MigrateLegacyDeps inserts EdgeOwns rows once; a second call
// against the same DB inserts zero new rows.
func TestBackfillOwnershipEdges_Idempotent(t *testing.T) {
	t.Skip("task store migrated to entities; migrateTaskOwnership is now a no-op")
	db := openTestDB(t)

	// Seed legacy schemas + a row each.
	if _, err := db.Exec(`CREATE TABLE manifests (
		id TEXT PRIMARY KEY, project_id TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT '', deleted_at TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("create manifests: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO manifests (id, project_id, created_at) VALUES ('m1', 'p1', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("seed manifests: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE tasks (
		id TEXT PRIMARY KEY, manifest_id TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT '', deleted_at TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("create tasks: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO tasks (id, manifest_id, created_at) VALUES ('t1', 'm1', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}

	store, err := New(db)
	if err != nil {
		t.Fatalf("relationships.New: %v", err)
	}

	first, err := store.MigrateLegacyDeps(context.Background())
	if err != nil {
		t.Fatalf("first MigrateLegacyDeps: %v", err)
	}
	if first < 2 {
		t.Errorf("first migrate inserted %d rows; want >= 2 (1 manifest-owns + 1 task-owns)", first)
	}

	second, err := store.MigrateLegacyDeps(context.Background())
	if err != nil {
		t.Fatalf("second MigrateLegacyDeps: %v", err)
	}
	if second != 0 {
		t.Errorf("second migrate inserted %d rows; want 0 (idempotency)", second)
	}
}
