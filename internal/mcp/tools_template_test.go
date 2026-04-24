package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/k8nstantin/OpenPraxis/internal/config"
	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/templates"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

func newTemplatesTestServer(t *testing.T) *Server {
	t.Helper()
	dsn := "file:tmpl?mode=memory&cache=shared&_journal_mode=WAL&_busy_timeout=5000"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := templates.InitSchema(db); err != nil {
		t.Fatalf("schema: %v", err)
	}
	store := templates.NewStore(db)
	if err := templates.Seed(context.Background(), store, "peer-test"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	n := &node.Node{
		Config:    &config.Config{Node: config.NodeConfig{UUID: "peer-test"}},
		Templates: store,
	}
	return &Server{node: n}
}

func callTpl(t *testing.T, h func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error), argsMap map[string]any) *mcplib.CallToolResult {
	t.Helper()
	req := mcplib.CallToolRequest{}
	req.Params.Arguments = argsMap
	res, err := h(context.Background(), req)
	if err != nil {
		t.Fatalf("handler err: %v", err)
	}
	if res == nil {
		t.Fatalf("nil result")
	}
	return res
}

func resText(res *mcplib.CallToolResult) string {
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(mcplib.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}

func TestTemplateCreateSetGetHistory_RoundTrip(t *testing.T) {
	s := newTemplatesTestServer(t)
	create := callTpl(t, s.handleTemplateCreate, map[string]any{
		"scope":    "task",
		"scope_id": "task-xyz",
		"section":  "preamble",
		"title":    "override",
		"body":     "CUSTOM",
		"reason":   "create",
	})
	if create.IsError {
		t.Fatalf("create errored: %s", resText(create))
	}
	var created templates.Template
	if err := json.Unmarshal([]byte(resText(create)), &created); err != nil {
		t.Fatalf("unmarshal create: %v", err)
	}
	if created.Body != "CUSTOM" {
		t.Fatalf("body = %q, want CUSTOM", created.Body)
	}

	set := callTpl(t, s.handleTemplateSet, map[string]any{
		"template_uid": created.TemplateUID,
		"body":         "UPDATED",
		"reason":       "test-set",
	})
	if set.IsError {
		t.Fatalf("set errored: %s", resText(set))
	}
	var updated templates.Template
	if err := json.Unmarshal([]byte(resText(set)), &updated); err != nil {
		t.Fatalf("unmarshal set: %v", err)
	}
	if updated.Body != "UPDATED" {
		t.Fatalf("updated body = %q", updated.Body)
	}

	got := callTpl(t, s.handleTemplateGet, map[string]any{"template_uid": created.TemplateUID})
	if got.IsError {
		t.Fatalf("get errored: %s", resText(got))
	}
	if !strings.Contains(resText(got), "UPDATED") {
		t.Fatalf("get result missing UPDATED: %s", resText(got))
	}

	hist := callTpl(t, s.handleTemplateHistory, map[string]any{"template_uid": created.TemplateUID})
	var histRows []*templates.Template
	if err := json.Unmarshal([]byte(resText(hist)), &histRows); err != nil {
		t.Fatalf("unmarshal history: %v", err)
	}
	if len(histRows) != 2 {
		t.Fatalf("history len = %d, want 2", len(histRows))
	}

	list := callTpl(t, s.handleTemplateList, map[string]any{"scope": "task", "scope_id": "task-xyz"})
	if list.IsError {
		t.Fatalf("list errored: %s", resText(list))
	}

	tomb := callTpl(t, s.handleTemplateTombstone, map[string]any{
		"template_uid": created.TemplateUID,
		"reason":       "cleanup",
	})
	if tomb.IsError {
		t.Fatalf("tombstone errored: %s", resText(tomb))
	}
}

// TestTemplateHandlers_AllReachable invokes every template handler and
// asserts each returns a non-error result. This proves all seven MCP
// tool entry points are wired and backed by the store — the registrar's
// tool registration is exercised indirectly (the handlers are only
// referenced from registerTemplateTools).
func TestTemplateHandlers_AllReachable(t *testing.T) {
	s := newTemplatesTestServer(t)
	create := callTpl(t, s.handleTemplateCreate, map[string]any{
		"scope":    "task",
		"scope_id": "task-disc",
		"section":  "preamble",
		"body":     "DISC",
	})
	if create.IsError {
		t.Fatalf("create: %s", resText(create))
	}
	var tmpl templates.Template
	if err := json.Unmarshal([]byte(resText(create)), &tmpl); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	cases := []struct {
		name string
		h    func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error)
		args map[string]any
	}{
		{"template_get", s.handleTemplateGet, map[string]any{"template_uid": tmpl.TemplateUID}},
		{"template_set", s.handleTemplateSet, map[string]any{"template_uid": tmpl.TemplateUID, "body": "V2"}},
		{"template_history", s.handleTemplateHistory, map[string]any{"template_uid": tmpl.TemplateUID}},
		{"template_list", s.handleTemplateList, map[string]any{"scope": "task"}},
		{"template_at", s.handleTemplateAt, map[string]any{"template_uid": tmpl.TemplateUID, "when": "2099-01-01T00:00:00Z"}},
		{"template_tombstone", s.handleTemplateTombstone, map[string]any{"template_uid": tmpl.TemplateUID}},
	}
	for _, c := range cases {
		res := callTpl(t, c.h, c.args)
		if res.IsError {
			t.Errorf("%s returned error: %s", c.name, resText(res))
		}
	}
}
