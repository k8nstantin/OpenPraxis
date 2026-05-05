package settings

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	sqlite3 "github.com/mattn/go-sqlite3"
)

// Fake lookups —————————————————————————————————————————————————————————————

type fakeTaskLookup struct {
	tasks map[string]TaskRec
	err   error
}

func (f *fakeTaskLookup) GetTaskForSettings(_ context.Context, taskID string) (TaskRec, error) {
	if f.err != nil {
		return TaskRec{}, f.err
	}
	rec, ok := f.tasks[taskID]
	if !ok {
		return TaskRec{}, fmt.Errorf("fake task lookup: task %q not found", taskID)
	}
	return rec, nil
}

type fakeManifestLookup struct {
	manifests map[string]ManifestRec
	err       error
}

func (f *fakeManifestLookup) GetManifestForSettings(_ context.Context, manifestID string) (ManifestRec, error) {
	if f.err != nil {
		return ManifestRec{}, f.err
	}
	rec, ok := f.manifests[manifestID]
	if !ok {
		return ManifestRec{}, fmt.Errorf("fake manifest lookup: manifest %q not found", manifestID)
	}
	return rec, nil
}

// Test helper ——————————————————————————————————————————————————————————————

// openResolverTestDB mirrors openStoreTestDB from store_test.go but also wires
// a Resolver with fake lookups that map a single (task → manifest → product)
// chain. Tests that need different lookup behavior can mutate the returned
// fake maps before calling the resolver.
func openResolverTestDB(t *testing.T) (*Resolver, *Store, *fakeTaskLookup, *fakeManifestLookup) {
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

	store := NewStore(db)
	tasks := &fakeTaskLookup{tasks: map[string]TaskRec{
		"t1": {ID: "t1", ManifestID: "m1"},
	}}
	manifests := &fakeManifestLookup{manifests: map[string]ManifestRec{
		"m1": {ID: "m1", ProductID: "p1"},
	}}
	r := NewResolver(store, tasks, manifests)
	return r, store, tasks, manifests
}

// Resolve tests ————————————————————————————————————————————————————————————

func TestResolver_Resolve_UnknownKey_ReturnsErrUnknownKey(t *testing.T) {
	r, _, _, _ := openResolverTestDB(t)
	_, err := r.Resolve(context.Background(), Scope{TaskID: "t1"}, "no_such_knob")
	if !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("expected ErrUnknownKey, got %v", err)
	}
}

func TestResolver_Resolve_TaskLevelWins(t *testing.T) {
	r, store, _, _ := openResolverTestDB(t)
	ctx := context.Background()

	mustSet(t, store, ScopeProduct, "p1", "max_turns", "10")
	mustSet(t, store, ScopeManifest, "m1", "max_turns", "20")
	mustSet(t, store, ScopeTask, "t1", "max_turns", "30")

	got, err := r.Resolve(ctx, Scope{TaskID: "t1", ManifestID: "m1", ProductID: "p1"}, "max_turns")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Source != ScopeTask || got.SourceID != "t1" {
		t.Errorf("source = %s/%s, want task/t1", got.Source, got.SourceID)
	}
	if got.Value.(int64) != 30 {
		t.Errorf("value = %v, want 30", got.Value)
	}
}

func TestResolver_Resolve_FallsThroughToManifest(t *testing.T) {
	r, store, _, _ := openResolverTestDB(t)
	ctx := context.Background()

	mustSet(t, store, ScopeProduct, "p1", "max_turns", "10")
	mustSet(t, store, ScopeManifest, "m1", "max_turns", "20")
	// no task-scope entry

	got, err := r.Resolve(ctx, Scope{TaskID: "t1", ManifestID: "m1", ProductID: "p1"}, "max_turns")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Source != ScopeManifest || got.SourceID != "m1" {
		t.Errorf("source = %s/%s, want manifest/m1", got.Source, got.SourceID)
	}
	if got.Value.(int64) != 20 {
		t.Errorf("value = %v, want 20", got.Value)
	}
}

func TestResolver_Resolve_FallsThroughToProduct(t *testing.T) {
	r, store, _, _ := openResolverTestDB(t)
	ctx := context.Background()

	mustSet(t, store, ScopeProduct, "p1", "max_turns", "10")
	// no manifest- or task-scope entry

	got, err := r.Resolve(ctx, Scope{TaskID: "t1", ManifestID: "m1", ProductID: "p1"}, "max_turns")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Source != ScopeProduct || got.SourceID != "p1" {
		t.Errorf("source = %s/%s, want product/p1", got.Source, got.SourceID)
	}
	if got.Value.(int64) != 10 {
		t.Errorf("value = %v, want 10", got.Value)
	}
}

