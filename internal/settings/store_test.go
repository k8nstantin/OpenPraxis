package settings

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// openStoreTestDB opens a fresh sqlite DB in t.TempDir() using the project's
// standard WAL + busy_timeout pragmas (see internal/memory/index.go:42-44),
// applies the settings schema, and returns (Store, db). The DB is closed on
// test cleanup.
func openStoreTestDB(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "settings.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("set WAL: %v", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		t.Fatalf("set busy_timeout: %v", err)
	}

	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	return NewStore(db), db
}

func TestStore_Set_InsertsNewRow(t *testing.T) {
	s, db := openStoreTestDB(t)
	ctx := context.Background()

	before := time.Now().Unix()
	if err := s.Set(ctx, ScopeProduct, "p1", "max_turns", `10`, "tester"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	var (
		scopeType, scopeID, key, value, updatedBy string
		updatedAt                                 int64
	)
	err := db.QueryRow(
		`SELECT scope_type, scope_id, key, value, updated_at, updated_by FROM settings WHERE key = ?`,
		"max_turns",
	).Scan(&scopeType, &scopeID, &key, &value, &updatedAt, &updatedBy)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}

	if scopeType != "product" || scopeID != "p1" || key != "max_turns" || value != "10" || updatedBy != "tester" {
		t.Fatalf("row mismatch: got (%s,%s,%s,%s,%s)", scopeType, scopeID, key, value, updatedBy)
	}
	if updatedAt < before {
		t.Fatalf("updated_at %d predates Set call %d", updatedAt, before)
	}
}

func TestStore_Set_UpdatesExistingRow(t *testing.T) {
	s, db := openStoreTestDB(t)
	ctx := context.Background()

	if err := s.Set(ctx, ScopeTask, "t1", "temperature", `"0.2"`, "alice"); err != nil {
		t.Fatalf("first Set: %v", err)
	}
	if err := s.Set(ctx, ScopeTask, "t1", "temperature", `"0.7"`, "bob"); err != nil {
		t.Fatalf("second Set: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM settings WHERE scope_type=? AND scope_id=? AND key=?`,
		"task", "t1", "temperature").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row after two Sets, got %d", count)
	}

	var value, updatedBy string
	if err := db.QueryRow(`SELECT value, updated_by FROM settings WHERE scope_type=? AND scope_id=? AND key=?`,
		"task", "t1", "temperature").Scan(&value, &updatedBy); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if value != `"0.7"` || updatedBy != "bob" {
		t.Fatalf("value=%q updated_by=%q after second Set", value, updatedBy)
	}
}

