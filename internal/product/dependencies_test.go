package product

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
	path := filepath.Join(t.TempDir(), "p.db") + "?_journal_mode=WAL&_busy_timeout=5000"
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

func mustCreate(t *testing.T, s *Store, title, status string) string {
	t.Helper()
	p, err := s.Create(title, "", status, "node", nil)
	if err != nil {
		t.Fatalf("Create(%q): %v", title, err)
	}
	return p.ID
}

// TestTerminalStatusClassification — locks the set so future drift
// between product + manifest terminal classification is caught.
func TestTerminalStatusClassification(t *testing.T) {
	cases := map[string]bool{
		"draft": false, "open": false,
		"closed": true, "archive": true,
		"": false, "unknown": false,
	}
	for status, want := range cases {
		if got := IsTerminalStatus(status); got != want {
			t.Errorf("IsTerminalStatus(%q) = %v, want %v", status, got, want)
		}
	}
}

// TestAddDep_BasicEdgeAndIdempotency — happy path plus repeat add
// must be a no-op, not an error.
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
	if len(deps) != 1 || deps[0].ID != b {
		t.Fatalf("ListDeps = %+v, want single row for B", deps)
	}
}

// TestAddDep_SelfLoopRejected — must return ErrSelfLoop, not a raw
// sqlite CHECK constraint error.
func TestAddDep_SelfLoopRejected(t *testing.T) {
	s := openDepsTestStore(t)
	ctx := context.Background()
	a := mustCreate(t, s, "A", "open")
	err := s.AddDep(ctx, a, a, "tester")
	if !errors.Is(err, ErrSelfLoop) {
		t.Fatalf("err = %v, want ErrSelfLoop", err)
	}
}

// TestAddDep_DirectCycleRejected — A→B exists, B→A would close a
// length-2 cycle.
func TestAddDep_DirectCycleRejected(t *testing.T) {
	s := openDepsTestStore(t)
	ctx := context.Background()
	a := mustCreate(t, s, "A", "open")
	b := mustCreate(t, s, "B", "open")
	if err := s.AddDep(ctx, a, b, "t"); err != nil {
		t.Fatal(err)
	}
	err := s.AddDep(ctx, b, a, "t")
	if !errors.Is(err, ErrCycle) {
		t.Fatalf("err = %v, want ErrCycle", err)
	}
	if !strings.Contains(err.Error(), a) || !strings.Contains(err.Error(), b) {
		t.Errorf("cycle error doesn't name the pair: %v", err)
	}
}

// TestAddDep_TransitiveCycleRejected — A→B, B→C. Adding C→A closes
// a length-3 cycle; DFS must catch it.
func TestAddDep_TransitiveCycleRejected(t *testing.T) {
	s := openDepsTestStore(t)
	ctx := context.Background()
	a := mustCreate(t, s, "A", "open")
	b := mustCreate(t, s, "B", "open")
	c := mustCreate(t, s, "C", "open")
	if err := s.AddDep(ctx, a, b, "t"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddDep(ctx, b, c, "t"); err != nil {
		t.Fatal(err)
	}
	err := s.AddDep(ctx, c, a, "t")
	if !errors.Is(err, ErrCycle) {
		t.Fatalf("err = %v, want ErrCycle (transitive)", err)
	}
}

// TestAddDep_DeepNonCyclingChain — verifies the "many levels deep"
// guarantee: A depends on B depends on C depends on D depends on E
// with no cycles must all insert. Session directive requires deep
// dep graphs to be supported.
func TestAddDep_DeepNonCyclingChain(t *testing.T) {
	s := openDepsTestStore(t)
	ctx := context.Background()
	ids := make([]string, 8)
	for i := range ids {
		ids[i] = mustCreate(t, s, string(rune('A'+i)), "open")
	}
	// Linear chain: 0→1→2→...→7
	for i := 0; i < len(ids)-1; i++ {
		if err := s.AddDep(ctx, ids[i], ids[i+1], "t"); err != nil {
			t.Fatalf("chain edge %d→%d: %v", i, i+1, err)
		}
	}
	// Attempting to close the chain 7→0 must be refused.
	if err := s.AddDep(ctx, ids[7], ids[0], "t"); !errors.Is(err, ErrCycle) {
		t.Fatalf("7→0 close attempt err = %v, want ErrCycle", err)
	}
	// But adding a cross-link that doesn't cycle (say 0→3) is fine.
	if err := s.AddDep(ctx, ids[0], ids[3], "t"); err != nil {
		t.Fatalf("0→3 shortcut: %v", err)
	}
}

// TestAddDep_NonCyclingDiamond — parallel deps are fine: A→B, A→C,
// B→D, C→D. Canonical pattern for a product waiting on two parallel
// workstreams.
func TestAddDep_NonCyclingDiamond(t *testing.T) {
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
		t.Fatal(err)
	}
	if len(deps) != 2 {
		t.Errorf("A deps = %d, want 2", len(deps))
	}
}

// TestRemoveDep_IdempotentOnMissingEdge — "after this call, the
// edge is gone" is the contract. Removing a non-existent edge
// returns nil, not an error.
func TestRemoveDep_IdempotentOnMissingEdge(t *testing.T) {
	s := openDepsTestStore(t)
	ctx := context.Background()
	a := mustCreate(t, s, "A", "open")
	b := mustCreate(t, s, "B", "open")
	if err := s.RemoveDep(ctx, a, b); err != nil {
		t.Fatalf("RemoveDep on missing edge: %v", err)
	}
}

// TestListDependents_InEdges — in-edge listing is how the
// activation walker finds impacted products when a parent closes.
func TestListDependents_InEdges(t *testing.T) {
	s := openDepsTestStore(t)
	ctx := context.Background()
	target := mustCreate(t, s, "target", "open")
	a := mustCreate(t, s, "A", "open")
	b := mustCreate(t, s, "B", "open")
	_ = mustCreate(t, s, "C", "open") // does NOT depend on target

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
		t.Fatalf("dependents = %d, want 2", len(got))
	}
}

// TestIsSatisfied_AllTerminalOrEmpty — empty dep set is vacuously
// satisfied; all-terminal is satisfied; any non-terminal makes it
// unsatisfied and the list names the blocker.
func TestIsSatisfied_AllTerminalOrEmpty(t *testing.T) {
	s := openDepsTestStore(t)
	ctx := context.Background()
	m := mustCreate(t, s, "M", "open")

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
	ok, blockers, err = s.IsSatisfied(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || len(blockers) != 0 {
		t.Fatalf("all-terminal IsSatisfied = (%v, %v), want (true, nil)", ok, blockers)
	}

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

// TestIsSatisfied_IgnoresDeletedDeps — deleted products must not
// count as blockers. Deleted entities are invisible to the rest of
// the system; their edges should be invisible too.
func TestIsSatisfied_IgnoresDeletedDeps(t *testing.T) {
	s := openDepsTestStore(t)
	ctx := context.Background()
	m := mustCreate(t, s, "M", "open")
	dep := mustCreate(t, s, "dep", "open")
	if err := s.AddDep(ctx, m, dep, "t"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE products SET deleted_at='2026-01-01T00:00:00Z' WHERE id=?`, dep); err != nil {
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
