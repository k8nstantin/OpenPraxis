package node

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
	"github.com/k8nstantin/OpenPraxis/internal/config"
	"github.com/k8nstantin/OpenPraxis/internal/manifest"
	"github.com/k8nstantin/OpenPraxis/internal/product"
	"github.com/k8nstantin/OpenPraxis/internal/task"
)

func newDescriptionTestNode(t *testing.T) *Node {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := comments.InitSchema(db); err != nil {
		t.Fatalf("comments schema: %v", err)
	}
	pStore, err := product.NewStore(db)
	if err != nil {
		t.Fatalf("product store: %v", err)
	}
	mStore, err := manifest.NewStore(db)
	if err != nil {
		t.Fatalf("manifest store: %v", err)
	}
	tStore, err := task.NewStore(db)
	if err != nil {
		t.Fatalf("task store: %v", err)
	}

	return &Node{
		Config: &config.Config{
			Node: config.NodeConfig{UUID: "peer-test-uuid"},
		},
		Products:  pStore,
		Manifests: mStore,
		Tasks:     tStore,
		Comments:  comments.NewStore(db),
	}
}

func countRevisions(t *testing.T, n *Node, target comments.TargetType, id string) int {
	t.Helper()
	ctx := context.Background()
	ct := comments.TypeDescriptionRevision
	rows, err := n.Comments.List(ctx, target, id, 100, &ct)
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	return len(rows)
}

func TestRecordDescriptionChange_NoOpWhenUnchanged(t *testing.T) {
	n := newDescriptionTestNode(t)
	ctx := context.Background()

	p, err := n.Products.Create("P1", "same body", "open", n.PeerID(), nil)
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	id, err := n.RecordDescriptionChange(ctx, comments.TargetProduct, p.ID, "same body", "")
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if id != "" {
		t.Fatalf("expected empty id on no-op, got %q", id)
	}
	if got := countRevisions(t, n, comments.TargetProduct, p.ID); got != 0 {
		t.Fatalf("expected 0 revisions, got %d", got)
	}
}

func TestRecordDescriptionChange_RecordsRevisionOnChange(t *testing.T) {
	n := newDescriptionTestNode(t)
	ctx := context.Background()

	p, err := n.Products.Create("P1", "original body", "open", n.PeerID(), nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	id, err := n.RecordDescriptionChange(ctx, comments.TargetProduct, p.ID, "new body", "")
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if id == "" {
		t.Fatalf("expected new comment id, got empty")
	}
	if got := countRevisions(t, n, comments.TargetProduct, p.ID); got != 1 {
		t.Fatalf("expected 1 revision, got %d", got)
	}

	c, err := n.Comments.Get(ctx, id)
	if err != nil {
		t.Fatalf("fetch revision: %v", err)
	}
	if c.Body != "new body" {
		t.Fatalf("body = %q, want 'new body'", c.Body)
	}
	if c.Author != n.PeerID() {
		t.Fatalf("author = %q, want peer id fallback", c.Author)
	}
	if c.Type != comments.TypeDescriptionRevision {
		t.Fatalf("type = %q", c.Type)
	}
}

func TestRecordDescriptionChange_WhitespaceOnlyChangeIsNoOp(t *testing.T) {
	n := newDescriptionTestNode(t)
	ctx := context.Background()

	p, err := n.Products.Create("P1", "body", "open", n.PeerID(), nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	id, err := n.RecordDescriptionChange(ctx, comments.TargetProduct, p.ID, "  body\n", "")
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if id != "" {
		t.Fatalf("expected no revision on whitespace-only change, got %q", id)
	}
}

func TestRecordDescriptionChange_ManifestUsesContent(t *testing.T) {
	n := newDescriptionTestNode(t)
	ctx := context.Background()

	m, err := n.Manifests.Create("M1", "short summary", "original spec", "draft", n.PeerID(), n.PeerID(), "", "", nil, nil)
	if err != nil {
		t.Fatalf("create manifest: %v", err)
	}

	id, err := n.RecordDescriptionChange(ctx, comments.TargetManifest, m.ID, "revised spec", "")
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if id == "" {
		t.Fatalf("expected revision to be recorded")
	}
	if got := countRevisions(t, n, comments.TargetManifest, m.ID); got != 1 {
		t.Fatalf("got %d revisions", got)
	}

	// Changing summary (not content) should NOT trigger a revision — the
	// helper compares against Content for manifests.
	id, err = n.RecordDescriptionChange(ctx, comments.TargetManifest, m.ID, "original spec", "")
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	// Wait — we just added a revision with 'revised spec' but left the
	// denormalised Content at 'original spec' because we never called
	// Update. The helper's current-body read will still see 'original
	// spec', so passing that as newBody must be a no-op.
	if id != "" {
		t.Fatalf("expected no-op when new body matches current content, got %q", id)
	}
}

func TestRecordDescriptionChange_TaskUsesDescription(t *testing.T) {
	n := newDescriptionTestNode(t)
	ctx := context.Background()

	tk, err := n.Tasks.Create("", "T1", "initial instructions", "", "", n.PeerID(), "", "")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	id, err := n.RecordDescriptionChange(ctx, comments.TargetTask, tk.ID, "updated instructions", "")
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if id == "" {
		t.Fatalf("expected revision recorded for task change")
	}
	if got := countRevisions(t, n, comments.TargetTask, tk.ID); got != 1 {
		t.Fatalf("got %d revisions", got)
	}
}

func TestRecordDescriptionChange_EmptyBodyIsNoOp(t *testing.T) {
	n := newDescriptionTestNode(t)
	ctx := context.Background()
	p, err := n.Products.Create("P1", "body", "open", n.PeerID(), nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id, err := n.RecordDescriptionChange(ctx, comments.TargetProduct, p.ID, "", "")
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if id != "" {
		t.Fatalf("expected no revision for empty body, got %q", id)
	}
}

func TestRecordDescriptionChange_MissingEntityIsNoOp(t *testing.T) {
	n := newDescriptionTestNode(t)
	ctx := context.Background()
	id, err := n.RecordDescriptionChange(ctx, comments.TargetProduct, "nonexistent-id", "body", "")
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if id != "" {
		t.Fatalf("expected empty id for missing entity, got %q", id)
	}
}

func TestRecordDescriptionChange_AuthorOverride(t *testing.T) {
	n := newDescriptionTestNode(t)
	ctx := context.Background()
	p, err := n.Products.Create("P1", "body", "open", n.PeerID(), nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id, err := n.RecordDescriptionChange(ctx, comments.TargetProduct, p.ID, "new body", "alice")
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	c, err := n.Comments.Get(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if c.Author != "alice" {
		t.Fatalf("author = %q, want alice", c.Author)
	}
}
