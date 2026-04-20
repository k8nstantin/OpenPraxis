package product

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

func containsProduct(ps []*Product, id string) bool {
	for _, p := range ps {
		if p.ID == id {
			return true
		}
	}
	return false
}

func TestSearch_Keyword(t *testing.T) {
	s := openSearchTestStore(t)
	a, _ := s.Create("Alpha widget", "", "open", "node", nil)
	b, _ := s.Create("Beta gizmo", "", "open", "node", nil)

	res, err := s.Search("widget", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !containsProduct(res, a.ID) || containsProduct(res, b.ID) {
		t.Fatalf("keyword mismatch: got %+v", res)
	}
}

func TestSearch_IDExact(t *testing.T) {
	s := openSearchTestStore(t)
	a, _ := s.Create("A", "", "open", "node", nil)
	_, _ = s.Create("B", "", "open", "node", nil)

	res, err := s.Search(a.ID, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res) != 1 || res[0].ID != a.ID {
		t.Fatalf("id-exact wanted [%s], got %+v", a.ID, res)
	}
}

func TestSearch_IDPrefix(t *testing.T) {
	s := openSearchTestStore(t)
	a, _ := s.Create("A", "", "open", "node", nil)
	_, _ = s.Create("B", "", "open", "node", nil)

	res, err := s.Search(a.Marker, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !containsProduct(res, a.ID) {
		t.Fatalf("id-prefix missing %s: got %+v", a.ID, res)
	}
}

func TestSearch_Unknown(t *testing.T) {
	s := openSearchTestStore(t)
	_, _ = s.Create("A", "", "open", "node", nil)

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
	_, _ = s.Create("A", "", "open", "node", nil)

	res, err := s.Search("  ", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("empty query should be empty, got %+v", res)
	}
}

func TestSearch_Tag(t *testing.T) {
	s := openSearchTestStore(t)
	a, _ := s.Create("A", "", "open", "node", []string{"alpha-tag"})
	_, _ = s.Create("B", "", "open", "node", []string{"beta-tag"})

	res, err := s.Search("alpha-tag", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !containsProduct(res, a.ID) {
		t.Fatalf("tag match missing %s: got %+v", a.ID, res)
	}
}
