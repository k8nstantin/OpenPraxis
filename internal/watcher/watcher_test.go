package watcher

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/yuin/goldmark"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
)

// initTestRepo creates a temp git repo with an initial commit on main. Every
// branch the test creates will include at least one new commit so the git
// gate's CommitCount check sees non-zero diff.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	run("commit", "--allow-empty", "-m", "init")
	return dir
}

func commitOnBranch(t *testing.T, dir, branch, filename string) {
	t.Helper()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("checkout", "-q", "-b", branch)
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	run("add", filename)
	run("commit", "-q", "-m", "work on "+branch)
	run("checkout", "-q", "main")
}

// TestResolveTaskBranch_ExactPreferred — when both forms exist, the exact
// plain-marker branch wins. Matches the historical contract for tasks that
// don't rename their branch.
func TestResolveTaskBranch_ExactPreferred(t *testing.T) {
	dir := initTestRepo(t)
	commitOnBranch(t, dir, "openpraxis/abc123-def", "exact.txt")
	commitOnBranch(t, dir, "openpraxis/abc123-def-m1-t1", "suffixed.txt")

	w := New(nil, dir, "", "node")
	got, ok := w.resolveTaskBranch("abc123-def")
	if !ok {
		t.Fatal("resolveTaskBranch ok=false, want exact match")
	}
	if got != "openpraxis/abc123-def" {
		t.Fatalf("got %q, want openpraxis/abc123-def (exact)", got)
	}
}

// TestResolveTaskBranch_SuffixedOnly — this is the production bug we're
// fixing: only the suffixed branch exists, the gate must still find it
// rather than reporting BranchExists=false.
func TestResolveTaskBranch_SuffixedOnly(t *testing.T) {
	dir := initTestRepo(t)
	commitOnBranch(t, dir, "openpraxis/019da13a-f33-m1-t1", "schema.go")

	w := New(nil, dir, "", "node")
	got, ok := w.resolveTaskBranch("019da13a-f33")
	if !ok {
		t.Fatal("resolveTaskBranch ok=false for suffixed-only branch; bug reproduces")
	}
	if got != "openpraxis/019da13a-f33-m1-t1" {
		t.Fatalf("got %q, want openpraxis/019da13a-f33-m1-t1", got)
	}
}

// TestResolveTaskBranch_None — neither form present, ok=false.
func TestResolveTaskBranch_None(t *testing.T) {
	dir := initTestRepo(t)

	w := New(nil, dir, "", "node")
	if _, ok := w.resolveTaskBranch("missing"); ok {
		t.Fatal("resolveTaskBranch ok=true for missing branch")
	}
}

// TestResolveTaskBranch_NoFalsePrefixMatch — a task marker must not match
// a different task whose marker happens to start with the same characters.
// "abc" must not resolve to a branch on marker "abcdef".
func TestResolveTaskBranch_NoFalsePrefixMatch(t *testing.T) {
	dir := initTestRepo(t)
	commitOnBranch(t, dir, "openpraxis/abcdef-m1", "other.txt")

	w := New(nil, dir, "", "node")
	if got, ok := w.resolveTaskBranch("abc"); ok {
		t.Fatalf("resolveTaskBranch matched %q for unrelated prefix", got)
	}
}

// openCommentsStore returns a comments.Store backed by a fresh sqlite DB in
// t.TempDir() with WAL + busy_timeout pragmas applied (visceral rule #10).
func openCommentsStore(t *testing.T) (*comments.Store, *sql.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "comments.db")
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
	if err := comments.InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	return comments.NewStore(db), db
}

// newAuditStore returns a watcher Store backed by a fresh sqlite DB so
// AuditTask can persist audits during integration tests.
func newAuditStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "audits.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open audit db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	s, err := NewStore(db)
	if err != nil {
		t.Fatalf("audit NewStore: %v", err)
	}
	return s
}

// brokenPoster always errors, used to verify audit does not fail when comment
// write fails (acceptance criterion).
type brokenPoster struct{ calls int }

func (b *brokenPoster) Add(_ context.Context, _ comments.TargetType, _, _ string,
	_ comments.CommentType, _ string) (comments.Comment, error) {
	b.calls++
	return comments.Comment{}, errors.New("comments: store unavailable")
}

