package templates

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// TestUpdateBody_SCD2 — a PUT closes the prior active row and appends a
// new current row atomically. History has exactly two rows for that uid
// after the update, with the new one being the sole active row.
func TestUpdateBody_SCD2(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	if err := Seed(ctx, store, "peer-a"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	original, err := store.Get(ctx, ScopeSystem, "", SectionPreamble)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if err := store.UpdateBody(ctx, original.TemplateUID, "NEW BODY", "tester", "test-change"); err != nil {
		t.Fatalf("update: %v", err)
	}

	current, err := store.GetByUID(ctx, original.TemplateUID)
	if err != nil {
		t.Fatalf("get by uid: %v", err)
	}
	if current.Body != "NEW BODY" {
		t.Fatalf("current body = %q, want NEW BODY", current.Body)
	}
	if current.ValidTo != "" {
		t.Fatalf("new row should be open, got valid_to=%q", current.ValidTo)
	}
	if current.ChangedBy != "tester" || current.Reason != "test-change" {
		t.Fatalf("audit columns not set: changed_by=%q reason=%q", current.ChangedBy, current.Reason)
	}

	history, err := store.History(ctx, original.TemplateUID)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("history len = %d, want 2", len(history))
	}
	if history[0].ValidTo != "" {
		t.Fatalf("newest row should be active, got valid_to=%q", history[0].ValidTo)
	}
	if history[1].ValidTo == "" {
		t.Fatalf("older row should be closed")
	}

	// Exactly one active row for that uid.
	var activeCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM prompt_templates WHERE template_uid=? AND valid_to=''`,
		original.TemplateUID).Scan(&activeCount); err != nil {
		t.Fatalf("active count: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("active count = %d, want 1", activeCount)
	}
}

func TestUpdateBody_UnknownUIDReturnsNotFound(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	if err := Seed(ctx, store, ""); err != nil {
		t.Fatalf("seed: %v", err)
	}
	err := store.UpdateBody(ctx, "does-not-exist", "x", "a", "b")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// TestUpdateBody_ConcurrentWriters exercises the partial unique index +
// BEGIN IMMEDIATE serialisation: 10 goroutines each PUT a different body
// to the same uid. Afterward exactly one active row must remain, and
// history must contain 11 rows total (seed + 10 updates). No writer may
// leave a second row with valid_to=''.
func TestUpdateBody_ConcurrentWriters(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	if err := Seed(ctx, store, ""); err != nil {
		t.Fatalf("seed: %v", err)
	}
	original, err := store.Get(ctx, ScopeSystem, "", SectionPreamble)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	const writers = 10
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := store.UpdateBody(ctx, original.TemplateUID, "body-"+time.Now().Format(time.RFC3339Nano), "w", "c"); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatalf("writer err: %v", e)
	}

	var activeCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM prompt_templates WHERE template_uid=? AND valid_to=''`,
		original.TemplateUID).Scan(&activeCount); err != nil {
		t.Fatalf("active count: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("active count = %d, want 1", activeCount)
	}
	history, err := store.History(ctx, original.TemplateUID)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) != writers+1 {
		t.Fatalf("history len = %d, want %d", len(history), writers+1)
	}
}

// TestCreate_RejectsDuplicate ensures we can't create a second active
// override at the same (scope, scope_id, section) triple.
func TestCreate_RejectsDuplicate(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	if _, err := store.Create(ctx, ScopeTask, "task-1", SectionPreamble, "override", "BODY-A", "tester", "first"); err != nil {
		t.Fatalf("create 1: %v", err)
	}
	_, err := store.Create(ctx, ScopeTask, "task-1", SectionPreamble, "override", "BODY-B", "tester", "second")
	if !errors.Is(err, ErrDuplicateOverride) {
		t.Fatalf("want ErrDuplicateOverride, got %v", err)
	}
}

// TestCreate_RejectsSystem ensures system-scope rows are seed-only.
func TestCreate_RejectsSystem(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	if _, err := store.Create(ctx, ScopeSystem, "", SectionPreamble, "t", "b", "a", "r"); err == nil {
		t.Fatalf("expected rejection of system-scope Create")
	}
}

