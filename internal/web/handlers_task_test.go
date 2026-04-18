package web

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gorilla/mux"

	"github.com/k8nstantin/OpenPraxis/internal/config"
	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/task"

	_ "github.com/mattn/go-sqlite3"
)

// newTaskOnlyNode builds a Node scaffold that wires ONLY the pieces the
// create/update task handlers touch: a task.Store over an in-test SQLite DB
// and a NodeConfig with a stable PeerID. Everything else (memory, ideas,
// manifest, runner) is left nil — the two handlers under test do not
// dereference those fields.
//
// We deliberately do NOT go through node.New because that constructor wires
// the real memory/embedding/ollama/mDNS stack and is not usable in a unit
// test. This test scaffolding is valid as long as the handler remains a
// straight call into n.Tasks.Create / n.Tasks.Update with n.PeerID() as the
// source_node.
func newTaskOnlyNode(t *testing.T) *node.Node {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "tasks.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on")
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
	tasks, err := task.NewStore(db)
	if err != nil {
		t.Fatalf("task.NewStore: %v", err)
	}
	return &node.Node{
		Config: &config.Config{
			Node: config.NodeConfig{UUID: "test-peer-uuid"},
		},
		Tasks: tasks,
	}
}

// TestHandleCreateTask_IgnoresLegacyMaxTurnsField — M4-T14 retired the
// max_turns field on POST /api/tasks. Legacy callers sending it should get
// a successful create back (the field is silently ignored and logged as a
// deprecation warning). The new task row must have no max_turns column
// because the column is gone — we assert via the Task struct round-trip
// that t.Title persisted, proving the INSERT worked without max_turns.
func TestHandleCreateTask_IgnoresLegacyMaxTurnsField(t *testing.T) {
	n := newTaskOnlyNode(t)

	// Legacy body: includes max_turns field that M4-T14 ignores.
	body := []byte(`{"title":"legacy-create","description":"d","schedule":"once","agent":"claude-code","max_turns":777}`)
	req := httptest.NewRequest(http.MethodPost, "/api/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	apiTaskCreate(n).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
	var got task.Task
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Title != "legacy-create" {
		t.Fatalf("title = %q, want %q", got.Title, "legacy-create")
	}
	// Task ID must be a real UUID-ish string (not empty).
	if got.ID == "" {
		t.Fatalf("returned task has empty ID")
	}
	// Readback confirms the row persisted. A column-write on the retired
	// max_turns column would have failed the INSERT — reaching here proves
	// the handler took the new no-column path.
	readback, err := n.Tasks.Get(got.ID)
	if err != nil {
		t.Fatalf("get readback: %v", err)
	}
	if readback == nil || readback.Title != "legacy-create" {
		t.Fatalf("readback mismatch: %+v", readback)
	}
}

// TestHandleUpdateTask_IgnoresLegacyMaxTurnsField — PATCH /api/tasks/:id
// with a legacy max_turns field must return 200 and update whatever OTHER
// fields were present. The max_turns in the body is logged and discarded;
// the task's unrelated column values remain untouched.
func TestHandleUpdateTask_IgnoresLegacyMaxTurnsField(t *testing.T) {
	n := newTaskOnlyNode(t)

	// Seed a task so we have something to PATCH.
	created, err := n.Tasks.Create("", "orig-title", "orig-desc", "once", "claude-code", n.PeerID(), "test", "")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	// PATCH with both a legitimate field (title) and the deprecated
	// max_turns. Expect 200, title updated, no error.
	body := []byte(`{"title":"patched-title","max_turns":555}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/tasks/"+created.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// Route through mux so mux.Vars("id") resolves.
	router := mux.NewRouter()
	router.HandleFunc("/api/tasks/{id}", apiTaskUpdate(n)).Methods(http.MethodPatch)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
	var got task.Task
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Title != "patched-title" {
		t.Fatalf("title = %q, want %q (legitimate fields must still flow through)", got.Title, "patched-title")
	}
	// Description should be unchanged — only title was in the request.
	if got.Description != "orig-desc" {
		t.Fatalf("description = %q, want %q (unchanged)", got.Description, "orig-desc")
	}
}