// emptyBranch creates a branch at the same commit as main — branch exists but
// has zero commits relative to base, which is what Gate 1 keys on.
func emptyBranch(t *testing.T, dir, branch string) {
	t.Helper()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("checkout", "-q", "-b", branch)
	run("checkout", "-q", "main")
}

func listTaskComments(t *testing.T, store *comments.Store, taskID string) []comments.Comment {
	t.Helper()
	out, err := store.List(context.Background(), comments.TargetTask, taskID, 100, nil)
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	return out
}

func assertOneWatcherFinding(t *testing.T, store *comments.Store, taskID string) comments.Comment {
	t.Helper()
	cs := listTaskComments(t, store, taskID)
	if len(cs) != 1 {
		t.Fatalf("expected 1 comment on task %s, got %d", taskID, len(cs))
	}
	c := cs[0]
	if c.Type != comments.TypeWatcherFinding {
		t.Fatalf("expected type %q, got %q", comments.TypeWatcherFinding, c.Type)
	}
	if c.Author != "watcher" {
		t.Fatalf("expected author %q, got %q", "watcher", c.Author)
	}
	return c
}

func TestWatcher_Gate1_Fail_PostsComment(t *testing.T) {
	dir := initTestRepo(t)
	// Branch exists but has zero commits vs main — Gate 1 fails.
	emptyBranch(t, dir, "openpraxis/gate1fail")

	store, _ := openCommentsStore(t)
	w := New(newAuditStore(t), dir, "true", "node")
	w.SetCommentPoster(store)

	audit := w.AuditTask("gate1fail", "T", "", "", "", "completed", 0, 0)
	if audit.GitPassed {
		t.Fatalf("expected GitPassed=false, got true (reason=%s)", audit.GitDetails.Reason)
	}
	c := assertOneWatcherFinding(t, store, "gate1fail")
	if !strings.Contains(c.Body, "Git gate observation") {
		t.Fatalf("body missing 'Git gate observation':\n%s", c.Body)
	}
	if !strings.Contains(c.Body, "openpraxis/gate1fail") {
		t.Fatalf("body missing branch name:\n%s", c.Body)
	}
}

func TestWatcher_Gate2_Fail_PostsComment(t *testing.T) {
	dir := initTestRepo(t)
	commitOnBranch(t, dir, "openpraxis/gate2fail", "work.go")

	store, _ := openCommentsStore(t)
	// buildCmd=false always exits non-zero; no build output required for body
	// check — failed-build body wraps whatever output the gate captured.
	w := New(newAuditStore(t), dir, "false", "node")
	w.SetCommentPoster(store)

	audit := w.AuditTask("gate2fail", "T", "", "", "", "completed", 0, 0)
	if audit.BuildPassed {
		t.Fatalf("expected BuildPassed=false, got true")
	}
	c := assertOneWatcherFinding(t, store, "gate2fail")
	if !strings.Contains(c.Body, "Build gate observation") {
		t.Fatalf("body missing 'Build gate observation':\n%s", c.Body)
	}
	if !strings.Contains(c.Body, "```") {
		t.Fatalf("body missing fenced code block:\n%s", c.Body)
	}
}

func TestWatcher_Gate3_Fail_PostsComment(t *testing.T) {
	dir := initTestRepo(t)
	commitOnBranch(t, dir, "openpraxis/gate3fail", "unrelated.txt")

	store, _ := openCommentsStore(t)
	w := New(newAuditStore(t), dir, "true", "node")
	w.SetCommentPoster(store)

	// Manifest mentions deliverables that will NOT appear in the diff.
	// extractDeliverables keys on `backtick-paths` in bullet lines.
	manifest := "## Scope\n\n" +
		"- `internal/missing/thing.go`\n" +
		"- `internal/missing/other.go`\n"

	audit := w.AuditTask("gate3fail", "T", "m", "M", manifest, "completed", 0, 0)
	if audit.ManifestPassed {
		t.Fatalf("expected ManifestPassed=false, got true (score=%v, reason=%s)",
			audit.ManifestScore, audit.ManifestDetails.Reason)
	}
	c := assertOneWatcherFinding(t, store, "gate3fail")
	if !strings.Contains(c.Body, "Manifest gate observation") {
		t.Fatalf("body missing 'Manifest gate observation':\n%s", c.Body)
	}
	if !strings.Contains(c.Body, "internal/missing/thing.go") {
		t.Fatalf("body missing missing-deliverable bullet:\n%s", c.Body)
	}
}

