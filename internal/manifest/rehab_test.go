package manifest

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// openRehabTestStore opens a manifest.Store on a temp DB. The manifest
// store is self-contained — the rehab handler is injected by tests so
// they can assert the handler ran + with what args, without pulling in
// the task package.
func openRehabTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "r.db") + "?_journal_mode=WAL&_busy_timeout=5000"
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

// recordingHandler captures every invocation so tests can verify call
// count + target manifest IDs. Thread-safe isn't required — the
// RemoveDep path is synchronous.
type recordingHandler struct {
	calls []string
}

func (r *recordingHandler) fn(_ context.Context, manifestID string) {
	r.calls = append(r.calls, manifestID)
}

// TestRemoveDep_FiresRehabHandler_OnRealEdge — the bread-and-butter
// path. An edge exists; operator removes it; the handler fires with
// the source manifest id. node.go wires this to the flip-to-pending
// rehab.
func TestRemoveDep_FiresRehabHandler_OnRealEdge(t *testing.T) {
	s := openRehabTestStore(t)
	ctx := context.Background()

	rec := &recordingHandler{}
	s.SetDepRemovedHandler(rec.fn)

	a, _ := s.Create("A", "", "", "open", "t", "node", "", "", nil, nil)
	b, _ := s.Create("B", "", "", "open", "t", "node", "", "", nil, nil)
	if err := s.AddDep(ctx, a.ID, b.ID, "tester"); err != nil {
		t.Fatalf("AddDep: %v", err)
	}

	if err := s.RemoveDep(ctx, a.ID, b.ID); err != nil {
		t.Fatalf("RemoveDep: %v", err)
	}
	if len(rec.calls) != 1 || rec.calls[0] != a.ID {
		t.Fatalf("handler calls = %v, want [%s]", rec.calls, a.ID)
	}
}

// TestRemoveDep_FiresRehabHandler_EvenOnNoOpDelete — RemoveDep is
// documented as idempotent. The handler fires even when the edge
// didn't exist, because "after this call, the edge is gone and any
// rehab that should have happened has happened" is the contract. A
// no-op delete followed by a no-op rehab is cheap and keeps callers
// from having to distinguish.
func TestRemoveDep_FiresRehabHandler_EvenOnNoOpDelete(t *testing.T) {
	s := openRehabTestStore(t)
	ctx := context.Background()

	rec := &recordingHandler{}
	s.SetDepRemovedHandler(rec.fn)

	a, _ := s.Create("A", "", "", "open", "t", "node", "", "", nil, nil)
	b, _ := s.Create("B", "", "", "open", "t", "node", "", "", nil, nil)
	// No AddDep first — the edge never existed.

	if err := s.RemoveDep(ctx, a.ID, b.ID); err != nil {
		t.Fatalf("RemoveDep: %v", err)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("handler calls = %d, want 1 (handler fires on no-op too)", len(rec.calls))
	}
}

// TestRemoveDep_NoHandler_StillWorks — nil handler means the manifest
// store is running standalone (tests, migrations). RemoveDep must
// still behave correctly. Wired-in callers are the exception, not the
// default.
func TestRemoveDep_NoHandler_StillWorks(t *testing.T) {
	s := openRehabTestStore(t)
	ctx := context.Background()
	// Explicitly leave SetDepRemovedHandler unset.

	a, _ := s.Create("A", "", "", "open", "t", "node", "", "", nil, nil)
	b, _ := s.Create("B", "", "", "open", "t", "node", "", "", nil, nil)
	if err := s.AddDep(ctx, a.ID, b.ID, "tester"); err != nil {
		t.Fatal(err)
	}

	if err := s.RemoveDep(ctx, a.ID, b.ID); err != nil {
		t.Fatalf("RemoveDep without handler: %v", err)
	}
	deps, err := s.ListDeps(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 0 {
		t.Errorf("expected edge gone, ListDeps returned %d rows", len(deps))
	}
}

// TestUpdate_DoesNotFireRehabHandler — RemoveDep is the sole trigger
// for the rehab handler. A manifest status update (even a close)
// routes through the terminal-transition handler, not this one.
// Keeping the paths distinct prevents accidental rehab firing when
// operator behavior doesn't match the semantic.
func TestUpdate_DoesNotFireRehabHandler(t *testing.T) {
	s := openRehabTestStore(t)
	ctx := context.Background()

	rec := &recordingHandler{}
	s.SetDepRemovedHandler(rec.fn)

	a, _ := s.Create("A", "", "", "open", "t", "node", "", "", nil, nil)

	// Close the manifest — different path, different handler.
	if err := s.Update(a.ID, a.Title, "", "", "closed", "", "", nil, nil); err != nil {
		t.Fatalf("Update to closed: %v", err)
	}
	// Also try a draft → open transition (non-terminal).
	if err := s.Update(a.ID, a.Title, "", "", "open", "", "", nil, nil); err != nil {
		t.Fatalf("Update back to open: %v", err)
	}

	if len(rec.calls) != 0 {
		t.Fatalf("rehab handler fired on Update path: %v; should only fire on RemoveDep", rec.calls)
	}
	_ = ctx // silence unused — kept for symmetry with other tests
}
