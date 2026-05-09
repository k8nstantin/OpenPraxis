package watcher

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
)

// CommentPoster is the minimal subset of *comments.Store the watcher calls to
// auto-post comment comments on gate decisions. Declared as an
// interface so tests can inject a broken implementation and verify the audit
// path does not fail on comment-write errors.
type CommentPoster interface {
	Add(ctx context.Context, target comments.TargetType, targetID, author string,
		cType comments.CommentType, body string) (comments.Comment, error)
}

// Watcher is the independent server-side task execution auditor.
// It runs OUTSIDE the agent session — the agent cannot access, override, or skip it.
type Watcher struct {
	store      *Store
	repoDir    string // git repo root
	buildCmd   string // build command (e.g. "go build ./...")
	sourceNode string
	comments   CommentPoster
}

// New creates a watcher.
func New(store *Store, repoDir, buildCmd, sourceNode string) *Watcher {
	if buildCmd == "" {
		buildCmd = "go build ./..."
	}
	return &Watcher{
		store:      store,
		repoDir:    repoDir,
		buildCmd:   buildCmd,
		sourceNode: sourceNode,
	}
}

// SetCommentPoster wires a comments store so gate-failure decisions auto-post
// comment comments on the audited task. A nil poster disables the
// feature (existing behavior).
func (w *Watcher) SetCommentPoster(p CommentPoster) { w.comments = p }

// AuditTask runs all three gates on a completed task.
// This is called by the task runner AFTER the agent claims completion.
// The agent has no say in the result.
func (w *Watcher) AuditTask(taskID, taskTitle, manifestID, manifestTitle, manifestContent, originalStatus string, actionCount int, costUSD float64) *Audit {
	now := time.Now().UTC()
	audit := &Audit{
		ID:             uuid.Must(uuid.NewV7()).String(),
		TaskID:         taskID,
		TaskTitle:      taskTitle,
		ManifestID:     manifestID,
		ManifestTitle:  manifestTitle,
		OriginalStatus: originalStatus,
		ActionCount:    actionCount,
		CostUSD:        costUSD,
		SourceNode:     w.sourceNode,
		AuditedAt:      now,
		CreatedAt:      now,
	}

	slog.Info("auditing task", "component", "watcher", "task_id", taskID, "title", taskTitle, "original_status", originalStatus, "actions", actionCount)

	// Gate 1: Git verification
	audit.GitDetails = w.RunGitGate(taskID)
	audit.GitPassed = audit.GitDetails.CommitCount > 0

	// Gate 2: Build verification
	audit.BuildDetails = w.RunBuildGate()
	audit.BuildPassed = audit.BuildDetails.ExitCode == 0

	// Gate 3: Manifest compliance (deliverable extraction from content)
	if manifestContent != "" {
		audit.ManifestDetails = w.RunManifestGate(taskID, manifestContent)
		if audit.ManifestDetails.TotalItems > 0 {
			audit.ManifestScore = float64(audit.ManifestDetails.DoneItems) / float64(audit.ManifestDetails.TotalItems)
			audit.ManifestPassed = audit.ManifestScore >= 0.5
		} else {
			// No deliverables extracted — pass by default
			audit.ManifestPassed = true
			audit.ManifestScore = 1.0
			audit.ManifestDetails.Reason = "no deliverables extracted from manifest"
		}
	} else {
		// No manifest content — skip this gate
		audit.ManifestPassed = true
		audit.ManifestScore = 1.0
		audit.ManifestDetails.Reason = "standalone task — no manifest to check against"
	}

	// Determine overall verdict
	if audit.GitPassed && audit.BuildPassed && audit.ManifestPassed {
		audit.Status = "passed"
		audit.FinalStatus = originalStatus
	} else if !audit.GitPassed {
		// No commits = instant fail — this is the strongest signal
		audit.Status = "failed"
		audit.FinalStatus = "failed"
	} else if !audit.BuildPassed {
		audit.Status = "failed"
		audit.FinalStatus = "failed"
	} else {
		// Manifest check failed but git + build passed
		audit.Status = "warning"
		audit.FinalStatus = originalStatus // keep original but flag it
	}

	slog.Info("audit completed", "component", "watcher", "task_id", audit.TaskID, "status", audit.Status,
		"git_passed", audit.GitPassed, "build_passed", audit.BuildPassed, "manifest_score", audit.ManifestScore*100,
		"original_status", audit.OriginalStatus, "final_status", audit.FinalStatus)

	// Persist
	if err := w.store.Record(audit); err != nil {
		slog.Error("failed to store audit", "component", "watcher", "error", err)
	}

	// Post comment comments on gate failures. All-pass posts nothing
	// (the green status badge on the task card already communicates). Comment
	// write errors are logged and swallowed — an audit never fails because a
	// comment failed to write.
	w.postFindings(taskID, audit)

	return audit
}

