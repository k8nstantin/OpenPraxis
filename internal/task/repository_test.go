package task

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func openRepoTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "repo.db") + "?_journal_mode=WAL&_busy_timeout=5000"
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	s, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s
}

// status reads tasks.status directly — scanTask doesn't expose it via Get
// for this use case we just want the raw column.
func readStatus(t *testing.T, s *Store, id string) string {
	t.Helper()
	var st string
	if err := s.db.QueryRow(`SELECT status FROM tasks WHERE id = ?`, id).Scan(&st); err != nil {
		t.Fatalf("read status for %s: %v", id, err)
	}
	return st
}

// TestActivateDependents_FlipsPendingToScheduled — reproduces the production
// bug. Create() sets status='pending' even when depends_on is non-empty, so
// before this fix ActivateDependents matched zero rows and every dependency
// chain had to be fired by hand. After the fix, a pending child whose
// depends_on matches the completed parent flips to 'scheduled'.
func TestActivateDependents_FlipsPendingToScheduled(t *testing.T) {
	s := openRepoTestStore(t)

	parent, err := s.Create("", "parent", "", "once", "claude-code", "node", "test", "")
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	child, err := s.Create("", "child", "", "once", "claude-code", "node", "test", parent.ID)
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	if got := readStatus(t, s, child.ID); got != "pending" {
		t.Fatalf("child initial status = %q, want pending (Create contract)", got)
	}

	n, err := s.ActivateDependents(parent.ID)
	if err != nil {
		t.Fatalf("ActivateDependents: %v", err)
	}
	if n != 1 {
		t.Fatalf("activated count = %d, want 1 (child should have been flipped)", n)
	}
	if got := readStatus(t, s, child.ID); got != "scheduled" {
		t.Fatalf("child status after activate = %q, want scheduled", got)
	}
}

// TestActivateDependents_StillHandlesWaiting — we loosened the status
// predicate, we did not replace it. Any path that parks a task at 'waiting'
// (reserved for future use; not written by Create today but referenced in
// the UI sort order) must continue to be picked up.
func TestActivateDependents_StillHandlesWaiting(t *testing.T) {
	s := openRepoTestStore(t)

	parent, _ := s.Create("", "p", "", "once", "claude-code", "node", "t", "")
	child, _ := s.Create("", "c", "", "once", "claude-code", "node", "t", parent.ID)
	if _, err := s.db.Exec(`UPDATE tasks SET status = 'waiting' WHERE id = ?`, child.ID); err != nil {
		t.Fatalf("force waiting: %v", err)
	}

	n, err := s.ActivateDependents(parent.ID)
	if err != nil {
		t.Fatalf("ActivateDependents: %v", err)
	}
	if n != 1 {
		t.Fatalf("activated count = %d, want 1 for waiting child", n)
	}
	if got := readStatus(t, s, child.ID); got != "scheduled" {
		t.Fatalf("status = %q, want scheduled", got)
	}
}

// TestActivateDependents_LeavesUnrelatedTasksAlone — the depends_on filter
// must still scope the update. Tasks in 'pending' that do NOT depend on the
// completed parent are untouched (otherwise we'd auto-fire every pending
// task in the DB every time anything completed).
func TestActivateDependents_LeavesUnrelatedTasksAlone(t *testing.T) {
	s := openRepoTestStore(t)

	parent, _ := s.Create("", "p", "", "once", "claude-code", "node", "t", "")
	child, _ := s.Create("", "c", "", "once", "claude-code", "node", "t", parent.ID)
	// Unrelated: no depends_on at all, just sitting pending.
	loose, _ := s.Create("", "loose", "", "once", "claude-code", "node", "t", "")
	// Unrelated: depends on someone else.
	otherParent, _ := s.Create("", "op", "", "once", "claude-code", "node", "t", "")
	cousin, _ := s.Create("", "cousin", "", "once", "claude-code", "node", "t", otherParent.ID)

	n, err := s.ActivateDependents(parent.ID)
	if err != nil {
		t.Fatalf("ActivateDependents: %v", err)
	}
	if n != 1 {
		t.Fatalf("activated count = %d, want 1 (only the direct child)", n)
	}
	if got := readStatus(t, s, child.ID); got != "scheduled" {
		t.Fatalf("child status = %q, want scheduled", got)
	}
	if got := readStatus(t, s, loose.ID); got != "pending" {
		t.Fatalf("loose pending sibling was modified: status = %q, want pending", got)
	}
	if got := readStatus(t, s, cousin.ID); got != "pending" {
		t.Fatalf("cousin depending on another parent was modified: status = %q, want pending", got)
	}
}

// TestActivateDependents_DoesNotTouchTerminalStates — running/completed/
// failed/cancelled children must NOT be rescheduled. The status predicate
// explicitly excludes them.
func TestActivateDependents_DoesNotTouchTerminalStates(t *testing.T) {
	s := openRepoTestStore(t)
	parent, _ := s.Create("", "p", "", "once", "claude-code", "node", "t", "")

	cases := []string{"running", "completed", "failed", "cancelled", "scheduled"}
	ids := map[string]string{}
	for _, status := range cases {
		child, _ := s.Create("", "c-"+status, "", "once", "claude-code", "node", "t", parent.ID)
		if _, err := s.db.Exec(`UPDATE tasks SET status = ? WHERE id = ?`, status, child.ID); err != nil {
			t.Fatalf("force %s: %v", status, err)
		}
		ids[status] = child.ID
	}

	n, err := s.ActivateDependents(parent.ID)
	if err != nil {
		t.Fatalf("ActivateDependents: %v", err)
	}
	if n != 0 {
		t.Fatalf("activated count = %d, want 0 (no pending/waiting children)", n)
	}
	for _, status := range cases {
		if got := readStatus(t, s, ids[status]); got != status {
			t.Fatalf("child in %q was mutated to %q", status, got)
		}
	}
}
