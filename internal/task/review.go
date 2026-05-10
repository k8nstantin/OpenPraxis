package task

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
)

var (
	ErrTaskNotCompleted   = errors.New("task is not in completed state")
	ErrEmptyReviewReason  = errors.New("review reason must not be empty")
	ErrReviewNotAvailable = errors.New("review API not wired")
)

// ReviewComment is a single review record (rejection or approval).
type ReviewComment struct {
	Type      string
	Author    string
	Body      string
	CreatedAt time.Time
}

// ReviewWriter persists review comments.
type ReviewWriter interface {
	AddReviewComment(ctx context.Context, taskID, author, ct, body string) error
}

// ReviewReader reads review comments for a task.
type ReviewReader interface {
	ListReviewCommentsForTask(ctx context.Context, taskID string, limit int) ([]ReviewComment, error)
}

// ReviewStatus summarises the current review state of a task.
type ReviewStatus struct {
	NeedsRework           bool
	HasApproval           bool
	LatestRejectionReason string
}

// CommentsReviewBackend adapts *comments.Store to satisfy ReviewWriter and
// ReviewReader. Wire it via Store.SetReviewCommentsAPI in production.
type CommentsReviewBackend struct {
	store *comments.Store
}

// NewCommentsReviewBackend wraps a comments.Store for use as the review
// comment backend.
func NewCommentsReviewBackend(s *comments.Store) *CommentsReviewBackend {
	return &CommentsReviewBackend{store: s}
}

func (b *CommentsReviewBackend) AddReviewComment(ctx context.Context, taskID, author, ct, body string) error {
	_, err := b.store.Add(ctx, comments.TargetEntity, taskID, author, comments.NormalizeType(ct), body)
	return err
}

func (b *CommentsReviewBackend) ListReviewCommentsForTask(ctx context.Context, taskID string, limit int) ([]ReviewComment, error) {
	rows, err := b.store.List(ctx, comments.TargetEntity, taskID, limit, nil)
	if err != nil {
		return nil, err
	}
	out := make([]ReviewComment, 0, len(rows))
	for _, c := range rows {
		if c.Type != comments.TypeReviewRejection && c.Type != comments.TypeReviewApproval {
			continue
		}
		out = append(out, ReviewComment{
			Type:      string(c.Type),
			Author:    c.Author,
			Body:      c.Body,
			CreatedAt: c.CreatedAt,
		})
	}
	return out, nil
}

// SetReviewCommentsAPI wires the review comment backend onto the Store.
func (s *Store) SetReviewCommentsAPI(w ReviewWriter, r ReviewReader) {
	s.reviewWriter = w
	s.reviewReader = r
}

// RejectCompletedTask records a review_rejection comment. The tasks table
// has been retired; status transitions are skipped. Only the comment is
// written so the rejection audit trail is preserved.
func (s *Store) RejectCompletedTask(ctx context.Context, taskID, reason, reviewer string) error {
	if s.reviewWriter == nil {
		return ErrReviewNotAvailable
	}
	if strings.TrimSpace(reason) == "" {
		return ErrEmptyReviewReason
	}
	if err := s.reviewWriter.AddReviewComment(ctx, taskID, reviewer, "review_rejection", reason); err != nil {
		return fmt.Errorf("write rejection comment: %w", err)
	}
	return nil
}

// ApproveCompletedTask records a review_approval comment. Approval does not
// change task status. If no writer is wired the call succeeds silently.
func (s *Store) ApproveCompletedTask(ctx context.Context, taskID, reviewer string) error {
	if s.reviewWriter == nil {
		return nil
	}
	if err := s.reviewWriter.AddReviewComment(ctx, taskID, reviewer, "review_approval", "approved"); err != nil {
		return fmt.Errorf("write approval comment: %w", err)
	}
	return nil
}

// TaskReviewStatus returns the current review state of a task by inspecting
// its comment stream. Latest comment wins between rejection and approval.
// Returns the zero value when no reviewReader is wired.
func (s *Store) TaskReviewStatus(ctx context.Context, taskID string) (ReviewStatus, error) {
	if s.reviewReader == nil {
		return ReviewStatus{}, nil
	}
	rows, err := s.reviewReader.ListReviewCommentsForTask(ctx, taskID, 100)
	if err != nil {
		return ReviewStatus{}, err
	}

	var latest *ReviewComment
	for i := range rows {
		c := &rows[i]
		if latest == nil || c.CreatedAt.After(latest.CreatedAt) {
			latest = c
		}
	}
	if latest == nil {
		return ReviewStatus{}, nil
	}
	if latest.Type == "review_rejection" {
		return ReviewStatus{NeedsRework: true, LatestRejectionReason: latest.Body}, nil
	}
	return ReviewStatus{HasApproval: true}, nil
}
