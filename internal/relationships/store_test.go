// Tests for the relationships store. Each test opens a fresh SQLite
// file in t.TempDir(), runs the migration (via New), then exercises one
// API surface. Pattern matches internal/manifest/dependencies_test.go +
// internal/task/repository_test.go for consistency.
//
// Coverage targets (manifest acceptance criteria):
//   - Migration runs idempotently
//   - Create on fresh edge writes one row with valid_to=''
//   - Create on existing edge closes prior + inserts new in one tx
//   - Validation errors (invalid kinds, self-loop) surface clean errors
//   - Remove closes current row; second call is a no-op
//   - Remove writes by/reason to the closing row's attribution columns
//   - ListOutgoing/ListIncoming return CURRENT edges only (valid_to='')
//   - History returns chronological order (oldest first)
//   - Walk respects edge_kind filter + max_depth clamp
//   - Walk dedupes via UNION (multi-path nodes appear once)
package relationships

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sort"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// openTestStore returns a fresh Store backed by a SQLite file in the
// test's TempDir. WAL + busy_timeout are set per visceral rule #10
// (consistent with how the other stores open their databases).
func openTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rel.db") + "?_journal_mode=WAL&_busy_timeout=5000"
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	s, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// mustCreate is a terse helper for tests that don't care about every
// Edge field — fills in sensible defaults and fails the test on error.
// Returns the Edge that was passed (with Metadata defaulted to "" if
// the caller left it blank).
func mustCreate(t *testing.T, s *Store, src, dst, kind string, opts ...func(*Edge)) Edge {
	t.Helper()
	e := Edge{
		SrcKind:   KindManifest,
		SrcID:     src,
		DstKind:   KindManifest,
		DstID:     dst,
		Kind:      kind,
		CreatedBy: "test",
		Reason:    "test fixture",
	}
	for _, opt := range opts {
		opt(&e)
	}
	if err := s.Create(context.Background(), e); err != nil {
		t.Fatalf("Create(%s→%s, %s): %v", src, dst, kind, err)
	}
	return e
}

// ─── Migration ──────────────────────────────────────────────────────

// TestNew_Idempotent — running the migration twice (or N times) must
// not error. CREATE TABLE IF NOT EXISTS + CREATE INDEX IF NOT EXISTS
// guarantee this; the test locks it in so a future change can't
// accidentally regress.
func TestNew_Idempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rel.db") + "?_journal_mode=WAL&_busy_timeout=5000"
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	for i := 0; i < 3; i++ {
		if _, err := New(db); err != nil {
			t.Fatalf("New iteration %d: %v", i, err)
		}
	}
}

// ─── Create ─────────────────────────────────────────────────────────

// TestCreate_Fresh — a single Create with no prior row produces exactly
// one row with valid_to='' (still current).
func TestCreate_Fresh(t *testing.T) {
	s := openTestStore(t)
	mustCreate(t, s, "M1", "M2", EdgeDependsOn)

	rows, err := s.ListOutgoing(context.Background(), "M1", EdgeDependsOn)
	if err != nil {
		t.Fatalf("ListOutgoing: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 outgoing row, got %d", len(rows))
	}
	if rows[0].DstID != "M2" || rows[0].ValidTo != "" {
		t.Fatalf("unexpected row: %+v", rows[0])
	}
}

