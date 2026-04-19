package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
	"github.com/k8nstantin/OpenPraxis/internal/node"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	_ "github.com/mattn/go-sqlite3"
)

// -------- test harness -------------------------------------------------------

type commentsHarness struct {
	db     *sql.DB
	store  *comments.Store
	server *Server
}

func newCommentsHarness(t *testing.T) *commentsHarness {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "comments.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("set WAL: %v", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		t.Fatalf("set busy_timeout: %v", err)
	}
	if err := comments.InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	store := comments.NewStore(db)

	// Build a minimal MCP server with just the comments tools registered.
	// registerCommentsTools only touches s.mcp + s.node.Comments, so we can
	// skip the rest of node.Node construction.
	s := &Server{
		mcp:  mcpserver.NewMCPServer("openpraxis-comments-test", "0.0.0", mcpserver.WithToolCapabilities(true)),
		node: &node.Node{Comments: store},
	}
	s.registerCommentsTools()

	return &commentsHarness{db: db, store: store, server: s}
}

// callTool invokes a registered tool handler directly off the MCPServer's tool
// registry so the test actually exercises the wiring (registration, schema,
// handler) rather than calling the handler method in isolation.
func (h *commentsHarness) callTool(t *testing.T, name string, argMap map[string]any) *mcplib.CallToolResult {
	t.Helper()
	tools := h.server.mcp.ListTools()
	st, ok := tools[name]
	if !ok {
		t.Fatalf("tool %q not registered", name)
	}
	req := mcplib.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = argMap
	res, err := st.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("%s handler returned error: %v", name, err)
	}
	if res == nil {
		t.Fatalf("%s handler returned nil result", name)
	}
	return res
}

func resultText(t *testing.T, res *mcplib.CallToolResult) string {
	t.Helper()
	var text string
	for _, c := range res.Content {
		if tc, ok := c.(mcplib.TextContent); ok {
			text += tc.Text
		}
	}
	return text
}

// expectErrCode asserts the result is an error envelope and the embedded
// {"code":"...","message":"..."} matches the expected code.
func expectErrCode(t *testing.T, res *mcplib.CallToolResult, want string) {
	t.Helper()
	if !res.IsError {
		t.Fatalf("expected error result; got success: %s", resultText(t, res))
	}
	var env struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(resultText(t, res)), &env); err != nil {
		t.Fatalf("error payload is not JSON: %v (raw=%q)", err, resultText(t, res))
	}
	if env.Code != want {
		t.Fatalf("error code: got %q want %q (msg=%q)", env.Code, want, env.Message)
	}
}

func expectOK(t *testing.T, res *mcplib.CallToolResult) string {
	t.Helper()
	if res.IsError {
		t.Fatalf("expected success; got error: %s", resultText(t, res))
	}
	return resultText(t, res)
}

// -------- comment_add --------------------------------------------------------

func TestCommentAdd_RoundTrip(t *testing.T) {
	h := newCommentsHarness(t)

	addRes := h.callTool(t, "comment_add", map[string]any{
		"target_type": "product",
		"target_id":   "p1",
		"author":      "tester",
		"type":        "user_note",
		"body":        "hello",
	})
	var added commentDTO
	if err := json.Unmarshal([]byte(expectOK(t, addRes)), &added); err != nil {
		t.Fatalf("unmarshal add: %v", err)
	}
	if added.Body != "hello" || added.Author != "tester" {
		t.Fatalf("unexpected stored comment: %+v", added)
	}
	if added.ID == "" {
		t.Fatal("missing id in response")
	}

	listRes := h.callTool(t, "comment_list", map[string]any{
		"target_type": "product",
		"target_id":   "p1",
	})
	var listed struct {
		Comments []commentDTO `json:"comments"`
	}
	if err := json.Unmarshal([]byte(expectOK(t, listRes)), &listed); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(listed.Comments) != 1 || listed.Comments[0].ID != added.ID {
		t.Fatalf("round-trip mismatch: %+v", listed.Comments)
	}
}

func TestCommentAdd_EmptyBody(t *testing.T) {
	h := newCommentsHarness(t)
	res := h.callTool(t, "comment_add", map[string]any{
		"target_type": "product",
		"target_id":   "p1",
		"author":      "tester",
		"type":        "user_note",
		"body":        "   ",
	})
	expectErrCode(t, res, CodeEmptyBody)
}

func TestCommentAdd_UnknownType(t *testing.T) {
	h := newCommentsHarness(t)
	res := h.callTool(t, "comment_add", map[string]any{
		"target_type": "product",
		"target_id":   "p1",
		"author":      "tester",
		"type":        "not_a_type",
		"body":        "hi",
	})
	expectErrCode(t, res, CodeUnknownType)
}