// TestAtTime_ReturnsBodyActiveAtTimestamp — seed, update, update again,
// then query AtTime for three distinct timestamps and verify the right
// body comes back for each.
func TestAtTime_ReturnsBodyActiveAtTimestamp(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	if err := Seed(ctx, store, ""); err != nil {
		t.Fatalf("seed: %v", err)
	}
	original, err := store.Get(ctx, ScopeSystem, "", SectionPreamble)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	seedBody := original.Body

	time.Sleep(5 * time.Millisecond)
	t1 := time.Now().UTC()
	time.Sleep(5 * time.Millisecond)
	if err := store.UpdateBody(ctx, original.TemplateUID, "v2", "tester", "r"); err != nil {
		t.Fatalf("update 1: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	t2 := time.Now().UTC()
	time.Sleep(5 * time.Millisecond)
	if err := store.UpdateBody(ctx, original.TemplateUID, "v3", "tester", "r"); err != nil {
		t.Fatalf("update 2: %v", err)
	}

	row, err := store.AtTime(ctx, original.TemplateUID, t1)
	if err != nil {
		t.Fatalf("at t1: %v", err)
	}
	if row.Body != seedBody {
		t.Fatalf("at t1 body = %q, want seed body", row.Body)
	}
	row, err = store.AtTime(ctx, original.TemplateUID, t2)
	if err != nil {
		t.Fatalf("at t2: %v", err)
	}
	if row.Body != "v2" {
		t.Fatalf("at t2 body = %q, want v2", row.Body)
	}
	now, err := store.AtTime(ctx, original.TemplateUID, time.Now().UTC())
	if err != nil {
		t.Fatalf("at now: %v", err)
	}
	if now.Body != "v3" {
		t.Fatalf("at now body = %q, want v3", now.Body)
	}
}

// TestTombstone_FallsThrough — after tombstoning a task-scope override,
// the resolver falls through to the system default again.
func TestTombstone_FallsThrough(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	if err := Seed(ctx, store, ""); err != nil {
		t.Fatalf("seed: %v", err)
	}
	uid, err := store.Create(ctx, ScopeTask, "task-1", SectionPreamble, "override", "CUSTOM", "tester", "init")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	r := NewResolver(store, nil, nil)
	body, err := r.Resolve(ctx, SectionPreamble, "task-1")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if body != "CUSTOM" {
		t.Fatalf("before tombstone body = %q, want CUSTOM", body)
	}
	if err := store.Tombstone(ctx, uid, "tester", "deleting"); err != nil {
		t.Fatalf("tombstone: %v", err)
	}
	body, err = r.Resolve(ctx, SectionPreamble, "task-1")
	if err != nil {
		t.Fatalf("resolve after tombstone: %v", err)
	}
	if body != defaultPreamble {
		t.Fatalf("after tombstone, expected fallback to system default; got %q", body)
	}
	// History still contains the rows but flagged deleted.
	hist, err := store.History(ctx, uid)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	for _, h := range hist {
		if h.DeletedAt == "" {
			t.Fatalf("history row not tombstoned: id=%d", h.ID)
		}
	}
}

// TestCloseStatus_FallsThrough — setting status='closed' on an override
// makes the resolver skip it and fall through.
func TestCloseStatus_FallsThrough(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	if err := Seed(ctx, store, ""); err != nil {
		t.Fatalf("seed: %v", err)
	}
	uid, err := store.Create(ctx, ScopeTask, "task-1", SectionPreamble, "override", "CUSTOM", "tester", "init")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.CloseStatus(ctx, uid, "tester", "close"); err != nil {
		t.Fatalf("close: %v", err)
	}
	r := NewResolver(store, nil, nil)
	body, err := r.Resolve(ctx, SectionPreamble, "task-1")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if body != defaultPreamble {
		t.Fatalf("closed override should fall through; got %q", body)
	}
}
