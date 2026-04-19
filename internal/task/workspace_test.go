package task

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initRemoteAndClone sets up a bare "remote" repo with a `main` branch that
// has one commit, then clones it locally. Returns (remoteDir, cloneDir).
// The clone has `origin` pointing at remote so the runner's worktree path
// exercises the same remote-resolution + fetch code the real server hits.
func initRemoteAndClone(t *testing.T) (string, string) {
	t.Helper()
	base := t.TempDir()
	remoteDir := filepath.Join(base, "remote.git")
	cloneDir := filepath.Join(base, "clone")
	seedDir := filepath.Join(base, "seed")

	runIn := func(dir string, args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v (in %s): %v\n%s", args, dir, err, out)
		}
	}

	// Bare remote.
	cmd := exec.Command("git", "init", "--bare", "-b", "main", remoteDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("init bare: %v\n%s", err, out)
	}

	// Seed repo: one commit on main, pushed to the bare remote so main exists upstream.
	cmd = exec.Command("git", "init", "-b", "main", seedDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("init seed: %v\n%s", err, out)
	}
	runIn(seedDir, "config", "user.email", "t@e.com")
	runIn(seedDir, "config", "user.name", "T")
	if err := os.WriteFile(filepath.Join(seedDir, "README.md"), []byte("seed\n"), 0644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	runIn(seedDir, "add", "README.md")
	runIn(seedDir, "commit", "-m", "seed")
	runIn(seedDir, "remote", "add", "origin", remoteDir)
	runIn(seedDir, "push", "origin", "main")

	// Clone. This is the "repo root" the runner will operate in.
	cmd = exec.Command("git", "clone", remoteDir, cloneDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("clone: %v\n%s", err, out)
	}
	runIn(cloneDir, "config", "user.email", "t@e.com")
	runIn(cloneDir, "config", "user.name", "T")

	return remoteDir, cloneDir
}

func newRunnerForWorkspace(repoDir string) *Runner {
	return &Runner{repoDir: repoDir}
}

// TestPrepareTaskWorkspace_CreatesIsolatedWorktreeOffMain — the success
// path we care about: the worktree exists on disk, is rooted at origin/main,
// and does NOT drag in whatever branch the parent repo happens to be on.
func TestPrepareTaskWorkspace_CreatesIsolatedWorktreeOffMain(t *testing.T) {
	_, repoDir := initRemoteAndClone(t)

	// Put the parent repo on a different branch so we'd notice if the
	// worktree inherited it.
	cmd := exec.Command("git", "checkout", "-b", "some-unrelated-branch")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("checkout unrelated: %v\n%s", err, out)
	}

	r := newRunnerForWorkspace(repoDir)
	workDir, baseSHA, err := r.prepareTaskWorkspace("019da13a-f337")
	if err != nil {
		t.Fatalf("prepareTaskWorkspace: %v", err)
	}
	defer r.cleanupTaskWorkspace(workDir)

	if !strings.HasPrefix(workDir, filepath.Join(repoDir, workspaceRoot)) {
		t.Fatalf("workDir %q not under %s/", workDir, workspaceRoot)
	}
	if _, err := os.Stat(filepath.Join(workDir, "README.md")); err != nil {
		t.Fatalf("README.md missing in worktree: %v", err)
	}

	// The worktree HEAD must match origin/main — not the parent's unrelated branch.
	out, err := runGit(repoDir, "rev-parse", "origin/main")
	if err != nil {
		t.Fatalf("rev-parse origin/main: %v", err)
	}
	wantSHA := strings.TrimSpace(out)
	if baseSHA != wantSHA {
		t.Fatalf("baseSHA = %q, want origin/main %q", baseSHA, wantSHA)
	}

	wtHead, err := runGit(workDir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("worktree HEAD: %v", err)
	}
	if strings.TrimSpace(wtHead) != wantSHA {
		t.Fatalf("worktree HEAD = %q, want %q", strings.TrimSpace(wtHead), wantSHA)
	}

	// The parent repo's branch must be untouched by the whole operation.
	parentBranch, err := runGit(repoDir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		t.Fatalf("parent branch: %v", err)
	}
	if strings.TrimSpace(parentBranch) != "some-unrelated-branch" {
		t.Fatalf("parent branch mutated to %q — worktree isolation violated", strings.TrimSpace(parentBranch))
	}
}