// postFindings writes one comment comment per failed gate. Safe to
// call with a nil comment poster (feature disabled).
func (w *Watcher) postFindings(taskID string, audit *Audit) {
	if w.comments == nil || taskID == "" {
		return
	}

	var bodies []string
	if !audit.GitPassed {
		bodies = append(bodies, gitFailureBody(audit.GitDetails))
	}
	if !audit.BuildPassed {
		bodies = append(bodies, buildFailureBody(audit.BuildDetails))
	}
	if !audit.ManifestPassed {
		bodies = append(bodies, manifestFailureBody(audit.ManifestDetails))
	}

	for _, body := range bodies {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := w.comments.Add(ctx, comments.TargetEntity, taskID, "watcher", comments.TypeWatcherFinding, body)
		cancel()
		if err != nil {
			slog.Warn("watcher failed to post finding comment",
				"component", "watcher", "task", taskID, "error", err)
		}
	}
}

func gitFailureBody(g GitResult) string {
	branch := g.Branch
	if branch == "" {
		branch = "(unresolved)"
	}
	return fmt.Sprintf("### Git gate observation\n"+
		"No commits on branch %s. Watcher cannot verify code-change work against git. "+
		"This is informational only — task state and downstream activation are unaffected; "+
		"the paired review task decides the outcome.\n\n"+
		"**Branch:** %s\n"+
		"**Expected commits:** ≥1\n"+
		"**Actual:** %d",
		branch, branch, g.CommitCount)
}

func buildFailureBody(b BuildResult) string {
	return fmt.Sprintf("### Build gate observation\n\nBuild check failed — informational only; paired review task decides outcome.\n\n```\n%s\n```", lastNLines(b.Output, 40))
}

