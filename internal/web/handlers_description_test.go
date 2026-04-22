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

	"github.com/k8nstantin/OpenPraxis/internal/comments"
	"github.com/k8nstantin/OpenPraxis/internal/config"
	"github.com/k8nstantin/OpenPraxis/internal/manifest"
	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/product"
	"github.com/k8nstantin/OpenPraxis/internal/task"
)

type descriptionTestEnv struct {
	node   *node.Node
	server *httptest.Server
	db     *sql.DB
}

func newDescriptionTestEnv(t *testing.T) *descriptionTestEnv {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "desc.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := comments.InitSchema(db); err != nil {
		t.Fatalf("comments schema: %v", err)
	}
	pStore, err := product.NewStore(db)
	if err != nil {
		t.Fatalf("product: %v", err)
	}
	mStore, err := manifest.NewStore(db)
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	tStore, err := task.NewStore(db)
	if err != nil {
		t.Fatalf("task: %v", err)
	}

	n := &node.Node{
		Config:    &config.Config{Node: config.NodeConfig{UUID: "peer-test"}},
		Products:  pStore,
		Manifests: mStore,
		Tasks:     tStore,
		Comments:  comments.NewStore(db),
	}

	r := mux.NewRouter()
	api := r.PathPrefix("/api").Subrouter()
	registerDescriptionRoutes(api, n)

	srv := httptest.NewServer(r)
	t.Cleanup(func() {
		srv.Close()
		db.Close()
	})
	return &descriptionTestEnv{node: n, server: srv, db: db}
}

