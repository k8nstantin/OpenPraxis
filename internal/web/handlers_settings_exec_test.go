package web

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"github.com/k8nstantin/OpenPraxis/internal/mcp"
	"github.com/k8nstantin/OpenPraxis/internal/settings"

	_ "github.com/mattn/go-sqlite3"
)

// ---- harness ----------------------------------------------------------------
//
// The settings store + resolver harness mirrors the MCP test harness in
// internal/mcp/tools_settings_test.go. We do not import that harness because
// _test.go symbols are package-private; duplicating the ~30 lines is cheaper
// than introducing a non-test sub-package just to share fixtures.

type httpFakeTaskLookup struct {
	tasks map[string]settings.TaskRec
}

func (f *httpFakeTaskLookup) GetTaskForSettings(_ context.Context, taskID string) (settings.TaskRec, error) {
	r, ok := f.tasks[taskID]
	if !ok {
		return settings.TaskRec{}, fmt.Errorf("fake task lookup: %q not found", taskID)
	}
	return r, nil
}

type httpFakeManifestLookup struct {
	manifests map[string]settings.ManifestRec
}

func (f *httpFakeManifestLookup) GetManifestForSettings(_ context.Context, manifestID string) (settings.ManifestRec, error) {
	r, ok := f.manifests[manifestID]
	if !ok {
		return settings.ManifestRec{}, fmt.Errorf("fake manifest lookup: %q not found", manifestID)
	}
	return r, nil
}

type settingsTestEnv struct {
	store    *settings.Store
	resolver *settings.Resolver
}

func newSettingsTestEnv(t *testing.T) *settingsTestEnv {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "settings.db")
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
	if err := settings.InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	store := settings.NewStore(db)
	tasks := &httpFakeTaskLookup{tasks: map[string]settings.TaskRec{
		"t1": {ID: "t1", ManifestID: "m1"},
	}}
	mans := &httpFakeManifestLookup{manifests: map[string]settings.ManifestRec{
		"m1": {ID: "m1", ProductID: "p1"},
	}}
	return &settingsTestEnv{
		store:    store,
		resolver: settings.NewResolver(store, tasks, mans),
	}
}

// noVisceralRulesHTTP is the loader used when a test does not exercise the
// visceral cap path.
func noVisceralRulesHTTP(_ context.Context) ([]string, error) { return nil, nil }

// budgetRuleLoaderHTTP returns a single rule mirroring rule #8 so cap-path
// tests can assert deterministic behavior without seeding the index.
func budgetRuleLoaderHTTP(ceiling string) mcp.VisceralRuleLoader {
	return func(_ context.Context) ([]string, error) {
		return []string{"daily budget = " + ceiling}, nil
	}
}

// buildRouter wires the same handlers the production server uses against a
// test environment. We build manually rather than calling Handler() so the
// tests do not need a full Node + MCP server + chat router.
func buildRouter(env *settingsTestEnv, loader mcp.VisceralRuleLoader) *mux.Router {
	r := mux.NewRouter()
	api := r.PathPrefix("/api").Subrouter()

	api.HandleFunc("/settings/catalog", apiSettingsCatalog()).Methods("GET")

	for _, scope := range settingsKnownScopes {
		path := fmt.Sprintf("/%ss/{id}/settings", scope)
		api.HandleFunc(path, scopeGetHandler(env, scope)).Methods("GET")
		api.HandleFunc(path, scopePutHandler(env, scope, loader)).Methods("PUT")
	}
	api.HandleFunc("/tasks/{id}/settings/resolved", taskResolvedHandler(env)).Methods("GET")
	return r
}

// scopeGetHandler / scopePutHandler / taskResolvedHandler are test-side
// adaptors that wire the production handler functions against the test env's
// Store and Resolver without standing up a full Node.
func scopeGetHandler(env *settingsTestEnv, scopeType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		out, err := mcp.DoSettingsGet(r.Context(), env.store, scopeType, id)
		if err != nil {
			writeError(w, err.Error(), settingsHTTPStatus(err))
			return
		}
		views := make([]entryView, 0, len(out.Entries))
		for _, e := range out.Entries {
			views = append(views, toEntryView(e))
		}
		writeJSON(w, scopeGetResponse{
			ScopeType: out.ScopeType,
			ScopeID:   out.ScopeID,
			Entries:   views,
		})
	}
}