func TestStore_Get_ReturnsErrNoRowsWhenMissing(t *testing.T) {
	s, _ := openStoreTestDB(t)
	ctx := context.Background()

	_, err := s.Get(ctx, ScopeManifest, "nope", "max_turns")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestStore_Get_ReturnsValueAndUpdatedAt(t *testing.T) {
	s, _ := openStoreTestDB(t)
	ctx := context.Background()

	before := time.Now().Add(-time.Second)
	if err := s.Set(ctx, ScopeManifest, "m1", "model", `"gpt-5"`, "carol"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	after := time.Now().Add(time.Second)

	got, err := s.Get(ctx, ScopeManifest, "m1", "model")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ScopeType != ScopeManifest {
		t.Errorf("ScopeType = %q, want %q", got.ScopeType, ScopeManifest)
	}
	if got.ScopeID != "m1" {
		t.Errorf("ScopeID = %q, want %q", got.ScopeID, "m1")
	}
	if got.Key != "model" {
		t.Errorf("Key = %q, want %q", got.Key, "model")
	}
	if got.Value != `"gpt-5"` {
		t.Errorf("Value = %q, want %q", got.Value, `"gpt-5"`)
	}
	if got.UpdatedBy != "carol" {
		t.Errorf("UpdatedBy = %q, want %q", got.UpdatedBy, "carol")
	}
	if got.UpdatedAt.Before(before) || got.UpdatedAt.After(after) {
		t.Errorf("UpdatedAt = %v, outside [%v, %v]", got.UpdatedAt, before, after)
	}
}

func TestStore_ListScope_ReturnsAllKeysAtScope(t *testing.T) {
	s, _ := openStoreTestDB(t)
	ctx := context.Background()

	mustSet := func(scope ScopeType, id, k, v string) {
		t.Helper()
		if err := s.Set(ctx, scope, id, k, v, ""); err != nil {
			t.Fatalf("Set: %v", err)
		}
	}
	mustSet(ScopeProduct, "p1", "max_turns", "10")
	mustSet(ScopeProduct, "p1", "temperature", `"0.2"`)
	mustSet(ScopeProduct, "p1", "model", `"gpt-5"`)
	// Different scope_id / scope_type — must NOT be returned.
	mustSet(ScopeProduct, "p2", "max_turns", "99")
	mustSet(ScopeManifest, "p1", "max_turns", "5")

	entries, err := s.ListScope(ctx, ScopeProduct, "p1")
	if err != nil {
		t.Fatalf("ListScope: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d: %+v", len(entries), entries)
	}

	gotKeys := make([]string, len(entries))
	for i, e := range entries {
		gotKeys[i] = e.Key
		if e.ScopeType != ScopeProduct || e.ScopeID != "p1" {
			t.Errorf("entry has wrong scope: %+v", e)
		}
	}
	// Ordered by key alphabetically.
	wantKeys := []string{"max_turns", "model", "temperature"}
	for i, k := range wantKeys {
		if gotKeys[i] != k {
			t.Errorf("entries[%d].Key = %q, want %q", i, gotKeys[i], k)
		}
	}
}

func TestStore_ListScope_EmptyWhenNoEntries(t *testing.T) {
	s, _ := openStoreTestDB(t)
	ctx := context.Background()

	entries, err := s.ListScope(ctx, ScopeTask, "does-not-exist")
	if err != nil {
		t.Fatalf("ListScope: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

func TestStore_Delete_RemovesSpecificKey(t *testing.T) {
	s, _ := openStoreTestDB(t)
	ctx := context.Background()

	if err := s.Set(ctx, ScopeTask, "t1", "max_turns", "10", ""); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Set(ctx, ScopeTask, "t1", "model", `"gpt-5"`, ""); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if err := s.Delete(ctx, ScopeTask, "t1", "max_turns"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := s.Get(ctx, ScopeTask, "t1", "max_turns"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected max_turns to be gone, got err=%v", err)
	}
	// Other key at the same scope must still be present.
	if _, err := s.Get(ctx, ScopeTask, "t1", "model"); err != nil {
		t.Fatalf("Get model after unrelated Delete: %v", err)
	}
}

func TestStore_Delete_NoopWhenMissing(t *testing.T) {
	s, _ := openStoreTestDB(t)
	ctx := context.Background()

	if err := s.Delete(ctx, ScopeSystem, "", "never-set"); err != nil {
		t.Fatalf("Delete of missing key returned err: %v", err)
	}
}

func TestStore_ListByKey_ReturnsAcrossScopes(t *testing.T) {
	s, _ := openStoreTestDB(t)
	ctx := context.Background()

	mustSet := func(scope ScopeType, id, k, v string) {
		t.Helper()
		if err := s.Set(ctx, scope, id, k, v, ""); err != nil {
			t.Fatalf("Set: %v", err)
		}
	}
	mustSet(ScopeSystem, "", "max_turns", "100")
	mustSet(ScopeProduct, "p1", "max_turns", "50")
	mustSet(ScopeManifest, "m1", "max_turns", "25")
	mustSet(ScopeTask, "t1", "max_turns", "10")
	// A different key at every scope must be ignored.
	mustSet(ScopeSystem, "", "model", `"gpt-5"`)
	mustSet(ScopeProduct, "p1", "model", `"gpt-5"`)

	entries, err := s.ListByKey(ctx, "max_turns")
	if err != nil {
		t.Fatalf("ListByKey: %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d: %+v", len(entries), entries)
	}
	for _, e := range entries {
		if e.Key != "max_turns" {
			t.Errorf("entry has wrong key: %+v", e)
		}
	}

	// Every scope tier must be represented exactly once.
	seen := make(map[ScopeType]bool)
	for _, e := range entries {
		seen[e.ScopeType] = true
	}
	for _, want := range []ScopeType{ScopeSystem, ScopeProduct, ScopeManifest, ScopeTask} {
		if !seen[want] {
			t.Errorf("missing scope %q in ListByKey result", want)
		}
	}
}

func TestStore_ConcurrentWrites_DoNotRaceUnderWAL(t *testing.T) {
	s, _ := openStoreTestDB(t)
	ctx := context.Background()

	const (
		goroutines = 10
		perRoutine = 100
	)

	var wg sync.WaitGroup
	errCh := make(chan error, goroutines*perRoutine)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perRoutine; i++ {
				key := fmt.Sprintf("k-%d-%d", g, i)
				value := fmt.Sprintf("%d", g*perRoutine+i)
				if err := s.Set(ctx, ScopeProduct, "shared", key, value, "racer"); err != nil {
					errCh <- err
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent Set: %v", err)
	}

	entries, err := s.ListScope(ctx, ScopeProduct, "shared")
	if err != nil {
		t.Fatalf("ListScope: %v", err)
	}
	if got, want := len(entries), goroutines*perRoutine; got != want {
		t.Fatalf("ListScope returned %d entries, want %d", got, want)
	}
}
