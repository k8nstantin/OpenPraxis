package node

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
	"github.com/k8nstantin/OpenPraxis/internal/config"
	"github.com/k8nstantin/OpenPraxis/internal/entity"
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
	eStore, err := entity.NewStore(db)
	if err != nil {
		t.Fatalf("entity store: %v", err)
	}
	tStore, err := task.NewStore(db)
	if err != nil {
		t.Fatalf("task store: %v", err)
	}

	return &Node{
		Config: &config.Config{
			Node: config.NodeConfig{UUID: "peer-test-uuid"},
		},
		Entities: eStore,
		Tasks:    tStore,
		Comments: comments.NewStore(db),
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

	p, err := n.Entities.Create(entity.TypeProduct, "P1", entity.StatusActive, nil, n.PeerID(), "test")
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	// currentDescription for an entity returns its Title, so new body "P1"
	// matches the stored title — should be a no-op.
	id, err := n.RecordDescriptionChange(ctx, comments.TargetProduct, p.EntityUID, "P1", "")
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if id != "" {
		t.Fatalf("expected empty id on no-op, got %q", id)
	}
	if got := countRevisions(t, n, comments.TargetProduct, p.EntityUID); got != 0 {
		t.Fatalf("expected 0 revisions, got %d", got)
	}
}

func TestRecordDescriptionChange_RecordsRevisionOnChange(t *testing.T) {
	n := newDescriptionTestNode(t)
	ctx := context.Background()

	p, err := n.Entities.Create(entity.TypeProduct, "P1", entity.StatusActive, nil, n.PeerID(), "test")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	id, err := n.RecordDescriptionChange(ctx, comments.TargetProduct, p.EntityUID, "new body", "")
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if id == "" {
		t.Fatalf("expected new comment id, got empty")
	}
	if got := countRevisions(t, n, comments.TargetProduct, p.EntityUID); got != 1 {
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

	p, err := n.Entities.Create(entity.TypeProduct, "body", entity.StatusActive, nil, n.PeerID(), "test")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	id, err := n.RecordDescriptionChange(ctx, comments.TargetProduct, p.EntityUID, "  body\n", "")
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if id != "" {
		t.Fatalf("expected no revision on whitespace-only change, got %q", id)
	}
}

func TestRecordDescriptionChange_ManifestEntity(t *testing.T) {
	n := newDescriptionTestNode(t)
	ctx := context.Background()

	m, err := n.Entities.Create(entity.TypeManifest, "original spec", entity.StatusDraft, nil, n.PeerID(), "test")
	if err != nil {
		t.Fatalf("create manifest entity: %v", err)
	}

	id, err := n.RecordDescriptionChange(ctx, comments.TargetManifest, m.EntityUID, "revised spec", "")
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if id == "" {
		t.Fatalf("expected revision to be recorded")
	}
	if got := countRevisions(t, n, comments.TargetManifest, m.EntityUID); got != 1 {
		t.Fatalf("got %d revisions", got)
	}

	// Passing the same body back should be a no-op.
	id, err = n.RecordDescriptionChange(ctx, comments.TargetManifest, m.EntityUID, "original spec", "")
	if err != nil {
		t.Fatalf("record no-op: %v", err)
	}
	if id != "" {
		t.Fatalf("expected no-op when new body matches current title, got %q", id)
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
	p, err := n.Entities.Create(entity.TypeProduct, "body", entity.StatusActive, nil, n.PeerID(), "test")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id, err := n.RecordDescriptionChange(ctx, comments.TargetProduct, p.EntityUID, "", "")
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
	p, err := n.Entities.Create(entity.TypeProduct, "body", entity.StatusActive, nil, n.PeerID(), "test")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id, err := n.RecordDescriptionChange(ctx, comments.TargetProduct, p.EntityUID, "new body", "alice")
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