func TestCommentAdd_UnknownTargetType(t *testing.T) {
	h := newCommentsHarness(t)
	res := h.callTool(t, "comment_add", map[string]any{
		"target_type": "widget",
		"target_id":   "p1",
		"author":      "tester",
		"type":        "user_note",
		"body":        "hi",
	})
	expectErrCode(t, res, CodeUnknownTargetType)
}

func TestCommentAdd_IsoTimestampsInResponse(t *testing.T) {
	h := newCommentsHarness(t)
	res := h.callTool(t, "comment_add", map[string]any{
		"target_type": "task",
		"target_id":   "t1",
		"author":      "tester",
		"type":        "agent_note",
		"body":        "iso-test",
	})
	var dto commentDTO
	if err := json.Unmarshal([]byte(expectOK(t, res)), &dto); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, err := time.Parse(time.RFC3339, dto.CreatedAtISO); err != nil {
		t.Fatalf("created_at_iso not RFC3339: %q: %v", dto.CreatedAtISO, err)
	}
	if dto.UpdatedAtISO != "" {
		t.Fatalf("updated_at_iso should be empty on fresh add; got %q", dto.UpdatedAtISO)
	}
}

// -------- comment_list -------------------------------------------------------

func TestCommentList_NewestFirst(t *testing.T) {
	h := newCommentsHarness(t)
	ctx := context.Background()

	// Seed via the store directly, spaced by one second to make ordering
	// deterministic since the Add unix-seconds grain is coarse.
	for i, body := range []string{"first", "second", "third"} {
		if _, err := h.store.Add(ctx, comments.TargetProduct, "p1", "tester",
			comments.TypeUserNote, body); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
		time.Sleep(1100 * time.Millisecond)
	}

	res := h.callTool(t, "comment_list", map[string]any{
		"target_type": "product",
		"target_id":   "p1",
	})
	var out struct {
		Comments []commentDTO `json:"comments"`
	}
	if err := json.Unmarshal([]byte(expectOK(t, res)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Comments) != 3 {
		t.Fatalf("expected 3, got %d", len(out.Comments))
	}
	if out.Comments[0].Body != "third" || out.Comments[2].Body != "first" {
		t.Fatalf("not newest-first: %+v", out.Comments)
	}
}

func TestCommentList_TypeFilter(t *testing.T) {
	h := newCommentsHarness(t)
	ctx := context.Background()

	if _, err := h.store.Add(ctx, comments.TargetManifest, "m1", "a",
		comments.TypeUserNote, "note"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := h.store.Add(ctx, comments.TargetManifest, "m1", "a",
		comments.TypeDecision, "decide"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	res := h.callTool(t, "comment_list", map[string]any{
		"target_type": "manifest",
		"target_id":   "m1",
		"type_filter": "decision",
	})
	var out struct {
		Comments []commentDTO `json:"comments"`
	}
	if err := json.Unmarshal([]byte(expectOK(t, res)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Comments) != 1 || out.Comments[0].Body != "decide" {
		t.Fatalf("filter mismatch: %+v", out.Comments)
	}

	// Empty string type_filter behaves as nil (all types).
	res = h.callTool(t, "comment_list", map[string]any{
		"target_type": "manifest",
		"target_id":   "m1",
		"type_filter": "",
	})
	if err := json.Unmarshal([]byte(expectOK(t, res)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Comments) != 2 {
		t.Fatalf("expected 2 with empty filter, got %d", len(out.Comments))
	}

	// Unknown filter type returns structured error.
	res = h.callTool(t, "comment_list", map[string]any{
		"target_type": "manifest",
		"target_id":   "m1",
		"type_filter": "bogus",
	})
	expectErrCode(t, res, CodeUnknownType)
}

func TestCommentList_LimitAndCap(t *testing.T) {
	h := newCommentsHarness(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if _, err := h.store.Add(ctx, comments.TargetTask, "t1", "a",
			comments.TypeAgentNote, "msg"); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	// limit=2 honored
	res := h.callTool(t, "comment_list", map[string]any{
		"target_type": "task",
		"target_id":   "t1",
		"limit":       float64(2),
	})
	var out struct {
		Comments []commentDTO `json:"comments"`
	}
	if err := json.Unmarshal([]byte(expectOK(t, res)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Comments) != 2 {
		t.Fatalf("limit=2 honored: got %d", len(out.Comments))
	}

	// limit=2000 capped server-side at 1000; with only 5 rows seeded the
	// response is 5 — but the request must not error out from oversized limit.
	res = h.callTool(t, "comment_list", map[string]any{
		"target_type": "task",
		"target_id":   "t1",
		"limit":       float64(2000),
	})
	if err := json.Unmarshal([]byte(expectOK(t, res)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Comments) != 5 {
		t.Fatalf("limit=2000 rows: got %d want 5", len(out.Comments))
	}
}

func TestCommentList_ScopeIsolation(t *testing.T) {
	h := newCommentsHarness(t)
	ctx := context.Background()

	if _, err := h.store.Add(ctx, comments.TargetProduct, "X", "a",
		comments.TypeUserNote, "p"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := h.store.Add(ctx, comments.TargetManifest, "X", "a",
		comments.TypeUserNote, "m"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	res := h.callTool(t, "comment_list", map[string]any{
		"target_type": "task",
		"target_id":   "X",
	})
	var out struct {
		Comments []commentDTO `json:"comments"`
	}
	if err := json.Unmarshal([]byte(expectOK(t, res)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Comments) != 0 {
		t.Fatalf("expected isolation, got %+v", out.Comments)
	}
}

// -------- comment_edit -------------------------------------------------------

func TestCommentEdit_UpdatesBodyAndTimestamp(t *testing.T) {
	h := newCommentsHarness(t)
	ctx := context.Background()

	c, err := h.store.Add(ctx, comments.TargetProduct, "p1", "a",
		comments.TypeUserNote, "before")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	time.Sleep(1100 * time.Millisecond)
	res := h.callTool(t, "comment_edit", map[string]any{
		"id":   c.ID,
		"body": "after",
	})
	var dto commentDTO
	if err := json.Unmarshal([]byte(expectOK(t, res)), &dto); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if dto.Body != "after" {
		t.Fatalf("body not updated: %q", dto.Body)
	}
	if dto.UpdatedAt == nil || dto.UpdatedAtISO == "" {
		t.Fatalf("updated_at missing after edit: %+v", dto)
	}
	if _, err := time.Parse(time.RFC3339, dto.UpdatedAtISO); err != nil {
		t.Fatalf("updated_at_iso bad: %v", err)
	}
}

func TestCommentEdit_NotFound(t *testing.T) {
	h := newCommentsHarness(t)
	res := h.callTool(t, "comment_edit", map[string]any{
		"id":   "does-not-exist",
		"body": "new body",
	})
	expectErrCode(t, res, CodeNotFound)
}

func TestCommentEdit_EmptyBody(t *testing.T) {
	h := newCommentsHarness(t)
	ctx := context.Background()
	c, err := h.store.Add(ctx, comments.TargetProduct, "p1", "a",
		comments.TypeUserNote, "body")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	res := h.callTool(t, "comment_edit", map[string]any{
		"id":   c.ID,
		"body": "   ",
	})
	expectErrCode(t, res, CodeEmptyBody)
}

// -------- comment_delete -----------------------------------------------------

func TestCommentDelete_Idempotent(t *testing.T) {
	h := newCommentsHarness(t)
	ctx := context.Background()

	c, err := h.store.Add(ctx, comments.TargetProduct, "p1", "a",
		comments.TypeUserNote, "body")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	res := h.callTool(t, "comment_delete", map[string]any{"id": c.ID})
	if s := expectOK(t, res); !strings.Contains(s, `"ok": true`) {
		t.Fatalf("first delete payload: %s", s)
	}
	res = h.callTool(t, "comment_delete", map[string]any{"id": c.ID})
	if s := expectOK(t, res); !strings.Contains(s, `"ok": true`) {
		t.Fatalf("second delete payload: %s", s)
	}
}

// -------- registration smoke -------------------------------------------------

func TestCommentTools_Registered(t *testing.T) {
	h := newCommentsHarness(t)
	tools := h.server.mcp.ListTools()

	for _, name := range []string{"comment_add", "comment_list", "comment_edit", "comment_delete"} {
		st, ok := tools[name]
		if !ok {
			t.Fatalf("%s not registered", name)
		}
		if st.Tool.Description == "" {
			t.Errorf("%s has empty description", name)
		}
	}

	// The add/list tool descriptions must reference every type from
	// Registry() so a newly added type shows up inline automatically.
	addDesc := tools["comment_add"].Tool.Description
	for _, info := range comments.Registry() {
		if !strings.Contains(addDesc, string(info.Type)) {
			t.Errorf("comment_add description missing type %q", info.Type)
		}
	}

	// Required-argument schema checks.
	for name, wantRequired := range map[string][]string{
		"comment_add":    {"target_type", "target_id", "author", "type", "body"},
		"comment_list":   {"target_type", "target_id"},
		"comment_edit":   {"id", "body"},
		"comment_delete": {"id"},
	} {
		got := tools[name].Tool.InputSchema.Required
		gotSet := map[string]bool{}
		for _, r := range got {
			gotSet[r] = true
		}
		for _, r := range wantRequired {
			if !gotSet[r] {
				t.Errorf("%s schema missing required %q (have %v)", name, r, got)
			}
		}
	}
}
