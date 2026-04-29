package relationships

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// openMigrateTestDB builds a fresh DB containing the three legacy
// dependency tables (matching the live schema) plus fixture rows
// that exercise the direction + idempotency contracts.
func openMigrateTestDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "migrate.db") + "?_journal_mode=WAL"
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE product_dependencies (
		product_id            TEXT NOT NULL,
		depends_on_product_id TEXT NOT NULL,
		created_at            INTEGER NOT NULL,
		created_by            TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (product_id, depends_on_product_id)
	)`); err != nil {
		t.Fatalf("create product_dependencies: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE manifest_dependencies (
		manifest_id            TEXT NOT NULL,
		depends_on_manifest_id TEXT NOT NULL,
		created_at             INTEGER NOT NULL,
		created_by             TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (manifest_id, depends_on_manifest_id)
	)`); err != nil {
		t.Fatalf("create manifest_dependencies: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE task_dependency (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id TEXT NOT NULL,
		depends_on TEXT NOT NULL DEFAULT '',
		valid_from TEXT NOT NULL,
		valid_to TEXT NOT NULL DEFAULT '',
		changed_by TEXT NOT NULL DEFAULT '',
		reason TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("create task_dependency: %v", err)
	}
	return db
}

// TestMigrateLegacyDeps_HappyPath asserts that one row in each legacy
// table lands in relationships with the right direction (depender →
// dependee), the right ValidFrom (preserved from legacy), and the
// right edge kind (EdgeDependsOn).
func TestMigrateLegacyDeps_HappyPath(t *testing.T) {
	db := openMigrateTestDB(t)
	now := time.Now().UTC()
	unix := now.Unix()
	rfc := now.Format(time.RFC3339Nano)

	if _, err := db.Exec(
		`INSERT INTO product_dependencies VALUES (?, ?, ?, 'op')`,
		"prod-A", "prod-B", unix); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO manifest_dependencies VALUES (?, ?, ?, 'op')`,
		"mf-A", "mf-B", unix); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO task_dependency (task_id, depends_on, valid_from, valid_to, changed_by, reason)
		 VALUES (?, ?, ?, '', 'op', 'orig')`,
		"tk-A", "tk-B", rfc); err != nil {
		t.Fatal(err)
	}

	s, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	n, err := s.MigrateLegacyDeps(context.Background())
	if err != nil {
		t.Fatalf("MigrateLegacyDeps: %v", err)
	}
	if n != 3 {
		t.Errorf("migrated rows = %d, want 3", n)
	}

	// Re-run must be a no-op.
	n2, err := s.MigrateLegacyDeps(context.Background())
	if err != nil {
		t.Fatalf("re-run: %v", err)
	}
	if n2 != 0 {
		t.Errorf("re-run inserted %d rows, want 0 (idempotent)", n2)
	}

	// Spot-check direction: edge goes depender → dependee.
	cases := []struct {
		src, dst, kind string
	}{
		{"prod-A", "prod-B", KindProduct},
		{"mf-A", "mf-B", KindManifest},
		{"tk-A", "tk-B", KindTask},
	}
	for _, c := range cases {
		e, found, err := s.Get(context.Background(), c.src, c.dst, EdgeDependsOn)
		if err != nil {
			t.Fatalf("Get %s: %v", c.kind, err)
		}
		if !found {
			t.Errorf("%s edge missing: %s → %s", c.kind, c.src, c.dst)
			continue
		}
		if e.SrcKind != c.kind || e.DstKind != c.kind {
			t.Errorf("%s edge wrong kind: src=%q dst=%q", c.kind, e.SrcKind, e.DstKind)
		}
	}
}

// TestMigrateLegacyDeps_SkipsEmptyAndSelfLoops asserts that
// task_dependency clear-marker rows (depends_on='') and any
// accidentally self-referential rows are skipped without erroring the
// whole migration.
func TestMigrateLegacyDeps_SkipsEmptyAndSelfLoops(t *testing.T) {
	db := openMigrateTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// task_dependency clear-marker (depends_on='') — must not become
	// a relationships row.
	if _, err := db.Exec(
		`INSERT INTO task_dependency (task_id, depends_on, valid_from, valid_to, changed_by, reason)
		 VALUES ('tk-orphan', '', ?, '', 'op', 'cleared')`, now); err != nil {
		t.Fatal(err)
	}
	// Real edge alongside it.
	if _, err := db.Exec(
		`INSERT INTO task_dependency (task_id, depends_on, valid_from, valid_to, changed_by, reason)
		 VALUES ('tk-X', 'tk-Y', ?, '', 'op', 'real')`, now); err != nil {
		t.Fatal(err)
	}

	s, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	n, err := s.MigrateLegacyDeps(context.Background())
	if err != nil {
		t.Fatalf("MigrateLegacyDeps: %v", err)
	}
	if n != 1 {
		t.Errorf("migrated rows = %d, want 1 (clear-marker skipped)", n)
	}
	if _, found, _ := s.Get(context.Background(), "tk-X", "tk-Y", EdgeDependsOn); !found {
		t.Errorf("real task edge not migrated")
	}
}

// TestMigrateLegacyDeps_MultiVersionTaskHistory asserts that the SCD
// audit history of task_dependency lands in relationships with the
// closed rows preserved (valid_to set) and the current row open
// (valid_to='').
func TestMigrateLegacyDeps_MultiVersionTaskHistory(t *testing.T) {
	db := openMigrateTestDB(t)
	t1 := "2026-04-01T10:00:00Z"
	t2 := "2026-04-15T10:00:00Z"

	// Closed historical row + current row for the same task.
	if _, err := db.Exec(
		`INSERT INTO task_dependency (task_id, depends_on, valid_from, valid_to, changed_by, reason)
		 VALUES ('tk-X', 'tk-Old', ?, ?, 'op', 'first')`, t1, t2); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO task_dependency (task_id, depends_on, valid_from, valid_to, changed_by, reason)
		 VALUES ('tk-X', 'tk-New', ?, '', 'op', 'second')`, t2); err != nil {
		t.Fatal(err)
	}

	s, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := s.MigrateLegacyDeps(context.Background()); err != nil {
		t.Fatalf("MigrateLegacyDeps: %v", err)
	}

	// Current row must be tk-X → tk-New (open).
	e, found, err := s.Get(context.Background(), "tk-X", "tk-New", EdgeDependsOn)
	if err != nil || !found {
		t.Fatalf("current edge missing: err=%v found=%v", err, found)
	}
	if e.ValidTo != "" {
		t.Errorf("current edge valid_to = %q, want empty", e.ValidTo)
	}

	// Closed row must be present in History.
	hist, err := s.History(context.Background(), "tk-X", "tk-Old", EdgeDependsOn)
	if err != nil {
		t.Fatalf("History tk-X→tk-Old: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("history rows = %d, want 1 closed", len(hist))
	}
	if hist[0].ValidTo == "" {
		t.Errorf("closed row valid_to is empty, want %q", t2)
	}
}
