package manifest

import (
	"database/sql"
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

func contains(ms []*Manifest, id string) bool {
	for _, m := range ms {
		if m.ID == id {
			return true
		}
	}
	return false
}

// TestSearch_Keyword covers the existing behavior: LIKE match on
// title/description/content/jira_refs/tags.
func TestSearch_Keyword(t *testing.T) {
	s := openSearchTestStore(t)
	a, _ := s.Create("Alpha widget", "", "", "open", "t", "node", "", "", nil, nil)
	b, _ := s.Create("Beta gizmo", "", "", "open", "t", "node", "", "", nil, nil)

	res, err := s.Search("widget", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !contains(res, a.ID) || contains(res, b.ID) {
		t.Fatalf("keyword match mismatch: got %+v", res)
	}
}

// TestSearch_IDExact — full UUID returns the exact manifest.
func TestSearch_IDExact(t *testing.T) {
	s := openSearchTestStore(t)
	a, _ := s.Create("A", "", "", "open", "t", "node", "", "", nil, nil)
	_, _ = s.Create("B", "", "", "open", "t", "node", "", "", nil, nil)

	res, err := s.Search(a.ID, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res) != 1 || res[0].ID != a.ID {
		t.Fatalf("id-exact match wanted [%s], got %+v", a.ID, res)
	}
}

// TestSearch_IDPrefix — marker (id[:12]) returns the owning manifest.
// This is the bread-and-butter scoped-id-search case from manifest M6.
func TestSearch_IDPrefix(t *testing.T) {
	s := openSearchTestStore(t)
	a, _ := s.Create("A", "", "", "open", "t", "node", "", "", nil, nil)
	_, _ = s.Create("B", "", "", "open", "t", "node", "", "", nil, nil)

	res, err := s.Search(a.Marker, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !contains(res, a.ID) {
		t.Fatalf("id-prefix match missing manifest %s: got %+v", a.ID, res)
	}
}

// TestSearch_Unknown — unknown marker / unknown keyword returns empty.
func TestSearch_Unknown(t *testing.T) {
	s := openSearchTestStore(t)
	_, _ = s.Create("A", "", "", "open", "t", "node", "", "", nil, nil)

	res, err := s.Search("no-such-thing-12345", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("unknown query should return empty, got %+v", res)
	}
}

// TestSearch_EmptyQuery — empty/whitespace returns nil without hitting
// the DB with a degenerate `%%` pattern that would match every row.
func TestSearch_EmptyQuery(t *testing.T) {
	s := openSearchTestStore(t)
	_, _ = s.Create("A", "", "", "open", "t", "node", "", "", nil, nil)

	res, err := s.Search("   ", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("empty query should return empty, got %+v", res)
	}
}
