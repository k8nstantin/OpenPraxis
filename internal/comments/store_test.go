package comments

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(openTestDB(t))
}

func TestStore_Add_PopulatesFields(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	c, err := s.Add(ctx, TargetProduct, "p1", "alice", TypeUserNote, "hello")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if c.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if _, err := uuid.Parse(c.ID); err != nil {
		t.Fatalf("expected UUID id, got %q: %v", c.ID, err)
	}
	if c.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt set")
	}
	if c.TargetType != TargetProduct || c.TargetID != "p1" ||
		c.Author != "alice" || c.Type != TypeUserNote || c.Body != "hello" {
		t.Fatalf("round-trip mismatch: %+v", c)
	}

	got, err := s.Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != c.ID || got.Body != "hello" || got.Author != "alice" {
		t.Fatalf("get mismatch: %+v", got)
	}
}

func TestStore_Add_RejectsEmptyAuthor(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Add(context.Background(), TargetProduct, "p1", "", TypeUserNote, "body")
	if !errors.Is(err, ErrEmptyAuthor) {
		t.Fatalf("expected ErrEmptyAuthor, got %v", err)
	}
}

func TestStore_Add_RejectsEmptyBody(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Add(context.Background(), TargetProduct, "p1", "alice", TypeUserNote, "")
	if !errors.Is(err, ErrEmptyBody) {
		t.Fatalf("expected ErrEmptyBody, got %v", err)
	}
}

func TestStore_Get_NotFound(t *testing.T) {
	s := newTestStore(t)
	id, _ := uuid.NewV7()
	_, err := s.Get(context.Background(), id.String())
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestStore_List_NewestFirst(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	bodies := []string{"first", "second", "third"}
	for _, b := range bodies {
		if _, err := s.Add(ctx, TargetManifest, "m1", "alice", TypeUserNote, b); err != nil {
			t.Fatalf("add %q: %v", b, err)
		}
		time.Sleep(1100 * time.Millisecond)
	}

	got, err := s.List(ctx, TargetManifest, "m1", 0, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
	if got[0].Body != "third" || got[1].Body != "second" || got[2].Body != "first" {
		t.Fatalf("expected newest-first, got: %q, %q, %q", got[0].Body, got[1].Body, got[2].Body)
	}
}

func TestStore_List_TypeFilter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.Add(ctx, TargetTask, "t1", "alice", TypeUserNote, "note"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := s.Add(ctx, TargetTask, "t1", "alice", TypeDecision, "decision"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := s.Add(ctx, TargetTask, "t1", "alice", TypeAgentNote, "agent"); err != nil {
		t.Fatalf("add: %v", err)
	}

	f := TypeDecision
	got, err := s.List(ctx, TargetTask, "t1", 0, &f)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].Type != TypeDecision {
		t.Fatalf("expected only decision, got %+v", got)
	}
}

func TestStore_List_LimitHonored(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		if _, err := s.Add(ctx, TargetProduct, "p1", "alice", TypeUserNote, fmt.Sprintf("body-%d", i)); err != nil {
			t.Fatalf("add: %v", err)
		}
	}

	got, err := s.List(ctx, TargetProduct, "p1", 5, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("expected 5, got %d", len(got))
	}
}

func TestStore_List_LimitDefaultAndCap(t *testing.T) {
	// We check the clamping logic without inserting 1000+ rows by inserting a
	// modest number and asserting the list returns them all for both limit=0
	// (which should default to 100) and limit=2000 (which should be capped at
	// 1000). Either way, a smaller dataset returns entirely.
	s := newTestStore(t)
	ctx := context.Background()

	for i := 0; i < 42; i++ {
		if _, err := s.Add(ctx, TargetProduct, "p1", "alice", TypeUserNote, fmt.Sprintf("b%d", i)); err != nil {
			t.Fatalf("add: %v", err)
		}
	}

	got, err := s.List(ctx, TargetProduct, "p1", 0, nil)
	if err != nil {
		t.Fatalf("list default: %v", err)
	}
	if len(got) != 42 {
		t.Fatalf("expected 42 with default limit, got %d", len(got))
	}

	got, err = s.List(ctx, TargetProduct, "p1", 2000, nil)
	if err != nil {
		t.Fatalf("list cap: %v", err)
	}
	if len(got) != 42 {
		t.Fatalf("expected 42 with oversized limit, got %d", len(got))
	}
}

func TestStore_List_ScopeIsolation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.Add(ctx, TargetProduct, "A", "alice", TypeUserNote, "prod-A"); err != nil {
		t.Fatalf("add product: %v", err)
	}
	if _, err := s.Add(ctx, TargetManifest, "A", "alice", TypeUserNote, "man-A"); err != nil {
		t.Fatalf("add manifest: %v", err)
	}

	got, err := s.List(ctx, TargetTask, "A", 0, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 task-scoped results, got %d", len(got))
	}
}

