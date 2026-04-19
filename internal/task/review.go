package task

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Review-loop sentinels. Handlers (HTTP / MCP) translate these into
// their appropriate error surfaces.
var (
	ErrTaskNotCompleted   = errors.New("task review: only completed tasks can be rejected or approved")
	ErrEmptyReviewReason  = errors.New("task review: rejection reason cannot be empty")
	ErrReviewNotAvailable = errors.New("task review: no review comment writer wired")
)

// ReviewComment is the trimmed view task.Store needs of a review
// comment row. comments.Comment carries more fields than TaskReviewStatus
// needs, so we hand-roll a narrower struct here to avoid an import
// on internal/comments.
type ReviewComment struct {
	ID        string
	Type      string // "review_rejection" | "review_approval"
	Author    string
	Body      string
	CreatedAt time.Time
}

// ReviewWriter is what task.Store calls to append a review
// comment (rejection or approval) to a task. node.go wires an adapter
// around comments.Store that satisfies this interface — keeps task
// free of an internal/comments import so the two packages stay
// independently testable.
type ReviewWriter interface {
	AddReviewComment(ctx context.Context, taskID, author, commentType, body string) error
}

// ReviewReader lists review-type comments for a task. Same injection
// pattern: node.go bridges comments.Store.ListByTarget to this shape.
type ReviewReader interface {
	ListReviewCommentsForTask(ctx context.Context, taskID string, limit int) ([]ReviewComment, error)
}

// SetReviewCommentsAPI wires both the writer and reader. Both can be
// the same object in production — the split lets tests stub just one
// side. Nil for either disables that half of the feature; methods
// that need a missing side return ErrReviewNotAvailable rather than
// panic.
func (s *Store) SetReviewCommentsAPI(w ReviewWriter, r ReviewReader) {
	s.reviewWriter = w
	s.reviewReader = r
}

// TaskReviewStatus is the derived view of a task's review state.
// NeedsRework is true iff the latest review-type comment (by
// created_at) is a rejection. An approval clears it; a subsequent
// rejection re-sets it.
type TaskReviewStatus struct {
	NeedsRework           bool      `json:"needs_rework"`
	HasApproval           bool      `json:"has_approval"`
	LatestRejectionAt     time.Time `json:"latest_rejection_at,omitempty"`
	LatestRejectionReason string    `json:"latest_rejection_reason,omitempty"`
	LatestRejectionBy     string    `json:"latest_rejection_by,omitempty"`
	LatestApprovalAt      time.Time `json:"latest_approval_at,omitempty"`
	LatestApprovalBy      string    `json:"latest_approval_by,omitempty"`
}

