package task

import (
	"strings"
	"testing"
)

// TestAllStatuses_StableOrder — the lifecycle order matters for UI
// dropdowns and docs. Any reordering is a breaking change visible to
// users, so lock it down.
func TestAllStatuses_StableOrder(t *testing.T) {
	want := []Status{
		StatusPending, StatusWaiting, StatusScheduled,
		StatusRunning, StatusPaused,
		StatusCompleted, StatusFailed, StatusCancelled,
	}
	got := AllStatuses()
	if len(got) != len(want) {
		t.Fatalf("AllStatuses len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestIsValidStatus — string round-trip catches typos in callers that
// still pass literal strings (HTTP handlers, migration SQL, etc.).
func TestIsValidStatus(t *testing.T) {
	for _, s := range AllStatuses() {
		if !IsValidStatus(string(s)) {
			t.Errorf("IsValidStatus(%q) = false, want true", s)
		}
	}
	bad := []string{"", "Running", "PENDING", "done", "error", "queued"}
	for _, s := range bad {
		if IsValidStatus(s) {
			t.Errorf("IsValidStatus(%q) = true, want false", s)
		}
	}
}

// TestIsTerminal — only Failed and Cancelled are terminal. Completed is
// explicitly NOT terminal because the watcher may downgrade it to Failed.
// If this test fails, the cmd/serve.go audit callback needs review.
func TestIsTerminal(t *testing.T) {
	cases := map[Status]bool{
		StatusPending:   false,
		StatusWaiting:   false,
		StatusScheduled: false,
		StatusRunning:   false,
		StatusPaused:    false,
		StatusCompleted: false, // downgradable by watcher — see Completed→Failed
		StatusFailed:    true,
		StatusCancelled: true,
	}
	for s, want := range cases {
		if got := IsTerminal(s); got != want {
			t.Errorf("IsTerminal(%q) = %v, want %v", s, got, want)
		}
	}
}

// TestCanTransition_AllLegalMoves — the transitions we documented MUST be
// allowed. Each row is a pair we deliberately support; breaking any of
// them is a regression in the state machine.
func TestCanTransition_AllLegalMoves(t *testing.T) {
	legal := [][2]Status{
		{StatusPending, StatusWaiting},
		{StatusPending, StatusScheduled},
		{StatusPending, StatusCancelled},
		{StatusWaiting, StatusScheduled},
		{StatusWaiting, StatusPending},
		{StatusWaiting, StatusCancelled},
		{StatusScheduled, StatusRunning},
		{StatusScheduled, StatusWaiting},
		{StatusScheduled, StatusPending},
		{StatusScheduled, StatusCancelled},
		{StatusRunning, StatusPaused},
		{StatusRunning, StatusCompleted},
		{StatusRunning, StatusFailed},
		{StatusRunning, StatusCancelled},
		{StatusPaused, StatusRunning},
		{StatusPaused, StatusCancelled},
		{StatusCompleted, StatusFailed}, // watcher downgrade — the one exception
	}
	for _, pair := range legal {
		if !CanTransition(pair[0], pair[1]) {
			t.Errorf("CanTransition(%q, %q) = false, want true (documented legal)", pair[0], pair[1])
		}
	}
}

// TestCanTransition_SelfLoopAlwaysTrue — writers that re-assert the
// current status are idempotent, not errors. Same state is always allowed
// so the UPDATE path doesn't reject a no-op.
func TestCanTransition_SelfLoopAlwaysTrue(t *testing.T) {
	for _, s := range AllStatuses() {
		if !CanTransition(s, s) {
			t.Errorf("CanTransition(%q, %q) = false; self-loop must be allowed", s, s)
		}
	}
}

// TestCanTransition_IllegalMovesRejected — the state machine's job is to
// catch these. Completed→Running would mean rerunning a closed task;
// Failed→anything would resurrect a terminal row; Cancelled→anything
// likewise. These must all be refused.
func TestCanTransition_IllegalMovesRejected(t *testing.T) {
	illegal := [][2]Status{
		{StatusCompleted, StatusRunning}, // already closed, can't reopen
		{StatusCompleted, StatusScheduled},
		{StatusCompleted, StatusCancelled}, // terminal after the fact isn't how cancel works
		{StatusFailed, StatusRunning},      // terminal, no resurrection
		{StatusFailed, StatusCompleted},
		{StatusCancelled, StatusScheduled},
		{StatusCancelled, StatusRunning},
		{StatusPending, StatusRunning}, // must go through Scheduled
		{StatusPending, StatusCompleted},
		{StatusWaiting, StatusRunning}, // must go Waiting→Scheduled→Running
		{StatusRunning, StatusScheduled},
		{StatusRunning, StatusPending},
	}
	for _, pair := range illegal {
		if CanTransition(pair[0], pair[1]) {
			t.Errorf("CanTransition(%q, %q) = true, want false", pair[0], pair[1])
		}
	}
}

// TestValidateTransition_ErrorMessageHelps — when the state machine
// rejects a move, the error should name the offending pair AND list what
// would have been legal. Silent "permission denied" errors waste operator
// debugging time.
func TestValidateTransition_ErrorMessageHelps(t *testing.T) {
	err := ValidateTransition(StatusPending, StatusRunning)
	if err == nil {
		t.Fatal("ValidateTransition(pending→running) should fail")
	}
	msg := err.Error()
	for _, want := range []string{"pending", "running", "scheduled", "waiting"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q: %s", want, msg)
		}
	}
}

// TestValidateTransition_TerminalHasDedicatedMessage — for Failed and
// Cancelled we want the error to say "terminal" explicitly, not just
// list empty allowed states — that's actionable context.
func TestValidateTransition_TerminalHasDedicatedMessage(t *testing.T) {
	err := ValidateTransition(StatusFailed, StatusRunning)
	if err == nil {
		t.Fatal("failed→running should fail")
	}
	if !strings.Contains(err.Error(), "terminal") {
		t.Errorf("terminal error message missing 'terminal': %s", err.Error())
	}
}

// TestStoreUpdateStatus_ValidatesTransition — the Store boundary is where
// the state machine is enforced. Writing an illegal transition must
// return an error and leave the row unchanged; writing a legal one must
// succeed and persist.
func TestStoreUpdateStatus_ValidatesTransition(t *testing.T) {
	s := openRepoTestStore(t)
	task, err := s.Create("", "x", "", "once", "claude-code", "node", "t", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Pending→Running is illegal (must go through Scheduled). Must reject.
	if err := s.UpdateStatus(task.ID, string(StatusRunning)); err == nil {
		t.Fatal("UpdateStatus pending→running: want error, got nil")
	}
	if got := readStatus(t, s, task.ID); got != "pending" {
		t.Fatalf("row mutated after rejected transition: status = %q", got)
	}
	// Pending→Scheduled is legal. Must succeed.
	if err := s.UpdateStatus(task.ID, string(StatusScheduled)); err != nil {
		t.Fatalf("UpdateStatus pending→scheduled: %v", err)
	}
	if got := readStatus(t, s, task.ID); got != "scheduled" {
		t.Fatalf("status = %q, want scheduled after legal transition", got)
	}
}