// TestCreate_Replace — Create on an edge that already has a current row
// closes the prior row + inserts a new current row, atomically. After
// the second Create:
//   - History has 2 rows total
//   - Only ONE row has valid_to='' (the new one)
//   - The closed row's valid_to equals the new row's valid_from (no gap)
func TestCreate_Replace(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	mustCreate(t, s, "M1", "M2", EdgeDependsOn, func(e *Edge) {
		e.Reason = "first version"
	})
	// Sleep 1ms so the second Create's now() ≠ the first's. Otherwise
	// they'd share valid_from and the PK constraint would reject the
	// second insert. Real callers won't hit this because production
	// load doesn't insert at sub-millisecond precision; tests need a
	// nudge.
	time.Sleep(1 * time.Millisecond)
	mustCreate(t, s, "M1", "M2", EdgeDependsOn, func(e *Edge) {
		e.Reason = "second version"
	})

	hist, err := s.History(ctx, "M1", "M2", EdgeDependsOn)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("expected 2 history rows, got %d", len(hist))
	}
	if hist[0].ValidTo == "" {
		t.Fatalf("oldest row should be closed, got valid_to=''")
	}
	if hist[1].ValidTo != "" {
		t.Fatalf("newest row should be current, got valid_to=%q", hist[1].ValidTo)
	}
	if hist[0].ValidTo != hist[1].ValidFrom {
		t.Errorf("audit gap: closed row valid_to=%q != new valid_from=%q",
			hist[0].ValidTo, hist[1].ValidFrom)
	}
}

// TestCreate_ValidationErrors — invalid kinds + self-loops fail with
// typed errors before reaching the DB. Schema CHECK would catch them
// too, but the Go-side validation gives clearer attribution.
func TestCreate_ValidationErrors(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	cases := []struct {
		name    string
		edge    Edge
		wantErr error
	}{
		{
			name: "bad src_kind",
			edge: Edge{SrcKind: "garbage", SrcID: "A", DstKind: KindManifest, DstID: "B", Kind: EdgeDependsOn},
			wantErr: ErrInvalidKind,
		},
		{
			name: "bad dst_kind",
			edge: Edge{SrcKind: KindManifest, SrcID: "A", DstKind: "garbage", DstID: "B", Kind: EdgeDependsOn},
			wantErr: ErrInvalidKind,
		},
		{
			name: "bad edge kind",
			edge: Edge{SrcKind: KindManifest, SrcID: "A", DstKind: KindManifest, DstID: "B", Kind: "garbage"},
			wantErr: ErrInvalidKind,
		},
		{
			name: "self-loop",
			edge: Edge{SrcKind: KindManifest, SrcID: "A", DstKind: KindManifest, DstID: "A", Kind: EdgeDependsOn},
			wantErr: ErrSelfLoop,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := s.Create(ctx, tc.edge)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Create: want %v, got %v", tc.wantErr, err)
			}
		})
	}
}

// ─── Remove ─────────────────────────────────────────────────────────

// TestRemove_ClosesCurrent — Remove on an existing current edge sets
// valid_to + writes the close-time by + reason into the row's
// attribution columns. Subsequent ListOutgoing returns nothing (the
// edge no longer exists at the current state).
func TestRemove_ClosesCurrent(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	mustCreate(t, s, "M1", "M2", EdgeDependsOn)
	if err := s.Remove(ctx, "M1", "M2", EdgeDependsOn, "alice", "no longer needed"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	out, err := s.ListOutgoing(ctx, "M1", EdgeDependsOn)
	if err != nil {
		t.Fatalf("ListOutgoing: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected 0 current outgoing, got %d", len(out))
	}

	hist, err := s.History(ctx, "M1", "M2", EdgeDependsOn)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("expected 1 history row, got %d", len(hist))
	}
	if hist[0].ValidTo == "" {
		t.Fatalf("row should be closed, got valid_to=''")
	}
	// Close-time attribution overwrites the original create-time values:
	// the row's created_by + reason now describe the CLOSE event.
	if hist[0].CreatedBy != "alice" || hist[0].Reason != "no longer needed" {
		t.Errorf("close attribution lost: by=%q reason=%q",
			hist[0].CreatedBy, hist[0].Reason)
	}
}

// TestRemove_Idempotent — Remove on a non-existent edge is a no-op
// success. Caller doesn't have to check "does this edge exist before
// I try to remove it?" — we return nil regardless.
func TestRemove_Idempotent(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	// Edge never created.
	if err := s.Remove(ctx, "M1", "M2", EdgeDependsOn, "alice", "no-op"); err != nil {
		t.Fatalf("Remove on missing edge: %v", err)
	}
	hist, _ := s.History(ctx, "M1", "M2", EdgeDependsOn)
	if len(hist) != 0 {
		t.Fatalf("Remove on missing edge wrote a phantom row: %+v", hist)
	}
}

