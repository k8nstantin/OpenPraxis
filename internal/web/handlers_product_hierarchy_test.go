package web

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/k8nstantin/OpenPraxis/internal/config"
	"github.com/k8nstantin/OpenPraxis/internal/manifest"
	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/product"
	"github.com/k8nstantin/OpenPraxis/internal/task"
	_ "github.com/mattn/go-sqlite3"
)

// hierarchyResponse matches the payload shape returned by
// apiProductHierarchy (the JSON structure products.js / product-dag.js
// consume). Kept in this test file so a schema change to the API forces
// a test update; the renderer relies on these exact fields.
type hierarchyResponse struct {
	ID       string             `json:"id"`
	Title    string             `json:"title"`
	Type     string             `json:"type"`
	Children []hierarchyManifest `json:"children"`
}

type hierarchyManifest struct {
	ID        string           `json:"id"`
	Title     string           `json:"title"`
	DependsOn string           `json:"depends_on"`
	Children  []hierarchyTask `json:"children"`
}

type hierarchyTask struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	DependsOn string `json:"depends_on"`
	Status    string `json:"status"`
}

// newHierarchyTestNode wires the stores the product hierarchy endpoint
// reads from: products, manifests, tasks. Uses an isolated in-memory
// SQLite so shapes don't leak between tests.
func newHierarchyTestNode(t *testing.T) *node.Node {
	t.Helper()
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared&_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	products, err := product.NewStore(db)
	if err != nil {
		t.Fatalf("product.NewStore: %v", err)
	}
	manifests, err := manifest.NewStore(db)
	if err != nil {
		t.Fatalf("manifest.NewStore: %v", err)
	}
	tasks, err := task.NewStore(db)
	if err != nil {
		t.Fatalf("task.NewStore: %v", err)
	}
	return &node.Node{
		Config:    &config.Config{Node: config.NodeConfig{UUID: "hier-test-peer"}},
		Products:  products,
		Manifests: manifests,
		Tasks:     tasks,
	}
}

func getHierarchy(t *testing.T, n *node.Node, productID string) hierarchyResponse {
	t.Helper()
	r := mux.NewRouter()
	r.HandleFunc("/api/products/{id}/hierarchy", apiProductHierarchy(n))
	req := httptest.NewRequest(http.MethodGet, "/api/products/"+productID+"/hierarchy", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET hierarchy: status %d body=%s", rec.Code, rec.Body.String())
	}
	var out hierarchyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode hierarchy: %v body=%s", err, rec.Body.String())
	}
	return out
}

// TestProductHierarchy_EmptyProduct — product with no manifests. Renderer
// must handle this without crashing; DAG shows just the product node.
func TestProductHierarchy_EmptyProduct(t *testing.T) {
	n := newHierarchyTestNode(t)
	p, err := n.Products.Create("Empty Product", "no children", "open", n.PeerID(), nil)
	if err != nil {
		t.Fatalf("product create: %v", err)
	}

	got := getHierarchy(t, n, p.ID)
	if got.ID != p.ID {
		t.Fatalf("want id %s, got %s", p.ID, got.ID)
	}
	if len(got.Children) != 0 {
		t.Fatalf("expected 0 manifests, got %d", len(got.Children))
	}
}

// TestProductHierarchy_LinearChain — 1 manifest with N tasks forming a
// single chain T1 -> T2 -> T3 -> T4 -> T5. Every task except the root
// has depends_on pointing to its predecessor. The DAG renderer draws
// one edge per depends_on — dagre handles the visual ordering.
func TestProductHierarchy_LinearChain(t *testing.T) {
	n := newHierarchyTestNode(t)
	p, _ := n.Products.Create("Linear Product", "", "open", n.PeerID(), nil)
	m, err := n.Manifests.Create("Linear Chain", "", "content", "open", "tester", n.PeerID(), p.ID, "", nil, nil)
	if err != nil {
		t.Fatalf("manifest create: %v", err)
	}

	// 5 tasks, each depends_on the previous.
	var prev string
	var ids []string
	for i := 1; i <= 5; i++ {
		title := "T" + string(rune('0'+i))
		tk, err := n.Tasks.Create(m.ID, title, "desc", "once", "claude-code", n.PeerID(), "tester", prev)
		if err != nil {
			t.Fatalf("task create %d: %v", i, err)
		}
		ids = append(ids, tk.ID)
		prev = tk.ID
	}

	got := getHierarchy(t, n, p.ID)
	if len(got.Children) != 1 {
		t.Fatalf("expected 1 manifest, got %d", len(got.Children))
	}
	tasks := got.Children[0].Children
	if len(tasks) != 5 {
		t.Fatalf("expected 5 tasks, got %d", len(tasks))
	}
	// Each task's depends_on must match its predecessor id (or empty for root).
	byID := map[string]hierarchyTask{}
	for _, tk := range tasks {
		byID[tk.ID] = tk
	}
	for i, id := range ids {
		tk := byID[id]
		var wantDep string
		if i > 0 {
			wantDep = ids[i-1]
		}
		if tk.DependsOn != wantDep {
			t.Fatalf("task #%d (%s) depends_on=%q want %q", i+1, id, tk.DependsOn, wantDep)
		}
	}
}

// TestProductHierarchy_ParallelPairs — 1 manifest with N independent
// root+child pairs (the INT MySQL backup/verify shape). Each root has
// empty depends_on; each child points at its root. None of the pairs
// share state with each other. This is the shape PR #146 originally
// tried to fix and is regression-sensitive.
func TestProductHierarchy_ParallelPairs(t *testing.T) {
	n := newHierarchyTestNode(t)
	p, _ := n.Products.Create("Parallel Pairs Product", "", "open", n.PeerID(), nil)
	m, err := n.Manifests.Create("Pairs Manifest", "", "content", "open", "tester", n.PeerID(), p.ID, "", nil, nil)
	if err != nil {
		t.Fatalf("manifest create: %v", err)
	}

	// 4 pairs: main-N (root) + verify-N (depends_on=main-N). No cross-pair links.
	type pair struct {
		mainID, verifyID string
	}
	var pairs []pair
	for i := 1; i <= 4; i++ {
		suffix := string(rune('0' + i))
		main, err := n.Tasks.Create(m.ID, "main-"+suffix, "desc", "once", "claude-code", n.PeerID(), "tester", "")
		if err != nil {
			t.Fatalf("main %d: %v", i, err)
		}
		verify, err := n.Tasks.Create(m.ID, "verify-"+suffix, "desc", "once", "claude-code", n.PeerID(), "tester", main.ID)
		if err != nil {
			t.Fatalf("verify %d: %v", i, err)
		}
		pairs = append(pairs, pair{mainID: main.ID, verifyID: verify.ID})
	}

	got := getHierarchy(t, n, p.ID)
	if len(got.Children) != 1 {
		t.Fatalf("expected 1 manifest, got %d", len(got.Children))
	}
	tasks := got.Children[0].Children
	if len(tasks) != 8 {
		t.Fatalf("expected 8 tasks, got %d", len(tasks))
	}

	byID := map[string]hierarchyTask{}
	for _, tk := range tasks {
		byID[tk.ID] = tk
	}
	for i, pr := range pairs {
		main := byID[pr.mainID]
		verify := byID[pr.verifyID]
		if main.DependsOn != "" {
			t.Fatalf("pair %d main depends_on=%q want empty (root)", i+1, main.DependsOn)
		}
		if verify.DependsOn != pr.mainID {
			t.Fatalf("pair %d verify depends_on=%q want main %q", i+1, verify.DependsOn, pr.mainID)
		}
	}
}
