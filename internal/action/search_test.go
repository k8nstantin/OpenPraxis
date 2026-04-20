package action

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func openSearchTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s.db") + "?_journal_mode=WAL&_busy_timeout=5000"
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	s, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s
}

func containsToolName(as []Action, name string) bool {
	for _, a := range as {
		if a.ToolName == name {
			return true
		}
	}
	return false
}

func TestSearch_Keyword(t *testing.T) {
	s := openSearchTestStore(t)
	if _, err := s.RecordForTask("t1", "node", "Alpha_widget", "in", "out", "/cwd"); err != nil {
		t.Fatalf("record: %v", err)
	}
	if _, err := s.RecordForTask("t2", "node", "Beta_gizmo", "in", "out", "/cwd"); err != nil {
		t.Fatalf("record: %v", err)
	}

	res, err := s.Search("widget", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !containsToolName(res, "Alpha_widget") || containsToolName(res, "Beta_gizmo") {
		t.Fatalf("keyword mismatch: got %+v", res)
	}
}

func TestSearch_IDExact(t *testing.T) {
	s := openSearchTestStore(t)
	id, err := s.RecordForTask("t1", "node", "Tool", "in", "out", "/cwd")
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	_, _ = s.RecordForTask("t2", "node", "Other", "in", "out", "/cwd")

	res, err := s.Search(fmt.Sprintf("%d", id), 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// id match may return both rows if the substring appears in other
	// auto-ids; require the exact target to be present.
	found := false
	for _, a := range res {
		if a.ID == fmt.Sprintf("%d", id) {
			found = true
		}
	}
	if !found {
		t.Fatalf("id-exact wanted id=%d, got %+v", id, res)
	}
}

func TestSearch_IDPrefix(t *testing.T) {
	s := openSearchTestStore(t)
	// Create enough rows so ids have a leading digit pattern we can
	// prefix-match. SQLite INTEGER PK starts at 1.
	for i := 0; i < 5; i++ {
		if _, err := s.RecordForTask("t", "node", "Tool", "in", "out", "/cwd"); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	res, err := s.Search("1", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res) == 0 {
		t.Fatalf("id-prefix '1' should match at least id=1, got empty")
	}
}

func TestSearch_Unknown(t *testing.T) {
	s := openSearchTestStore(t)
	_, _ = s.RecordForTask("t1", "node", "Tool", "in", "out", "/cwd")

	res, err := s.Search("no-such-thing-xyz-987", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("unknown should be empty, got %+v", res)
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	s := openSearchTestStore(t)
	_, _ = s.RecordForTask("t1", "node", "Tool", "in", "out", "/cwd")

	res, err := s.Search("  ", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("empty query should be empty, got %+v", res)
	}
}
