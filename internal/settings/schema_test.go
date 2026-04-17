package settings

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// openTestDB opens a fresh sqlite DB in t.TempDir() using the project's
// standard WAL + busy_timeout pragmas, applies the settings schema, and
// returns the handle. The DB is closed automatically on test cleanup.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "settings.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("set WAL: %v", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		t.Fatalf("set busy_timeout: %v", err)
	}

	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	return db
}

func TestSchema_CreatesSettingsTable(t *testing.T) {
	db := openTestDB(t)

	var name string
	err := db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='settings'`,
	).Scan(&name)
	if err != nil {
		t.Fatalf("lookup settings table: %v", err)
	}
	if name != "settings" {
		t.Fatalf("expected settings table, got %q", name)
	}

	// Verify the expected columns exist and carry the expected not-null flag.
	type col struct {
		name    string
		notnull int
	}
	want := map[string]int{
		"scope_type": 1,
		"scope_id":   1,
		"key":        1,
		"value":      1,
		"updated_at": 1,
		"updated_by": 1,
	}
	rows, err := db.Query(`PRAGMA table_info(settings)`)
	if err != nil {
		t.Fatalf("pragma table_info: %v", err)
	}
	defer rows.Close()
	got := make(map[string]int)
	for rows.Next() {
		var (
			cid        int
			name, typ  string
			notnull, _ int
			dfltValue  sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dfltValue, &pk); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		got[name] = notnull
	}
	for k, v := range want {
		if g, ok := got[k]; !ok {
			t.Errorf("missing column %q", k)
		} else if g != v {
			t.Errorf("column %q notnull=%d, want %d", k, g, v)
		}
	}

	// Re-running InitSchema must be idempotent.
	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema second call: %v", err)
	}
}

func TestSchema_PrimaryKeyEnforced(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec(
		`INSERT INTO settings (scope_type, scope_id, key, value, updated_at, updated_by) VALUES (?, ?, ?, ?, ?, ?)`,
		"product", "p1", "max_turns", "10", 1000, "tester",
	)
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}

	_, err = db.Exec(
		`INSERT INTO settings (scope_type, scope_id, key, value, updated_at, updated_by) VALUES (?, ?, ?, ?, ?, ?)`,
		"product", "p1", "max_turns", "20", 2000, "tester",
	)
	if err == nil {
		t.Fatal("expected duplicate primary key insert to fail, got nil error")
	}
}

func TestSchema_ScopeTypeCheckConstraint(t *testing.T) {
	db := openTestDB(t)

	for _, valid := range []string{"system", "product", "manifest", "task"} {
		_, err := db.Exec(
			`INSERT INTO settings (scope_type, scope_id, key, value, updated_at, updated_by) VALUES (?, ?, ?, ?, ?, ?)`,
			valid, "id-"+valid, "k", "\"v\"", 1, "",
		)
		if err != nil {
			t.Errorf("valid scope_type %q rejected: %v", valid, err)
		}
	}

	_, err := db.Exec(
		`INSERT INTO settings (scope_type, scope_id, key, value, updated_at, updated_by) VALUES (?, ?, ?, ?, ?, ?)`,
		"invalid-scope", "x", "k", "\"v\"", 1, "",
	)
	if err == nil {
		t.Fatal("expected CHECK constraint to reject invalid scope_type, got nil error")
	}
}

func TestSchema_IndexPresent(t *testing.T) {
	db := openTestDB(t)

	var name string
	err := db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_settings_scope'`,
	).Scan(&name)
	if err != nil {
		t.Fatalf("lookup idx_settings_scope: %v", err)
	}
	if name != "idx_settings_scope" {
		t.Fatalf("expected idx_settings_scope, got %q", name)
	}
}
