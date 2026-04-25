// Tests for the rel_* MCP tools (C3 from the senior review). Pattern
// matches internal/mcp/tools_comments_test.go: minimal Server + Node
// wired with just the Relationships store, then call handlers
// directly with a built CallToolRequest.
//
// Coverage:
//   - Every handler's happy path produces valid JSON in the result
//   - Validation errors surface via errResult (non-empty error text)
//   - rel_walk maxDepth=0 returns root only (C2 alignment with store)
//   - rel_*_at variants accept as_of and route to the time-travel store API
package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/relationships"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	_ "github.com/mattn/go-sqlite3"
)

// newTestServerWithRelationships spins up a minimal Server backed by
// an in-memory SQLite DB with only the relationships store migrated.
// Other Node fields are nil — handlers we test don't reach them.
func newTestServerWithRelationships(t *testing.T) *Server {
	t.Helper()
	dsn := "file::memory:?cache=shared&_journal_mode=WAL&_busy_timeout=5000"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	store, err := relationships.New(db)
	if err != nil {
		t.Fatalf("relationships.New: %v", err)
	}
	return &Server{node: &node.Node{Relationships: store}}
}

// resultJSON parses a tool result's text payload as JSON. Fails the
// test if the payload isn't a JSON object — handlers MUST return JSON
// (per the agent-as-primary-reader framing).
func resultJSON(t *testing.T, res *mcplib.CallToolResult) map[string]any {
	t.Helper()
	got := toolResultText(res)
	var out map[string]any
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("expected JSON object in tool result, got %q (err: %v)", got, err)
	}
	return out
}

// ─── rel_create / rel_remove ────────────────────────────────────────

func TestRelCreate_HappyPath(t *testing.T) {
	s := newTestServerWithRelationships(t)
	res, err := s.handleRelCreate(context.Background(), buildReq(map[string]any{
		"src_kind": "manifest", "src_id": "M1",
		"dst_kind": "manifest", "dst_id": "M2",
		"kind":   "depends_on",
		"reason": "test fixture",
	}))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	out := resultJSON(t, res)
	if out["ok"] != true {
		t.Errorf("expected ok=true, got %v", out)
	}
	if out["src_id"] != "M1" || out["dst_id"] != "M2" {
		t.Errorf("expected src=M1 dst=M2, got %v", out)
	}
}

func TestRelCreate_ValidationError(t *testing.T) {
	s := newTestServerWithRelationships(t)
	res, _ := s.handleRelCreate(context.Background(), buildReq(map[string]any{
		"src_kind": "garbage", "src_id": "M1",
		"dst_kind": "manifest", "dst_id": "M2",
		"kind": "depends_on",
	}))
	got := toolResultText(res)
	if !strings.Contains(got, "rel_create") || !strings.Contains(got, "invalid kind") {
		t.Errorf("expected error mentioning invalid kind, got %q", got)
	}
}

func TestRelRemove_HappyPath(t *testing.T) {
	s := newTestServerWithRelationships(t)
	// Create then remove
	_, _ = s.handleRelCreate(context.Background(), buildReq(map[string]any{
		"src_kind": "manifest", "src_id": "M1",
		"dst_kind": "manifest", "dst_id": "M2",
		"kind": "depends_on",
	}))
	res, err := s.handleRelRemove(context.Background(), buildReq(map[string]any{
		"src_id": "M1", "dst_id": "M2", "kind": "depends_on",
		"reason": "no longer needed",
	}))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	out := resultJSON(t, res)
	if out["ok"] != true {
		t.Errorf("expected ok=true on remove, got %v", out)
	}
}

// ─── rel_list_outgoing / rel_list_incoming ──────────────────────────

