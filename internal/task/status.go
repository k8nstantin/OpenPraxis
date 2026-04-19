package task

import (
	"fmt"
	"sort"
	"strings"
)

// Status is the canonical lifecycle state of a task row.
//
// This is a closed set of eight values. New code MUST use these constants
// instead of literal strings so the compiler flags typos and downstream
// exhaustive switches don't miss a case.
type Status string

const (
	// StatusPending — the task exists but has nothing arming it to fire.
	// No dependency, no schedule, no manual start. Stays here forever
	// unless the operator does one of those three things. New rows with
	// empty depends_on and no next_run_at land here.
	StatusPending Status = "pending"

	// StatusWaiting — the task has an unmet dependency and will auto-fire
	// when its parent task reaches StatusCompleted. Written by
	// Store.Create when depends_on is set and the parent isn't done, and
	// by the scheduler when it dequeues a task whose dep has regressed.
	// Distinct from Pending: Waiting participates in auto-activation,
	// Pending does not.
	StatusWaiting Status = "waiting"

	// StatusScheduled — armed for execution. The scheduler's dequeue
	// query picks up rows in this state where next_run_at <= now.
	StatusScheduled Status = "scheduled"

	// StatusRunning — agent subprocess is live.
	StatusRunning Status = "running"

	// StatusPaused — running process has been SIGSTOP'd by the operator
	// and can be resumed. Distinct from Scheduled: a paused task is
	// mid-execution with a live PID, not queued.
	StatusPaused Status = "paused"

	// StatusCompleted — agent exited and the watcher audit either passed
	// or returned a warning (warnings don't downgrade). Terminal unless
	// the watcher subsequently downgrades to Failed.
	StatusCompleted Status = "completed"

	// StatusFailed — agent exited with error, hit the cost cap, timed
	// out, OR was downgraded from Completed by a failing watcher audit.
	// Terminal.
	StatusFailed Status = "failed"

	// StatusCancelled — operator called task_cancel from any pre-terminal
	// state. Terminal.
	StatusCancelled Status = "cancelled"
)

// AllStatuses returns the eight canonical statuses in lifecycle order. Used
// by UI dropdowns and docs so the order stays stable.
func AllStatuses() []Status {
	return []Status{
		StatusPending, StatusWaiting, StatusScheduled,
		StatusRunning, StatusPaused,
		StatusCompleted, StatusFailed, StatusCancelled,
	}
}

// IsValidStatus reports whether s is one of the eight canonical statuses.
// Case-sensitive — callers must pass the exact string constant.
func IsValidStatus(s string) bool {
	for _, v := range AllStatuses() {
		if string(v) == s {
			return true
		}
	}
	return false
}

// IsTerminal reports whether the task can no longer transition out of this
// state under normal operation. Note: Completed is NOT terminal because
// the watcher can downgrade it to Failed — that's the one documented
// exception the state machine allows.
func IsTerminal(s Status) bool {
	return s == StatusFailed || s == StatusCancelled
}

// validTransitions encodes the state machine. Keys are the current state,
// values are the set of legal next states. Missing keys mean "no
// transitions allowed from this state" — used for Failed and Cancelled,
// which are fully terminal.
//
// Completed → Failed is the only transition out of a post-run state. It
// exists specifically for the watcher audit downgrade path (cmd/serve.go
// around line 227) and nowhere else should write that transition.
var validTransitions = map[Status]map[Status]bool{
	StatusPending: {
		StatusWaiting:   true, // operator or migration attaches a dependency
		StatusScheduled: true, // manual start or schedule attached
		StatusCancelled: true,
	},
	StatusWaiting: {
		StatusScheduled: true, // parent dep completed — ActivateDependents
		StatusPending:   true, // operator removed the dependency
		StatusCancelled: true,
	},
	StatusScheduled: {
		StatusRunning:   true, // scheduler dispatched
		StatusWaiting:   true, // dep regressed before dispatch
		StatusPending:   true, // operator cleared next_run_at + dep
		StatusCancelled: true,
	},
	StatusRunning: {
		StatusPaused:    true,
		StatusCompleted: true,
		StatusFailed:    true,
		StatusCancelled: true,
	},
	StatusPaused: {
		StatusRunning:   true,
		StatusCancelled: true,
	},
	StatusCompleted: {
		// Watcher audit downgrade is the only legal move out.
		StatusFailed: true,
	},
	// StatusFailed and StatusCancelled have no entries — terminal.
}

// CanTransition reports whether status `from` may move to `to`. Same-state
// "transitions" (from == to) return true as a convenience for idempotent
// writers — a redundant UPDATE isn't a semantic violation.
func CanTransition(from, to Status) bool {
	if from == to {
		return true
	}
	if !IsValidStatus(string(from)) {
		return false
	}
	if !IsValidStatus(string(to)) {
		return false
	}
	next, ok := validTransitions[from]
	if !ok {
		return false
	}
	return next[to]
}

// ValidateTransition returns nil if from → to is allowed, otherwise an
// error describing what was attempted and what would have been legal. Used
// by Store.UpdateStatus so operators + tests see the exact reason a write
// was rejected.
func ValidateTransition(from, to Status) error {
	if CanTransition(from, to) {
		return nil
	}
	if !IsValidStatus(string(from)) {
		return fmt.Errorf("invalid source status %q", from)
	}
	if !IsValidStatus(string(to)) {
		return fmt.Errorf("invalid target status %q", to)
	}
	allowed := []string{}
	for s := range validTransitions[from] {
		allowed = append(allowed, string(s))
	}
	sort.Strings(allowed)
	if len(allowed) == 0 {
		return fmt.Errorf("status %q is terminal; cannot transition to %q", from, to)
	}
	return fmt.Errorf("illegal transition %q → %q; legal next states from %q: %s",
		from, to, from, strings.Join(allowed, ", "))
}
