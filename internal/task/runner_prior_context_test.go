package task

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
	"github.com/k8nstantin/OpenPraxis/internal/execution"
)

// openPriorContextStores spins up an in-memory sqlite DB with the
// execution_log + comments schemas and returns wired stores. Used by
// the prior_context tests below to exercise the real query path
// buildPrompt takes against `*execution.Store` and `*comments.Store`.
func openPriorContextStores(t *testing.T) (*execution.Store, *comments.Store) {
	t.Helper()
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared&_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := execution.InitSchema(db); err != nil {
		t.Fatalf("execution InitSchema: %v", err)
	}
	if err := comments.InitSchema(db); err != nil {
		t.Fatalf("comments InitSchema: %v", err)
	}
	return execution.NewStore(db), comments.NewStore(db)
}

// seedRun inserts one execution_log row for entityID with the supplied
// summary fields. RunUID is derived from suffix so the test can assert
// the rendered run digest deterministically.
func seedRun(t *testing.T, store *execution.Store, entityID, runSuffix string, turns, actions int, cost float64) {
	t.Helper()
	row := execution.Row{
		RunUID:           "run-" + runSuffix + "-aaaaaaaa",
		EntityUID:        entityID,
		Event:            execution.EventCompleted,
		TerminalReason:   "success",
		Turns:            turns,
		Actions:          actions,
		EstimatedCostUSD: cost,
		DurationMS:       1234,
		Branch:           "openpraxis/" + entityID,
		LinesAdded:       10,
		LinesRemoved:     2,
		CreatedBy:        "test",
	}
	if err := store.Insert(context.Background(), row); err != nil {
		t.Fatalf("insert run %s: %v", runSuffix, err)
	}
	// Stagger created_at so ListByEntity ordering is stable across rows.
	time.Sleep(2 * time.Millisecond)
}

// TestBuildPrompt_PriorRuns_RenderedInBlock asserts that two completed
// runs in execution_log render into the <prior_context> section of the
// prompt with a distinct line per run.
func TestBuildPrompt_PriorRuns_RenderedInBlock(t *testing.T) {
	exec, cmt := openPriorContextStores(t)
	taskID := "019dba9f-9c5b-76ba-8221-e7e11093887f"
	seedRun(t, exec, taskID, "first", 5, 12, 0.123)
	seedRun(t, exec, taskID, "secnd", 7, 20, 0.456)

	knobs := runtimeKnobs{
		BranchPrefix:           "openpraxis",
		PromptMaxCommentChars:  2000,
		PromptMaxContextPct:    0.40,
		PromptPriorRunsLimit:   5,
		PromptBuildTimeoutSecs: 5,
	}
	got, err := buildPrompt(&Task{ID: taskID, Title: "T"}, "M", "m body", "", knobs, nil, exec, cmt)
	if err != nil {
		t.Fatalf("buildPrompt: %v", err)
	}
	if !strings.Contains(got, "<prior_context>") {
		t.Fatalf("prompt missing <prior_context> block; got:\n%s", got)
	}
	if !strings.Contains(got, "## Prior Runs") {
		t.Fatalf("prior_context missing 'Prior Runs' header; got:\n%s", got)
	}
	if !strings.Contains(got, "Run #1 (run-secn)") || !strings.Contains(got, "Run #2 (run-firs)") {
		t.Fatalf("prior_context missing both run digests (newest-first, 8-char run uid); got:\n%s", got)
	}
}

// TestBuildPrompt_PriorRuns_FirstRunOmitsBlock — when no execution_log
// rows exist for the task, the prior_context section must render empty
// (template guard) and the block markers must not appear at all.
func TestBuildPrompt_PriorRuns_FirstRunOmitsBlock(t *testing.T) {
	exec, cmt := openPriorContextStores(t)
	knobs := runtimeKnobs{
		BranchPrefix:             "openpraxis",
		PromptMaxCommentChars:    2000,
		PromptMaxContextPct:      0.40,
		PromptPriorRunsLimit:     5,
		PromptPriorCommentsLimit: 3,
		PromptBuildTimeoutSecs:   5,
	}
	got, err := buildPrompt(&Task{ID: "no-history-task", Title: "T"}, "M", "m body", "", knobs, nil, exec, cmt)
	if err != nil {
		t.Fatalf("buildPrompt: %v", err)
	}
	if strings.Contains(got, "<prior_context>") {
		t.Fatalf("prior_context block should be absent on first run; got:\n%s", got)
	}
}