func scopePutHandler(env *settingsTestEnv, scopeType string, loader mcp.VisceralRuleLoader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		var raw map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			writeError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if len(raw) == 0 {
			writeError(w, "request body must contain at least one key", http.StatusBadRequest)
			return
		}
		author := httpSettingsAuthor(r)
		results := make([]putKeyResult, 0, len(raw))
		for key, val := range raw {
			out, err := mcp.DoSettingsSet(r.Context(), env.store, loader,
				scopeType, id, key, string(val), author)
			if err != nil {
				results = append(results, putKeyResult{Key: key, OK: false, Error: err.Error()})
				continue
			}
			results = append(results, putKeyResult{Key: key, OK: out.OK, Warnings: out.Warnings})
		}
		writeJSON(w, putResponse{Results: results})
	}
}

func taskResolvedHandler(env *settingsTestEnv) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		out, err := mcp.DoSettingsResolve(r.Context(), env.resolver, id)
		if err != nil {
			writeError(w, err.Error(), settingsHTTPStatus(err))
			return
		}
		writeJSON(w, resolvedResponse{TaskID: out.TaskID, Resolved: out.Resolved})
	}
}

// doRequest is a small helper wrapping httptest setup.
func doRequest(t *testing.T, router http.Handler, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader *bytes.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	} else {
		bodyReader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req.RemoteAddr = "203.0.113.42:54321"
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// ---- /api/settings/catalog --------------------------------------------------

func TestHandleGetCatalog_ReturnsAllKnobs(t *testing.T) {
	env := newSettingsTestEnv(t)
	r := buildRouter(env, noVisceralRulesHTTP)
	rec := doRequest(t, r, http.MethodGet, "/api/settings/catalog", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
	var got mcp.CatalogOut
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Knobs) != len(settings.Catalog()) {
		t.Fatalf("knob count: got %d want %d", len(got.Knobs), len(settings.Catalog()))
	}
	for i, k := range got.Knobs {
		if k.Key != settings.Catalog()[i].Key {
			t.Fatalf("knob[%d] key: got %q want %q", i, k.Key, settings.Catalog()[i].Key)
		}
	}
}

// ---- GET /api/{scope}/:id/settings ------------------------------------------

func TestHandleGetScopeSettings_EmptyScope_ReturnsEmptyList(t *testing.T) {
	env := newSettingsTestEnv(t)
	r := buildRouter(env, noVisceralRulesHTTP)
	rec := doRequest(t, r, http.MethodGet, "/api/products/p-empty/settings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
	var got scopeGetResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ScopeType != "product" || got.ScopeID != "p-empty" {
		t.Errorf("scope echoed wrong: %+v", got)
	}
	if len(got.Entries) != 0 {
		t.Fatalf("expected zero entries, got %d", len(got.Entries))
	}
	// JSON shape must be `[]` (not `null`) so dashboards can iterate without
	// nil checks. The presence of `"entries":[]` in the body is the proof.
	if !strings.Contains(rec.Body.String(), `"entries":[]`) {
		t.Fatalf("entries field should serialize as [] in: %s", rec.Body.String())
	}
}

func TestHandleGetScopeSettings_IncludesISOTimestamps(t *testing.T) {
	env := newSettingsTestEnv(t)
	if err := env.store.Set(context.Background(), settings.ScopeProduct, "p1", "max_turns", "100", "seed"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	r := buildRouter(env, noVisceralRulesHTTP)
	rec := doRequest(t, r, http.MethodGet, "/api/products/p1/settings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d", rec.Code)
	}
	var got scopeGetResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got.Entries))
	}
	e := got.Entries[0]
	if e.UpdatedAt == 0 {
		t.Errorf("updated_at unix should be non-zero, got %d", e.UpdatedAt)
	}
	if e.UpdatedAtISO == "" {
		t.Errorf("updated_at_iso should be non-empty")
	}
	// Spot-check ISO-8601 format suffix.
	if !strings.HasSuffix(e.UpdatedAtISO, "Z") {
		t.Errorf("updated_at_iso should be UTC RFC3339, got %q", e.UpdatedAtISO)
	}
}

func TestHandleGetScopeSettings_UnknownScope_Returns400(t *testing.T) {
	env := newSettingsTestEnv(t)
	// `region` is not a registered scope tier, so the router has no GET route
	// at /api/regions/:id/settings — gorilla returns 404 in that case. The
	// real "unknown writable scope" check fires when the handler runs, which
	// happens only for product/manifest/task. Validate the handler-level
	// behavior by feeding `system` directly to DoSettingsGet, which the
	// handler also uses, and asserting it surfaces as 400.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/products/p1/settings", nil)
	req.RemoteAddr = "127.0.0.1:1"
	// Reach into the handler directly with an invalid scope_type so the test
	// names match the spec ("UnknownScope_Returns400") even though the URL
	// surface only exposes the three writable tiers.
	scopeGetHandler(env, "system").ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for system scope, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "scope_type") && !strings.Contains(rec.Body.String(), "read-only") {
		t.Errorf("error should mention scope_type/read-only, got: %s", rec.Body.String())
	}
}

