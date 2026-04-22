package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
	"github.com/k8nstantin/OpenPraxis/internal/config"
	"github.com/k8nstantin/OpenPraxis/internal/manifest"
	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/product"
	"github.com/k8nstantin/OpenPraxis/internal/task"
)

func newDescriptionTestServer(t *testing.T) *Server {
	t.Helper()
	dsn := "file::memory:?cache=shared&_journal_mode=WAL&_busy_timeout=5000"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := comments.InitSchema(db); err != nil {
		t.Fatalf("comments schema: %v", err)
	}
	p, err := product.NewStore(db)
	if err != nil {
		t.Fatalf("product: %v", err)
	}
	m, err := manifest.NewStore(db)
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	tk, err := task.NewStore(db)
	if err != nil {
		t.Fatalf("task: %v", err)
	}
	n := &node.Node{
		Config:    &config.Config{Node: config.NodeConfig{UUID: "peer-test"}},
		Products:  p,
		Manifests: m,
		Tasks:     tk,
		Comments:  comments.NewStore(db),
	}
	return &Server{node: n}
}

// seedProductHistory creates a product and a two-revision history, returning
// the full product id + the older revision's comment id.
func seedProductHistory(t *testing.T, s *Server) (productID, olderRevID, newerRevID string) {
	t.Helper()
	ctx := context.Background()
	p, err := s.node.Products.Create("P", "v1", "open", s.node.PeerID(), nil)
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	// v1 seed (current == new, so RecordDescriptionChange would no-op).
	rev1, err := s.node.Comments.Add(ctx, comments.TargetProduct, p.ID, "alice", comments.TypeDescriptionRevision, "v1")
	if err != nil {
		t.Fatalf("seed rev1: %v", err)
	}
	rev2ID, err := s.node.RecordDescriptionChange(ctx, comments.TargetProduct, p.ID, "v2", "bob")
	if err != nil {
		t.Fatalf("record rev2: %v", err)
	}
	if err := s.node.Products.Update(p.ID, p.Title, "v2", p.Status, p.Tags); err != nil {
		t.Fatalf("update: %v", err)
	}
	return p.ID, rev1.ID, rev2ID
}

func TestDescriptionHistory_Tool(t *testing.T) {
	s := newDescriptionTestServer(t)
	pid, _, _ := seedProductHistory(t, s)

	res, err := s.handleDescriptionHistory(context.Background(), buildReq(map[string]any{
		"target_type": "product",
		"target_id":   pid,
	}))
	if err != nil {
		t.Fatalf("tool: %v", err)
	}
	if isErrResult(res) {
		t.Fatalf("unexpected error result: %s", toolResultText(res))
	}
	var payload struct {
		Items []node.RevisionEntry `json:"items"`
	}
	if err := json.Unmarshal([]byte(toolResultText(res)), &payload); err != nil {
		t.Fatalf("decode: %v raw=%s", err, toolResultText(res))
	}
	if len(payload.Items) != 2 {
		t.Fatalf("want 2 items, got %d", len(payload.Items))
	}
	if payload.Items[0].Body != "v2" {
		t.Fatalf("newest body = %q", payload.Items[0].Body)
	}
}

func TestDescriptionHistory_Tool_RejectsBadTargetType(t *testing.T) {
	s := newDescriptionTestServer(t)
	res, _ := s.handleDescriptionHistory(context.Background(), buildReq(map[string]any{
		"target_type": "peer",
		"target_id":   "x",
	}))
	if !isErrResult(res) {
		t.Fatalf("expected error, got %q", toolResultText(res))
	}
}

func TestDescriptionGetRevision_Tool(t *testing.T) {
	s := newDescriptionTestServer(t)
	pid, olderID, _ := seedProductHistory(t, s)

	res, err := s.handleDescriptionGetRevision(context.Background(), buildReq(map[string]any{
		"target_type": "product",
		"target_id":   pid,
		"revision_id": olderID,
	}))
	if err != nil {
		t.Fatalf("tool: %v", err)
	}
	if isErrResult(res) {
		t.Fatalf("unexpected error: %s", toolResultText(res))
	}
	var rev node.RevisionEntry
	if err := json.Unmarshal([]byte(toolResultText(res)), &rev); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rev.Body != "v1" || rev.Version != 1 {
		t.Fatalf("got body=%q version=%d; want v1 / 1", rev.Body, rev.Version)
	}
}

func TestDescriptionGetRevision_Tool_ForeignRevisionRejected(t *testing.T) {
	s := newDescriptionTestServer(t)
	pid1, _, _ := seedProductHistory(t, s)
	_, olderOther, _ := seedProductHistory(t, s)

	res, _ := s.handleDescriptionGetRevision(context.Background(), buildReq(map[string]any{
		"target_type": "product",
		"target_id":   pid1,
		"revision_id": olderOther, // belongs to the other product
	}))
	if !isErrResult(res) {
		t.Fatalf("expected error; got %s", toolResultText(res))
	}
}

func TestDescriptionRestore_Tool(t *testing.T) {
	s := newDescriptionTestServer(t)
	pid, olderID, _ := seedProductHistory(t, s)

	res, err := s.handleDescriptionRestore(context.Background(), buildReq(map[string]any{
		"target_type": "product",
		"target_id":   pid,
		"revision_id": olderID,
		"author":      "carol",
	}))
	if err != nil {
		t.Fatalf("tool: %v", err)
	}
	if isErrResult(res) {
		t.Fatalf("unexpected error: %s", toolResultText(res))
	}
	msg := toolResultText(res)
	if !strings.Contains(msg, "Restored product") {
		t.Fatalf("unexpected message: %q", msg)
	}

	// Denormalised column rolled back to v1.
	p, err := s.node.Products.Get(pid)
	if err != nil || p == nil {
		t.Fatalf("get: %v", err)
	}
	if p.Description != "v1" {
		t.Fatalf("description after restore = %q want v1", p.Description)
	}

	// History now has 3 rows.
	all, err := s.node.DescriptionHistory(context.Background(), comments.TargetProduct, pid, 100)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("history len=%d want 3", len(all))
	}
	if all[0].Body != "v1" || all[0].Author != "carol" {
		t.Fatalf("newest row = (%s, %s) want (v1, carol)", all[0].Body, all[0].Author)
	}
}

func TestDescriptionRestore_Tool_NoOpWhenCurrent(t *testing.T) {
	s := newDescriptionTestServer(t)
	pid, _, newerID := seedProductHistory(t, s)

	res, err := s.handleDescriptionRestore(context.Background(), buildReq(map[string]any{
		"target_type": "product",
		"target_id":   pid,
		"revision_id": newerID, // body matches current — no-op
	}))
	if err != nil {
		t.Fatalf("tool: %v", err)
	}
	if isErrResult(res) {
		t.Fatalf("unexpected error: %s", toolResultText(res))
	}
	if !strings.Contains(toolResultText(res), "no-op") {
		t.Fatalf("expected no-op message, got: %s", toolResultText(res))
	}
}