func TestResolver_Resolve_FallsThroughToSystem(t *testing.T) {
	r, _, _, _ := openResolverTestDB(t)
	ctx := context.Background()

	got, err := r.Resolve(ctx, Scope{TaskID: "t1", ManifestID: "m1", ProductID: "p1"}, "max_turns")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Source != ScopeSystem || got.SourceID != "" {
		t.Errorf("source = %s/%s, want system/<empty>", got.Source, got.SourceID)
	}
	// system default for max_turns is 3 per Catalog (catalog.go); cast to int.
	wantSystem, _ := SystemDefault("max_turns")
	if got.Value != wantSystem {
		t.Errorf("value = %v, want system default %v", got.Value, wantSystem)
	}
}

func TestResolver_Resolve_ReturnsCorrectProvenance(t *testing.T) {
	r, store, _, _ := openResolverTestDB(t)
	ctx := context.Background()

	mustSet(t, store, ScopeProduct, "p1", "max_turns", "10")
	mustSet(t, store, ScopeManifest, "m1", "max_turns", "20")

	cases := []struct {
		name       string
		scope      Scope
		wantSource ScopeType
		wantID     string
	}{
		{
			name:       "no task scope, manifest wins over product",
			scope:      Scope{TaskID: "t1", ManifestID: "m1", ProductID: "p1"},
			wantSource: ScopeManifest, wantID: "m1",
		},
		{
			name:       "no manifest scope override, manifest wins over product (manifest is set)",
			scope:      Scope{TaskID: "t1", ManifestID: "m1", ProductID: "p1"},
			wantSource: ScopeManifest, wantID: "m1",
		},
		{
			name:       "only product scope set explicitly (skips empty task/manifest IDs)",
			scope:      Scope{ProductID: "p1"},
			wantSource: ScopeProduct, wantID: "p1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := r.Resolve(ctx, tc.scope, "max_turns")
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if got.Source != tc.wantSource || got.SourceID != tc.wantID {
				t.Errorf("source = %s/%s, want %s/%s", got.Source, got.SourceID, tc.wantSource, tc.wantID)
			}
		})
	}
}

// Typed-decoding tests ——————————————————————————————————————————————————————

func TestResolver_Resolve_TypedDecoding_IntKnob(t *testing.T) {
	r, store, _, _ := openResolverTestDB(t)
	ctx := context.Background()
	mustSet(t, store, ScopeTask, "t1", "max_turns", "42")

	got, err := r.Resolve(ctx, Scope{TaskID: "t1"}, "max_turns")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	v, ok := got.Value.(int64)
	if !ok {
		t.Fatalf("value type = %T, want int64", got.Value)
	}
	if v != 42 {
		t.Errorf("value = %d, want 42", v)
	}
}

func TestResolver_Resolve_TypedDecoding_FloatKnob(t *testing.T) {
	r, store, _, _ := openResolverTestDB(t)
	ctx := context.Background()
	mustSet(t, store, ScopeTask, "t1", "temperature", "0.7")

	got, err := r.Resolve(ctx, Scope{TaskID: "t1"}, "temperature")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	v, ok := got.Value.(float64)
	if !ok {
		t.Fatalf("value type = %T, want float64", got.Value)
	}
	if v != 0.7 {
		t.Errorf("value = %v, want 0.7", v)
	}
}

func TestResolver_Resolve_TypedDecoding_EnumKnob(t *testing.T) {
	r, store, _, _ := openResolverTestDB(t)
	ctx := context.Background()
	mustSet(t, store, ScopeTask, "t1", "reasoning_effort", `"high"`)

	got, err := r.Resolve(ctx, Scope{TaskID: "t1"}, "reasoning_effort")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	v, ok := got.Value.(string)
	if !ok {
		t.Fatalf("value type = %T, want string", got.Value)
	}
	if v != "high" {
		t.Errorf("value = %q, want %q", v, "high")
	}
}

func TestResolver_Resolve_TypedDecoding_MultiselectKnob(t *testing.T) {
	r, store, _, _ := openResolverTestDB(t)
	ctx := context.Background()
	mustSet(t, store, ScopeTask, "t1", "allowed_tools", `["Bash","Read"]`)

	got, err := r.Resolve(ctx, Scope{TaskID: "t1"}, "allowed_tools")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	v, ok := got.Value.([]string)
	if !ok {
		t.Fatalf("value type = %T, want []string", got.Value)
	}
	if len(v) != 2 || v[0] != "Bash" || v[1] != "Read" {
		t.Errorf("value = %v, want [Bash Read]", v)
	}
}