func manifestFailureBody(m ManifestResult) string {
	var missing, found []string
	for _, d := range m.Deliverables {
		switch d.Status {
		case "missing":
			missing = append(missing, d.Item)
		case "done", "partial":
			found = append(found, d.Item)
		}
	}

	var sb strings.Builder
	sb.WriteString("### Manifest gate observation\n\nManifest deliverable check failed — informational only; paired review task decides outcome.\n\n")
	if len(missing) == 0 {
		sb.WriteString("Deliverables missing: (none identified individually)\n")
	} else {
		sb.WriteString("Deliverables missing:\n")
		for _, item := range missing {
			sb.WriteString("- ")
			sb.WriteString(item)
			sb.WriteString("\n")
		}
	}
	if len(found) > 0 {
		sb.WriteString("\nDeliverables found in diff:\n")
		for _, item := range found {
			sb.WriteString("- ")
			sb.WriteString(item)
			sb.WriteString("\n")
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

// lastNLines returns the trailing n non-empty-trimmed lines of s, preserving
// original order. If s has fewer than n lines, all of s is returned.
func lastNLines(s string, n int) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// resolveTaskBranch finds the branch this task committed on. Accepts either
// the plain openpraxis/<taskID> form or a suffixed openpraxis/<marker>-*
// form. Returns the chosen branch and ok=true; ok=false if no branch
// matches. Prefers the exact match; otherwise returns the most recently
// updated suffixed branch (newest commit on the branch tip).
func (w *Watcher) resolveTaskBranch(taskID string) (string, bool) {
	exact := "openpraxis/" + taskID
	pattern := exact + "-*"
	// --sort=-committerdate gives newest-first; --format=%(refname:short)
	// strips the refs/heads/ prefix.
	cmd := exec.Command("git", "branch",
		"--list", exact, pattern,
		"--sort=-committerdate",
		"--format=%(refname:short)")
	cmd.Dir = w.repoDir
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	var suffixed string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		if name == exact {
			return exact, true
		}
		if suffixed == "" {
			suffixed = name
		}
	}
	if suffixed != "" {
		return suffixed, true
	}
	return "", false
}

// RunGitGate checks if the task branch has commits.
//
// Branch resolution accepts both the plain marker branch
// (openpraxis/<taskID>) and any suffixed variant
// (openpraxis/<taskID>-*). Task descriptions sometimes direct the agent
// to include a human-readable suffix like `-m1-t1` in the branch name, and
// before this change the gate looked only at the exact plain form and
// reported BranchExists=false — which flipped completed runs to failed even
// when all the work was on disk and pushed.
func (w *Watcher) RunGitGate(taskID string) GitResult {
	result := GitResult{}
	branch, ok := w.resolveTaskBranch(taskID)
	if !ok {
		result.BranchExists = false
		result.Branch = "openpraxis/" + taskID
		result.Reason = fmt.Sprintf("no branch matching openpraxis/%s or openpraxis/%s-* found", taskID, taskID)
		return result
	}
	result.BranchExists = true
	result.Branch = branch

	// Find base branch — try sandbox, then main
	baseBranch := "sandbox"
	cmd := exec.Command("git", "rev-parse", "--verify", baseBranch)
	cmd.Dir = w.repoDir
	if err := cmd.Run(); err != nil {
		baseBranch = "main"
	}

	// Count commits unique to task branch vs base
	cmd = exec.Command("git", "log", "--oneline", baseBranch+".."+branch)
	cmd.Dir = w.repoDir
	out, err := cmd.Output()
	if err != nil {
		result.CommitCount = 0
		result.Reason = fmt.Sprintf("failed to compare %s..%s: %v", baseBranch, branch, err)
		return result
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		result.CommitCount = 0
	} else {
		result.CommitCount = len(lines)
	}

	// Get diff stats: task branch vs base branch
	cmd = exec.Command("git", "diff", "--stat", baseBranch+"..."+branch)
	cmd.Dir = w.repoDir
	out, _ = cmd.Output()
	statLines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range statLines {
		if strings.Contains(line, "insertion") || strings.Contains(line, "deletion") {
			// Parse summary line: " X files changed, Y insertions(+), Z deletions(-)"
			fmt.Sscanf(strings.TrimSpace(line), "%d files changed", &result.FilesChanged)
			if idx := strings.Index(line, "insertion"); idx > 0 {
				part := strings.TrimSpace(line[:idx])
				parts := strings.Split(part, ",")
				if len(parts) > 1 {
					fmt.Sscanf(strings.TrimSpace(parts[len(parts)-1]), "%d", &result.Insertions)
				}
			}
			if idx := strings.Index(line, "deletion"); idx > 0 {
				parts := strings.Split(line[:idx], ",")
				if len(parts) > 0 {
					fmt.Sscanf(strings.TrimSpace(parts[len(parts)-1]), "%d", &result.Deletions)
				}
			}
		}
	}

	// Check for uncommitted changes — only tracked modified files, not pre-existing untracked
	cmd = exec.Command("git", "status", "--porcelain", "-uno")
	cmd.Dir = w.repoDir
	out, _ = cmd.Output()
	if len(strings.TrimSpace(string(out))) > 0 {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line != "" {
				file := strings.TrimSpace(line[2:])
				result.UncommittedFiles = append(result.UncommittedFiles, file)
			}
		}
	}

	if result.CommitCount == 0 && len(result.UncommittedFiles) > 0 {
		result.Reason = fmt.Sprintf("ZERO commits but %d uncommitted modified files — agent did work but never committed", len(result.UncommittedFiles))
	} else if result.CommitCount == 0 {
		result.Reason = "ZERO commits and no changes — agent produced nothing"
	} else {
		result.Reason = fmt.Sprintf("%d commits on %s (vs %s), %d files changed", result.CommitCount, branch, baseBranch, result.FilesChanged)
	}

	return result
}

// RunBuildGate runs the build command and checks if it compiles.
func (w *Watcher) RunBuildGate() BuildResult {
	result := BuildResult{}

	parts := strings.Fields(w.buildCmd)
	if len(parts) == 0 {
		result.ExitCode = 0
		result.Reason = "no build command configured"
		return result
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Dir = w.repoDir

	out, err := cmd.CombinedOutput()
	output := string(out)

	// Filter out known warnings (cgo deprecation warnings)
	filtered := filterBuildOutput(output)

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = 1
		}
		result.Output = filtered
		result.Reason = fmt.Sprintf("build failed (exit %d): %s", result.ExitCode, truncate(filtered, 500))
	} else {
		result.ExitCode = 0
		result.Output = filtered
		result.Reason = "build passed"
	}

	return result
}

