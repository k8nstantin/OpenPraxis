package task

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/k8nstantin/OpenPraxis/internal/action"
)

// fakeExecReview is an in-memory ExecutionReviewChecker. has reports whether
// the task has an agent-authored execution_review comment. err forces the
// checker to return an error so the warn-and-continue path can be exercised.
type fakeExecReview struct {
	has    map[string]bool
	err    error
	calls  int
	lastID string
}

func (f *fakeExecReview) HasAgentExecutionReview(_ context.Context, taskID string) (bool, error) {
	f.calls++
	f.lastID = taskID
	if f.err != nil {
		return false, f.err
	}
	return f.has[taskID], nil
}

// openAmnesiaDB opens an isolated sqlite DB with WAL+busy_timeout and
// initializes the action store (which owns the amnesia table). Returns the
// store and a t.Cleanup-registered close.
func openAmnesiaDB(t *testing.T) *action.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "actions.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("WAL: %v", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		t.Fatalf("busy_timeout: %v", err)
	}
	s, err := action.NewStore(db)
	if err != nil {
		t.Fatalf("action.NewStore: %v", err)
	}
	return s
}

// TestRunner_PromptTemplate_IncludesClosingSection — the spawn prompt must
// tell the agent to call comment_add with type=execution_review before
// finishing, and must interpolate the full task ID into target_id.
func TestRunner_PromptTemplate_IncludesClosingSection(t *testing.T) {
	task := &Task{ID: "019da142-479c-70d2-865b-d6e593883e3f", Title: "demo"}
	got := buildPrompt(task, "Manifest X", "manifest body", "rules body")

	mustContain(t, got, "<closing_protocol>")
	mustContain(t, got, "mcp__openpraxis__comment_add")
	mustContain(t, got, `type        = "execution_review"`)
	mustContain(t, got, `author      = "agent"`)
	// target_id must be the full task ID so the comment lands on the right row.
	mustContain(t, got, `target_id   = "019da142-479c-70d2-865b-d6e593883e3f"`)
}

// TestRunner_MissingExecutionReview_FlagsAmnesia — when the task finishes
// with status=completed / reason=success and no agent execution_review
// comment exists, the runner records an amnesia flag scoped to the task.
func TestRunner_MissingExecutionReview_FlagsAmnesia(t *testing.T) {
	r, _, _, _ := newRunnerHarness(t)
	actions := openAmnesiaDB(t)
	r.actions = actions
	checker := &fakeExecReview{has: map[string]bool{}} // nothing recorded
	r.SetExecutionReviewChecker(checker)

	r.enforceExecutionReview(context.Background(), "t-abc", "t-abc", "completed", "success")

	if checker.calls != 1 {
		t.Fatalf("checker calls = %d, want 1", checker.calls)
	}
	if checker.lastID != "t-abc" {
		t.Fatalf("checker taskID = %q, want t-abc", checker.lastID)
	}
	list, err := actions.ListAmnesiaByTask("t-abc", 10)
	if err != nil {
		t.Fatalf("ListAmnesiaByTask: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("amnesia rows = %d, want 1", len(list))
	}
	if list[0].RuleMarker != "exec-review" {
		t.Fatalf("RuleMarker = %q, want exec-review", list[0].RuleMarker)
	}
	if list[0].MatchedPattern != "missing_execution_review" {
		t.Fatalf("MatchedPattern = %q, want missing_execution_review", list[0].MatchedPattern)
	}
}

// TestRunner_WithExecutionReview_NoAmnesia — when the comment exists, the
// runner records no amnesia flag.
func TestRunner_WithExecutionReview_NoAmnesia(t *testing.T) {
	r, _, _, _ := newRunnerHarness(t)
	actions := openAmnesiaDB(t)
	r.actions = actions
	checker := &fakeExecReview{has: map[string]bool{"t-ok": true}}
	r.SetExecutionReviewChecker(checker)

	r.enforceExecutionReview(context.Background(), "t-ok", "t-ok", "completed", "success")

	list, err := actions.ListAmnesiaByTask("t-ok", 10)
	if err != nil {
		t.Fatalf("ListAmnesiaByTask: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("amnesia rows = %d, want 0", len(list))
	}
}

// TestRunner_FailedTask_SkipsExecReviewGate — the gate fires only for
// status=completed/reason=success. Failures and other terminal reasons
// (max_turns, cost_cap, timeout) must not trip the check.
func TestRunner_FailedTask_SkipsExecReviewGate(t *testing.T) {
	r, _, _, _ := newRunnerHarness(t)
	actions := openAmnesiaDB(t)
	r.actions = actions
	checker := &fakeExecReview{has: map[string]bool{}}
	r.SetExecutionReviewChecker(checker)

	cases := []struct {
		status string
		reason string
	}{
		{"failed", "process_error"},
		{"failed", "timeout"},
		{"failed", "cost_cap"},
		{"completed", "max_turns"}, // completed-but-not-success also skips
	}
	for _, c := range cases {
		r.enforceExecutionReview(context.Background(), "t-x", "t-x", c.status, c.reason)
	}
	if checker.calls != 0 {
		t.Fatalf("checker fired %d times for non-success terminals, want 0", checker.calls)
	}
	list, _ := actions.ListAmnesiaByTask("t-x", 10)
	if len(list) != 0 {
		t.Fatalf("amnesia rows = %d, want 0", len(list))
	}
}

// TestRunner_NilChecker_IsNoOp — when no checker is wired, the gate does
// nothing and no amnesia rows are written.
func TestRunner_NilChecker_IsNoOp(t *testing.T) {
	r, _, _, _ := newRunnerHarness(t)
	actions := openAmnesiaDB(t)
	r.actions = actions
	// Deliberately do NOT call SetExecutionReviewChecker.

	r.enforceExecutionReview(context.Background(), "t-nil", "t-nil", "completed", "success")

	list, _ := actions.ListAmnesiaByTask("t-nil", 10)
	if len(list) != 0 {
		t.Fatalf("amnesia rows = %d, want 0 (checker not wired)", len(list))
	}
}

// TestRunner_CheckerError_IsLoggedNotRecorded — a checker that returns an
// error must neither record amnesia nor panic. The gate degrades to "warn".
func TestRunner_CheckerError_IsLoggedNotRecorded(t *testing.T) {
	r, _, _, _ := newRunnerHarness(t)
	actions := openAmnesiaDB(t)
	r.actions = actions
	checker := &fakeExecReview{err: errors.New("boom")}
	r.SetExecutionReviewChecker(checker)

	r.enforceExecutionReview(context.Background(), "t-err", "t-err", "completed", "success")

	list, _ := actions.ListAmnesiaByTask("t-err", 10)
	if len(list) != 0 {
		t.Fatalf("amnesia rows = %d, want 0 (checker erred)", len(list))
	}
}

func mustContain(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Fatalf("prompt missing %q\n---prompt---\n%s", sub, s)
	}
}
