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
	"strings"
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

// TestCreate_ValidationErrors — every validation path that USED to be
// enforced by DB-level CHECK constraints (now removed for portability,
// 2026-04-25) must be caught in Go. If any of these regress, malformed
// rows would land silently in the DB. The schema has no safety net —
// Go is the safety net.
func TestCreate_ValidationErrors(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	cases := []struct {
		name    string
		edge    Edge
		wantErr error
	}{
		{
			name: "empty src_id",
			edge: Edge{SrcKind: KindManifest, SrcID: "", DstKind: KindManifest, DstID: "B", Kind: EdgeDependsOn},
			wantErr: ErrEmptyID,
		},
		{
			name: "empty dst_id",
			edge: Edge{SrcKind: KindManifest, SrcID: "A", DstKind: KindManifest, DstID: "", Kind: EdgeDependsOn},
			wantErr: ErrEmptyID,
		},
		{
			name: "both empty (would otherwise look like self-loop)",
			edge: Edge{SrcKind: KindManifest, SrcID: "", DstKind: KindManifest, DstID: "", Kind: EdgeDependsOn},
			wantErr: ErrEmptyID,
		},
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

// TestRemove_ValidationErrors — Remove must reject the same shapes
// (empty IDs, bad edge kind) that Create rejects. Without these checks
// a typo could silently no-op against the DB and the caller wouldn't
// know.
func TestRemove_ValidationErrors(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	cases := []struct {
		name    string
		srcID   string
		dstID   string
		kind    string
		wantErr error
	}{
		{"empty src_id", "", "B", EdgeDependsOn, ErrEmptyID},
		{"empty dst_id", "A", "", EdgeDependsOn, ErrEmptyID},
		{"bad edge kind", "A", "B", "garbage", ErrInvalidKind},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := s.Remove(ctx, tc.srcID, tc.dstID, tc.kind, "test", "test")
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Remove: want %v, got %v", tc.wantErr, err)
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

// ─── Time-travel readers (ListOutgoingAt / ListIncomingAt / WalkAt) ────────

// TestListOutgoingAt_PastSnapshot — re-target an edge over time, verify
// past-vs-current state differs. Re-target = Remove(A→B) + Create(A→C)
// since A→B and A→C are distinct edges in the SCD model (different dst).
//
// Timeline:
//
//	t_pre:  capture timestamp BEFORE any edge exists
//	T0:     Create A→B  (valid_from=T0, valid_to='')
//	t_mid:  capture timestamp WHILE A→B is current
//	T1:     Remove A→B   (closes A→B at T1; valid_to set)
//	T2:     Create A→C   (separate fresh edge)
//	t_now:  current state = only A→C
//
// Expected at t_pre: 0 edges (nothing existed)
// Expected at t_mid: A→B (still current then)
// Expected now:      A→C
func TestListOutgoingAt_PastSnapshot(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	tPre := time.Now().UTC().Add(-1 * time.Millisecond).Format(time.RFC3339Nano)

	mustCreate(t, s, "A", "B", EdgeDependsOn, func(e *Edge) { e.Reason = "v1" })
	time.Sleep(5 * time.Millisecond)
	tMid := time.Now().UTC().Format(time.RFC3339Nano)
	time.Sleep(2 * time.Millisecond)

	// Query AT tPre — before any edge existed.
	if got, _ := s.ListOutgoingAt(ctx, "A", EdgeDependsOn, tPre); len(got) != 0 {
		t.Errorf("at tPre (pre-creation), expected 0 edges, got %d", len(got))
	}

	// Query AT tMid (after Create) — should see A→B.
	got, err := s.ListOutgoingAt(ctx, "A", EdgeDependsOn, tMid)
	if err != nil {
		t.Fatalf("ListOutgoingAt: %v", err)
	}
	if len(got) != 1 || got[0].DstID != "B" {
		t.Fatalf("at tMid, expected A→B, got %+v", got)
	}

	// Re-target: explicit Remove A→B, then Create A→C (distinct edge).
	time.Sleep(2 * time.Millisecond)
	if err := s.Remove(ctx, "A", "B", EdgeDependsOn, "test", "re-target"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	mustCreate(t, s, "A", "C", EdgeDependsOn, func(e *Edge) { e.Reason = "v2" })

	// Query AT tMid again — A→B was current then, A→C didn't exist.
	got, err = s.ListOutgoingAt(ctx, "A", EdgeDependsOn, tMid)
	if err != nil {
		t.Fatalf("ListOutgoingAt past: %v", err)
	}
	dstSet := map[string]bool{}
	for _, e := range got {
		dstSet[e.DstID] = true
	}
	if !dstSet["B"] {
		t.Errorf("at tMid, expected to see B: %+v", got)
	}
	if dstSet["C"] {
		t.Errorf("at tMid, A→C should NOT have existed yet: %+v", got)
	}

	// Query NOW (current state) — only A→C.
	currentEdges, _ := s.ListOutgoing(ctx, "A", EdgeDependsOn)
	if len(currentEdges) != 1 || currentEdges[0].DstID != "C" {
		t.Errorf("current state should be A→C only, got %+v", currentEdges)
	}
}

// TestListOutgoingAt_EmptyAsOfDelegatesToCurrent — empty asOf falls
// through to ListOutgoing (hot-path partial-index reader). Verifies
// the optimization: empty asOf → same result as calling ListOutgoing
// directly.
func TestListOutgoingAt_EmptyAsOfDelegatesToCurrent(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	mustCreate(t, s, "A", "B", EdgeDependsOn)
	mustCreate(t, s, "A", "C", EdgeDependsOn)

	currentDirect, _ := s.ListOutgoing(ctx, "A", EdgeDependsOn)
	currentViaAt, _ := s.ListOutgoingAt(ctx, "A", EdgeDependsOn, "")

	if len(currentDirect) != len(currentViaAt) {
		t.Errorf("empty asOf should match ListOutgoing: direct=%d viaAt=%d",
			len(currentDirect), len(currentViaAt))
	}
}

// TestListOutgoingAt_InvalidTimestamp — non-ISO8601 asOf rejected.
func TestListOutgoingAt_InvalidTimestamp(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	_, err := s.ListOutgoingAt(ctx, "A", EdgeDependsOn, "not a timestamp")
	if err == nil {
		t.Fatal("expected error on garbage asOf, got nil")
	}
	if !strings.Contains(err.Error(), "as_of must be ISO8601") {
		t.Errorf("expected 'as_of must be ISO8601' in error, got: %v", err)
	}
}

// TestListIncomingAt_PastSnapshot — mirror of ListOutgoingAt_PastSnapshot.
// Multiple sources point at one dst; one is closed; query at past time
// should see both, query now should see only the survivor.
func TestListIncomingAt_PastSnapshot(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	mustCreate(t, s, "M1", "M0", EdgeDependsOn)
	mustCreate(t, s, "M2", "M0", EdgeDependsOn)
	time.Sleep(5 * time.Millisecond)
	t1 := time.Now().UTC().Format(time.RFC3339Nano)
	time.Sleep(2 * time.Millisecond)

	// Close M1→M0 (Remove).
	if err := s.Remove(ctx, "M1", "M0", EdgeDependsOn, "test", "removed"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Past query: M1→M0 + M2→M0 BOTH current at t1.
	past, err := s.ListIncomingAt(ctx, "M0", EdgeDependsOn, t1)
	if err != nil {
		t.Fatalf("ListIncomingAt: %v", err)
	}
	if len(past) != 2 {
		t.Errorf("at t1, expected 2 incoming, got %d: %+v", len(past), past)
	}

	// Current: only M2→M0 left.
	now, _ := s.ListIncoming(ctx, "M0", EdgeDependsOn)
	if len(now) != 1 || now[0].SrcID != "M2" {
		t.Errorf("current should be only M2→M0, got %+v", now)
	}
}

// TestWalkAt_PastSnapshot — three-node chain whose tail is re-targeted.
// Verifies time-travel walks return the structure that was current at
// asOf, not what's current now. Re-target uses explicit Remove+Create
// (B→C and B→D are distinct edges, not metadata-versions of the same).
//
// Timeline:
//
//	T0: Create A→B + Create B→C   (chain A→B→C)
//	t1: capture
//	T1: Remove B→C, Create B→D    (chain A→B→D from now on)
//
// Expected at t1: walk(A) = {A, B, C}
// Expected now:   walk(A) = {A, B, D}
func TestWalkAt_PastSnapshot(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	mustCreate(t, s, "A", "B", EdgeDependsOn)
	mustCreate(t, s, "B", "C", EdgeDependsOn)
	time.Sleep(5 * time.Millisecond)
	t1 := time.Now().UTC().Format(time.RFC3339Nano)
	time.Sleep(2 * time.Millisecond)
	// Re-target B's outgoing: close B→C, open B→D.
	if err := s.Remove(ctx, "B", "C", EdgeDependsOn, "test", "re-target"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	mustCreate(t, s, "B", "D", EdgeDependsOn)

	pastRows, err := s.WalkAt(ctx, "A", KindManifest, []string{EdgeDependsOn}, 10, t1)
	pastIDs := walkIDs(t, pastRows, err)
	if !pastIDs["A"] || !pastIDs["B"] || !pastIDs["C"] {
		t.Errorf("at t1, expected {A,B,C}, got %v", pastIDs)
	}
	if pastIDs["D"] {
		t.Errorf("at t1, D shouldn't exist yet: %v", pastIDs)
	}

	nowRows, err := s.Walk(ctx, "A", KindManifest, []string{EdgeDependsOn}, 10)
	nowIDs := walkIDs(t, nowRows, err)
	if !nowIDs["A"] || !nowIDs["B"] || !nowIDs["D"] {
		t.Errorf("now, expected {A,B,D}, got %v", nowIDs)
	}
	if nowIDs["C"] {
		t.Errorf("now, C should be gone: %v", nowIDs)
	}
}

// TestWalkAt_EmptyAsOfDelegatesToWalk — same delegation behavior as
// ListOutgoingAt: empty asOf → hot-path Walk.
func TestWalkAt_EmptyAsOfDelegatesToWalk(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	mustCreate(t, s, "A", "B", EdgeDependsOn)
	mustCreate(t, s, "B", "C", EdgeDependsOn)

	direct, _ := s.Walk(ctx, "A", KindManifest, []string{EdgeDependsOn}, 10)
	viaAt, _ := s.WalkAt(ctx, "A", KindManifest, []string{EdgeDependsOn}, 10, "")

	if len(direct) != len(viaAt) {
		t.Errorf("empty asOf delegation broken: direct=%d viaAt=%d", len(direct), len(viaAt))
	}
}

// ─── Reader empty-ID rejection (C1 from self-review) ────────────────────────

// TestReaders_RejectEmptyIDs — every reader must reject empty IDs at
// the top with ErrEmptyID. Previously they would silently scan with an
// empty key (returning nothing) which masked buggy callers. With CHECK
// constraints removed for portability, Go is the only safety net.
func TestReaders_RejectEmptyIDs(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	cases := []struct {
		name string
		fn   func() error
	}{
		{"ListOutgoing empty src", func() error {
			_, err := s.ListOutgoing(ctx, "", EdgeDependsOn)
			return err
		}},
		{"ListIncoming empty dst", func() error {
			_, err := s.ListIncoming(ctx, "", EdgeDependsOn)
			return err
		}},
		{"History empty src", func() error {
			_, err := s.History(ctx, "", "B", EdgeDependsOn)
			return err
		}},
		{"History empty dst", func() error {
			_, err := s.History(ctx, "A", "", EdgeDependsOn)
			return err
		}},
		{"Walk empty root", func() error {
			_, err := s.Walk(ctx, "", KindManifest, nil, 10)
			return err
		}},
		{"ListOutgoingAt empty src", func() error {
			_, err := s.ListOutgoingAt(ctx, "", EdgeDependsOn, "2026-01-01T00:00:00Z")
			return err
		}},
		{"ListIncomingAt empty dst", func() error {
			_, err := s.ListIncomingAt(ctx, "", EdgeDependsOn, "2026-01-01T00:00:00Z")
			return err
		}},
		{"WalkAt empty root", func() error {
			_, err := s.WalkAt(ctx, "", KindManifest, nil, 10, "2026-01-01T00:00:00Z")
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn()
			if !errors.Is(err, ErrEmptyID) {
				t.Errorf("expected ErrEmptyID, got %v", err)
			}
		})
	}
}

// ─── BackfillRow ─────────────────────────────────────────────────────────────

// TestBackfillRow_PreservesValidFromValidTo — backfill must accept
// caller-controlled valid_from AND valid_to, including a fully-closed
// historical row with both set to past timestamps. PR/M2's task_dependency
// migration depends on this.
func TestBackfillRow_PreservesValidFromValidTo(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	// Insert a closed historical row from a fictional 2025 backfill.
	pastFrom := "2025-06-01T00:00:00Z"
	pastTo := "2025-09-01T00:00:00Z"
	if err := s.BackfillRow(ctx, Edge{
		SrcKind: KindManifest, SrcID: "M1",
		DstKind: KindManifest, DstID: "M2",
		Kind:      EdgeDependsOn,
		ValidFrom: pastFrom,
		ValidTo:   pastTo,
		CreatedBy: "backfill",
		Reason:    "PR/M2 from task_dependency",
	}); err != nil {
		t.Fatalf("BackfillRow: %v", err)
	}

	hist, _ := s.History(ctx, "M1", "M2", EdgeDependsOn)
	if len(hist) != 1 {
		t.Fatalf("expected 1 history row, got %d", len(hist))
	}
	if hist[0].ValidFrom != pastFrom || hist[0].ValidTo != pastTo {
		t.Errorf("intervals not preserved: from=%q to=%q", hist[0].ValidFrom, hist[0].ValidTo)
	}
	// Closed row should NOT appear in current state.
	if got, _ := s.ListOutgoing(ctx, "M1", EdgeDependsOn); len(got) != 0 {
		t.Errorf("closed historical row leaked into current state: %+v", got)
	}
}

// TestBackfillRow_RequiresValidFrom — empty ValidFrom rejected. Backfill
// callers MUST preserve historical timing; defaulting to now() would
// silently corrupt the timeline.
func TestBackfillRow_RequiresValidFrom(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	err := s.BackfillRow(ctx, Edge{
		SrcKind: KindManifest, SrcID: "M1",
		DstKind: KindManifest, DstID: "M2",
		Kind:      EdgeDependsOn,
		ValidFrom: "", // missing — should reject
	})
	if err == nil {
		t.Fatal("expected error on empty ValidFrom")
	}
	if !strings.Contains(err.Error(), "ValidFrom") {
		t.Errorf("error should mention ValidFrom, got: %v", err)
	}
}

// TestBackfillRow_DuplicateRejectedByPK — same (src, dst, kind, valid_from)
// inserted twice fails the second time on PK constraint. Callers can use
// this for "skip already-migrated" semantics.
func TestBackfillRow_DuplicateRejectedByPK(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	e := Edge{
		SrcKind: KindManifest, SrcID: "M1",
		DstKind: KindManifest, DstID: "M2",
		Kind:      EdgeDependsOn,
		ValidFrom: "2025-06-01T00:00:00Z",
		ValidTo:   "2025-09-01T00:00:00Z",
		CreatedBy: "backfill",
	}
	if err := s.BackfillRow(ctx, e); err != nil {
		t.Fatalf("first BackfillRow: %v", err)
	}
	err := s.BackfillRow(ctx, e)
	if err == nil {
		t.Fatal("expected PK error on duplicate, got nil")
	}
}

// ─── Health ──────────────────────────────────────────────────────────────────

// TestHealth_CountsCurrentVsTotal — Health distinguishes current edges
// from total-with-history. Add 2 edges, close one, expect current=1
// total=2 (the closed row stays in the table per SCD-2).
func TestHealth_CountsCurrentVsTotal(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	mustCreate(t, s, "A", "B", EdgeDependsOn)
	mustCreate(t, s, "A", "C", EdgeDependsOn)
	if err := s.Remove(ctx, "A", "B", EdgeDependsOn, "test", "removed"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	h, err := s.Health(ctx)
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if !h.TableExists {
		t.Errorf("TableExists should be true after migration")
	}
	if h.CurrentEdges != 1 {
		t.Errorf("expected 1 current edge (A→C), got %d", h.CurrentEdges)
	}
	if h.TotalRows != 2 {
		t.Errorf("expected 2 total rows (closed A→B + current A→C), got %d", h.TotalRows)
	}
}

// ─── ListOutgoingForMany ─────────────────────────────────────────────────────

// TestListOutgoingForMany_BatchesQuery — three sources each with edges,
// one IN-clause query returns everything grouped by src_id. Replaces the
// N+1 pattern.
func TestListOutgoingForMany_BatchesQuery(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	mustCreate(t, s, "A", "X", EdgeDependsOn)
	mustCreate(t, s, "A", "Y", EdgeDependsOn)
	mustCreate(t, s, "B", "Z", EdgeDependsOn)
	// C has no edges intentionally.

	got, err := s.ListOutgoingForMany(ctx, []string{"A", "B", "C"}, EdgeDependsOn)
	if err != nil {
		t.Fatalf("ListOutgoingForMany: %v", err)
	}
	if len(got["A"]) != 2 {
		t.Errorf("A should have 2 edges, got %d", len(got["A"]))
	}
	if len(got["B"]) != 1 {
		t.Errorf("B should have 1 edge, got %d", len(got["B"]))
	}
	if len(got["C"]) != 0 {
		t.Errorf("C should have 0 edges, got %d", len(got["C"]))
	}
}

// TestListOutgoingForMany_EmptySrcIDs — passing an empty slice returns
// an empty map, not nil, and not an error.
func TestListOutgoingForMany_EmptySrcIDs(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	got, err := s.ListOutgoingForMany(ctx, []string{}, "")
	if err != nil {
		t.Fatalf("expected no error on empty input, got %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

// TestListOutgoingForMany_RejectsEmptyEntry — slice containing "" is
// caller error.
func TestListOutgoingForMany_RejectsEmptyEntry(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	_, err := s.ListOutgoingForMany(ctx, []string{"A", "", "B"}, "")
	if !errors.Is(err, ErrEmptyID) {
		t.Errorf("expected ErrEmptyID, got %v", err)
	}
}

// walkIDs is a tiny helper: run a Walk* call, fail the test on error,
// return a set-of-IDs map for terse membership assertions.
func walkIDs(t *testing.T, rows []WalkRow, err error) map[string]bool {
	t.Helper()
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	out := map[string]bool{}
	for _, r := range rows {
		out[r.ID] = true
	}
	return out
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
