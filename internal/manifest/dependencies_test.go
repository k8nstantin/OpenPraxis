package manifest

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func openDepsTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "m.db") + "?_journal_mode=WAL&_busy_timeout=5000"
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

// mustCreate is a terse helper: create a manifest with reasonable defaults
// and return its id. Tests that care about dep edges rarely care about
// manifest fields beyond id + status, so we skip the rest.
func mustCreate(t *testing.T, s *Store, title, status string) string {
	t.Helper()
	m, err := s.Create(title, "", "", status, "test", "node", "", "", nil, nil)
	if err != nil {
		t.Fatalf("Create(%q): %v", title, err)
	}
	return m.ID
}

// TestTerminalStatusClassification — IsTerminalStatus is used by
// IsSatisfied's predicate; if the set changes silently, dep activation
// semantics drift. Lock the classification down.
func TestTerminalStatusClassification(t *testing.T) {
	cases := map[string]bool{
		"draft":   false,
		"open":    false,
		"closed":  true,
		"archive": true,
		"":        false,
		"unknown": false,
	}
	for status, want := range cases {
		if got := IsTerminalStatus(status); got != want {
			t.Errorf("IsTerminalStatus(%q) = %v, want %v", status, got, want)
		}
	}
}

// TestAddDep_BasicEdgeAndIdempotency — the simple happy path plus a
// repeat add. The second add must be a no-op, not an error; operators
// (and MCP callers that retry on transient failures) rely on
// idempotency.
func TestAddDep_BasicEdgeAndIdempotency(t *testing.T) {
	s := openDepsTestStore(t)
	ctx := context.Background()

	a := mustCreate(t, s, "A", "open")
	b := mustCreate(t, s, "B", "open")

	if err := s.AddDep(ctx, a, b, "tester"); err != nil {
		t.Fatalf("first AddDep: %v", err)
	}
	if err := s.AddDep(ctx, a, b, "tester"); err != nil {
		t.Fatalf("second AddDep (idempotency): %v", err)
	}

	deps, err := s.ListDeps(ctx, a)
	if err != nil {
		t.Fatalf("ListDeps: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("ListDeps returned %d rows, want 1 (idempotent add)", len(deps))
	}
	if deps[0].ID != b {
		t.Errorf("deps[0].ID = %q, want %q", deps[0].ID, b)
	}
}

// TestAddDep_SelfLoopRejected — the trivial mistake must be refused with
// ErrSelfLoop, not bubbled as a raw sqlite CHECK-constraint message.
func TestAddDep_SelfLoopRejected(t *testing.T) {
	s := openDepsTestStore(t)
	ctx := context.Background()
	a := mustCreate(t, s, "A", "open")

	err := s.AddDep(ctx, a, a, "tester")
	if !errors.Is(err, ErrSelfLoop) {
		t.Fatalf("AddDep(self-loop) err = %v, want ErrSelfLoop", err)
	}
}

// TestAddDep_CycleDetection_Direct — A→B exists; adding B→A closes a
// length-2 cycle and must be rejected with ErrCycle.
func TestAddDep_CycleDetection_Direct(t *testing.T) {
	s := openDepsTestStore(t)
	ctx := context.Background()

	a := mustCreate(t, s, "A", "open")
	b := mustCreate(t, s, "B", "open")

	if err := s.AddDep(ctx, a, b, "tester"); err != nil {
		t.Fatalf("A→B: %v", err)
	}
	err := s.AddDep(ctx, b, a, "tester")
	if !errors.Is(err, ErrCycle) {
		t.Fatalf("B→A err = %v, want ErrCycle (direct cycle)", err)
	}
	if !strings.Contains(err.Error(), b) || !strings.Contains(err.Error(), a) {
		t.Errorf("cycle error doesn't name the edge: %v", err)
	}
}

// TestAddDep_CycleDetection_Transitive — A→B, B→C. Adding C→A would
// close a length-3 cycle. DFS must catch this, not just direct edges.
func TestAddDep_CycleDetection_Transitive(t *testing.T) {
	s := openDepsTestStore(t)
	ctx := context.Background()

	a := mustCreate(t, s, "A", "open")
	b := mustCreate(t, s, "B", "open")
	c := mustCreate(t, s, "C", "open")

	if err := s.AddDep(ctx, a, b, "t"); err != nil {
		t.Fatalf("A→B: %v", err)
	}
	if err := s.AddDep(ctx, b, c, "t"); err != nil {
		t.Fatalf("B→C: %v", err)
	}

	err := s.AddDep(ctx, c, a, "t")
	if !errors.Is(err, ErrCycle) {
		t.Fatalf("C→A err = %v, want ErrCycle (transitive)", err)
	}
}

// TestAddDep_NonCyclingParallel — diamond-shaped deps are fine. A→B,
// A→C, B→D, C→D — no cycle, all four edges must insert. This is the
// pattern where a manifest waits on two parallel workstreams.
func TestAddDep_NonCyclingParallel(t *testing.T) {
	s := openDepsTestStore(t)
	ctx := context.Background()

	a := mustCreate(t, s, "A", "open")
	b := mustCreate(t, s, "B", "open")
	c := mustCreate(t, s, "C", "open")
	d := mustCreate(t, s, "D", "open")

	for _, pair := range [][2]string{{a, b}, {a, c}, {b, d}, {c, d}} {
		if err := s.AddDep(ctx, pair[0], pair[1], "t"); err != nil {
			t.Fatalf("%s→%s: %v", pair[0], pair[1], err)
		}
	}

	deps, err := s.ListDeps(ctx, a)
	if err != nil {
		t.Fatalf("ListDeps: %v", err)
	}
	if len(deps) != 2 {
		t.Errorf("A deps = %d, want 2 (B and C)", len(deps))
	}
}

// TestRemoveDep_IdempotentOnMissingEdge — removing an edge that doesn't
// exist must not error. MCP/HTTP callers retry freely; the API contract
// is "after this call, the edge is gone."
func TestRemoveDep_IdempotentOnMissingEdge(t *testing.T) {
	s := openDepsTestStore(t)
	ctx := context.Background()
	a := mustCreate(t, s, "A", "open")
	b := mustCreate(t, s, "B", "open")

	if err := s.RemoveDep(ctx, a, b); err != nil {
		t.Fatalf("RemoveDep on missing edge: %v", err)
	}
}

// TestListDependents_InEdges — ListDependents returns in-edges, i.e.
// manifests that depend on *this* one. Used by the activation walker on
// manifest close.
func TestListDependents_InEdges(t *testing.T) {
	s := openDepsTestStore(t)
	ctx := context.Background()

	target := mustCreate(t, s, "target", "open")
	a := mustCreate(t, s, "A", "open")
	b := mustCreate(t, s, "B", "open")
	c := mustCreate(t, s, "C", "open") // does NOT depend on target

	if err := s.AddDep(ctx, a, target, "t"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddDep(ctx, b, target, "t"); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListDependents(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("dependents = %d, want 2 (A, B) — saw %+v", len(got), got)
	}
	gotIDs := map[string]bool{got[0].ID: true, got[1].ID: true}
	if !gotIDs[a] || !gotIDs[b] {
		t.Errorf("missing expected dependents A or B; got ids: %v", gotIDs)
	}
	if gotIDs[c] {
		t.Errorf("ListDependents returned C, which doesn't depend on target")
	}
}

// TestIsSatisfied_AllTerminalOrEmpty — a manifest with no deps is
// trivially satisfied. A manifest whose every dep is closed/archive is
// satisfied. Any non-terminal dep makes it unsatisfied and the
// unsatisfied list names the blocker.
func TestIsSatisfied_AllTerminalOrEmpty(t *testing.T) {
	s := openDepsTestStore(t)
	ctx := context.Background()

	m := mustCreate(t, s, "M", "open")

	// No deps — satisfied vacuously.
	ok, blockers, err := s.IsSatisfied(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || len(blockers) != 0 {
		t.Fatalf("no-deps IsSatisfied = (%v, %v), want (true, nil)", ok, blockers)
	}

	closedDep := mustCreate(t, s, "closed-dep", "closed")
	archiveDep := mustCreate(t, s, "archive-dep", "archive")
	openDep := mustCreate(t, s, "open-dep", "open")

	for _, d := range []string{closedDep, archiveDep} {
		if err := s.AddDep(ctx, m, d, "t"); err != nil {
			t.Fatal(err)
		}
	}

	// Two deps, both terminal.
	ok, blockers, err = s.IsSatisfied(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || len(blockers) != 0 {
		t.Fatalf("all-terminal IsSatisfied = (%v, %v), want (true, nil)", ok, blockers)
	}

	// Add a non-terminal dep — must flip to unsatisfied with the open one named.
	if err := s.AddDep(ctx, m, openDep, "t"); err != nil {
		t.Fatal(err)
	}
	ok, blockers, err = s.IsSatisfied(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatalf("IsSatisfied = true, want false (open dep blocking)")
	}
	if len(blockers) != 1 || blockers[0] != openDep {
		t.Fatalf("blockers = %v, want [%s]", blockers, openDep)
	}
}

// TestIsSatisfied_IgnoresDeletedDeps — a dep manifest that's been
// soft-deleted (deleted_at != '') must not count as a blocker. Deleted
// manifests are invisible to the rest of the system; their edges should
// be invisible too.
func TestIsSatisfied_IgnoresDeletedDeps(t *testing.T) {
	s := openDepsTestStore(t)
	ctx := context.Background()

	m := mustCreate(t, s, "M", "open")
	dep := mustCreate(t, s, "dep", "open")

	if err := s.AddDep(ctx, m, dep, "t"); err != nil {
		t.Fatal(err)
	}

	// Soft-delete the dep.
	if _, err := s.db.Exec(`UPDATE manifests SET deleted_at = '2026-01-01T00:00:00Z' WHERE id = ?`, dep); err != nil {
		t.Fatal(err)
	}

	ok, blockers, err := s.IsSatisfied(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || len(blockers) != 0 {
		t.Fatalf("IsSatisfied with deleted dep = (%v, %v), want (true, nil)", ok, blockers)
	}
}

// TestSyncLegacyDependsOn_DualWrite — writing to the join table keeps
// the legacy comma-separated column in sync, so readers that still query
// `manifests.depends_on` see the same edges. After the follow-up PR that
// drops that column, this test becomes stale and can be removed.
func TestSyncLegacyDependsOn_DualWrite(t *testing.T) {
	s := openDepsTestStore(t)
	ctx := context.Background()

	a := mustCreate(t, s, "A", "open")
	b := mustCreate(t, s, "B", "open")
	c := mustCreate(t, s, "C", "open")

	if err := s.AddDep(ctx, a, b, "t"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddDep(ctx, a, c, "t"); err != nil {
		t.Fatal(err)
	}

	var legacy string
	if err := s.db.QueryRow(`SELECT depends_on FROM manifests WHERE id = ?`, a).Scan(&legacy); err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(legacy, ",")
	if len(parts) != 2 {
		t.Fatalf("legacy depends_on = %q, want 2 comma-separated ids", legacy)
	}

	// Remove one — legacy column must reflect the removal.
	if err := s.RemoveDep(ctx, a, b); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT depends_on FROM manifests WHERE id = ?`, a).Scan(&legacy); err != nil {
		t.Fatal(err)
	}
	if legacy != c {
		t.Fatalf("legacy after remove = %q, want %q", legacy, c)
	}
}

// TestBackfillLegacyDependsOn_Idempotent — pre-existing legacy column
// values must migrate into the join table on store init, and re-running
// the backfill must be a no-op. New installs hit this path the first
// time the store opens a DB with legacy rows already in it.
func TestBackfillLegacyDependsOn_Idempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.db") + "?_journal_mode=WAL&_busy_timeout=5000"
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}

	a := mustCreate(t, s, "A", "open")
	b := mustCreate(t, s, "B", "open")
	c := mustCreate(t, s, "C", "open")

	// Simulate a legacy row: write comma-separated depends_on directly,
	// bypassing AddDep, and wipe the join table so only the legacy
	// column carries the edge.
	if _, err := s.db.Exec(`UPDATE manifests SET depends_on = ? WHERE id = ?`, b+","+c, a); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM manifest_dependencies WHERE manifest_id = ?`, a); err != nil {
		t.Fatal(err)
	}

	// First backfill: two rows inserted.
	n, err := s.BackfillLegacyDependsOn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("first backfill inserted %d, want 2", n)
	}

	// Second run: no-op.
	n, err = s.BackfillLegacyDependsOn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("second backfill inserted %d, want 0 (idempotent)", n)
	}

	deps, err := s.ListDeps(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 2 {
		t.Fatalf("deps after backfill = %d, want 2", len(deps))
	}
}