func TestResolver_Resolve_TypedDecoding_StringKnob(t *testing.T) {
	r, store, _, _ := openResolverTestDB(t)
	ctx := context.Background()
	mustSet(t, store, ScopeTask, "t1", "default_model", `"gpt-5"`)

	got, err := r.Resolve(ctx, Scope{TaskID: "t1"}, "default_model")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	v, ok := got.Value.(string)
	if !ok {
		t.Fatalf("value type = %T, want string", got.Value)
	}
	if v != "gpt-5" {
		t.Errorf("value = %q, want %q", v, "gpt-5")
	}
}

// ResolveAll tests ——————————————————————————————————————————————————————————

func TestResolver_ResolveAll_IncludesEveryCatalogKnob(t *testing.T) {
	r, _, _, _ := openResolverTestDB(t)
	ctx := context.Background()

	out, err := r.ResolveAll(ctx, Scope{TaskID: "t1", ManifestID: "m1", ProductID: "p1"})
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}
	if len(out) != len(Catalog()) {
		t.Fatalf("ResolveAll returned %d entries, want %d", len(out), len(Catalog()))
	}
	for _, knob := range Catalog() {
		got, ok := out[knob.Key]
		if !ok {
			t.Errorf("key %q missing from ResolveAll output", knob.Key)
			continue
		}
		if got.Source != ScopeSystem {
			t.Errorf("key %q source = %s, want system (no overrides set)", knob.Key, got.Source)
		}
	}
}

func TestResolver_ResolveAll_ThreeQueriesRegardlessOfCatalogSize(t *testing.T) {
	store, counter := openCountingResolverDB(t)
	tasks := &fakeTaskLookup{tasks: map[string]TaskRec{
		"t1": {ID: "t1", ManifestID: "m1"},
	}}
	manifests := &fakeManifestLookup{manifests: map[string]ManifestRec{
		"m1": {ID: "m1", ProductID: "p1"},
	}}
	r := NewResolver(store, tasks, manifests)
	ctx := context.Background()

	// Pre-populate every tier with one entry so ResolveAll has reason to
	// query at every scope. Setup queries are intentionally NOT zeroed yet —
	// we reset the counter immediately before the call we want to measure.
	mustSet(t, store, ScopeProduct, "p1", "max_turns", "10")
	mustSet(t, store, ScopeManifest, "m1", "temperature", "0.5")
	mustSet(t, store, ScopeTask, "t1", "default_model", `"gpt-5"`)

	counter.Store(0)
	if _, err := r.ResolveAll(ctx, Scope{TaskID: "t1", ManifestID: "m1", ProductID: "p1"}); err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}
	if got := counter.Load(); got != 3 {
		t.Fatalf("ResolveAll issued %d DB queries, want exactly 3 (one per non-system tier)", got)
	}
}

func TestResolver_ResolveAll_MixedProvenance(t *testing.T) {
	r, store, _, _ := openResolverTestDB(t)
	ctx := context.Background()

	mustSet(t, store, ScopeTask, "t1", "max_turns", "30")
	mustSet(t, store, ScopeManifest, "m1", "temperature", "0.9")
	mustSet(t, store, ScopeProduct, "p1", "default_model", `"claude-sonnet-4-6"`)

	out, err := r.ResolveAll(ctx, Scope{TaskID: "t1", ManifestID: "m1", ProductID: "p1"})
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}

	cases := []struct {
		key        string
		wantSource ScopeType
		wantID     string
	}{
		{"max_turns", ScopeTask, "t1"},
		{"temperature", ScopeManifest, "m1"},
		{"default_model", ScopeProduct, "p1"},
		{"approval_mode", ScopeSystem, ""},
	}
	for _, tc := range cases {
		got := out[tc.key]
		if got.Source != tc.wantSource || got.SourceID != tc.wantID {
			t.Errorf("%s source = %s/%s, want %s/%s",
				tc.key, got.Source, got.SourceID, tc.wantSource, tc.wantID)
		}
	}
}

// NormalizeScope tests ——————————————————————————————————————————————————————