// TestBuildPrompt_PriorRunsLimit_ZeroDisables verifies that
// PromptPriorRunsLimit=0 fully disables run injection even when rows
// exist. Combined with PromptPriorCommentsLimit=0 the block must
// disappear entirely.
func TestBuildPrompt_PriorRunsLimit_ZeroDisables(t *testing.T) {
	exec, cmt := openPriorContextStores(t)
	taskID := "limit-zero-task"
	seedRun(t, exec, taskID, "first", 3, 5, 0.05)

	knobs := runtimeKnobs{
		BranchPrefix:             "openpraxis",
		PromptMaxCommentChars:    2000,
		PromptMaxContextPct:      0.40,
		PromptPriorRunsLimit:     0,
		PromptPriorCommentsLimit: 0,
		PromptBuildTimeoutSecs:   5,
	}
	got, err := buildPrompt(&Task{ID: taskID, Title: "T"}, "M", "m body", "", knobs, nil, exec, cmt)
	if err != nil {
		t.Fatalf("buildPrompt: %v", err)
	}
	if strings.Contains(got, "<prior_context>") {
		t.Fatalf("prior_context must be absent when limits are 0; got:\n%s", got)
	}
}

// TestBuildPrompt_BudgetOverflow_DropsOldest — when the combined size
// of PriorRuns exceeds budget=defaultModelContextChars*pct, the oldest
// entries (tail of the slice) are dropped first until the sum fits.
// PriorRuns is newest-first (matching ListByEntity DESC ordering), so
// the tail is the oldest.
func TestBuildPrompt_BudgetOverflow_DropsOldest(t *testing.T) {
	exec, cmt := openPriorContextStores(t)
	taskID := "budget-overflow-task"
	// Three runs — each digest line is well under 2000 chars but sum ~600 chars.
	seedRun(t, exec, taskID, "oldst", 1, 1, 0.01)
	seedRun(t, exec, taskID, "midle", 2, 2, 0.02)
	seedRun(t, exec, taskID, "newst", 3, 3, 0.03)

	// pct chosen so budget = 200_000 * pct < 2 * line length.
	// One digest line is roughly 130 chars; pct=0.001 → budget=200 chars,
	// which fits one line and forces dropping the oldest two.
	knobs := runtimeKnobs{
		BranchPrefix:           "openpraxis",
		PromptMaxCommentChars:  2000,
		PromptMaxContextPct:    0.001,
		PromptPriorRunsLimit:   5,
		PromptBuildTimeoutSecs: 5,
	}
	got, err := buildPrompt(&Task{ID: taskID, Title: "T"}, "M", "m body", "", knobs, nil, exec, cmt)
	if err != nil {
		t.Fatalf("buildPrompt: %v", err)
	}
	if !strings.Contains(got, "<prior_context>") {
		t.Fatalf("budget overflow should still keep at least one run; got:\n%s", got)
	}
	// Run UIDs are truncated to 8 chars in the digest line. "run-newst" → "run-news",
	// "run-oldst" → "run-olds". Newest must survive; oldest must be dropped.
	if !strings.Contains(got, "run-news") {
		t.Fatalf("newest run digest should survive budget pruning; got:\n%s", got)
	}
	if strings.Contains(got, "run-olds") {
		t.Fatalf("oldest run digest should be dropped first under budget; got:\n%s", got)
	}
}

// TestBuildPrompt_OtherComments_RenderedInBlock — agent-authored
// comments on the task surface as the "Related Comments" subsection.
func TestBuildPrompt_OtherComments_RenderedInBlock(t *testing.T) {
	exec, cmt := openPriorContextStores(t)
	taskID := "comments-task"
	if _, err := cmt.Add(context.Background(), comments.TargetEntity, taskID, "agent", comments.TypeComment, "first finding"); err != nil {
		t.Fatalf("add comment: %v", err)
	}

	knobs := runtimeKnobs{
		BranchPrefix:             "openpraxis",
		PromptMaxCommentChars:    2000,
		PromptMaxContextPct:      0.40,
		PromptPriorCommentsLimit: 3,
		PromptBuildTimeoutSecs:   5,
	}
	got, err := buildPrompt(&Task{ID: taskID, Title: "T"}, "M", "m body", "", knobs, nil, exec, cmt)
	if err != nil {
		t.Fatalf("buildPrompt: %v", err)
	}
	if !strings.Contains(got, "## Related Comments") {
		t.Fatalf("prior_context missing 'Related Comments' header; got:\n%s", got)
	}
	if !strings.Contains(got, "first finding") {
		t.Fatalf("comment body missing from prior_context; got:\n%s", got)
	}
}

// TestBuildPrompt_PromptBuildTimeout_DoesNotHang — a 1-second timeout
// must not cause buildPrompt to error or stall on the happy path. This
// confirms the bounded context replaces the prior context.Background()
// without breaking normal queries (regression guard).
func TestBuildPrompt_PromptBuildTimeout_DoesNotHang(t *testing.T) {
	exec, cmt := openPriorContextStores(t)
	knobs := runtimeKnobs{
		BranchPrefix:           "openpraxis",
		PromptMaxCommentChars:  2000,
		PromptMaxContextPct:    0.40,
		PromptPriorRunsLimit:   5,
		PromptBuildTimeoutSecs: 1,
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := buildPrompt(&Task{ID: "timeout-task", Title: "T"}, "M", "m body", "", knobs, nil, exec, cmt); err != nil {
			t.Errorf("buildPrompt under 1s timeout: %v", err)
		}
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("buildPrompt did not return within 3s under PromptBuildTimeoutSecs=1")
	}
}