// RunManifestGate extracts deliverables from the manifest and checks which exist in the git diff.
func (w *Watcher) RunManifestGate(taskID, manifestContent string) ManifestResult {
	result := ManifestResult{}

	// Extract deliverables from manifest content
	deliverables := extractDeliverables(manifestContent)
	result.TotalItems = len(deliverables)

	if len(deliverables) == 0 {
		result.Reason = "no concrete deliverables found in manifest"
		return result
	}

	// Get the git diff for this task branch
	branch := "openpraxis/" + taskID
	cmd := exec.Command("git", "diff", "--name-only", branch+"^.."+branch)
	cmd.Dir = w.repoDir
	out, _ := cmd.Output()
	changedFiles := strings.Split(strings.TrimSpace(string(out)), "\n")

	// Also check uncommitted changes
	cmd = exec.Command("git", "status", "--porcelain")
	cmd.Dir = w.repoDir
	statusOut, _ := cmd.Output()
	for _, line := range strings.Split(string(statusOut), "\n") {
		if len(line) > 3 {
			changedFiles = append(changedFiles, strings.TrimSpace(line[2:]))
		}
	}

	// Get the full diff content for keyword matching
	cmd = exec.Command("git", "diff", "HEAD")
	cmd.Dir = w.repoDir
	diffOut, _ := cmd.Output()
	diffContent := strings.ToLower(string(diffOut))

	// Check each deliverable
	for _, d := range deliverables {
		status := "missing"
		evidence := ""

		lowerItem := strings.ToLower(d)

		// Check if any changed files match the deliverable
		for _, f := range changedFiles {
			if f == "" {
				continue
			}
			lowerFile := strings.ToLower(f)
			// Check if the deliverable mentions this file or directory
			if strings.Contains(lowerItem, filepath.Base(lowerFile)) ||
				strings.Contains(lowerFile, extractPath(lowerItem)) {
				status = "done"
				evidence = f
				break
			}
		}

		// If not found by file match, check diff content for keywords
		if status == "missing" {
			keywords := extractKeywords(d)
			matchCount := 0
			for _, kw := range keywords {
				if strings.Contains(diffContent, strings.ToLower(kw)) {
					matchCount++
				}
			}
			if len(keywords) > 0 && float64(matchCount)/float64(len(keywords)) >= 0.5 {
				status = "partial"
				evidence = fmt.Sprintf("found %d/%d keywords in diff", matchCount, len(keywords))
			}
		}

		result.Deliverables = append(result.Deliverables, Deliverable{
			Item:     d,
			Status:   status,
			Evidence: evidence,
		})

		switch status {
		case "done":
			result.DoneItems++
		case "partial":
			result.DoneItems++ // Count partial as done for scoring
		}
	}

	result.MissingItems = result.TotalItems - result.DoneItems
	result.Reason = fmt.Sprintf("%d/%d deliverables found", result.DoneItems, result.TotalItems)

	return result
}

