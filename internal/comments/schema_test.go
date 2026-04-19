package comments

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// openTestDB opens a fresh sqlite DB in t.TempDir() using the project's
// standard WAL + busy_timeout pragmas (visceral rule #10), applies the
// comments schema, and returns the handle.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "comments.db")
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

func TestInitSchema_CreatesCommentsTable(t *testing.T) {
	db := openTestDB(t)

	var name string
	err := db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='comments'`,
	).Scan(&name)
	if err != nil {
		t.Fatalf("lookup comments table: %v", err)
	}
	if name != "comments" {
		t.Fatalf("expected comments table, got %q", name)
	}

	want := map[string]int{
		"id":          1,
		"target_type": 1,
		"target_id":   1,
		"author":      1,
		"type":        1,
		"body":        1,
		"created_at":  1,
		"updated_at":  0,
		"parent_id":   0,
	}
	rows, err := db.Query(`PRAGMA table_info(comments)`)
	if err != nil {
		t.Fatalf("pragma table_info: %v", err)
	}
	defer rows.Close()
	got := make(map[string]int)
	for rows.Next() {
		var (
			cid       int
			name, typ string
			notnull   int
			dfltValue sql.NullString
			pk        int
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

	// Idempotent.
	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema second call: %v", err)
	}
}

func TestInitSchema_PrimaryKeyEnforced(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec(
		`INSERT INTO comments (id, target_type, target_id, author, type, body, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"id-1", "product", "p1", "alice", "user_note", "hello", 1000,
	)
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}

	_, err = db.Exec(
		`INSERT INTO comments (id, target_type, target_id, author, type, body, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"id-1", "product", "p1", "bob", "user_note", "world", 2000,
	)
	if err == nil {
		t.Fatal("expected duplicate primary key insert to fail, got nil error")
	}
}

func TestInitSchema_TargetTypeCheckConstraint(t *testing.T) {
	db := openTestDB(t)

	for i, valid := range []string{"product", "manifest", "task"} {
		_, err := db.Exec(
			`INSERT INTO comments (id, target_type, target_id, author, type, body, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			valid+"-id", valid, "tid", "alice", "user_note", "x", int64(i+1),
		)
		if err != nil {
			t.Errorf("valid target_type %q rejected: %v", valid, err)
		}
	}

	_, err := db.Exec(
		`INSERT INTO comments (id, target_type, target_id, author, type, body, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"bad-id", "foo", "tid", "alice", "user_note", "x", 1,
	)
	if err == nil {
		t.Fatal("expected CHECK constraint to reject invalid target_type, got nil error")
	}
}

func TestInitSchema_IndexesPresent(t *testing.T) {
	db := openTestDB(t)

	for _, idx := range []string{"idx_comments_target", "idx_comments_author"} {
		var name string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='index' AND name=?`,
			idx,
		).Scan(&name)
		if err != nil {
			t.Fatalf("lookup %s: %v", idx, err)
		}
		if name != idx {
			t.Fatalf("expected %s, got %q", idx, name)
		}
	}
}