// ─── ListOutgoing / ListIncoming ────────────────────────────────────

// TestListOutgoing_FiltersByEdgeKind — populating multiple edge kinds
// from the same source and verifying the kind filter restricts results
// correctly. Empty kind returns all kinds.
func TestListOutgoing_FiltersByEdgeKind(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	mustCreate(t, s, "M1", "M2", EdgeDependsOn)
	mustCreate(t, s, "M1", "T1", EdgeLinksTo, func(e *Edge) {
		e.DstKind = KindTask
	})
	mustCreate(t, s, "M1", "M3", EdgeDependsOn)

	// Kind-specific filter
	deps, _ := s.ListOutgoing(ctx, "M1", EdgeDependsOn)
	if len(deps) != 2 {
		t.Errorf("expected 2 depends_on edges, got %d", len(deps))
	}

	links, _ := s.ListOutgoing(ctx, "M1", EdgeLinksTo)
	if len(links) != 1 {
		t.Errorf("expected 1 links_to edge, got %d", len(links))
	}

	// Empty kind = all
	all, _ := s.ListOutgoing(ctx, "M1", "")
	if len(all) != 3 {
		t.Errorf("expected 3 total edges, got %d", len(all))
	}
}

// TestListOutgoing_CurrentOnly — closed edges (valid_to != '') do NOT
// appear in ListOutgoing. The partial index on valid_to='' is exactly
// what makes this fast; the test locks in the correctness side.
func TestListOutgoing_CurrentOnly(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	mustCreate(t, s, "M1", "M2", EdgeDependsOn)
	if err := s.Remove(ctx, "M1", "M2", EdgeDependsOn, "alice", "removed"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	out, _ := s.ListOutgoing(ctx, "M1", EdgeDependsOn)
	if len(out) != 0 {
		t.Errorf("expected 0 current edges after Remove, got %d", len(out))
	}
}

// TestListIncoming_ReverseDirection — ListIncoming finds edges by
// dst_id rather than src_id. Useful for "who depends on this?" queries.
func TestListIncoming_ReverseDirection(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	// Three manifests all depend on M0
	mustCreate(t, s, "M1", "M0", EdgeDependsOn)
	mustCreate(t, s, "M2", "M0", EdgeDependsOn)
	mustCreate(t, s, "M3", "M0", EdgeDependsOn)

	in, err := s.ListIncoming(ctx, "M0", EdgeDependsOn)
	if err != nil {
		t.Fatalf("ListIncoming: %v", err)
	}
	if len(in) != 3 {
		t.Errorf("expected 3 incoming edges to M0, got %d", len(in))
	}
}

// ─── History ────────────────────────────────────────────────────────

// TestHistory_AscendingByValidFrom — multiple versions of one edge, in
// chronological order. The closed row(s) come first, the current row
// (if any) last.
func TestHistory_AscendingByValidFrom(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	mustCreate(t, s, "M1", "M2", EdgeDependsOn)
	time.Sleep(2 * time.Millisecond)
	mustCreate(t, s, "M1", "M2", EdgeDependsOn, func(e *Edge) { e.Reason = "v2" })
	time.Sleep(2 * time.Millisecond)
	mustCreate(t, s, "M1", "M2", EdgeDependsOn, func(e *Edge) { e.Reason = "v3" })

	hist, err := s.History(ctx, "M1", "M2", EdgeDependsOn)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(hist))
	}

	// Confirm ordering: each row's valid_from should be ≤ the next's.
	for i := 1; i < len(hist); i++ {
		if hist[i-1].ValidFrom > hist[i].ValidFrom {
			t.Errorf("history out of order at index %d: %s > %s",
				i, hist[i-1].ValidFrom, hist[i].ValidFrom)
		}
	}
	// Last row is current; first two are closed.
	if hist[2].ValidTo != "" {
		t.Errorf("expected last row current, got valid_to=%q", hist[2].ValidTo)
	}
	if hist[0].ValidTo == "" || hist[1].ValidTo == "" {
		t.Errorf("expected first two rows closed, got valid_to='' on one")
	}
}

