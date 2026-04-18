package watcher

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
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