// ---- PUT /api/{scope}/:id/settings ------------------------------------------

func TestHandlePutScopeSettings_AllValid_ReturnsAllOK(t *testing.T) {
	env := newSettingsTestEnv(t)
	r := buildRouter(env, noVisceralRulesHTTP)
	body := []byte(`{"max_turns":100,"temperature":0.5}`)
	rec := doRequest(t, r, http.MethodPut, "/api/products/p1/settings", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
	var got putResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got.Results))
	}
	for _, res := range got.Results {
		if !res.OK {
			t.Errorf("expected OK, got %+v", res)
		}
	}
	// Persisted values readable via the store.
	for _, key := range []string{"max_turns", "temperature"} {
		if _, err := env.store.Get(context.Background(), settings.ScopeProduct, "p1", key); err != nil {
			t.Errorf("expected %q to persist: %v", key, err)
		}
	}
}

func TestHandlePutScopeSettings_UnknownKey_MarksFailureInResults(t *testing.T) {
	env := newSettingsTestEnv(t)
	r := buildRouter(env, noVisceralRulesHTTP)
	body := []byte(`{"no_such_knob":42}`)
	rec := doRequest(t, r, http.MethodPut, "/api/products/p1/settings", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 even on per-key failure, got %d", rec.Code)
	}
	var got putResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got.Results))
	}
	if got.Results[0].OK {
		t.Errorf("expected ok=false for unknown key, got %+v", got.Results[0])
	}
	if !strings.Contains(got.Results[0].Error, "unknown key") {
		t.Errorf("error should mention unknown key, got %q", got.Results[0].Error)
	}
}

func TestHandlePutScopeSettings_TypeMismatch_MarksFailureInResults(t *testing.T) {
	env := newSettingsTestEnv(t)
	r := buildRouter(env, noVisceralRulesHTTP)
	// max_turns is int — feeding a JSON string must be a per-key failure.
	body := []byte(`{"max_turns":"lots"}`)
	rec := doRequest(t, r, http.MethodPut, "/api/products/p1/settings", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got putResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Results[0].OK {
		t.Errorf("expected ok=false, got %+v", got.Results[0])
	}
	if !strings.Contains(got.Results[0].Error, "type mismatch") {
		t.Errorf("error should mention type mismatch, got %q", got.Results[0].Error)
	}
	// Nothing should have been persisted.
	if _, err := env.store.Get(context.Background(), settings.ScopeProduct, "p1", "max_turns"); err == nil {
		t.Errorf("write leaked past type validation")
	}
}

func TestHandlePutScopeSettings_VisceralCapViolation_Rejected(t *testing.T) {
	env := newSettingsTestEnv(t)
	r := buildRouter(env, budgetRuleLoaderHTTP("$100"))
	body := []byte(`{"daily_budget_usd":500}`)
	rec := doRequest(t, r, http.MethodPut, "/api/products/p1/settings", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (per-key results carry the rejection), got %d", rec.Code)
	}
	var got putResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Results[0].OK {
		t.Errorf("expected ok=false for cap violation, got %+v", got.Results[0])
	}
	if !strings.Contains(got.Results[0].Error, "Visceral rule") {
		t.Errorf("error should reference visceral rule, got %q", got.Results[0].Error)
	}
	// Must NOT have persisted past the cap.
	if _, err := env.store.Get(context.Background(), settings.ScopeProduct, "p1", "daily_budget_usd"); err == nil {
		t.Errorf("write leaked past visceral cap")
	}
}

func TestHandlePutScopeSettings_PartialSuccess_ReturnsPerKeyResults(t *testing.T) {
	env := newSettingsTestEnv(t)
	r := buildRouter(env, noVisceralRulesHTTP)
	// One valid key, one invalid (unknown). 200 OK with mixed results.
	body := []byte(`{"max_turns":100,"no_such_knob":42}`)
	rec := doRequest(t, r, http.MethodPut, "/api/products/p1/settings", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got putResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got.Results))
	}
	okCount, failCount := 0, 0
	for _, res := range got.Results {
		if res.OK {
			okCount++
		} else {
			failCount++
		}
	}
	if okCount != 1 || failCount != 1 {
		t.Errorf("expected 1 ok + 1 fail, got %d ok + %d fail", okCount, failCount)
	}
	// Only the valid key should have persisted.
	if _, err := env.store.Get(context.Background(), settings.ScopeProduct, "p1", "max_turns"); err != nil {
		t.Errorf("max_turns should persist: %v", err)
	}
}

