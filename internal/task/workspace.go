package task

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// workspaceRoot is the default directory under the repo where per-task
// worktrees live. Kept inside the repo (not /tmp) so disk layout is
// predictable and the operator can inspect a stuck task's workspace
// without hunting. Overridable via the `worktree_base_dir` knob which
// may be absolute (out-of-tree) or relative (joined onto the repo dir).
const workspaceRoot = ".openpraxis-work"

// prepareTaskWorkspace materializes a fresh git worktree for the given task
// at `<baseDir>/<taskID>`, checked out to a detached HEAD on `<remote>/main`.
// baseDir defaults to `<repoDir>/.openpraxis-work` when empty; absolute
// paths are honoured verbatim so operators can park worktrees on a
// dedicated volume. The agent then creates its own task branch inside
// that worktree, so:
//
//   - Each task starts from a clean, up-to-date copy of main — no more
//     stacking one task branch on top of another.
//   - The operator's own checkout at repoDir is never touched. Uncommitted
//     edits in their working tree survive task execution.
//   - The branch the agent creates inside the worktree lives in the shared
//     .git, so push + PR creation work as usual.
//
// Returns the absolute worktree path and the base commit SHA (the agent's
// starting point — recorded for audit/debug). The caller is responsible for
// calling cleanupTaskWorkspace after the agent exits.
func (r *Runner) prepareTaskWorkspace(taskID, baseDir string) (workDir, baseSHA string, err error) {
	repoDir, err := r.resolveRepoDir()
	if err != nil {
		return "", "", err
	}

	remote, err := detectDefaultRemote(repoDir)
	if err != nil {
		return "", "", fmt.Errorf("detect remote: %w", err)
	}

	// Fetch latest main so the worktree starts current. Network failures
	// are not fatal — proceed with the local ref; the operator's next
	// sync will reconcile.
	if _, ferr := runGit(repoDir, "fetch", remote, "main"); ferr != nil {
		slog.Warn("fetch main failed; using local ref",
			"component", "runner", "remote", remote, "error", ferr)
	}

	workDir = resolveWorkspacePath(repoDir, baseDir, taskID)

	// A stale worktree from a prior crashed run would block `worktree add`.
	// Clean it unconditionally — if it's not registered, `worktree remove`
	// no-ops; if it is, this drops it so the new add succeeds.
	_, _ = runGit(repoDir, "worktree", "remove", "--force", workDir)
	// Also nuke the directory in case the parent worktree metadata was
	// already pruned but files linger on disk.
	_ = os.RemoveAll(workDir)

	if err := os.MkdirAll(filepath.Dir(workDir), 0o755); err != nil {
		return "", "", fmt.Errorf("mkdir worktree parent: %w", err)
	}

	// Detached HEAD on origin/main — the agent will run `git checkout -b
	// openpraxis/<marker>-...` itself, matching the branch name from the
	// task description. Pre-creating the branch here would force a naming
	// convention we don't want to couple to.
	target := remote + "/main"
	if _, err := runGit(repoDir, "worktree", "add", "--detach", workDir, target); err != nil {
		return "", "", fmt.Errorf("worktree add at %s: %w", target, err)
	}

	shaOut, err := runGit(workDir, "rev-parse", "HEAD")
	if err != nil {
		// Worktree created but we couldn't read the SHA — still usable.
		slog.Warn("read worktree HEAD failed", "component", "runner", "error", err)
	} else {
		baseSHA = strings.TrimSpace(shaOut)
	}

	return workDir, baseSHA, nil
}

// cleanupTaskWorkspace removes the worktree after the agent finishes. The
// branch the agent created stays — it carries the commits + PR history.
func (r *Runner) cleanupTaskWorkspace(workDir string) {
	if workDir == "" {
		return
	}
	repoDir, err := r.resolveRepoDir()
	if err != nil {
		slog.Warn("cleanup workspace: repo dir unresolved", "component", "runner", "workdir", workDir, "error", err)
		return
	}
	if _, err := runGit(repoDir, "worktree", "remove", "--force", workDir); err != nil {
		slog.Warn("worktree remove failed",
			"component", "runner", "workdir", workDir, "error", err)
	}
}

// resolveWorkspacePath joins baseDir + taskID into an absolute path
// under which the task's worktree lives. baseDir defaults to
// `<repoDir>/.openpraxis-work` when empty. Absolute baseDir values
// are honoured verbatim so operators can park worktrees on a
// dedicated volume outside the repo.
//
// Relative baseDirs are cleaned and validated to prevent path traversal:
// a configured value of "../../../tmp" would otherwise escape the repo tree.
func resolveWorkspacePath(repoDir, baseDir, taskID string) string {
	if baseDir == "" {
		baseDir = workspaceRoot
	}
	if filepath.IsAbs(baseDir) {
		return filepath.Join(filepath.Clean(baseDir), taskID)
	}
	// Reject relative paths that contain ".." components after cleaning —
	// filepath.Join would silently resolve them out of repoDir.
	clean := filepath.Clean(baseDir)
	if strings.HasPrefix(clean, "..") {
		clean = workspaceRoot
	}
	return filepath.Join(repoDir, clean, taskID)
}

// resolveRepoDir returns r.repoDir if set, otherwise falls back to the
// server's current working directory. Memoization would be premature —
// Getwd is cheap and this is once per task spawn.
func (r *Runner) resolveRepoDir() (string, error) {
	if r.repoDir != "" {
		return r.repoDir, nil
	}
	return os.Getwd()
}

// detectDefaultRemote picks the remote git should fetch from. Prefers
// "origin" when present; otherwise returns whatever `git remote` lists
// first. Empty slice means the repo has no remote configured.
func detectDefaultRemote(repoDir string) (string, error) {
	out, err := runGit(repoDir, "remote")
	if err != nil {
		return "", err
	}
	var first string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		if name == "origin" {
			return name, nil
		}
		if first == "" {
			first = name
		}
	}
	if first == "" {
		return "", fmt.Errorf("no git remote configured in %s", repoDir)
	}
	return first, nil
}

// runGit executes a git command in dir, capturing stderr into the returned
// error so callers surface the git message rather than just "exit status 1".
func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}
