package task

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// legacyBuildPrompt is a verbatim copy of the pre-RC/M1 hardcoded
// buildPrompt. Kept here as an oracle so the refactored buildPrompt
// can be asserted byte-identical without depending on the committed
// snapshot file being in sync.
func legacyBuildPrompt(t *Task, manifestTitle, manifestContent, visceralRules string) string {
	var b strings.Builder
	b.WriteString("You are executing a scheduled task for OpenPraxis.\n\n")

	if visceralRules != "" {
		b.WriteString("<visceral_rules>\n")
		b.WriteString("MANDATORY — follow every rule without exception.\n\n")
		b.WriteString(visceralRules)
		b.WriteString("\n</visceral_rules>\n\n")
	}

	b.WriteString(fmt.Sprintf("<manifest_spec title=%q>\n", manifestTitle))
	b.WriteString(manifestContent)
	b.WriteString("\n</manifest_spec>\n\n")

	b.WriteString(fmt.Sprintf("<task title=%q id=%q>\n", t.Title, t.ID))
	if t.Description != "" {
		b.WriteString(t.Description)
		b.WriteString("\n")
	}
	b.WriteString("</task>\n\n")

	b.WriteString("<instructions>\n")
	b.WriteString("Follow the manifest spec exactly. Work autonomously.\n")
	b.WriteString("Call visceral_rules and visceral_confirm first.\n")
	b.WriteString("</instructions>\n\n")

	marker := t.ID
	if len(marker) >= 12 {
		marker = marker[:12]
	}
	b.WriteString("<git_workflow>\n")
	b.WriteString("MANDATORY — every task gets its own branch and PR.\n\n")
	b.WriteString("1. Before making ANY code changes, create a new branch:\n")
	b.WriteString(fmt.Sprintf("   git checkout -b openpraxis/%s\n", marker))
	b.WriteString("2. Make all your changes on this branch.\n")
	b.WriteString("3. Commit your work with a descriptive message.\n")
	b.WriteString(fmt.Sprintf("4. Push the branch: git push -u origin openpraxis/%s\n", marker))
	b.WriteString("5. Create a pull request using: gh pr create --title \"<title>\" --body \"<summary>\"\n")
	b.WriteString("6. Include the PR URL in your final output.\n\n")
	b.WriteString("NEVER work on an existing branch. NEVER push to main.\n")
	b.WriteString("</git_workflow>\n\n")

	b.WriteString("<closing_protocol>\n")
	b.WriteString("MANDATORY — before your final commit+push, call the MCP tool:\n\n")
	b.WriteString("    mcp__openpraxis__comment_add\n")
	b.WriteString(fmt.Sprintf("      target_type = \"task\"\n      target_id   = \"%s\"\n", t.ID))
	b.WriteString("      type        = \"execution_review\"\n")
	b.WriteString("      author      = \"agent\"\n")
	b.WriteString("      body        = <markdown summary>\n\n")
	b.WriteString("The body should include:\n")
	b.WriteString("- **What shipped** — files created/edited, key APIs, what's testable\n")
	b.WriteString("- **Gates self-check** — which acceptance-criteria bullets you verified locally (git gate: commits exist; build gate: go build passes; manifest gate: deliverables addressed)\n")
	b.WriteString("- **What the next task should expect** — APIs, error codes, file layout\n")
	b.WriteString("- **Anything surprising** — bugs found, decisions taken, followups to file\n\n")
	b.WriteString("This comment is your execution review — the canonical per-task home. The runner records an amnesia flag if this call is missing.\n")
	b.WriteString("</closing_protocol>\n\n")

	b.WriteString("Report completion when done.\n")
	return b.String()
}

