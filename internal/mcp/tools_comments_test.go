package mcp

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
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
		"target_type": "task",
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
	if !strings.Contains(got, "task") || !strings.Contains(got, "execution_review") {
		t.Errorf("expected target/type echoed, got %q", got)
	}
}

func TestCommentAdd_RejectsUnknownType(t *testing.T) {
	s := newTestServerWithComments(t)
	res, err := s.handleCommentAdd(context.Background(), buildReq(map[string]any{
		"target_type": "task",
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
		"target_type": "task",
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
