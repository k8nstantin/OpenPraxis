package task

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeReviewBackend collects writes and exposes them for reads so one
// stub satisfies both ReviewWriter + ReviewReader without a real
// comments store.
type fakeReviewBackend struct {
	mu   sync.Mutex
	rows map[string][]ReviewComment // taskID → comments newest-last
	now  func() time.Time
}

func newFakeReviewBackend() *fakeReviewBackend {
	return &fakeReviewBackend{
		rows: map[string][]ReviewComment{},
		now:  func() time.Time { return time.Now().UTC() },
	}
}

func (f *fakeReviewBackend) AddReviewComment(_ context.Context, taskID, author, ct, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows[taskID] = append(f.rows[taskID], ReviewComment{
		Type:      ct,
		Author:    author,
		Body:      body,
		CreatedAt: f.now(),
	})
	return nil
}

func (f *fakeReviewBackend) ListReviewCommentsForTask(_ context.Context, taskID string, _ int) ([]ReviewComment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]ReviewComment, len(f.rows[taskID]))
	copy(out, f.rows[taskID])
	return out, nil
}

// seedCompletedTask forces a task into the completed state bypassing
// the state machine — test fixture only.
func seedCompletedTask(t *testing.T, s *Store, title string) *Task {
	t.Helper()
	tk, err := s.Create("", title, "", "once", "claude-code", "node", "t", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := s.db.Exec(`UPDATE tasks SET status = 'completed' WHERE id = ?`, tk.ID); err != nil {
		t.Fatalf("force completed: %v", err)
	}
	tk.Status = "completed"
	return tk
}

// TestRejectCompletedTask_FlipsToScheduledAndRecordsComment — the
// headline behavior. A completed task is rejected; status goes back
// to scheduled, block_reason stays empty (the task is armed, not
// blocked), and a review_rejection comment is appended with the
// reason + reviewer.
func TestRejectCompletedTask_FlipsToScheduledAndRecordsComment(t *testing.T) {
	s := openRepoTestStore(t)
	be := newFakeReviewBackend()
	s.SetReviewCommentsAPI(be, be)

	tk := seedCompletedTask(t, s, "done-work")

	if err := s.RejectCompletedTask(context.Background(), tk.ID, "tests are flaky", "cal"); err != nil {
		t.Fatalf("RejectCompletedTask: %v", err)
	}

	if got := readStatus(t, s, tk.ID); got != "scheduled" {
		t.Errorf("status = %q, want scheduled", got)
	}
	if br := readBlockReason(t, s, tk.ID); br != "" {
		t.Errorf("block_reason = %q, want empty (task is armed)", br)
	}

	rows, err := be.ListReviewCommentsForTask(context.Background(), tk.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rejection comments = %d, want 1", len(rows))
	}
	if rows[0].Type != "review_rejection" {
		t.Errorf("comment type = %q, want review_rejection", rows[0].Type)
	}
	if rows[0].Body != "tests are flaky" {
		t.Errorf("comment body = %q, want the reason verbatim", rows[0].Body)
	}
	if rows[0].Author != "cal" {
		t.Errorf("comment author = %q, want 'cal'", rows[0].Author)
	}
}

// TestRejectCompletedTask_RejectsNonCompletedWithSentinel — only
// status=completed can be rejected. Running / pending / failed all
// return ErrTaskNotCompleted with a message naming the current
// status.
func TestRejectCompletedTask_RejectsNonCompletedWithSentinel(t *testing.T) {
	s := openRepoTestStore(t)
	s.SetReviewCommentsAPI(newFakeReviewBackend(), nil)

	for _, bad := range []string{"pending", "scheduled", "running", "failed", "cancelled"} {
		tk, _ := s.Create("", bad+"-task", "", "once", "claude-code", "node", "t", "")
		if _, err := s.db.Exec(`UPDATE tasks SET status = ? WHERE id = ?`, bad, tk.ID); err != nil {
			t.Fatalf("force %s: %v", bad, err)
		}
		err := s.RejectCompletedTask(context.Background(), tk.ID, "nope", "cal")
		if !errors.Is(err, ErrTaskNotCompleted) {
			t.Errorf("reject(%s) err = %v, want ErrTaskNotCompleted", bad, err)
		}
	}
}

// TestRejectCompletedTask_EmptyReasonRejected — a rejection without a
// reason is useless for the next-pass agent. Sentinel error so
// handlers can render a clear HTTP 400.
func TestRejectCompletedTask_EmptyReasonRejected(t *testing.T) {
	s := openRepoTestStore(t)
	s.SetReviewCommentsAPI(newFakeReviewBackend(), nil)
	tk := seedCompletedTask(t, s, "done")

	err := s.RejectCompletedTask(context.Background(), tk.ID, "", "cal")
	if !errors.Is(err, ErrEmptyReviewReason) {
		t.Fatalf("err = %v, want ErrEmptyReviewReason", err)
	}
	// Task must be unchanged.
	if got := readStatus(t, s, tk.ID); got != "completed" {
		t.Errorf("status = %q, want completed (unchanged on rejection of empty reason)", got)
	}
}

// TestApproveCompletedTask_AppendsCommentNoStatusChange — approval
// is a signal, not a transition. The comment is written; the task
// stays completed.
func TestApproveCompletedTask_AppendsCommentNoStatusChange(t *testing.T) {
	s := openRepoTestStore(t)
	be := newFakeReviewBackend()
	s.SetReviewCommentsAPI(be, be)

	tk := seedCompletedTask(t, s, "done")
	if err := s.ApproveCompletedTask(context.Background(), tk.ID, "cal"); err != nil {
		t.Fatalf("ApproveCompletedTask: %v", err)
	}
	if got := readStatus(t, s, tk.ID); got != "completed" {
		t.Errorf("status = %q, want completed (unchanged)", got)
	}
	rows, _ := be.ListReviewCommentsForTask(context.Background(), tk.ID, 10)
	if len(rows) != 1 || rows[0].Type != "review_approval" {
		t.Fatalf("approval rows = %+v, want 1 approval comment", rows)
	}
}

// TestTaskReviewStatus_LatestWinsBetweenRejectionAndApproval — the
// order of review operations matters. If the operator rejected, then
// approved, HasApproval=true. If approved, then later re-rejected,
// NeedsRework=true. Timestamp comparison on the comment stream is the
// tiebreaker.
func TestTaskReviewStatus_LatestWinsBetweenRejectionAndApproval(t *testing.T) {
	s := openRepoTestStore(t)
	be := newFakeReviewBackend()
	s.SetReviewCommentsAPI(be, be)

	// Drive time manually so created_at ordering is deterministic.
	clock := time.Date(2026, 4, 19, 10, 0, 0, 0, time.UTC)
	be.now = func() time.Time { clock = clock.Add(time.Minute); return clock }

	tk := seedCompletedTask(t, s, "done")

	// reject → approve → reject. Final state should be NeedsRework.
	if err := s.RejectCompletedTask(context.Background(), tk.ID, "r1", "cal"); err != nil {
		t.Fatal(err)
	}
	// Reset status to completed so approve is legal (Reject flipped
	// it; tests at this layer don't care about a real run between).
	if _, err := s.db.Exec(`UPDATE tasks SET status = 'completed' WHERE id = ?`, tk.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.ApproveCompletedTask(context.Background(), tk.ID, "cal"); err != nil {
		t.Fatal(err)
	}
	if err := s.RejectCompletedTask(context.Background(), tk.ID, "r2", "cal"); err != nil {
		t.Fatal(err)
	}

	st, err := s.TaskReviewStatus(context.Background(), tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !st.NeedsRework {
		t.Errorf("NeedsRework = false, want true (latest comment is the second rejection)")
	}
	if st.HasApproval {
		t.Errorf("HasApproval = true, want false (approval is older than latest rejection)")
	}
	if !strings.Contains(st.LatestRejectionReason, "r2") {
		t.Errorf("LatestRejectionReason = %q, want to name the second rejection", st.LatestRejectionReason)
	}
}

// TestTaskReviewStatus_NilReaderReturnsEmpty — scheduler / basic
// list paths that don't care about review state shouldn't pay a
// penalty or see errors just because the review API isn't wired.
func TestTaskReviewStatus_NilReaderReturnsEmpty(t *testing.T) {
	s := openRepoTestStore(t)
	// No SetReviewCommentsAPI call.
	tk := seedCompletedTask(t, s, "done")

	st, err := s.TaskReviewStatus(context.Background(), tk.ID)
	if err != nil {
		t.Fatalf("TaskReviewStatus: %v", err)
	}
	if st.NeedsRework || st.HasApproval {
		t.Errorf("expected empty status with nil reader, got %+v", st)
	}
}

// TestRejectCompletedTask_RequiresReviewWriter — the writer half of
// the API must be wired before a rejection can persist. Nil writer
// returns ErrReviewNotAvailable, task is unchanged.
func TestRejectCompletedTask_RequiresReviewWriter(t *testing.T) {
	s := openRepoTestStore(t)
	// No writer wired.
	tk := seedCompletedTask(t, s, "done")

	err := s.RejectCompletedTask(context.Background(), tk.ID, "reason", "cal")
	if !errors.Is(err, ErrReviewNotAvailable) {
		t.Fatalf("err = %v, want ErrReviewNotAvailable", err)
	}
	if got := readStatus(t, s, tk.ID); got != "completed" {
		t.Errorf("status = %q, want completed (unchanged)", got)
	}
}
