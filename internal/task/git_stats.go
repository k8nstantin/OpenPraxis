package task

import (
	"strconv"
	"strings"
)

// RunGitStats collects lines added/removed, files changed, commit count,
// HEAD commit SHA, and current branch since baseSHA in workDir.
// Returns zero values on any error — git stats are best-effort.
func RunGitStats(workDir, baseSHA string) (linesAdded, linesRemoved, filesChanged, commits int, headSHA, branch string) {
	if workDir == "" || baseSHA == "" {
		return
	}

	// Lines added/removed/files — git diff --shortstat baseSHA..HEAD
	if out, err := runGit(workDir, "diff", "--shortstat", baseSHA+"..HEAD"); err == nil {
		linesAdded, linesRemoved, filesChanged = parseShortstat(out)
	}

	// Commit count since baseSHA
	if out, err := runGit(workDir, "rev-list", "--count", baseSHA+"..HEAD"); err == nil {
		if n, err := strconv.Atoi(strings.TrimSpace(out)); err == nil {
			commits = n
		}
	}

	// HEAD SHA
	if out, err := runGit(workDir, "rev-parse", "HEAD"); err == nil {
		headSHA = strings.TrimSpace(out)
	}

	// Current branch
	if out, err := runGit(workDir, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		branch = strings.TrimSpace(out)
	}

	return
}

// parseShortstat parses "git diff --shortstat" output:
// " 5 files changed, 123 insertions(+), 45 deletions(-)"
func parseShortstat(s string) (added, removed, files int) {
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		var n int
		if strings.Contains(part, "insertion") {
			n, _ = strconv.Atoi(strings.Fields(part)[0])
			added = n
		} else if strings.Contains(part, "deletion") {
			n, _ = strconv.Atoi(strings.Fields(part)[0])
			removed = n
		} else if strings.Contains(part, "file") {
			n, _ = strconv.Atoi(strings.Fields(part)[0])
			files = n
		}
	}
	return
}

