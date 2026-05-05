package web

import (
	"bytes"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
	"github.com/k8nstantin/OpenPraxis/internal/entity"
	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/task"

	"github.com/gorilla/mux"
	_ "github.com/mattn/go-sqlite3"
)

// TestAddComment_RejectsShortPrefix_Task — markers are dead. POST to
// /api/tasks/<short-prefix>/comments must 404 rather than silently writing
// a comment row keyed on a non-canonical id.
func TestAddComment_RejectsShortPrefix_Task(t *testing.T) {
	n, tasks, commentsStore := setupResolveTestNode(t)

	tk, err := tasks.Create("", "Example task", "desc", "once", "claude-code", "", "", "")
	if err != nil {
		t.Fatalf("Create task: %v", err)
	}
	if len(tk.ID) != 36 {
		t.Fatalf("expected 36-char UUID, got %q", tk.ID)
	}
	shortPrefix := tk.ID[:12]

	api := mux.NewRouter().PathPrefix("/api").Subrouter()
	registerCommentsRoutesFromNode(api, n)

	body := `{"author":"operator","type":"user_note","body":"posted via short prefix"}`
	req := httptest.NewRequest("POST", "/api/tasks/"+shortPrefix+"/comments", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for short-prefix target, got status=%d body=%s", rec.Code, rec.Body.String())
	}

	for _, id := range []string{shortPrefix, tk.ID} {
		cs, err := commentsStore.List(req.Context(), comments.TargetTask, id, 10, nil)
		if err != nil {
			t.Fatalf("List %q: %v", id, err)
		}
		if len(cs) != 0 {
			t.Fatalf("expected 0 comments for %q, got %d", id, len(cs))
		}
	}
}

func TestListComments_RejectsShortPrefix_Task(t *testing.T) {
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

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for short-prefix list target, got status=%d body=%s", rec.Code, rec.Body.String())
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
	entities, err := entity.NewStore(db)
	if err != nil {
		t.Fatalf("NewStore entities: %v", err)
	}
	cs := comments.NewStore(db)

	return &node.Node{
		Comments: cs,
		Tasks:    tasks,
		Entities: entities,
	}, tasks, cs
}