func TestResolver_NormalizeScope_FillsManifestFromTaskID(t *testing.T) {
	r, _, _, _ := openResolverTestDB(t)
	got, err := r.NormalizeScope(context.Background(), Scope{TaskID: "t1"})
	if err != nil {
		t.Fatalf("NormalizeScope: %v", err)
	}
	if got.ManifestID != "m1" {
		t.Errorf("ManifestID = %q, want m1", got.ManifestID)
	}
	if got.ProductID != "p1" {
		t.Errorf("ProductID = %q, want p1 (chained promotion)", got.ProductID)
	}
}

func TestResolver_NormalizeScope_FillsProductFromManifestID(t *testing.T) {
	r, _, _, _ := openResolverTestDB(t)
	got, err := r.NormalizeScope(context.Background(), Scope{ManifestID: "m1"})
	if err != nil {
		t.Fatalf("NormalizeScope: %v", err)
	}
	if got.ProductID != "p1" {
		t.Errorf("ProductID = %q, want p1", got.ProductID)
	}
	if got.TaskID != "" {
		t.Errorf("TaskID = %q, want unchanged empty", got.TaskID)
	}
}

func TestResolver_NormalizeScope_RespectsExplicitOverrides(t *testing.T) {
	r, _, _, _ := openResolverTestDB(t)
	in := Scope{TaskID: "t1", ManifestID: "different-m", ProductID: "different-p"}
	got, err := r.NormalizeScope(context.Background(), in)
	if err != nil {
		t.Fatalf("NormalizeScope: %v", err)
	}
	if got.ManifestID != "different-m" {
		t.Errorf("ManifestID overwritten: got %q, want %q", got.ManifestID, "different-m")
	}
	if got.ProductID != "different-p" {
		t.Errorf("ProductID overwritten: got %q, want %q", got.ProductID, "different-p")
	}
}

func TestResolver_NormalizeScope_LookupError_PropagatesUpward(t *testing.T) {
	r, _, tasks, _ := openResolverTestDB(t)
	tasks.err = errors.New("boom")

	_, err := r.NormalizeScope(context.Background(), Scope{TaskID: "t1"})
	if !errors.Is(err, ErrResolveLookupFailed) {
		t.Fatalf("expected ErrResolveLookupFailed, got %v", err)
	}
}

// Helpers ——————————————————————————————————————————————————————————————————

func mustSet(t *testing.T, s *Store, scope ScopeType, id, key, value string) {
	t.Helper()
	if err := s.Set(context.Background(), scope, id, key, value, "test"); err != nil {
		t.Fatalf("Set %s/%s/%s: %v", scope, id, key, err)
	}
}

// Counting driver wrapper ———————————————————————————————————————————————————
//
// Used only by TestResolver_ResolveAll_ThreeQueriesRegardlessOfCatalogSize
// to guard the "≤3 DB queries per ResolveAll" performance contract. The
// wrapper counts driver.Conn.Prepare(Context) calls — database/sql funnels
// every QueryContext / ExecContext through Prepare when the conn does NOT
// implement the QueryerContext / ExecerContext fast-paths, which is the
// case here because we deliberately omit those methods on countingConn.

var (
	countingDriverOnce sync.Once
	countingCounter    atomic.Int64
)

func openCountingResolverDB(t *testing.T) (*Store, *atomic.Int64) {
	t.Helper()
	countingDriverOnce.Do(func() {
		sql.Register("sqlite3_settings_counting", &countingDriver{inner: &sqlite3.SQLiteDriver{}})
	})

	dbPath := filepath.Join(t.TempDir(), "settings.db")
	db, err := sql.Open("sqlite3_settings_counting",
		dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open counting db: %v", err)
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

	return NewStore(db), &countingCounter
}

type countingDriver struct{ inner driver.Driver }

func (d *countingDriver) Open(name string) (driver.Conn, error) {
	c, err := d.inner.Open(name)
	if err != nil {
		return nil, err
	}
	return &countingConn{inner: c}, nil
}

type countingConn struct {
	inner driver.Conn
}

func (c *countingConn) Prepare(query string) (driver.Stmt, error) {
	countingCounter.Add(1)
	return c.inner.Prepare(query)
}

func (c *countingConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	countingCounter.Add(1)
	if cp, ok := c.inner.(driver.ConnPrepareContext); ok {
		return cp.PrepareContext(ctx, query)
	}
	return c.inner.Prepare(query)
}

func (c *countingConn) Begin() (driver.Tx, error) { return c.inner.Begin()}
func (c *countingConn) Close() error              { return c.inner.Close() }
