package web

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gorilla/mux"
	_ "github.com/mattn/go-sqlite3"

	"github.com/k8nstantin/OpenPraxis/internal/config"
	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/templates"
)

type templateTestEnv struct {
	node   *node.Node
	server *httptest.Server
	store  *templates.Store
}

func newTemplateTestEnv(t *testing.T) *templateTestEnv {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "tpl.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
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
	r := mux.NewRouter()
	api := r.PathPrefix("/api").Subrouter()
	registerTemplateRoutes(api, n)
	srv := httptest.NewServer(r)
	t.Cleanup(func() {
		srv.Close()
		db.Close()
	})
	return &templateTestEnv{node: n, server: srv, store: store}
}

func (e *templateTestEnv) do(t *testing.T, method, path string, body any) (*http.Response, []byte) {
	t.Helper()
	var buf io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.server.URL+path, buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, respBody
}

// TestTemplate_PutCloses_PriorAndAppendsNew covers acceptances 1, 2, 5:
// a PUT against the system preamble uid closes the prior row and the
// NEXT resolve for any task sees the new body (no restart needed).
func TestTemplate_PutCloses_PriorAndAppendsNew(t *testing.T) {
	env := newTemplateTestEnv(t)
	// Locate the seeded preamble row.
	rows, err := env.store.List(context.Background(), templates.ScopeSystem, templates.SectionPreamble)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 preamble row, got %d", len(rows))
	}
	uid := rows[0].TemplateUID

	resp, body := env.do(t, http.MethodPut, "/api/templates/"+uid,
		map[string]string{"body": "NEW_PREAMBLE", "changed_by": "tester", "reason": "rc-m2-test"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT status = %d: %s", resp.StatusCode, body)
	}

	// History newest-first.
	resp, body = env.do(t, http.MethodGet, "/api/templates/"+uid+"/history", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("history status = %d", resp.StatusCode)
	}
	var hist []*templates.Template
	if err := json.Unmarshal(body, &hist); err != nil {
		t.Fatalf("hist unmarshal: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("history len = %d, want 2", len(hist))
	}
	if hist[0].Body != "NEW_PREAMBLE" || hist[0].ValidTo != "" {
		t.Fatalf("newest row wrong: body=%q valid_to=%q", hist[0].Body, hist[0].ValidTo)
	}
	if hist[0].ChangedBy == "" || hist[0].Reason == "" {
		t.Fatalf("audit columns blank on new row")
	}
	if hist[1].ValidTo == "" {
		t.Fatalf("older row should be closed")
	}

	// Resolver: the next spawn sees the new body.
	resolver := templates.NewResolver(env.store, nil, nil)
	got, err := resolver.Resolve(context.Background(), templates.SectionPreamble, "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "NEW_PREAMBLE" {
		t.Fatalf("resolver got %q, want NEW_PREAMBLE", got)
	}
}

func TestTemplate_PostCreatesOverride(t *testing.T) {
	env := newTemplateTestEnv(t)
	resp, body := env.do(t, http.MethodPost, "/api/templates", map[string]string{
		"scope":      "task",
		"scope_id":   "task-1",
		"section":    "preamble",
		"title":      "override",
		"body":       "OVERRIDDEN",
		"changed_by": "tester",
		"reason":     "test",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
	var row templates.Template
	if err := json.Unmarshal(body, &row); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if row.TemplateUID == "" {
		t.Fatalf("empty uid")
	}
	// Duplicate rejected.
	resp2, _ := env.do(t, http.MethodPost, "/api/templates", map[string]string{
		"scope": "task", "scope_id": "task-1", "section": "preamble", "body": "x",
	})
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate status = %d, want 409", resp2.StatusCode)
	}
}

func TestTemplate_AtTime(t *testing.T) {
	env := newTemplateTestEnv(t)
	rows, _ := env.store.List(context.Background(), templates.ScopeSystem, templates.SectionPreamble)
	uid := rows[0].TemplateUID

	resp, _ := env.do(t, http.MethodGet, "/api/templates/"+uid+"/at?t=2099-01-01T00:00:00Z", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("at status = %d", resp.StatusCode)
	}
	resp2, _ := env.do(t, http.MethodGet, "/api/templates/"+uid+"/at?t=1999-01-01T00:00:00Z", nil)
	if resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("at before seed status = %d, want 404", resp2.StatusCode)
	}
}

// TestTemplate_Preview covers acceptance 6: the preview endpoint renders
// the template body against task data via text/template and returns a
// structured error inline rather than crashing.
func TestTemplate_Preview(t *testing.T) {
	env := newTemplateTestEnv(t)
	uid, err := env.store.Create(context.Background(), templates.ScopeTask, "task-xyz",
		templates.SectionPreamble, "override", "Hello {{.Task.ID}}", "tester", "init")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// No task store wired on this env → .Task.ID is empty; render still succeeds.
	resp, body := env.do(t, http.MethodGet,
		"/api/templates/preview?template_uid="+uid+"&task_id=task-xyz", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preview status = %d: %s", resp.StatusCode, body)
	}
	var out map[string]string
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out["rendered"] != "Hello " {
		t.Fatalf("rendered = %q, want %q", out["rendered"], "Hello ")
	}

	// Broken template → surfaces [render error:...] payload, not a 500.
	broken, err := env.store.Create(context.Background(), templates.ScopeTask, "task-broken",
		templates.SectionVisceralRules, "bad", "{{.Nope", "tester", "init")
	if err != nil {
		t.Fatalf("create broken: %v", err)
	}
	resp2, body2 := env.do(t, http.MethodGet,
		"/api/templates/preview?template_uid="+broken, nil)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("broken preview status = %d: %s", resp2.StatusCode, body2)
	}
	var out2 map[string]string
	if err := json.Unmarshal(body2, &out2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out2["error"] == "" {
		t.Fatalf("expected error field populated, got %q", out2["error"])
	}
}

// TestTemplate_Restore covers acceptances 4 + 5: POST /restore fetches
// the body active at from_valid_from and appends a new history row with
// reason="restored from <ts>".
func TestTemplate_Restore(t *testing.T) {
	env := newTemplateTestEnv(t)
	rows, _ := env.store.List(context.Background(), templates.ScopeSystem, templates.SectionPreamble)
	uid := rows[0].TemplateUID
	origBody := rows[0].Body
	origTS := rows[0].ValidFrom

	// Move forward: v2.
	resp, body := env.do(t, http.MethodPut, "/api/templates/"+uid,
		map[string]string{"body": "v2-body", "changed_by": "tester", "reason": "v2"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT v2 status = %d: %s", resp.StatusCode, body)
	}

	// Restore the seeded v1 by its valid_from.
	resp2, body2 := env.do(t, http.MethodPost,
		"/api/templates/"+uid+"/restore?from_valid_from="+origTS+"&changed_by=tester", nil)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("restore status = %d: %s", resp2.StatusCode, body2)
	}

	// Current body now matches v1; history has 3 rows with the newest reason.
	resp3, body3 := env.do(t, http.MethodGet, "/api/templates/"+uid+"/history", nil)
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("history status = %d", resp3.StatusCode)
	}
	var hist []*templates.Template
	if err := json.Unmarshal(body3, &hist); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(hist) != 3 {
		t.Fatalf("history len = %d, want 3", len(hist))
	}
	if hist[0].Body != origBody {
		t.Fatalf("restored body = %q, want %q", hist[0].Body, origBody)
	}
	if hist[0].Reason == "" || hist[0].Reason[:8] != "restored" {
		t.Fatalf("restored reason = %q, want prefix 'restored'", hist[0].Reason)
	}

	// Bad timestamp → 404.
	resp4, _ := env.do(t, http.MethodPost,
		"/api/templates/"+uid+"/restore?from_valid_from=1900-01-01T00:00:00Z", nil)
	if resp4.StatusCode != http.StatusNotFound {
		t.Fatalf("restore-missing status = %d, want 404", resp4.StatusCode)
	}
}

func TestTemplate_DeleteTombstones(t *testing.T) {
	env := newTemplateTestEnv(t)
	uid, err := env.store.Create(context.Background(), templates.ScopeTask, "task-1",
		templates.SectionPreamble, "override", "OVERRIDE", "test", "init")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	resp, body := env.do(t, http.MethodDelete, "/api/templates/"+uid+"?reason=cleanup", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d: %s", resp.StatusCode, body)
	}
	// Resolver should fall through to system now.
	resolver := templates.NewResolver(env.store, nil, nil)
	got, err := resolver.Resolve(context.Background(), templates.SectionPreamble, "task-1")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got == "OVERRIDE" {
		t.Fatalf("tombstoned row still winning")
	}
}
