package web

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/k8nstantin/OpenPraxis/internal/action"
	"github.com/k8nstantin/OpenPraxis/internal/config"
	"github.com/k8nstantin/OpenPraxis/internal/idea"
	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/product"
	"github.com/k8nstantin/OpenPraxis/internal/task"

	_ "github.com/mattn/go-sqlite3"
)

// newSearchNode wires the 4 store types exercised by the search handlers
// (tasks, products, ideas, actions) over an isolated SQLite file. The
// memory/conversation/manifest stores are left nil because the tests in
// this file only cover the NEW routes (tasks/products/ideas/actions) plus
// the GET-alias handlers — the aliases for memories/manifests/conversations
// are thin shims over existing semantic-search paths exercised by their
// existing POST-path test coverage.
func newSearchNode(t *testing.T) *node.Node {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "search.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("set WAL: %v", err)
	}

	tasks, err := task.NewStore(db)
	if err != nil {
		t.Fatalf("task.NewStore: %v", err)
	}
	products, err := product.NewStore(db)
	if err != nil {
		t.Fatalf("product.NewStore: %v", err)
	}
	ideas, err := idea.NewStore(db)
	if err != nil {
		t.Fatalf("idea.NewStore: %v", err)
	}
	actions, err := action.NewStore(db)
	if err != nil {
		t.Fatalf("action.NewStore: %v", err)
	}
	return &node.Node{
		Config:   &config.Config{Node: config.NodeConfig{UUID: "test-peer-uuid"}},
		Tasks:    tasks,
		Products: products,
		Ideas:    ideas,
		Actions:  actions,
	}
}

func doGET(t *testing.T, h http.HandlerFunc, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestTasksSearch_KeywordAndID(t *testing.T) {
	n := newSearchNode(t)
	a, _ := n.Tasks.Create("", "alpha-task", "desc", "once", "claude-code", n.PeerID(), "test", "")
	_, _ = n.Tasks.Create("", "beta-task", "desc", "once", "claude-code", n.PeerID(), "test", "")

	rec := doGET(t, apiTasksSearch(n), "/api/tasks/search?q=alpha")
	if rec.Code != 200 {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	var got []task.Task
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("keyword: want [%s], got %+v", a.ID, got)
	}

	// id-exact
	rec = doGET(t, apiTasksSearch(n), "/api/tasks/search?q="+a.ID)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("id-exact: want [%s], got %+v", a.ID, got)
	}
}

func TestTasksSearch_EmptyQuery(t *testing.T) {
	n := newSearchNode(t)
	_, _ = n.Tasks.Create("", "alpha", "", "once", "claude-code", n.PeerID(), "test", "")
	rec := doGET(t, apiTasksSearch(n), "/api/tasks/search?q=")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var got []task.Task
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 0 {
		t.Fatalf("empty q should yield empty, got %+v", got)
	}
}

func TestProductsSearch_KeywordAndPrefix(t *testing.T) {
	n := newSearchNode(t)
	a, _ := n.Products.Create("Alpha widget", "", "open", "node", nil)
	_, _ = n.Products.Create("Beta gizmo", "", "open", "node", nil)

	rec := doGET(t, apiProductsSearch(n), "/api/products/search?q=widget&limit=10")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var got []product.Product
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("keyword: want [%s], got %+v", a.ID, got)
	}

	rec = doGET(t, apiProductsSearch(n), "/api/products/search?q="+a.Marker)
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	found := false
	for _, p := range got {
		if p.ID == a.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("prefix: want %s in results, got %+v", a.ID, got)
	}
}

func TestIdeasSearch_Keyword(t *testing.T) {
	n := newSearchNode(t)
	a, _ := n.Ideas.Create("RocketIdea", "d", "open", "low", "me", "node", "", nil)
	_, _ = n.Ideas.Create("OtherThing", "d", "open", "low", "me", "node", "", nil)

	rec := doGET(t, apiIdeasSearch(n), "/api/ideas/search?q=rocket")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var got []idea.Idea
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("keyword: want [%s], got %+v", a.ID, got)
	}
}