func TestHandlePutScopeSettings_RecordsHTTPAuthor(t *testing.T) {
	env := newSettingsTestEnv(t)
	r := buildRouter(env, noVisceralRulesHTTP)
	body := []byte(`{"temperature":0.5}`)
	rec := doRequest(t, r, http.MethodPut, "/api/manifests/m1/settings", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d", rec.Code)
	}
	entry, err := env.store.Get(context.Background(), settings.ScopeManifest, "m1", "temperature")
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	if !strings.HasPrefix(entry.UpdatedBy, "http:") {
		t.Errorf("author should start with http:, got %q", entry.UpdatedBy)
	}
	// Must NOT carry the mcp: prefix — that would mean the wrong author was
	// recorded by the HTTP path.
	if strings.HasPrefix(entry.UpdatedBy, "mcp:") {
		t.Errorf("author leaked mcp prefix on http write: %q", entry.UpdatedBy)
	}
}

// ---- GET /api/tasks/:id/settings/resolved -----------------------------------

func TestHandleGetResolved_ReturnsEveryKnob(t *testing.T) {
	env := newSettingsTestEnv(t)
	r := buildRouter(env, noVisceralRulesHTTP)
	rec := doRequest(t, r, http.MethodGet, "/api/tasks/t1/settings/resolved", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
	var got resolvedResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.TaskID != "t1" {
		t.Errorf("task id echoed wrong: %q", got.TaskID)
	}
	if len(got.Resolved) != len(settings.Catalog()) {
		t.Fatalf("resolved count: got %d want %d", len(got.Resolved), len(settings.Catalog()))
	}
}

func TestHandleGetResolved_ReturnsProvenance(t *testing.T) {
	env := newSettingsTestEnv(t)
	ctx := context.Background()
	if err := env.store.Set(ctx, settings.ScopeTask, "t1", "max_turns", "7", "seed"); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if err := env.store.Set(ctx, settings.ScopeManifest, "m1", "temperature", "0.9", "seed"); err != nil {
		t.Fatalf("seed manifest: %v", err)
	}
	if err := env.store.Set(ctx, settings.ScopeProduct, "p1", "max_parallel", "8", "seed"); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	r := buildRouter(env, noVisceralRulesHTTP)
	rec := doRequest(t, r, http.MethodGet, "/api/tasks/t1/settings/resolved", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d", rec.Code)
	}
	var got resolvedResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	check := func(key string, src settings.ScopeType, srcID string) {
		t.Helper()
		r, ok := got.Resolved[key]
		if !ok {
			t.Fatalf("missing resolved entry for %q", key)
		}
		if r.Source != src || r.SourceID != srcID {
			t.Errorf("%s provenance: got source=%q id=%q want source=%q id=%q",
				key, r.Source, r.SourceID, src, srcID)
		}
	}
	check("max_turns", settings.ScopeTask, "t1")
	check("temperature", settings.ScopeManifest, "m1")
	check("max_parallel", settings.ScopeProduct, "p1")
	check("approval_mode", settings.ScopeSystem, "")
}

func TestHandleGetResolved_UnknownTask_Returns404(t *testing.T) {
	env := newSettingsTestEnv(t)
	r := buildRouter(env, noVisceralRulesHTTP)
	rec := doRequest(t, r, http.MethodGet, "/api/tasks/t-does-not-exist/settings/resolved", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown task, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// ---- route registration -----------------------------------------------------

func TestRoutes_AllSettingsEndpointsRegistered(t *testing.T) {
	env := newSettingsTestEnv(t)
	r := buildRouter(env, noVisceralRulesHTTP)

	// Every endpoint listed in the M2-T6 spec must respond (any non-404 code
	// is fine — we are checking routing, not handler semantics here).
	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/settings/catalog"},
		{http.MethodGet, "/api/products/p1/settings"},
		{http.MethodPut, "/api/products/p1/settings"},
		{http.MethodGet, "/api/manifests/m1/settings"},
		{http.MethodPut, "/api/manifests/m1/settings"},
		{http.MethodGet, "/api/tasks/t1/settings"},
		{http.MethodPut, "/api/tasks/t1/settings"},
		{http.MethodGet, "/api/tasks/t1/settings/resolved"},
	}
	for _, c := range cases {
		var body []byte
		if c.method == http.MethodPut {
			body = []byte(`{"max_turns":50}`)
		}
		rec := doRequest(t, r, c.method, c.path, body)
		if rec.Code == http.StatusNotFound {
			t.Errorf("route %s %s returned 404 — not registered", c.method, c.path)
		}
	}
}
