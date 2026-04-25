package manifest

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// TestList_LimitZeroIsUnbounded — regression test for the bulk-fetch
// bug. Insert 75 manifests (more than the old 50-cap), call List("", 0),
// expect 75 returned. Was: silently truncated to 50, breaking the
// apiTasksByPeer bulk-fetch optimization which then synthesised
// "Unknown" labels for tasks beyond the 50th manifest.
func TestList_LimitZeroIsUnbounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.db") + "?_journal_mode=WAL&_busy_timeout=5000"
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	s, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	const N = 75
	for i := 0; i < N; i++ {
		if _, err := s.Create("title", "", "", "open", "test", "node", "", "", nil, nil); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	// limit=0: must return ALL manifests, not silently cap at 50.
	all, err := s.List("", 0)
	if err != nil {
		t.Fatalf("List(0): %v", err)
	}
	if len(all) != N {
		t.Errorf("limit=0 should return all %d manifests, got %d (was the bug)", N, len(all))
	}

	// Positive limit still caps as documented.
	page, err := s.List("", 25)
	if err != nil {
		t.Fatalf("List(25): %v", err)
	}
	if len(page) != 25 {
		t.Errorf("limit=25 should return 25 manifests, got %d", len(page))
	}

	// Negative limit defensively falls back to 50.
	defensive, err := s.List("", -1)
	if err != nil {
		t.Fatalf("List(-1): %v", err)
	}
	if len(defensive) != 50 {
		t.Errorf("limit<0 should fall back to 50, got %d", len(defensive))
	}
}