func TestWatcher_AllPass_NoComment(t *testing.T) {
	dir := initTestRepo(t)
	commitOnBranch(t, dir, "openpraxis/allpass", "work.go")

	store, _ := openCommentsStore(t)
	w := New(newAuditStore(t), dir, "true", "node")
	w.SetCommentPoster(store)

	audit := w.AuditTask("allpass", "T", "", "", "", "completed", 0, 0)
	if audit.Status != "passed" {
		t.Fatalf("expected status=passed, got %q (git=%v build=%v manifest=%v)",
			audit.Status, audit.GitPassed, audit.BuildPassed, audit.ManifestPassed)
	}
	if cs := listTaskComments(t, store, "allpass"); len(cs) != 0 {
		t.Fatalf("expected 0 comments on all-pass, got %d", len(cs))
	}
}

func TestWatcher_CommentWriteFailure_DoesNotBlockAudit(t *testing.T) {
	dir := initTestRepo(t)
	emptyBranch(t, dir, "openpraxis/brokencomments")

	auditStore := newAuditStore(t)
	broken := &brokenPoster{}
	w := New(auditStore, dir, "true", "node")
	w.SetCommentPoster(broken)

	audit := w.AuditTask("task-broken", "T", "", "", "", "completed", 0, 0)
	if audit == nil {
		t.Fatal("audit is nil — broken comment store must not stop the audit path")
	}
	if audit.Status != "failed" {
		t.Fatalf("expected status=failed on git gate fail, got %q", audit.Status)
	}
	// Audit must still be persisted even though the comment write failed.
	stored, err := auditStore.GetByTask("task-broken")
	if err != nil || stored == nil {
		t.Fatalf("audit not persisted: err=%v stored=%v", err, stored)
	}
	if broken.calls == 0 {
		t.Fatal("expected poster.Add to be called at least once")
	}
}

func TestWatcher_BodyFormat_Markdown(t *testing.T) {
	// Build fake audit results covering all three failure bodies and confirm
	// goldmark renders them without errors (i.e. they are well-formed).
	bodies := []string{
		gitFailureBody(GitResult{Branch: "openpraxis/xyz", CommitCount: 0}),
		buildFailureBody(BuildResult{Output: "pkg/foo.go:1: undefined: Bar"}),
		manifestFailureBody(ManifestResult{Deliverables: []Deliverable{
			{Item: "internal/a.go", Status: "missing"},
			{Item: "internal/b.go", Status: "done"},
		}}),
	}
	for i, body := range bodies {
		var buf bytes.Buffer
		if err := goldmark.New().Convert([]byte(body), &buf); err != nil {
			t.Fatalf("body %d goldmark convert: %v\nbody:\n%s", i, err, body)
		}
		rendered := buf.String()
		if !strings.Contains(rendered, "<h3>") {
			t.Fatalf("body %d missing rendered <h3>:\n%s", i, rendered)
		}
	}
	// The build body specifically must include a fenced code block that
	// renders to <pre><code>.
	buildBody := bodies[1]
	var buf bytes.Buffer
	if err := goldmark.New().Convert([]byte(buildBody), &buf); err != nil {
		t.Fatalf("build body convert: %v", err)
	}
	if !strings.Contains(buf.String(), "<pre>") {
		t.Fatalf("build body missing <pre> render:\n%s", buf.String())
	}
}

// TestRunGitGate_ReportsResolvedBranch — the audit output must record which
// branch was actually checked so downgrades are diagnosable from the audit
// row without re-running the gate.
func TestRunGitGate_ReportsResolvedBranch(t *testing.T) {
	dir := initTestRepo(t)
	commitOnBranch(t, dir, "openpraxis/019da13a-f33-m1-t1", "schema.go")

	w := New(nil, dir, "", "node")
	res := w.RunGitGate("019da13a-f33")
	if !res.BranchExists {
		t.Fatalf("BranchExists=false; want true. Reason: %s", res.Reason)
	}
	if res.Branch != "openpraxis/019da13a-f33-m1-t1" {
		t.Fatalf("Branch = %q, want openpraxis/019da13a-f33-m1-t1", res.Branch)
	}
	if res.CommitCount < 1 {
		t.Fatalf("CommitCount = %d, want >=1", res.CommitCount)
	}
}
