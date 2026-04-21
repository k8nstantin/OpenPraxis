package web

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
	"github.com/k8nstantin/OpenPraxis/internal/manifest"
	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/product"
	"github.com/k8nstantin/OpenPraxis/internal/task"

	"github.com/gorilla/mux"
	_ "github.com/mattn/go-sqlite3"
)

// TestAddComment_ResolvesShortMarker — the bug this PR fixes. HTTP POST to
// /api/tasks/<short-marker>/comments MUST write target_id = full UUID,
// matching what MCP comment_add already does after PR #136.
func TestAddComment_ResolvesShortMarker_Task(t *testing.T) {
	n, tasks, commentsStore := setupResolveTestNode(t)

	tk, err := tasks.Create("", "Example task", "desc", "once", "claude-code", "", "", "")
	if err != nil {
		t.Fatalf("Create task: %v", err)
	}
	if len(tk.ID) != 36 {
		t.Fatalf("expected 36-char UUID, got %q", tk.ID)
	}
	shortMarker := tk.ID[:12]

	api := mux.NewRouter().PathPrefix("/api").Subrouter()
	registerCommentsRoutesFromNode(api, n)

	body := `{"author":"operator","type":"user_note","body":"posted via short marker"}`
	req := httptest.NewRequest("POST", "/api/tasks/"+shortMarker+"/comments", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var view commentView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if view.TargetID != tk.ID {
		t.Fatalf("target_id not canonicalized: got %q, want %q", view.TargetID, tk.ID)
	}

	cs, err := commentsStore.List(req.Context(), comments.TargetTask, tk.ID, 10, nil)
	if err != nil {
		t.Fatalf("List full UUID: %v", err)
	}
	if len(cs) != 1 {
		t.Fatalf("expected 1 comment on full UUID, got %d", len(cs))
	}
	orphans, _ := commentsStore.List(req.Context(), comments.TargetTask, shortMarker, 10, nil)
	if len(orphans) != 0 {
		t.Fatalf("expected 0 orphans on short marker, got %d", len(orphans))
	}
}

func TestListComments_ResolvesShortMarker_Task(t *testing.T) {
	n, tasks, commentsStore := setupResolveTestNode(t)
	tk, _ := tasks.Create("", "Example", "desc", "once", "claude-code", "", "", "")

	if _, err := commentsStore.Add(t.Context(), comments.TargetTask, tk.ID, "operator",
		comments.TypeUserNote, "seeded"); err != nil {
		t.Fatalf("seed comment: %v", err)
	}

	api := mux.NewRouter().PathPrefix("/api").Subrouter()
	registerCommentsRoutesFromNode(api, n)

	req := httptest.NewRequest("GET", "/api/tasks/"+tk.ID[:8]+"/comments", nil)
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out listCommentsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Comments) != 1 {
		t.Fatalf("expected 1 comment via short marker, got %d", len(out.Comments))
	}
}

func TestAddComment_RejectsNonExistentTarget_Task(t *testing.T) {
	n, _, _ := setupResolveTestNode(t)
	api := mux.NewRouter().PathPrefix("/api").Subrouter()
	registerCommentsRoutesFromNode(api, n)

	body := `{"author":"op","type":"user_note","body":"x"}`
	req := httptest.NewRequest("POST", "/api/tasks/deadbeef-nope/comments", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent target, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func setupResolveTestNode(t *testing.T) (*node.Node, *task.Store, *comments.Store) {
	t.Helper()
	dsn := "file::memory:?cache=shared&_journal_mode=WAL&_busy_timeout=5000"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := comments.InitSchema(db); err != nil {
		t.Fatalf("InitSchema comments: %v", err)
	}
	tasks, err := task.NewStore(db)
	if err != nil {
		t.Fatalf("NewStore tasks: %v", err)
	}
	manifests, err := manifest.NewStore(db)
	if err != nil {
		t.Fatalf("NewStore manifests: %v", err)
	}
	products, err := product.NewStore(db)
	if err != nil {
		t.Fatalf("NewStore products: %v", err)
	}
	cs := comments.NewStore(db)

	return &node.Node{
		Comments:  cs,
		Tasks:     tasks,
		Manifests: manifests,
		Products:  products,
	}, tasks, cs
}