// TestRunner_BuildPrompt_ByteIdentical ensures the RC/M1 resolver +
// renderer path reproduces the pre-refactor hardcoded buildPrompt byte
// for byte. Also round-trips against testdata/runner_prompt_snapshot.txt
// so an accidental template edit is caught by CI even if someone also
// breaks legacyBuildPrompt in the same change.
//
// Set UPDATE_SNAPSHOT=1 to rewrite the snapshot file (used exactly once,
// at check-in time).
func TestRunner_BuildPrompt_ByteIdentical(t *testing.T) {
	sample := &Task{
		ID:          "019dba9f-9c5b-76ba-8221-e7e11093887f",
		Title:       "Sample Task",
		Description: "Do the thing.\nSecond line.",
		ManifestID:  "019dba89-45d1-76ba-8221-000000000000",
	}
	manifestTitle := "Sample Manifest"
	manifestContent := "This is the manifest body.\n\n## Section\n\nMore text."
	visceralRules := "1. first rule\n2. second rule"

	legacy := legacyBuildPrompt(sample, manifestTitle, manifestContent, visceralRules)

	got, err := buildPrompt(sample, manifestTitle, manifestContent, visceralRules, "openpraxis", nil)
	if err != nil {
		t.Fatalf("buildPrompt: %v", err)
	}
	if got != legacy {
		diffPreview(t, "buildPrompt vs legacy", got, legacy)
		t.Fatalf("buildPrompt output diverged from legacy (len got=%d, legacy=%d)", len(got), len(legacy))
	}

	snapshotPath := filepath.Join("testdata", "runner_prompt_snapshot.txt")
	if os.Getenv("UPDATE_SNAPSHOT") == "1" {
		if err := os.MkdirAll(filepath.Dir(snapshotPath), 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(snapshotPath, []byte(got), 0o644); err != nil {
			t.Fatalf("write snapshot: %v", err)
		}
		t.Logf("wrote snapshot %s (%d bytes)", snapshotPath, len(got))
		return
	}

	want, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read snapshot (run with UPDATE_SNAPSHOT=1 to create): %v", err)
	}
	if string(want) != got {
		diffPreview(t, "buildPrompt vs snapshot file", got, string(want))
		t.Fatalf("buildPrompt output diverged from testdata snapshot")
	}
}

// TestRunner_BuildPrompt_VisceralRulesEmpty covers the skip-when-empty
// branch: the visceral_rules section must disappear entirely, not
// render as an empty wrapper.
func TestRunner_BuildPrompt_VisceralRulesEmpty(t *testing.T) {
	sample := &Task{ID: "abc", Title: "x"}
	got, err := buildPrompt(sample, "M", "m body", "", "openpraxis", nil)
	if err != nil {
		t.Fatalf("buildPrompt: %v", err)
	}
	if strings.Contains(got, "<visceral_rules>") {
		t.Fatalf("visceral_rules section should be absent when VisceralRules is empty; got:\n%s", got)
	}
	legacy := legacyBuildPrompt(sample, "M", "m body", "")
	if got != legacy {
		diffPreview(t, "empty-visceral vs legacy", got, legacy)
		t.Fatalf("buildPrompt diverged on empty visceral rules")
	}
}

// diffPreview prints the first 400 chars of each side + the first
// position at which the two strings diverge so `go test` surfaces the
// actual mismatch instead of forcing the reader to diff two blobs by hand.
func diffPreview(t *testing.T, label, got, want string) {
	t.Helper()
	t.Logf("%s — diff", label)
	minLen := len(got)
	if len(want) < minLen {
		minLen = len(want)
	}
	for i := 0; i < minLen; i++ {
		if got[i] != want[i] {
			start := i - 20
			if start < 0 {
				start = 0
			}
			end := i + 40
			if end > minLen {
				end = minLen
			}
			t.Logf("first diff at byte %d:\n  got : %q\n  want: %q", i, got[start:end], want[start:end])
			return
		}
	}
	if len(got) != len(want) {
		t.Logf("content matches up to %d bytes; lengths differ got=%d want=%d", minLen, len(got), len(want))
	}
}
