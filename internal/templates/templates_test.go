package templates

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "templates.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("wal: %v", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		t.Fatalf("busy: %v", err)
	}
	if err := InitSchema(db); err != nil {
		t.Fatalf("init: %v", err)
	}
	return db
}

// TestSeed_InsertsSevenSystemRows verifies acceptance #1: fresh DB →
// seed writes exactly seven active system rows, one per section.
func TestSeed_InsertsSevenSystemRows(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	if err := Seed(ctx, store, "peer-xyz"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM prompt_templates`).Scan(&total); err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != 7 {
		t.Fatalf("row count = %d, want 7", total)
	}

	rows, err := store.List(ctx, ScopeSystem, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 7 {
		t.Fatalf("system rows = %d, want 7", len(rows))
	}

	seen := map[string]bool{}
	for _, r := range rows {
		seen[r.Section] = true
		if r.Scope != ScopeSystem || r.ScopeID != "" {
			t.Errorf("unexpected scope (%s,%s) for %s", r.Scope, r.ScopeID, r.Section)
		}
		if r.ValidTo != "" || r.DeletedAt != "" {
			t.Errorf("seed row %s should be active (valid_to=%q deleted_at=%q)", r.Section, r.ValidTo, r.DeletedAt)
		}
		if r.ChangedBy != "system-seed" {
			t.Errorf("changed_by = %q, want system-seed", r.ChangedBy)
		}
		if r.Body == "" {
			t.Errorf("section %q body is empty", r.Section)
		}
	}
	for _, s := range Sections {
		if !seen[s] {
			t.Errorf("missing section %q", s)
		}
	}
}

// TestSeed_Idempotent — a second Seed call after the first is a no-op.
func TestSeed_Idempotent(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	if err := Seed(ctx, store, "peer-a"); err != nil {
		t.Fatalf("seed 1: %v", err)
	}
	if err := Seed(ctx, store, "peer-a"); err != nil {
		t.Fatalf("seed 2: %v", err)
	}

	var total int
	_ = db.QueryRow(`SELECT COUNT(*) FROM prompt_templates`).Scan(&total)
	if total != 7 {
		t.Fatalf("after re-seed count = %d, want 7", total)
	}
}

// TestStore_GetAndGetByUID exercises the two read paths on a seeded DB.
func TestStore_GetAndGetByUID(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	if err := Seed(ctx, store, "peer-a"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got, err := store.Get(ctx, ScopeSystem, "", SectionPreamble)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Body != defaultPreamble {
		t.Fatalf("preamble body mismatch")
	}
	byUID, err := store.GetByUID(ctx, got.TemplateUID)
	if err != nil {
		t.Fatalf("GetByUID: %v", err)
	}
	if byUID.ID != got.ID {
		t.Fatalf("GetByUID returned row %d, want %d", byUID.ID, got.ID)
	}
}

// TestResolver_FallsThroughToSystem — with no task/manifest/product rows
// overlaid, a resolve at every section should return the system body.
func TestResolver_FallsThroughToSystem(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	if err := Seed(ctx, store, ""); err != nil {
		t.Fatalf("seed: %v", err)
	}
	r := NewResolver(store, nil, nil)
	for _, sec := range Sections {
		body, err := r.Resolve(ctx, sec, "")
		if err != nil {
			t.Fatalf("resolve %s: %v", sec, err)
		}
		if body == "" {
			t.Fatalf("resolve %s returned empty", sec)
		}
	}
}

// TestResolver_TaskScopeWins — inserting a task-scope row for one
// section masks the system default for that task only.
func TestResolver_TaskScopeWins(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	if err := Seed(ctx, store, ""); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err := db.ExecContext(ctx,
		`INSERT INTO prompt_templates
		(template_uid, title, scope, scope_id, section, body, status, tags,
		 source_node, valid_from, valid_to, changed_by, reason, created_at, deleted_at)
		VALUES ('task-uid', 'override', 'task', 'task-1', ?, 'OVERRIDDEN', 'open', '[]',
		        '', '2026-01-01T00:00:00Z', '', 'test', 'override', '2026-01-01T00:00:00Z', '')`,
		SectionPreamble)
	if err != nil {
		t.Fatalf("insert override: %v", err)
	}
	r := NewResolver(store, nil, nil)
	body, err := r.Resolve(ctx, SectionPreamble, "task-1")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if body != "OVERRIDDEN" {
		t.Fatalf("task override not returned; got %q", body)
	}
	// Another task should still get the system default.
	body2, err := r.Resolve(ctx, SectionPreamble, "task-2")
	if err != nil {
		t.Fatalf("resolve other: %v", err)
	}
	if body2 != defaultPreamble {
		t.Fatalf("unrelated task should see system default")
	}
}

// TestRender_Printf ensures the %q verb round-trips identical to
// fmt.Sprintf — the rendered prompt relies on it for <task title=%q>.
func TestRender_Printf(t *testing.T) {
	out, err := Render(`title={{printf "%q" .Task.Title}}`, PromptData{Task: TaskView{Title: `he said "hi"`}})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	want := `title="he said \"hi\""`
	if out != want {
		t.Fatalf("printf render = %q, want %q", out, want)
	}
}
