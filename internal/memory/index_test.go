package memory

import (
	"testing"
	"time"
)

// newTestIndex creates a throwaway Index in t.TempDir with a small vec
// dimension so tests can insert dummy embeddings.
func newTestIndex(t *testing.T) *Index {
	t.Helper()
	dir := t.TempDir()
	idx, err := NewIndex(dir, 4)
	if err != nil {
		t.Fatalf("NewIndex: %v", err)
	}
	t.Cleanup(func() { idx.Close() })
	return idx
}

func seed(t *testing.T, idx *Index, id, path, l0 string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	m := &Memory{
		ID: id, Path: path, L0: l0, L1: l0, L2: l0,
		Type: "insight", Scope: "project", Project: "p", Domain: "d",
		Tags:      []string{},
		CreatedAt: now, UpdatedAt: now, AccessedAt: now,
	}
	if err := idx.Upsert(m, []float32{0.1, 0.2, 0.3, 0.4}); err != nil {
		t.Fatalf("Upsert %s: %v", id, err)
	}
}

func TestGetByIDPrefix_ShortMarker(t *testing.T) {
	idx := newTestIndex(t)
	seed(t, idx, "019daac8-cdb3-7e77-b995-8706b3414128", "/project/p/d/alpha", "alpha")

	got, err := idx.GetByIDPrefix("019daac8")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got == nil || got.Path != "/project/p/d/alpha" {
		t.Fatalf("expected alpha, got %+v", got)
	}
}

func TestGetByIDPrefixAll_Ambiguous(t *testing.T) {
	idx := newTestIndex(t)
	seed(t, idx, "019daac8-cdb3-7e77-b995-8706b3414128", "/project/p/d/alpha", "alpha")
	seed(t, idx, "019daac8-fff0-7000-aaaa-000000000001", "/project/p/d/beta", "beta")

	mems, err := idx.GetByIDPrefixAll("019daac8", 10)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(mems) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(mems))
	}
}

func TestGetByIDPrefixAll_LongPrefixDisambiguates(t *testing.T) {
	idx := newTestIndex(t)
	seed(t, idx, "019daac8-cdb3-7e77-b995-8706b3414128", "/project/p/d/alpha", "alpha")
	seed(t, idx, "019daac8-fff0-7000-aaaa-000000000001", "/project/p/d/beta", "beta")

	// Full 36-char UUID — single match via rung 2.
	mems, err := idx.GetByIDPrefixAll("019daac8-cdb3-7e77-b995-8706b3414128", 10)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(mems) != 1 || mems[0].Path != "/project/p/d/alpha" {
		t.Fatalf("expected single alpha match, got %+v", mems)
	}

	// 16-char prefix — still disambiguates.
	mems, err = idx.GetByIDPrefixAll("019daac8-cdb3-7e", 10)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(mems) != 1 || mems[0].Path != "/project/p/d/alpha" {
		t.Fatalf("expected single alpha match for 16-char prefix, got %+v", mems)
	}
}

func TestFindByIDSubstring_DashMangled(t *testing.T) {
	idx := newTestIndex(t)
	seed(t, idx, "019daac8-cdb3-7e77-b995-8706b3414128", "/project/p/d/alpha", "alpha")
	seed(t, idx, "019daac8-fff0-7000-aaaa-000000000001", "/project/p/d/beta", "beta")

	// "cdb3-7e77" appears only inside alpha's id. rung-2 prefix match would
	// never hit; substring search must.
	mems, err := idx.FindByIDSubstring("cdb3-7e77", 10)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(mems) != 1 || mems[0].Path != "/project/p/d/alpha" {
		t.Fatalf("expected alpha via substring, got %+v", mems)
	}
}

func TestFindByIDSubstring_TwelveCharDashPartial(t *testing.T) {
	idx := newTestIndex(t)
	seed(t, idx, "019daac8-cdb3-7e77-b995-8706b3414128", "/project/p/d/alpha", "alpha")

	// "019daac8-cdb" is a 12-char marker. Rung-2 LIKE 'prefix%' hits because
	// the stored id does start with it — this is the primary bug: the caller
	// (handleRecall) previously only tried prefix when len<=8.
	mems, err := idx.GetByIDPrefixAll("019daac8-cdb", 10)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(mems) != 1 {
		t.Fatalf("expected single alpha match for 12-char marker, got %d", len(mems))
	}
}

func TestGetByID_UnknownReturnsNil(t *testing.T) {
	idx := newTestIndex(t)
	seed(t, idx, "019daac8-cdb3-7e77-b995-8706b3414128", "/project/p/d/alpha", "alpha")

	m, err := idx.GetByID("deadbeef-nope")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if m != nil {
		t.Fatalf("expected nil, got %+v", m)
	}
	prefixes, err := idx.GetByIDPrefixAll("deadbeef", 10)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(prefixes) != 0 {
		t.Fatalf("expected 0, got %d", len(prefixes))
	}
	subs, err := idx.FindByIDSubstring("deadbeef", 10)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(subs) != 0 {
		t.Fatalf("expected 0 substring matches, got %d", len(subs))
	}
}

func TestGetByPath_And_ListByPrefix(t *testing.T) {
	idx := newTestIndex(t)
	seed(t, idx, "019daac8-cdb3-7e77-b995-8706b3414128", "/project/openpraxis/sessions/state-2026-04-20-afternoon", "session")
	seed(t, idx, "019daac8-fff0-7000-aaaa-000000000001", "/project/openpraxis/sessions/state-2026-04-19-morning", "session2")
	seed(t, idx, "019daac8-aaaa-7000-aaaa-000000000002", "/project/other/unrelated", "other")

	m, err := idx.GetByPath("/project/openpraxis/sessions/state-2026-04-20-afternoon")
	if err != nil || m == nil {
		t.Fatalf("expected path hit: %v %+v", err, m)
	}

	mems, err := idx.ListByPrefix("/project/openpraxis/sessions/", 10)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(mems) != 2 {
		t.Fatalf("expected 2 under sessions prefix, got %d", len(mems))
	}
}
