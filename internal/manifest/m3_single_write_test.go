package manifest

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/k8nstantin/OpenPraxis/internal/relationships"
)

// openM3TestStore opens a fresh DB + manifest store + relationships
// backend wired together. Mirrors the production wiring in
// internal/node/node.go but stripped to the bits this regression test
// needs.
func openM3TestStore(t *testing.T) (*Store, *relationships.Store, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "m3.db") + "?_journal_mode=WAL&_busy_timeout=5000"
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	rels, err := relationships.New(db)
	if err != nil {
		t.Fatalf("relationships.New: %v", err)
	}
	s, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	s.SetRelationshipsBackend(rels)
	return s, rels, db
}

// TestManifestCreate_WritesEdgeOwnsOnly — creating a manifest under a
// product writes (a) one EdgeOwns row in `relationships` and (b) leaves
// no `project_id` column on the manifests table. Single-write
// regression for PR/M3.
func TestManifestCreate_WritesEdgeOwnsOnly(t *testing.T) {
	s, rels, db := openM3TestStore(t)
	productID := "prod-m3-create"

	m, err := s.Create("M3 manifest", "d", "c", "open", "test", "src", productID, "", nil, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// (a) One EdgeOwns row pointing product → manifest.
	edges, err := rels.ListIncoming(context.Background(), m.ID, relationships.EdgeOwns)
	if err != nil {
		t.Fatalf("ListIncoming: %v", err)
	}
	found := false
	for _, e := range edges {
		if e.SrcID == productID && e.SrcKind == relationships.KindProduct && e.DstID == m.ID && e.Kind == relationships.EdgeOwns {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no EdgeOwns(product=%s → manifest=%s) row", productID, m.ID)
	}

	// (b) PRAGMA table_info(manifests) does NOT list project_id.
	rows, err := db.Query(`PRAGMA table_info(manifests)`)
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
		if strings.EqualFold(name, "project_id") {
			t.Errorf("manifests still has project_id column post-PR/M3")
		}
	}

	// Read-back: Get populates ProjectID through the rels lookup.
	got, err := s.Get(m.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.ProjectID != productID {
		t.Errorf("Get.ProjectID = %v, want %s", got, productID)
	}
}