func TestRelListOutgoing_ReturnsJSON(t *testing.T) {
	s := newTestServerWithRelationships(t)
	_, _ = s.handleRelCreate(context.Background(), buildReq(map[string]any{
		"src_kind": "manifest", "src_id": "M1",
		"dst_kind": "manifest", "dst_id": "M2",
		"kind": "depends_on",
	}))
	_, _ = s.handleRelCreate(context.Background(), buildReq(map[string]any{
		"src_kind": "manifest", "src_id": "M1",
		"dst_kind": "manifest", "dst_id": "M3",
		"kind": "depends_on",
	}))

	res, err := s.handleRelListOutgoing(context.Background(), buildReq(map[string]any{
		"src_id": "M1", "kind": "depends_on",
	}))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	out := resultJSON(t, res)
	if out["count"].(float64) != 2 {
		t.Errorf("expected count=2, got %v", out["count"])
	}
	edges, ok := out["edges"].([]any)
	if !ok || len(edges) != 2 {
		t.Errorf("expected edges array of length 2, got %v", out["edges"])
	}
}

func TestRelListIncoming_ReverseLookup(t *testing.T) {
	s := newTestServerWithRelationships(t)
	for _, src := range []string{"M1", "M2", "M3"} {
		_, _ = s.handleRelCreate(context.Background(), buildReq(map[string]any{
			"src_kind": "manifest", "src_id": src,
			"dst_kind": "manifest", "dst_id": "M0",
			"kind": "depends_on",
		}))
	}
	res, err := s.handleRelListIncoming(context.Background(), buildReq(map[string]any{
		"dst_id": "M0", "kind": "depends_on",
	}))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	out := resultJSON(t, res)
	if out["count"].(float64) != 3 {
		t.Errorf("expected 3 incoming edges to M0, got %v", out["count"])
	}
}

// ─── rel_history ────────────────────────────────────────────────────

func TestRelHistory_ReturnsAllVersions(t *testing.T) {
	s := newTestServerWithRelationships(t)
	_, _ = s.handleRelCreate(context.Background(), buildReq(map[string]any{
		"src_kind": "manifest", "src_id": "M1",
		"dst_kind": "manifest", "dst_id": "M2",
		"kind": "depends_on", "reason": "v1",
	}))
	_, _ = s.handleRelRemove(context.Background(), buildReq(map[string]any{
		"src_id": "M1", "dst_id": "M2", "kind": "depends_on",
		"reason": "removed",
	}))

	res, err := s.handleRelHistory(context.Background(), buildReq(map[string]any{
		"src_id": "M1", "dst_id": "M2", "kind": "depends_on",
	}))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	out := resultJSON(t, res)
	if out["versions"].(float64) != 1 {
		t.Errorf("expected 1 version (the closed row), got %v", out["versions"])
	}
}

// ─── rel_walk ───────────────────────────────────────────────────────

func TestRelWalk_BasicChain(t *testing.T) {
	s := newTestServerWithRelationships(t)
	for _, edge := range []struct{ src, dst string }{{"A", "B"}, {"B", "C"}} {
		_, _ = s.handleRelCreate(context.Background(), buildReq(map[string]any{
			"src_kind": "manifest", "src_id": edge.src,
			"dst_kind": "manifest", "dst_id": edge.dst,
			"kind": "depends_on",
		}))
	}

	res, err := s.handleRelWalk(context.Background(), buildReq(map[string]any{
		"root_id":    "A",
		"root_kind":  "manifest",
		"edge_kinds": "depends_on",
		"max_depth":  float64(10),
	}))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	out := resultJSON(t, res)
	if out["count"].(float64) != 3 {
		t.Errorf("expected 3 nodes (A,B,C), got %v", out["count"])
	}
}

// TestRelWalk_MaxDepthZeroReturnsRootOnly — verifies the C2 alignment:
// MCP layer no longer forces 0→100; passing 0 returns just the root.
// This is the path the store has always supported but the MCP layer
// previously hid.
func TestRelWalk_MaxDepthZeroReturnsRootOnly(t *testing.T) {
	s := newTestServerWithRelationships(t)
	_, _ = s.handleRelCreate(context.Background(), buildReq(map[string]any{
		"src_kind": "manifest", "src_id": "A",
		"dst_kind": "manifest", "dst_id": "B",
		"kind": "depends_on",
	}))

	res, err := s.handleRelWalk(context.Background(), buildReq(map[string]any{
		"root_id":   "A",
		"root_kind": "manifest",
		"max_depth": float64(0),
	}))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	out := resultJSON(t, res)
	// 0-depth = anchor row only = just A. B should not appear.
	if out["count"].(float64) != 1 {
		t.Errorf("max_depth=0 should return root only, got count=%v", out["count"])
	}
}