// ─── Walk ───────────────────────────────────────────────────────────

// TestWalk_BasicChain — A → B → C linear chain. Walk from A should
// return all three nodes (A at depth 0, B at 1, C at 2).
func TestWalk_BasicChain(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	mustCreate(t, s, "A", "B", EdgeDependsOn)
	mustCreate(t, s, "B", "C", EdgeDependsOn)

	rows, err := s.Walk(ctx, "A", KindManifest, []string{EdgeDependsOn}, 10)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 walk rows (A,B,C), got %d: %+v", len(rows), rows)
	}

	// Build a id→depth map for clearer assertions than positional.
	depth := map[string]int{}
	for _, r := range rows {
		depth[r.ID] = r.Depth
	}
	if depth["A"] != 0 || depth["B"] != 1 || depth["C"] != 2 {
		t.Errorf("unexpected depths: %v", depth)
	}
}

// TestWalk_DepthClamp — maxDepth=1 should return root + direct
// children only, not grandchildren.
func TestWalk_DepthClamp(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	mustCreate(t, s, "A", "B", EdgeDependsOn)
	mustCreate(t, s, "B", "C", EdgeDependsOn)

	rows, err := s.Walk(ctx, "A", KindManifest, []string{EdgeDependsOn}, 1)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	sort.Strings(ids)

	want := []string{"A", "B"}
	if len(ids) != len(want) || ids[0] != want[0] || ids[1] != want[1] {
		t.Errorf("expected %v, got %v", want, ids)
	}
}

// TestWalk_EdgeKindFilter — when multiple edge kinds exist, only the
// requested kind is followed.
func TestWalk_EdgeKindFilter(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	mustCreate(t, s, "P", "M", EdgeOwns, func(e *Edge) {
		e.SrcKind = KindProduct
		e.DstKind = KindManifest
	})
	mustCreate(t, s, "M", "T", EdgeDependsOn, func(e *Edge) {
		e.SrcKind = KindManifest
		e.DstKind = KindTask
	})

	// Walk from P following ONLY 'owns' should reach M but not T
	// (the M→T edge is depends_on, filtered out).
	rows, err := s.Walk(ctx, "P", KindProduct, []string{EdgeOwns}, 10)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	for _, r := range rows {
		if r.ID == "T" {
			t.Errorf("walk leaked through depends_on edge: reached T despite filter")
		}
	}
	// Should have P and M.
	if len(rows) != 2 {
		t.Errorf("expected P + M only, got %d rows: %+v", len(rows), rows)
	}
}

// TestWalk_DedupesViaUnion — diamond shape: A→B→D and A→C→D. UNION (not
// UNION ALL) in the CTE means D appears exactly once in the walk
// despite being reachable via two paths.
func TestWalk_DedupesViaUnion(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	mustCreate(t, s, "A", "B", EdgeDependsOn)
	mustCreate(t, s, "A", "C", EdgeDependsOn)
	mustCreate(t, s, "B", "D", EdgeDependsOn)
	mustCreate(t, s, "C", "D", EdgeDependsOn)

	rows, err := s.Walk(ctx, "A", KindManifest, []string{EdgeDependsOn}, 10)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	// Count occurrences of each id; D should appear exactly once.
	count := map[string]int{}
	for _, r := range rows {
		count[r.ID]++
	}
	if count["D"] != 1 {
		t.Errorf("UNION dedupe failed — D appears %d times: %+v", count["D"], rows)
	}
	if len(rows) != 4 { // A, B, C, D
		t.Errorf("expected 4 unique nodes (A,B,C,D), got %d", len(rows))
	}
}
