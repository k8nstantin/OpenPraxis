package web

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/k8nstantin/OpenPraxis/internal/action"
	"github.com/k8nstantin/OpenPraxis/internal/config"
	"github.com/k8nstantin/OpenPraxis/internal/node"

	_ "github.com/mattn/go-sqlite3"
)

// newSearchNode wires the store types exercised by the search handlers
// (actions) over an isolated SQLite file.
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

	actions, err := action.NewStore(db)
	if err != nil {
		t.Fatalf("action.NewStore: %v", err)
	}
	return &node.Node{
		Config:  &config.Config{Node: config.NodeConfig{UUID: "test-peer-uuid"}},
		Actions: actions,
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
	t.Skip("task search migrated to entity search")
}

func TestTasksSearch_EmptyQuery(t *testing.T) {
	t.Skip("task search migrated to entity search")
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
	// Envelope shape per pagination + snippet support: { items, total, offset, limit, has_more }.
	var env struct {
		Items []struct {
			action.Action
			SnippetHTML string `json:"snippet_html"`
		} `json:"items"`
		Total   int  `json:"total"`
		HasMore bool `json:"has_more"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rec.Body.String())
	}
	if env.Total != 1 || len(env.Items) != 1 || env.Items[0].ToolName != "zebra_tool" {
		t.Fatalf("keyword: want zebra_tool, got total=%d items=%+v", env.Total, env.Items)
	}
	if !strings.Contains(env.Items[0].SnippetHTML, "<mark>") {
		t.Fatalf("expected <mark>-wrapped snippet, got %q", env.Items[0].SnippetHTML)
	}
}

// The following tests enforce the response-shape contract from manifest
// 019dac18-638: every GET /api/<type>/search returns a flat JSON array and
// never `null` on empty. They use raw-bytes comparison because nil slices
// marshal to "null" while empty slices marshal to "[]" — and the frontend
// can't distinguish those without guarding every caller.

func TestTasksSearch_EmptyResultIsArrayNotNull(t *testing.T) {
	t.Skip("task search migrated to entity search")
}

func TestActionsSearch_EmptyResultIsEnvelope(t *testing.T) {
	// Actions search moved to the paginated envelope shape so the UI can
	// drive infinite scroll. Empty result is still well-formed: items=[]
	// rather than a bare JSON array.
	n := newSearchNode(t)
	rec := doGET(t, apiActionsSearch(n), "/api/actions/search?q=zzz_no_such_action")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var env struct {
		Items   []any `json:"items"`
		Total   int   `json:"total"`
		HasMore bool  `json:"has_more"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rec.Body.String())
	}
	if env.Total != 0 || len(env.Items) != 0 || env.HasMore {
		t.Fatalf("want empty envelope, got %+v", env)
	}
	if env.Items == nil {
		t.Fatalf("items must be [] not null")
	}
}

// TestTasksSearch_ShapeMatchesListEndpoint — task search migrated to entity search.
func TestTasksSearch_ShapeMatchesListEndpoint(t *testing.T) {
	t.Skip("task search migrated to entity search")
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