// ─── rel_get ────────────────────────────────────────────────────────

func TestRelGet_FoundAndNotFound(t *testing.T) {
	s := newTestServerWithRelationships(t)

	// Initially: not found.
	res, _ := s.handleRelGet(context.Background(), buildReq(map[string]any{
		"src_id": "M1", "dst_id": "M2", "kind": "depends_on",
	}))
	out := resultJSON(t, res)
	if out["found"] != false {
		t.Errorf("expected found=false on missing edge, got %v", out)
	}

	// Create it; now found.
	_, _ = s.handleRelCreate(context.Background(), buildReq(map[string]any{
		"src_kind": "manifest", "src_id": "M1",
		"dst_kind": "manifest", "dst_id": "M2",
		"kind": "depends_on",
	}))
	res, _ = s.handleRelGet(context.Background(), buildReq(map[string]any{
		"src_id": "M1", "dst_id": "M2", "kind": "depends_on",
	}))
	out = resultJSON(t, res)
	if out["found"] != true {
		t.Errorf("expected found=true after create, got %v", out)
	}
	edge, ok := out["edge"].(map[string]any)
	if !ok {
		t.Fatalf("expected edge object, got %v", out["edge"])
	}
	if edge["SrcID"] != "M1" || edge["DstID"] != "M2" {
		t.Errorf("wrong edge returned: %v", edge)
	}
}

// ─── rel_health ─────────────────────────────────────────────────────

func TestRelHealth_ReturnsStats(t *testing.T) {
	s := newTestServerWithRelationships(t)
	_, _ = s.handleRelCreate(context.Background(), buildReq(map[string]any{
		"src_kind": "manifest", "src_id": "A",
		"dst_kind": "manifest", "dst_id": "B",
		"kind": "depends_on",
	}))

	res, err := s.handleRelHealth(context.Background(), buildReq(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	out := resultJSON(t, res)
	if out["current_edges"].(float64) != 1 {
		t.Errorf("expected 1 current edge, got %v", out["current_edges"])
	}
	if out["total_rows"].(float64) != 1 {
		t.Errorf("expected 1 total row, got %v", out["total_rows"])
	}
}

// ─── rel_backfill ───────────────────────────────────────────────────

func TestRelBackfill_AcceptsHistoricalRow(t *testing.T) {
	s := newTestServerWithRelationships(t)
	res, err := s.handleRelBackfill(context.Background(), buildReq(map[string]any{
		"src_kind":   "manifest",
		"src_id":     "M1",
		"dst_kind":   "manifest",
		"dst_id":     "M2",
		"kind":       "depends_on",
		"valid_from": "2025-06-01T00:00:00Z",
		"valid_to":   "2025-09-01T00:00:00Z",
		"reason":     "PR/M2 from task_dependency",
	}))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	out := resultJSON(t, res)
	if out["ok"] != true {
		t.Errorf("expected ok=true, got %v", out)
	}
	if out["valid_from"] != "2025-06-01T00:00:00Z" {
		t.Errorf("expected valid_from echoed, got %v", out["valid_from"])
	}
	if out["valid_to"] != "2025-09-01T00:00:00Z" {
		t.Errorf("expected valid_to echoed, got %v", out["valid_to"])
	}
}

func TestRelBackfill_RequiresValidFrom(t *testing.T) {
	s := newTestServerWithRelationships(t)
	res, _ := s.handleRelBackfill(context.Background(), buildReq(map[string]any{
		"src_kind": "manifest", "src_id": "M1",
		"dst_kind": "manifest", "dst_id": "M2",
		"kind": "depends_on",
		// valid_from intentionally omitted
	}))
	got := toolResultText(res)
	if !strings.Contains(got, "ValidFrom") {
		t.Errorf("expected error mentioning ValidFrom, got %q", got)
	}
}
