package task

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/k8nstantin/OpenPraxis/internal/relationships"
)

// TestTaskCreate_WritesEdgeOwnsOnly — creating a task under a manifest
// writes (a) one EdgeOwns(manifest → task) row in `relationships` and
// (b) leaves no `manifest_id` column on the tasks table.
func TestTaskCreate_WritesEdgeOwnsOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m3-task.db") + "?_journal_mode=WAL&_busy_timeout=5000"
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	s, err := NewStore(db)
	if err != nil {
		t.Fatalf("task.NewStore: %v", err)
	}
	rels := s.rels // task.NewStore auto-wires a backend

	manifestID := "m-m3"
	tk, err := s.Create(manifestID, "T", "", "once", "claude-code", "node", "test", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// (a) EdgeOwns row from manifest → task.
	edges, err := rels.ListIncoming(context.Background(), tk.ID, relationships.EdgeOwns)
	if err != nil {
		t.Fatalf("ListIncoming: %v", err)
	}
	found := false
	for _, e := range edges {
		if e.SrcID == manifestID && e.SrcKind == relationships.KindManifest && e.Kind == relationships.EdgeOwns {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no EdgeOwns(manifest=%s → task=%s) row", manifestID, tk.ID)
	}

	// (b) PRAGMA table_info(tasks) does NOT list manifest_id.
	rows, err := db.Query(`PRAGMA table_info(tasks)`)
	if err != nil {
		t.Fatalf("pragma: %v", err)
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
			t.Fatalf("scan pragma: %v", err)
		}
		if strings.EqualFold(name, "manifest_id") {
			t.Errorf("tasks still has manifest_id column post-PR/M3")
		}
	}

	// Read-back: Get populates ManifestID through the rels lookup.
	got, err := s.Get(tk.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.ManifestID != manifestID {
		t.Errorf("Get.ManifestID = %v, want %s", got, manifestID)
	}
}