func TestIdeasSearch_Unknown(t *testing.T) {
	n := newSearchNode(t)
	_, _ = n.Ideas.Create("SomeIdea", "d", "open", "low", "me", "node", "", nil)
	rec := doGET(t, apiIdeasSearch(n), "/api/ideas/search?q=nomatchxyz987")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var got []idea.Idea
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 0 {
		t.Fatalf("unknown query should yield empty, got %+v", got)
	}
}

func TestActionsSearch_Keyword(t *testing.T) {
	n := newSearchNode(t)
	if err := n.Actions.Record("s1", "node", "zebra_tool", "{}", "{}", "/tmp"); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := n.Actions.Record("s1", "node", "other_tool", "{}", "{}", "/tmp"); err != nil {
		t.Fatalf("record: %v", err)
	}

	rec := doGET(t, apiActionsSearch(n), "/api/actions/search?q=zebra")
	if rec.Code != 200 {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	var got []action.Action
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 1 || got[0].ToolName != "zebra_tool" {
		t.Fatalf("keyword: want zebra_tool, got %+v", got)
	}
}

// The following tests enforce the response-shape contract from manifest
// 019dac18-638: every GET /api/<type>/search returns a flat JSON array and
// never `null` on empty. They use raw-bytes comparison because nil slices
// marshal to "null" while empty slices marshal to "[]" — and the frontend
// can't distinguish those without guarding every caller.

func TestTasksSearch_EmptyResultIsArrayNotNull(t *testing.T) {
	n := newSearchNode(t)
	rec := doGET(t, apiTasksSearch(n), "/api/tasks/search?q=zzz_no_such_task")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	if body := rec.Body.String(); body != "[]" && body != "[]\n" {
		t.Fatalf("want `[]`, got %q", body)
	}
}

func TestProductsSearch_EmptyResultIsArrayNotNull(t *testing.T) {
	n := newSearchNode(t)
	rec := doGET(t, apiProductsSearch(n), "/api/products/search?q=zzz_no_such_product")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	if body := rec.Body.String(); body != "[]" && body != "[]\n" {
		t.Fatalf("want `[]`, got %q", body)
	}
}

func TestIdeasSearch_EmptyResultIsArrayNotNull(t *testing.T) {
	n := newSearchNode(t)
	rec := doGET(t, apiIdeasSearch(n), "/api/ideas/search?q=zzz_no_such_idea")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	if body := rec.Body.String(); body != "[]" && body != "[]\n" {
		t.Fatalf("want `[]`, got %q", body)
	}
}

func TestActionsSearch_EmptyResultIsArrayNotNull(t *testing.T) {
	n := newSearchNode(t)
	rec := doGET(t, apiActionsSearch(n), "/api/actions/search?q=zzz_no_such_action")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	if body := rec.Body.String(); body != "[]" && body != "[]\n" {
		t.Fatalf("want `[]`, got %q", body)
	}
}

// TestTasksSearch_ShapeMatchesListEndpoint asserts that a successful task
// search returns entities flat (no outer {task: ...} wrapper), so that the
// frontend can render `r.id`/`r.title` directly.
func TestTasksSearch_ShapeMatchesListEndpoint(t *testing.T) {
	n := newSearchNode(t)
	a, _ := n.Tasks.Create("", "shape-alpha", "desc", "once", "claude-code", n.PeerID(), "test", "")
	rec := doGET(t, apiTasksSearch(n), "/api/tasks/search?q=shape-alpha")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var raw []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	if len(raw) != 1 {
		t.Fatalf("want 1 result, got %d", len(raw))
	}
	if _, wrapped := raw[0]["task"]; wrapped {
		t.Fatalf("search result is wrapped under 'task' key; want flat: %+v", raw[0])
	}
	if id, _ := raw[0]["id"].(string); id != a.ID {
		t.Fatalf("want id=%s, got %+v", a.ID, raw[0])
	}
}

func TestParseSearchParams_LimitAndAlias(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/x/search?query=hello&limit=7", nil)
	q, lim := parseSearchParams(req)
	if q != "hello" {
		t.Fatalf("q: got %q", q)
	}
	if lim != 7 {
		t.Fatalf("limit: got %d", lim)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/x/search", nil)
	q, lim = parseSearchParams(req)
	if q != "" || lim != 50 {
		t.Fatalf("defaults: got q=%q lim=%d", q, lim)
	}
}
