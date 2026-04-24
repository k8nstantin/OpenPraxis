package templates

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// fakeAgentLookup answers AgentForTask from a static map — enough to
// exercise the resolver's agent-scope walk without importing the task
// package (which would create an import cycle).
type fakeAgentLookup struct {
	m map[string]string
}

func (f *fakeAgentLookup) AgentForTask(_ context.Context, taskID string) (string, error) {
	if f == nil {
		return "", nil
	}
	return f.m[taskID], nil
}

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

// TestSeed_InsertsSevenSystemRows verifies acceptance #1 from RC/M1:
// fresh DB → seed writes exactly seven active system rows, one per
// section. With RC/M6 layered on top, the total row count is system(7)
// + codex(7) + cursor(7) = 21, so this test scopes its count to the
// system tier.
func TestSeed_InsertsSevenSystemRows(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	if err := Seed(ctx, store, "peer-xyz"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var sys int
	if err := db.QueryRow(`SELECT COUNT(*) FROM prompt_templates WHERE scope='system'`).Scan(&sys); err != nil {
		t.Fatalf("count: %v", err)
	}
	if sys != 7 {
		t.Fatalf("system row count = %d, want 7", sys)
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

	var total, sys, agent int
	_ = db.QueryRow(`SELECT COUNT(*) FROM prompt_templates`).Scan(&total)
	_ = db.QueryRow(`SELECT COUNT(*) FROM prompt_templates WHERE scope='system'`).Scan(&sys)
	_ = db.QueryRow(`SELECT COUNT(*) FROM prompt_templates WHERE scope='agent'`).Scan(&agent)
	if sys != 7 {
		t.Fatalf("after re-seed system count = %d, want 7", sys)
	}
	if agent != 14 {
		t.Fatalf("after re-seed agent count = %d, want 14", agent)
	}
	if total != 21 {
		t.Fatalf("after re-seed total count = %d, want 21", total)
	}
}

// TestSeed_InsertsAgentRows verifies RC/M6 acceptance #1: fresh seed
// writes 14 active agent-scope rows (7 codex + 7 cursor).
func TestSeed_InsertsAgentRows(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	if err := Seed(ctx, store, "peer-xyz"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rows, err := store.List(ctx, ScopeAgent, "")
	if err != nil {
		t.Fatalf("list agent: %v", err)
	}
	if len(rows) != 14 {
		t.Fatalf("agent rows = %d, want 14", len(rows))
	}

	byAgent := map[string]map[string]string{}
	for _, r := range rows {
		if byAgent[r.ScopeID] == nil {
			byAgent[r.ScopeID] = map[string]string{}
		}
		byAgent[r.ScopeID][r.Section] = r.Body
	}
	for _, agent := range SeededAgents {
		sections := byAgent[agent]
		if len(sections) != 7 {
			t.Fatalf("%s rows = %d, want 7", agent, len(sections))
		}
		for _, s := range Sections {
			if sections[s] == "" {
				t.Errorf("%s missing section %s", agent, s)
			}
		}
		// Markdown frame — must contain at least one `## ` heading and
		// must not re-use the Claude XML block opener.
		joined := ""
		for _, b := range sections {
			joined += b
		}
		if !strings.Contains(joined, "## ") {
			t.Errorf("%s bodies missing markdown headings", agent)
		}
		if strings.Contains(joined, "<visceral_rules>") || strings.Contains(joined, "<manifest_spec") {
			t.Errorf("%s bodies should not contain Claude XML tags", agent)
		}
	}
}

// TestSeed_IdempotentPerBucket — after the full seed, a follow-up Seed()
// doesn't duplicate rows and also doesn't create a second copy of any
// bucket. A bucket manually emptied mid-flight is NOT re-seeded (the
// gate is per-bucket presence, not per-section).
func TestSeed_IdempotentPerBucket(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	if err := Seed(ctx, store, ""); err != nil {
		t.Fatalf("seed 1: %v", err)
	}
	if err := Seed(ctx, store, ""); err != nil {
		t.Fatalf("seed 2: %v", err)
	}

	for _, agent := range SeededAgents {
		var n int
		_ = db.QueryRow(`SELECT COUNT(*) FROM prompt_templates WHERE scope='agent' AND scope_id=?`, agent).Scan(&n)
		if n != 7 {
			t.Fatalf("agent %s rows = %d, want 7", agent, n)
		}
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

// TestResolver_AgentScope exercises RC/M6 acceptance #4. With seeded
// agent rows in place, a task whose effective agent is "codex" or
// "cursor" resolves to that agent's markdown-heading body, while a
// task on "claude-code" (no agent row) falls through to the system
// XML body. The same section resolves to three distinct strings.
func TestResolver_AgentScope(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	if err := Seed(ctx, store, ""); err != nil {
		t.Fatalf("seed: %v", err)
	}

	agents := &fakeAgentLookup{m: map[string]string{
		"task-codex":  "codex",
		"task-cursor": "cursor",
		"task-claude": "claude-code",
	}}
	r := NewResolver(store, nil, agents)

	gitCodex, err := r.Resolve(ctx, SectionGitWorkflow, "task-codex")
	if err != nil {
		t.Fatalf("resolve codex: %v", err)
	}
	if !strings.Contains(gitCodex, "## Git Workflow") {
		t.Errorf("codex git_workflow should contain '## Git Workflow'; got %q", gitCodex)
	}
	if strings.Contains(gitCodex, "<git_workflow>") {
		t.Errorf("codex git_workflow should not contain XML tag")
	}

	gitCursor, err := r.Resolve(ctx, SectionGitWorkflow, "task-cursor")
	if err != nil {
		t.Fatalf("resolve cursor: %v", err)
	}
	if !strings.Contains(gitCursor, "## Git Workflow") {
		t.Errorf("cursor git_workflow should contain '## Git Workflow'; got %q", gitCursor)
	}

	gitClaude, err := r.Resolve(ctx, SectionGitWorkflow, "task-claude")
	if err != nil {
		t.Fatalf("resolve claude: %v", err)
	}
	if !strings.Contains(gitClaude, "<git_workflow>") {
		t.Errorf("claude-code git_workflow should contain XML tag; got %q", gitClaude)
	}

	// Preamble carries the agent self-identifier, so codex/cursor/claude
	// must all produce distinct strings for the same section.
	pCodex, _ := r.Resolve(ctx, SectionPreamble, "task-codex")
	pCursor, _ := r.Resolve(ctx, SectionPreamble, "task-cursor")
	pClaude, _ := r.Resolve(ctx, SectionPreamble, "task-claude")
	if pCodex == pCursor || pCodex == pClaude || pCursor == pClaude {
		t.Errorf("preambles should be distinct — codex=%q cursor=%q claude=%q", pCodex, pCursor, pClaude)
	}
	if !strings.Contains(pCodex, "Codex") {
		t.Errorf("codex preamble missing Codex identifier: %q", pCodex)
	}
	if !strings.Contains(pCursor, "Cursor") {
		t.Errorf("cursor preamble missing Cursor identifier: %q", pCursor)
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