// TestPrepareTaskWorkspace_StaleWorktreeReclaimed — the scenario where a
// previous task crashed without cleaning up. Calling prepare a second time
// with the same id must succeed and land on a fresh worktree, not error out.
func TestPrepareTaskWorkspace_StaleWorktreeReclaimed(t *testing.T) {
	_, repoDir := initRemoteAndClone(t)
	r := newRunnerForWorkspace(repoDir)

	workDir1, _, err := r.prepareTaskWorkspace("019da13a-f337")
	if err != nil {
		t.Fatalf("first prepare: %v", err)
	}
	// Simulate a crashed task: worktree still on disk, still registered.

	workDir2, _, err := r.prepareTaskWorkspace("019da13a-f337")
	if err != nil {
		t.Fatalf("second prepare (stale reclaim): %v", err)
	}
	if workDir1 != workDir2 {
		t.Fatalf("reclaim path mismatch: %q vs %q", workDir1, workDir2)
	}
	if _, err := os.Stat(filepath.Join(workDir2, "README.md")); err != nil {
		t.Fatalf("README.md missing after reclaim: %v", err)
	}
	r.cleanupTaskWorkspace(workDir2)
}

// TestCleanupTaskWorkspace_RemovesDirButKeepsBranch — after cleanup the
// directory is gone, but any branch the agent created inside the worktree
// still resolves (it's in shared .git). This is the property that keeps
// the PR alive after the worktree directory is thrown away.
func TestCleanupTaskWorkspace_RemovesDirButKeepsBranch(t *testing.T) {
	_, repoDir := initRemoteAndClone(t)
	r := newRunnerForWorkspace(repoDir)

	workDir, _, err := r.prepareTaskWorkspace("019da13a-f337")
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}

	// Agent-simulated: create a branch + commit inside the worktree.
	if _, err := runGit(workDir, "checkout", "-b", "openpraxis/019da13a-f337-m1-t1"); err != nil {
		t.Fatalf("branch: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "new.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := runGit(workDir, "add", "new.txt"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := runGit(workDir, "commit", "-m", "task work"); err != nil {
		t.Fatalf("commit: %v", err)
	}

	r.cleanupTaskWorkspace(workDir)

	if _, err := os.Stat(workDir); !os.IsNotExist(err) {
		t.Fatalf("workDir still exists after cleanup: err=%v", err)
	}
	// Branch must still resolve in the parent repo after cleanup — this is
	// what keeps the PR working after we throw the worktree away.
	if _, err := runGit(repoDir, "rev-parse", "openpraxis/019da13a-f337-m1-t1"); err != nil {
		t.Fatalf("branch lost after cleanup: %v", err)
	}
}

// TestDetectDefaultRemote_PrefersOrigin — given multiple remotes, origin wins.
func TestDetectDefaultRemote_PrefersOrigin(t *testing.T) {
	_, repoDir := initRemoteAndClone(t)
	if _, err := runGit(repoDir, "remote", "add", "zzz", "/dev/null"); err != nil {
		t.Fatalf("add zzz: %v", err)
	}
	got, err := detectDefaultRemote(repoDir)
	if err != nil {
		t.Fatalf("detectDefaultRemote: %v", err)
	}
	if got != "origin" {
		t.Fatalf("got %q, want origin", got)
	}
}

// TestDetectDefaultRemote_FallsBackToFirst — if origin isn't configured,
// the first remote listed is used. Covers dev machines that use a custom
// remote name (e.g. `github`) instead of origin.
func TestDetectDefaultRemote_FallsBackToFirst(t *testing.T) {
	_, repoDir := initRemoteAndClone(t)
	// Rename origin so only "github" remains.
	if _, err := runGit(repoDir, "remote", "rename", "origin", "github"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	got, err := detectDefaultRemote(repoDir)
	if err != nil {
		t.Fatalf("detectDefaultRemote: %v", err)
	}
	if got != "github" {
		t.Fatalf("got %q, want github (the only remote)", got)
	}
}