// extractDeliverables parses manifest content for concrete deliverables.
// Looks for: file paths, "Add X to Y", "Create X", "New X", section headers with "File:" prefix.
func extractDeliverables(content string) []string {
	var deliverables []string
	seen := make(map[string]bool)

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// "File:" or "**File:**" patterns
		if strings.Contains(trimmed, "File:") || strings.Contains(trimmed, "**File:**") {
			item := strings.TrimPrefix(trimmed, "**File:**")
			item = strings.TrimPrefix(item, "File:")
			item = strings.TrimSpace(item)
			item = strings.Trim(item, "`")
			if item != "" && !seen[item] {
				deliverables = append(deliverables, item)
				seen[item] = true
			}
			continue
		}

		// Section headers that look like deliverables: "### 1. Something"
		if strings.HasPrefix(trimmed, "### ") && (strings.Contains(trimmed, "Add ") || strings.Contains(trimmed, "Create ") || strings.Contains(trimmed, "New ") || strings.Contains(trimmed, "Wire ") || strings.Contains(trimmed, "Build ") || strings.Contains(trimmed, "Hook ")) {
			item := strings.TrimPrefix(trimmed, "### ")
			// Strip leading numbers like "1. "
			for i, c := range item {
				if c == '.' && i < 4 {
					item = strings.TrimSpace(item[i+1:])
					break
				}
			}
			if !seen[item] {
				deliverables = append(deliverables, item)
				seen[item] = true
			}
			continue
		}

		// "## Gate X: Something" or "## Section" headers
		if strings.HasPrefix(trimmed, "## ") && !strings.HasPrefix(trimmed, "## Problem") && !strings.HasPrefix(trimmed, "## Architecture") {
			item := strings.TrimPrefix(trimmed, "## ")
			if len(item) > 5 && len(item) < 120 && !seen[item] {
				deliverables = append(deliverables, item)
				seen[item] = true
			}
		}

		// Bullet points with file paths: "- `internal/watcher/store.go`"
		if (strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ")) && strings.Contains(trimmed, "`") {
			// Extract backtick content
			parts := strings.Split(trimmed, "`")
			for i := 1; i < len(parts); i += 2 {
				if strings.Contains(parts[i], "/") || strings.HasSuffix(parts[i], ".go") || strings.HasSuffix(parts[i], ".js") {
					if !seen[parts[i]] {
						deliverables = append(deliverables, parts[i])
						seen[parts[i]] = true
					}
				}
			}
		}
	}

	return deliverables
}

// extractPath tries to find a file path in a deliverable description.
func extractPath(s string) string {
	// Look for common path patterns
	parts := strings.Fields(s)
	for _, p := range parts {
		p = strings.Trim(p, "`\"'()")
		if strings.Contains(p, "/") && (strings.HasSuffix(p, ".go") || strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".html") || strings.HasSuffix(p, ".css") || strings.Contains(p, "internal/")) {
			return p
		}
	}
	return ""
}

// extractKeywords pulls meaningful words from a deliverable for diff matching.
func extractKeywords(d string) []string {
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "to": true, "for": true, "in": true, "on": true,
		"with": true, "and": true, "or": true, "of": true, "is": true, "are": true, "be": true,
		"from": true, "by": true, "at": true, "as": true, "this": true, "that": true,
		"add": true, "create": true, "new": true, "update": true, "file": true,
	}

	var keywords []string
	for _, word := range strings.Fields(strings.ToLower(d)) {
		word = strings.Trim(word, "`*_#-—()[].,;:\"'")
		if len(word) > 3 && !stopWords[word] {
			keywords = append(keywords, word)
		}
	}
	return keywords
}

// filterBuildOutput removes known harmless warnings from build output.
func filterBuildOutput(output string) string {
	var filtered []string
	for _, line := range strings.Split(output, "\n") {
		// Skip cgo deprecation warnings on macOS
		if strings.Contains(line, "is deprecated") && strings.Contains(line, "sqlite3") {
			continue
		}
		if strings.Contains(line, "note:") && strings.Contains(line, "deprecated") {
			continue
		}
		if strings.Contains(line, "cgo-gcc-prolog") {
			continue
		}
		if strings.TrimSpace(line) != "" {
			filtered = append(filtered, line)
		}
	}
	return strings.Join(filtered, "\n")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