func (e *descriptionTestEnv) do(t *testing.T, method, path string, body any) (*http.Response, []byte) {
	t.Helper()
	var buf io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, e.server.URL+path, buf)
	if err != nil {
		t.Fatalf("req: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp, out
}

// seedProductRevisions creates a product with body1 as the initial
// denormalised body, then applies body2 as a fresh edit (revision + UPDATE)
// so tests have a non-trivial history to read / restore from. The first
// revision is backfill-style (seeded directly) because RecordDescriptionChange
// no-ops when current == new.
func (e *descriptionTestEnv) seedProductRevisions(t *testing.T, body1, body2 string) (id string) {
	t.Helper()
	ctx := nil2ctx()
	p, err := e.node.Products.Create("P", body1, "open", e.node.PeerID(), nil)
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	id = p.ID
	// Seed v1 directly — mirrors DV/M1 backfill path.
	if _, err := e.node.Comments.Add(ctx, comments.TargetProduct, id, "alice", comments.TypeDescriptionRevision, body1); err != nil {
		t.Fatalf("seed rev1: %v", err)
	}
	// Record the edit BEFORE the UPDATE (same order as production handler).
	if _, err := e.node.RecordDescriptionChange(ctx, comments.TargetProduct, id, body2, "bob"); err != nil {
		t.Fatalf("record 2: %v", err)
	}
	if err := e.node.Products.Update(id, p.Title, body2, p.Status, p.Tags); err != nil {
		t.Fatalf("update: %v", err)
	}
	return id
}

// nil2ctx is a tiny helper so seed code stays readable.
func nil2ctx() context.Context { return context.Background() }

// ---- history ---------------------------------------------------------------

func TestDescriptionHistory_Product(t *testing.T) {
	env := newDescriptionTestEnv(t)
	id := env.seedProductRevisions(t, "v1", "v2")

	resp, body := env.do(t, "GET", "/api/products/"+id+"/description/history", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	var out struct {
		Items []revisionView `json:"items"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode: %v body=%s", err, body)
	}
	if len(out.Items) != 2 {
		t.Fatalf("want 2 items, got %d", len(out.Items))
	}
	// Newest first.
	if out.Items[0].Body != "v2" {
		t.Fatalf("item[0].body = %q want v2", out.Items[0].Body)
	}
	if out.Items[1].Body != "v1" {
		t.Fatalf("item[1].body = %q want v1", out.Items[1].Body)
	}
	// Versions: oldest = 1, newest = 2.
	if out.Items[0].Version != 2 || out.Items[1].Version != 1 {
		t.Fatalf("versions = (%d, %d) want (2, 1)", out.Items[0].Version, out.Items[1].Version)
	}
}

func TestDescriptionHistory_UnknownEntity_404(t *testing.T) {
	env := newDescriptionTestEnv(t)
	resp, _ := env.do(t, "GET", "/api/products/bogus/description/history", nil)
	if resp.StatusCode != 404 {
		t.Fatalf("status=%d want 404", resp.StatusCode)
	}
}

// ---- get-revision ----------------------------------------------------------

func TestDescriptionGetRevision_OK(t *testing.T) {
	env := newDescriptionTestEnv(t)
	id := env.seedProductRevisions(t, "v1", "v2")

	// Find the revision IDs via history.
	_, body := env.do(t, "GET", "/api/products/"+id+"/description/history", nil)
	var hist struct {
		Items []revisionView `json:"items"`
	}
	_ = json.Unmarshal(body, &hist)
	older := hist.Items[1]

	resp, raw := env.do(t, "GET", "/api/products/"+id+"/description/revisions/"+older.ID, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d raw=%s", resp.StatusCode, raw)
	}
	var got revisionView
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Body != "v1" {
		t.Fatalf("body=%q want v1", got.Body)
	}
	if got.Version != 1 {
		t.Fatalf("version=%d want 1", got.Version)
	}
}

func TestDescriptionGetRevision_WrongEntity_404(t *testing.T) {
	env := newDescriptionTestEnv(t)
	id := env.seedProductRevisions(t, "v1", "v2")

	// Create a second product with a revision; try to read its revision via
	// the first product's URL.
	otherID := env.seedProductRevisions(t, "other-v1", "other-v2")
	_, body := env.do(t, "GET", "/api/products/"+otherID+"/description/history", nil)
	var hist struct {
		Items []revisionView `json:"items"`
	}
	_ = json.Unmarshal(body, &hist)
	foreign := hist.Items[0].ID

	resp, raw := env.do(t, "GET", "/api/products/"+id+"/description/revisions/"+foreign, nil)
	if resp.StatusCode != 404 {
		t.Fatalf("status=%d raw=%s want 404", resp.StatusCode, raw)
	}
}

// ---- restore ---------------------------------------------------------------

func TestDescriptionRestore_Product(t *testing.T) {
	env := newDescriptionTestEnv(t)
	id := env.seedProductRevisions(t, "v1", "v2")

	_, body := env.do(t, "GET", "/api/products/"+id+"/description/history", nil)
	var hist struct {
		Items []revisionView `json:"items"`
	}
	_ = json.Unmarshal(body, &hist)
	olderID := hist.Items[1].ID // v1

	resp, raw := env.do(t, "POST", "/api/products/"+id+"/description/restore/"+olderID, restoreRequest{Author: "carol"})
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d raw=%s", resp.StatusCode, raw)
	}
	var rr restoreResponse
	if err := json.Unmarshal(raw, &rr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !rr.Restored {
		t.Fatalf("restored=false want true (body=%s)", raw)
	}
	if rr.NewID == "" {
		t.Fatalf("new_revision_id empty")
	}

	// After restore: denormalised column is v1.
	p, err := env.node.Products.Get(id)
	if err != nil || p == nil {
		t.Fatalf("get product: %v", err)
	}
	if p.Description != "v1" {
		t.Fatalf("product.description = %q want v1", p.Description)
	}

	// History now has 3 rows, newest is the restore (v1).
	_, body = env.do(t, "GET", "/api/products/"+id+"/description/history", nil)
	var hist2 struct {
		Items []revisionView `json:"items"`
	}
	_ = json.Unmarshal(body, &hist2)
	if len(hist2.Items) != 3 {
		t.Fatalf("len(history)=%d want 3", len(hist2.Items))
	}
	if hist2.Items[0].Body != "v1" {
		t.Fatalf("newest body=%q want v1", hist2.Items[0].Body)
	}
	if hist2.Items[0].Author != "carol" {
		t.Fatalf("newest author=%q want carol", hist2.Items[0].Author)
	}
}

func TestDescriptionRestore_NoOpWhenAlreadyCurrent(t *testing.T) {
	env := newDescriptionTestEnv(t)
	id := env.seedProductRevisions(t, "v1", "v2")

	_, body := env.do(t, "GET", "/api/products/"+id+"/description/history", nil)
	var hist struct {
		Items []revisionView `json:"items"`
	}
	_ = json.Unmarshal(body, &hist)
	newest := hist.Items[0].ID // body matches current

	resp, raw := env.do(t, "POST", "/api/products/"+id+"/description/restore/"+newest, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d raw=%s", resp.StatusCode, raw)
	}
	var rr restoreResponse
	_ = json.Unmarshal(raw, &rr)
	if rr.Restored {
		t.Fatalf("expected restored=false (no-op), got true (raw=%s)", raw)
	}
}

// ---- manifest scope --------------------------------------------------------

func TestDescriptionHistory_Manifest(t *testing.T) {
	env := newDescriptionTestEnv(t)
	m, err := env.node.Manifests.Create("M1", "summary", "spec-v1", "draft", env.node.PeerID(), env.node.PeerID(), "", "", nil, nil)
	if err != nil {
		t.Fatalf("create manifest: %v", err)
	}
	ctx := nil2ctx()
	// Seed v1 directly (current body == new body → RecordDescriptionChange
	// would no-op).
	if _, err := env.node.Comments.Add(ctx, comments.TargetManifest, m.ID, "alice", comments.TypeDescriptionRevision, "spec-v1"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Record the edit BEFORE the UPDATE (production handler order).
	if _, err := env.node.RecordDescriptionChange(ctx, comments.TargetManifest, m.ID, "spec-v2", "bob"); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := env.node.Manifests.Update(m.ID, m.Title, m.Description, "spec-v2", m.Status, m.ProjectID, m.DependsOn, m.JiraRefs, m.Tags); err != nil {
		t.Fatalf("update manifest: %v", err)
	}

	resp, body := env.do(t, "GET", "/api/manifests/"+m.ID+"/description/history", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	var out struct {
		Items []revisionView `json:"items"`
	}
	_ = json.Unmarshal(body, &out)
	if len(out.Items) != 2 {
		t.Fatalf("want 2, got %d", len(out.Items))
	}
	if out.Items[0].Body != "spec-v2" {
		t.Fatalf("body[0]=%q want spec-v2", out.Items[0].Body)
	}
}

// ---- task scope ------------------------------------------------------------

func TestDescriptionRestore_Task(t *testing.T) {
	env := newDescriptionTestEnv(t)
	tk, err := env.node.Tasks.Create("", "T1", "instr-v1", "", "", env.node.PeerID(), "", "")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	ctx := nil2ctx()
	if _, err := env.node.Comments.Add(ctx, comments.TargetTask, tk.ID, "alice", comments.TypeDescriptionRevision, "instr-v1"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := env.node.RecordDescriptionChange(ctx, comments.TargetTask, tk.ID, "instr-v2", "bob"); err != nil {
		t.Fatalf("record: %v", err)
	}
	v2 := "instr-v2"
	if _, err := env.node.Tasks.Update(tk.ID, nil, &v2); err != nil {
		t.Fatalf("update: %v", err)
	}

	_, body := env.do(t, "GET", "/api/tasks/"+tk.ID+"/description/history", nil)
	var hist struct {
		Items []revisionView `json:"items"`
	}
	_ = json.Unmarshal(body, &hist)
	if len(hist.Items) != 2 {
		t.Fatalf("want 2 revisions, got %d", len(hist.Items))
	}
	olderID := hist.Items[1].ID

	resp, raw := env.do(t, "POST", "/api/tasks/"+tk.ID+"/description/restore/"+olderID, restoreRequest{Author: "carol"})
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d raw=%s", resp.StatusCode, raw)
	}
	after, err := env.node.Tasks.Get(tk.ID)
	if err != nil || after == nil {
		t.Fatalf("get: %v", err)
	}
	if after.Description != "instr-v1" {
		t.Fatalf("task desc after restore = %q want instr-v1", after.Description)
	}
}