// RejectCompletedTask flips a completed task back to scheduled for
// another execution pass and attaches a review_rejection comment with
// the given reason + reviewer. The two writes are not wrapped in a
// transaction — SQLite's serializability on a single process plus
// WAL's snapshot semantics make the ordering visible-but-not-atomic.
// Worst case: comment persists without the status flip (rare, only
// on a crash between the two Execs). Operator-visible residue is an
// out-of-sync task that can be re-rejected; no data is lost.
func (s *Store) RejectCompletedTask(ctx context.Context, taskID, reason, reviewer string) error {
	if s.reviewWriter == nil {
		return ErrReviewNotAvailable
	}
	if reason == "" {
		return ErrEmptyReviewReason
	}

	// Resolve + guard current status.
	var fullID, currentStatus string
	if err := s.db.QueryRowContext(ctx,
		`SELECT id, status FROM tasks WHERE (id = ? OR id LIKE ?) AND deleted_at = ''`,
		taskID, taskID+"%").Scan(&fullID, &currentStatus); err != nil {
		return err
	}
	if currentStatus != string(StatusCompleted) {
		return fmt.Errorf("%w: current status=%s", ErrTaskNotCompleted, currentStatus)
	}

	// Append the rejection comment first so there's a record even if
	// the status update fails.
	if err := s.reviewWriter.AddReviewComment(ctx, fullID, reviewer, "review_rejection", reason); err != nil {
		return fmt.Errorf("write rejection comment: %w", err)
	}

	// Status transition. completed → scheduled is legal per #93; no
	// block_reason since the task is armed for re-run.
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET status = ?, block_reason = '', next_run_at = ?, updated_at = ? WHERE id = ?`,
		string(StatusScheduled), now, now, fullID)
	return err
}

// ApproveCompletedTask attaches a review_approval comment. Does NOT
// change the task status — approval is an input to manifest closure
// warnings, not a state-machine transition. Rejecting an already-
// approved task is allowed (the new rejection comment supersedes the
// approval per created_at ordering in TaskReviewStatus).
func (s *Store) ApproveCompletedTask(ctx context.Context, taskID, reviewer string) error {
	if s.reviewWriter == nil {
		return ErrReviewNotAvailable
	}
	var fullID, currentStatus string
	if err := s.db.QueryRowContext(ctx,
		`SELECT id, status FROM tasks WHERE (id = ? OR id LIKE ?) AND deleted_at = ''`,
		taskID, taskID+"%").Scan(&fullID, &currentStatus); err != nil {
		return err
	}
	if currentStatus != string(StatusCompleted) {
		return fmt.Errorf("%w: current status=%s", ErrTaskNotCompleted, currentStatus)
	}
	body := "Approved."
	if reviewer == "" {
		reviewer = "reviewer"
	}
	return s.reviewWriter.AddReviewComment(ctx, fullID, reviewer, "review_approval", body)
}

// TaskReviewStatus reports whether the task has an outstanding
// rejection or standing approval. The latest review comment wins —
// if a rejection was posted after an approval, NeedsRework=true and
// HasApproval=false.
func (s *Store) TaskReviewStatus(ctx context.Context, taskID string) (TaskReviewStatus, error) {
	if s.reviewReader == nil {
		// No reader wired — return an empty status rather than a
		// hard error so callers that don't care about review state
		// (the scheduler, basic list views) degrade silently.
		return TaskReviewStatus{}, nil
	}
	// Resolve the full ID so a marker-prefix input still hits the
	// reader with the canonical id the comments table stores.
	var fullID string
	if err := s.db.QueryRowContext(ctx,
		`SELECT id FROM tasks WHERE (id = ? OR id LIKE ?) AND deleted_at = ''`,
		taskID, taskID+"%").Scan(&fullID); err != nil {
		return TaskReviewStatus{}, err
	}
	comments, err := s.reviewReader.ListReviewCommentsForTask(ctx, fullID, 200)
	if err != nil {
		return TaskReviewStatus{}, err
	}

	// Walk comments newest-first. First rejection we see is the
	// latest; first approval we see is the latest. "Latest wins" is
	// decided by which of {latestRejection, latestApproval} is most
	// recent by CreatedAt.
	var latestRej, latestApp *ReviewComment
	for i := range comments {
		c := &comments[i]
		switch c.Type {
		case "review_rejection":
			if latestRej == nil || c.CreatedAt.After(latestRej.CreatedAt) {
				latestRej = c
			}
		case "review_approval":
			if latestApp == nil || c.CreatedAt.After(latestApp.CreatedAt) {
				latestApp = c
			}
		}
	}
	var st TaskReviewStatus
	if latestRej != nil {
		st.LatestRejectionAt = latestRej.CreatedAt
		st.LatestRejectionReason = latestRej.Body
		st.LatestRejectionBy = latestRej.Author
	}
	if latestApp != nil {
		st.LatestApprovalAt = latestApp.CreatedAt
		st.LatestApprovalBy = latestApp.Author
	}
	switch {
	case latestRej == nil && latestApp == nil:
		// Never reviewed.
	case latestRej != nil && latestApp == nil:
		st.NeedsRework = true
	case latestApp != nil && latestRej == nil:
		st.HasApproval = true
	default:
		// Both exist — most recent wins.
		if latestRej.CreatedAt.After(latestApp.CreatedAt) {
			st.NeedsRework = true
		} else {
			st.HasApproval = true
		}
	}
	return st, nil
}
