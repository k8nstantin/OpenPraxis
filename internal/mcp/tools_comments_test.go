package mcp

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
	"github.com/k8nstantin/OpenPraxis/internal/entity"
	"github.com/k8nstantin/OpenPraxis/internal/node"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	_ "github.com/mattn/go-sqlite3"
)

func newTestServerWithComments(t *testing.T) *Server {
	t.Helper()
	dsn := "file::memory:?cache=shared&_journal_mode=WAL&_busy_timeout=5000"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := comments.InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	return &Server{node: &node.Node{Comments: comments.NewStore(db)}}
}

func buildReq(argMap map[string]any) mcplib.CallToolRequest {
	return mcplib.CallToolRequest{
		Params: mcplib.CallToolParams{Arguments: argMap},
	}
}

func TestCommentAdd_HappyPath_Task(t *testing.T) {
	s := newTestServerWithComments(t)
	res, err := s.handleCommentAdd(context.Background(), buildReq(map[string]any{
		"target_type": "entity",
		"target_id":   "019dab05-5da9-7f0b-b5c2-6f4920c91a69",
		"author":      "agent",
		"type":        "execution_review",
		"body":        "test body — what shipped, gates self-check, followups",
	}))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res == nil {
		t.Fatalf("nil result")
	}
	got := toolResultText(res)
	if !strings.Contains(got, "Comment added") {
		t.Errorf("expected success message, got %q", got)
	}
	if !strings.Contains(got, "entity") || !strings.Contains(got, "comment") {
		t.Errorf("expected target/type echoed, got %q", got)
	}
}

func TestCommentAdd_RejectsUnknownType(t *testing.T) {
	s := newTestServerWithComments(t)
	res, err := s.handleCommentAdd(context.Background(), buildReq(map[string]any{
		"target_type": "entity",
		"target_id":   "t1",
		"author":      "agent",
		"type":        "not_a_real_type",
		"body":        "x",
	}))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !isErrResult(res) {
		t.Fatalf("expected tool error, got %q", toolResultText(res))
	}
}

func TestCommentAdd_RejectsUnknownTargetType(t *testing.T) {
	s := newTestServerWithComments(t)
	res, _ := s.handleCommentAdd(context.Background(), buildReq(map[string]any{
		"target_type": "peer",
		"target_id":   "p1",
		"author":      "agent",
		"type":        "execution_review",
		"body":        "x",
	}))
	if !isErrResult(res) {
		t.Fatalf("expected target_type rejection, got %q", toolResultText(res))
	}
}

func TestCommentAdd_RejectsEmptyBody(t *testing.T) {
	s := newTestServerWithComments(t)
	res, _ := s.handleCommentAdd(context.Background(), buildReq(map[string]any{
		"target_type": "entity",
		"target_id":   "t1",
		"author":      "agent",
		"type":        "execution_review",
		"body":        "",
	}))
	if !isErrResult(res) {
		t.Fatalf("expected empty-body rejection, got %q", toolResultText(res))
	}
}

func toolResultText(r *mcplib.CallToolResult) string {
	if r == nil {
		return ""
	}
	for _, c := range r.Content {
		if tc, ok := c.(mcplib.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

func isErrResult(r *mcplib.CallToolResult) bool {
	if r == nil {
		return false
	}
	return r.IsError
}

// newTestServerWithAllStores wires Comments + Entities
// so the resolveCommentTarget path can validate target full UUIDs.
func newTestServerWithAllStores(t *testing.T) (*Server, *sql.DB) {
	t.Helper()
	dsn := "file::memory:?cache=shared&_journal_mode=WAL&_busy_timeout=5000"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := comments.InitSchema(db); err != nil {
		t.Fatalf("InitSchema comments: %v", err)
	}
	entities, err := entity.NewStore(db)
	if err != nil {
		t.Fatalf("NewStore entities: %v", err)
	}

	n := &node.Node{
		Comments: comments.NewStore(db),
		Entities: entities,
	}
	return &Server{node: n}, db
}

// TestCommentAdd_RejectsShortPrefix_Task — markers are dead. A short
// 12-char prefix must NOT resolve to a task; the handler returns an
// error rather than silently writing an orphan row.
func TestCommentAdd_RejectsShortPrefix_Task(t *testing.T) {
	s, _ := newTestServerWithAllStores(t)

	// Tasks are now entities — create a task entity to get a real UUID.
	tk, err := s.node.Entities.Create(entity.TypeTask, "Example task", entity.StatusActive, nil, "", "test")
	if err != nil {
		t.Fatalf("Create task entity: %v", err)
	}
	taskID := tk.EntityUID
	if len(taskID) != 36 {
		t.Fatalf("expected 36-char UUID, got %q (len=%d)", taskID, len(taskID))
	}

	shortPrefix := taskID[:12]
	res, err := s.handleCommentAdd(context.Background(), buildReq(map[string]any{
		"target_type": "entity",
		"target_id":   shortPrefix,
		"author":      "agent",
		"type":        "review_approval",
		"body":        "**APPROVE** — short prefix should be rejected",
	}))
	if err != nil {
		t.Fatalf("handleCommentAdd: %v", err)
	}
	if !isErrResult(res) {
		t.Fatalf("expected error for short-prefix target_id, got success: %q", toolResultText(res))
	}

	// No comment row should exist for either the short prefix or the full UUID.
	for _, id := range []string{shortPrefix, taskID} {
		cs, err := s.node.Comments.List(context.Background(),
			comments.TargetTask, id, 10, nil)
		if err != nil {
			t.Fatalf("List %q: %v", id, err)
		}
		if len(cs) != 0 {
			t.Fatalf("expected 0 comments for %q, got %d", id, len(cs))
		}
	}
}

func TestCommentAdd_FullUUIDPassthrough_Task(t *testing.T) {
	s, _ := newTestServerWithAllStores(t)
	tk, err := s.node.Entities.Create(entity.TypeTask, "Task", entity.StatusActive, nil, "", "test")
	if err != nil {
		t.Fatalf("Create task entity: %v", err)
	}
	taskID := tk.EntityUID

	res, _ := s.handleCommentAdd(context.Background(), buildReq(map[string]any{
		"target_type": "entity",
		"target_id":   taskID, // full UUID
		"author":      "agent",
		"type":        "execution_review",
		"body":        "x",
	}))
	if isErrResult(res) {
		t.Fatalf("full-UUID should pass through: %q", toolResultText(res))
	}
	cs, _ := s.node.Comments.List(context.Background(),
		comments.TargetTask, taskID, 10, nil)
	if len(cs) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(cs))
	}
}

func TestCommentAdd_RejectsNonExistentTarget_Task(t *testing.T) {
	s, _ := newTestServerWithAllStores(t)

	res, _ := s.handleCommentAdd(context.Background(), buildReq(map[string]any{
		"target_type": "entity",
		"target_id":   "deadbeef-nope",
		"author":      "agent",
		"type":        "execution_review",
		"body":        "x",
	}))
	if !isErrResult(res) {
		t.Fatalf("expected not-found error, got %q", toolResultText(res))
	}
	if !strings.Contains(toolResultText(res), "not found") {
		t.Fatalf("expected 'not found' in error, got %q", toolResultText(res))
	}
}