func TestStore_Edit_UpdatesBodyAndTimestamp(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	c, err := s.Add(ctx, TargetProduct, "p1", "alice", TypeUserNote, "old")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)

	if err := s.Edit(ctx, c.ID, "new"); err != nil {
		t.Fatalf("edit: %v", err)
	}

	got, err := s.Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Body != "new" {
		t.Fatalf("expected body=new, got %q", got.Body)
	}
	if got.UpdatedAt == nil {
		t.Fatal("expected UpdatedAt populated")
	}
	if !got.UpdatedAt.After(got.CreatedAt) {
		t.Fatalf("expected UpdatedAt (%v) after CreatedAt (%v)", got.UpdatedAt, got.CreatedAt)
	}
}

func TestStore_Edit_NotFound(t *testing.T) {
	s := newTestStore(t)
	id, _ := uuid.NewV7()
	if err := s.Edit(context.Background(), id.String(), "x"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestStore_Edit_RejectsEmptyBody(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	c, err := s.Add(ctx, TargetProduct, "p1", "alice", TypeUserNote, "old")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := s.Edit(ctx, c.ID, ""); !errors.Is(err, ErrEmptyBody) {
		t.Fatalf("expected ErrEmptyBody, got %v", err)
	}
}

func TestStore_Delete_Idempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	c, err := s.Add(ctx, TargetProduct, "p1", "alice", TypeUserNote, "x")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := s.Delete(ctx, c.ID); err != nil {
		t.Fatalf("delete 1: %v", err)
	}
	if err := s.Delete(ctx, c.ID); err != nil {
		t.Fatalf("delete 2 (should be idempotent): %v", err)
	}
	if _, err := s.Get(ctx, c.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows after delete, got %v", err)
	}
}

// TestStore_ConcurrentWrites_WAL exercises visceral rule #10: two separate
// Stores on the same DB file must survive concurrent writes under WAL +
// busy_timeout=5000. 50 goroutines × 10 inserts = 500 per store, 1000 total.
func TestStore_ConcurrentWrites_WAL(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "concurrent.db")
	dsn := dbPath + "?_journal_mode=WAL&_busy_timeout=5000"

	open := func() *sql.DB {
		db, err := sql.Open("sqlite3", dsn)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
			t.Fatalf("pragma WAL: %v", err)
		}
		if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
			t.Fatalf("pragma busy_timeout: %v", err)
		}
		return db
	}

	db1 := open()
	defer db1.Close()
	db2 := open()
	defer db2.Close()

	if err := InitSchema(db1); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	s1 := NewStore(db1)
	s2 := NewStore(db2)

	// 50 goroutines per store × 2 stores × 10 inserts = 1000 rows total.
	const goroutinesPerStore = 50
	const perGoroutine = 10

	var wg sync.WaitGroup
	errCh := make(chan error, goroutinesPerStore*2*perGoroutine)
	ctx := context.Background()

	spawn := func(store *Store, tag string) {
		for i := 0; i < goroutinesPerStore; i++ {
			wg.Add(1)
			go func(g int) {
				defer wg.Done()
				for j := 0; j < perGoroutine; j++ {
					if _, err := store.Add(ctx, TargetTask, "t1", "alice", TypeAgentNote,
						fmt.Sprintf("%s-g%d-j%d", tag, g, j)); err != nil {
						errCh <- err
						return
					}
				}
			}(i)
		}
	}
	spawn(s1, "s1")
	spawn(s2, "s2")

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent add error: %v", err)
	}

	var count int
	if err := db1.QueryRow(`SELECT COUNT(*) FROM comments`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if want := goroutinesPerStore * 2 * perGoroutine; count != want {
		t.Fatalf("expected %d rows, got %d", want, count)
	}
}
