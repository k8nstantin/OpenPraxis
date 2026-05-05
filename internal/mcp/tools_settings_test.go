package mcp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/k8nstantin/OpenPraxis/internal/settings"

	_ "github.com/mattn/go-sqlite3"
)

// -------- test harness -------------------------------------------------------

type fakeTaskLookup struct {
	tasks map[string]settings.TaskRec
}

func (f *fakeTaskLookup) GetTaskForSettings(_ context.Context, taskID string) (settings.TaskRec, error) {
	r, ok := f.tasks[taskID]
	if !ok {
		return settings.TaskRec{}, fmt.Errorf("fake task lookup: %q not found", taskID)
	}
	return r, nil
}

type fakeManifestLookup struct {
	manifests map[string]settings.ManifestRec
}

func (f *fakeManifestLookup) GetManifestForSettings(_ context.Context, manifestID string) (settings.ManifestRec, error) {
	r, ok := f.manifests[manifestID]
	if !ok {
		return settings.ManifestRec{}, fmt.Errorf("fake manifest lookup: %q not found", manifestID)
	}
	return r, nil
}

// settingsHarness wires a real settings.Store (in-memory sqlite) and a
// Resolver with fake lookups covering one task → manifest → product chain
// (t1 → m1 → p1). Individual tests mutate the fake maps or Store as needed.
type settingsHarness struct {
	db       *sql.DB
	store    *settings.Store
	resolver *settings.Resolver
	tasks    *fakeTaskLookup
	mans     *fakeManifestLookup
}

func newSettingsHarness(t *testing.T) *settingsHarness {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "settings.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
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
	if err := settings.InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	store := settings.NewStore(db)
	tasks := &fakeTaskLookup{tasks: map[string]settings.TaskRec{
		"t1": {ID: "t1", ManifestID: "m1"},
	}}
	mans := &fakeManifestLookup{manifests: map[string]settings.ManifestRec{
		"m1": {ID: "m1", ProductID: "p1"},
	}}
	return &settingsHarness{
		db:       db,
		store:    store,
		resolver: settings.NewResolver(store, tasks, mans),
		tasks:    tasks,
		mans:     mans,
	}
}

// noVisceralRules is the VisceralRuleLoader used when the test does not care
// about visceral clamping.
func noVisceralRules(_ context.Context) ([]string, error) { return nil, nil }

// -------- settings_catalog ---------------------------------------------------

func TestTool_SettingsCatalog_ReturnsAllKnobs(t *testing.T) {
	out := DoSettingsCatalog()
	if got, want := len(out.Knobs), len(settings.Catalog()); got != want {
		t.Fatalf("knob count: got %d want %d", got, want)
	}
	// Order must match catalog order — UIs depend on it.
	for i, k := range out.Knobs {
		if k.Key != settings.Catalog()[i].Key {
			t.Fatalf("knob[%d] key: got %q want %q", i, k.Key, settings.Catalog()[i].Key)
		}
	}
}

func TestTool_SettingsCatalog_EveryKnobHasRequiredFields(t *testing.T) {
	out := DoSettingsCatalog()
	for _, k := range out.Knobs {
		if k.Key == "" {
			t.Errorf("knob has empty key: %+v", k)
		}
		if k.Type == "" {
			t.Errorf("knob %q has empty type", k.Key)
		}
		if k.Description == "" {
			t.Errorf("knob %q has empty description", k.Key)
		}
		// Every knob must declare a default so the resolver can fall back.
		if _, ok := settings.SystemDefault(k.Key); !ok {
			t.Errorf("knob %q has no SystemDefault", k.Key)
		}
	}
}

// -------- settings_get -------------------------------------------------------

func TestTool_SettingsGet_ReturnsExplicitOnly(t *testing.T) {
	h := newSettingsHarness(t)
	ctx := context.Background()

	// Seed explicit entries at product + manifest scopes. settings_get at the
	// product scope must return ONLY the product row, not the manifest row.
	if err := h.store.Set(ctx, settings.ScopeProduct, "p1", "max_turns", "100", "seed"); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	if err := h.store.Set(ctx, settings.ScopeManifest, "m1", "max_turns", "200", "seed"); err != nil {
		t.Fatalf("seed manifest: %v", err)
	}

	out, err := DoSettingsGet(ctx, h.store, "product", "p1")
	if err != nil {
		t.Fatalf("DoSettingsGet: %v", err)
	}
	if len(out.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d: %+v", len(out.Entries), out.Entries)
	}
	if out.Entries[0].Value != "100" {
		t.Errorf("explicit value: got %q want %q", out.Entries[0].Value, "100")
	}
	if out.ScopeType != "product" || out.ScopeID != "p1" {
		t.Errorf("scope echoed incorrectly: %+v", out)
	}
}

func TestTool_SettingsGet_EmptyScope_ReturnsEmptyList(t *testing.T) {
	h := newSettingsHarness(t)
	out, err := DoSettingsGet(context.Background(), h.store, "task", "t-empty")
	if err != nil {
		t.Fatalf("DoSettingsGet: %v", err)
	}
	if len(out.Entries) != 0 {
		t.Fatalf("expected empty, got %+v", out.Entries)
	}
	// Must be an empty non-nil slice so JSON serializes to [] not null.
	if out.Entries == nil {
		t.Fatal("Entries is nil; expected empty slice")
	}
}

func TestTool_SettingsGet_UnknownScopeType_Returns400(t *testing.T) {
	h := newSettingsHarness(t)
	_, err := DoSettingsGet(context.Background(), h.store, "region", "us-east")
	if err == nil {
		t.Fatal("expected error for unknown scope_type, got nil")
	}
	if !strings.Contains(err.Error(), "scope_type") {
		t.Errorf("error should mention scope_type, got: %v", err)
	}
	// system scope is declared but not writable via the MCP surface.
	if _, err := DoSettingsGet(context.Background(), h.store, "system", "whatever"); err == nil {
		t.Fatal("expected error for system scope, got nil")
	}
}

// -------- settings_set -------------------------------------------------------

func TestTool_SettingsSet_ValidValue_Persists(t *testing.T) {
	h := newSettingsHarness(t)
	ctx := context.Background()

	out, err := DoSettingsSet(ctx, h.store, noVisceralRules,
		"product", "p1", "max_turns", "250", "mcp:sess-A")
	if err != nil {
		t.Fatalf("DoSettingsSet: %v", err)
	}
	if !out.OK {
		t.Fatalf("OK=false: %+v", out)
	}
	if out.Entry == nil || out.Entry.Value != "250" {
		t.Fatalf("readback entry wrong: %+v", out.Entry)
	}

	// Verify persistence with a direct store read.
	entry, err := h.store.Get(ctx, settings.ScopeProduct, "p1", "max_turns")
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	if entry.Value != "250" {
		t.Errorf("persisted value: got %q want %q", entry.Value, "250")
	}
}

func TestTool_SettingsSet_TypeMismatch_Returns400(t *testing.T) {
	h := newSettingsHarness(t)
	// max_turns is int; passing a JSON string must hard-fail.
	_, err := DoSettingsSet(context.Background(), h.store, noVisceralRules,
		"product", "p1", "max_turns", `"lots"`, "mcp:sess-A")
	if err == nil {
		t.Fatal("expected type mismatch error, got nil")
	}
	if !errors.Is(err, settings.ErrTypeMismatch) {
		t.Errorf("expected ErrTypeMismatch, got %v", err)
	}

	// Nothing should have been written.
	if _, gerr := h.store.Get(context.Background(), settings.ScopeProduct, "p1", "max_turns"); !errors.Is(gerr, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows after hard-fail; got %v", gerr)
	}
}

func TestTool_SettingsSet_UnknownKey_Returns400(t *testing.T) {
	h := newSettingsHarness(t)
	_, err := DoSettingsSet(context.Background(), h.store, noVisceralRules,
		"product", "p1", "no_such_knob", "42", "mcp:sess-A")
	if err == nil {
		t.Fatal("expected unknown-key error, got nil")
	}
	if !errors.Is(err, settings.ErrUnknownKey) {
		t.Errorf("expected ErrUnknownKey, got %v", err)
	}
}

func TestTool_SettingsSet_SliderOverRange_ReturnsWarningButPersists(t *testing.T) {
	h := newSettingsHarness(t)
	ctx := context.Background()

	// max_turns slider caps at 10000 per catalog. 99999 should warn, not block.
	out, err := DoSettingsSet(ctx, h.store, noVisceralRules,
		"product", "p1", "max_turns", "99999", "mcp:sess-A")
	if err != nil {
		t.Fatalf("DoSettingsSet: %v", err)
	}
	if !out.OK {
		t.Fatalf("OK=false: %+v", out)
	}
	if len(out.Warnings) == 0 {
		t.Fatalf("expected at least one warning, got none")
	}
	entry, err := h.store.Get(ctx, settings.ScopeProduct, "p1", "max_turns")
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	if entry.Value != "99999" {
		t.Errorf("persisted value: got %q want %q", entry.Value, "99999")
	}
}

func TestTool_SettingsSet_RecordsAuthor(t *testing.T) {
	h := newSettingsHarness(t)
	ctx := context.Background()

	author := "mcp:sess-deadbeef"
	if _, err := DoSettingsSet(ctx, h.store, noVisceralRules,
		"manifest", "m1", "temperature", "0.5", author); err != nil {
		t.Fatalf("DoSettingsSet: %v", err)
	}
	entry, err := h.store.Get(ctx, settings.ScopeManifest, "m1", "temperature")
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	if entry.UpdatedBy != author {
		t.Errorf("author: got %q want %q", entry.UpdatedBy, author)
	}
}

// -------- settings_resolve ---------------------------------------------------

func TestTool_SettingsResolve_ReturnsProvenanceForEveryKnob(t *testing.T) {
	h := newSettingsHarness(t)
	ctx := context.Background()

	// Seed one override at each scope tier so provenance is unambiguous.
	if err := h.store.Set(ctx, settings.ScopeTask, "t1", "max_turns", "7", "seed"); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if err := h.store.Set(ctx, settings.ScopeManifest, "m1", "temperature", "0.9", "seed"); err != nil {
		t.Fatalf("seed manifest: %v", err)
	}
	if err := h.store.Set(ctx, settings.ScopeProduct, "p1", "max_parallel", "8", "seed"); err != nil {
		t.Fatalf("seed product: %v", err)
	}

	out, err := DoSettingsResolve(ctx, h.resolver, "t1")
	if err != nil {
		t.Fatalf("DoSettingsResolve: %v", err)
	}
	if got, want := len(out.Resolved), len(settings.Catalog()); got != want {
		t.Fatalf("resolved count: got %d want %d", got, want)
	}

	assertSource := func(key string, src settings.ScopeType, srcID string) {
		t.Helper()
		r, ok := out.Resolved[key]
		if !ok {
			t.Fatalf("missing resolved entry for %q", key)
		}
		if r.Source != src || r.SourceID != srcID {
			t.Errorf("%s provenance: got source=%q id=%q want source=%q id=%q",
				key, r.Source, r.SourceID, src, srcID)
		}
	}

	assertSource("max_turns", settings.ScopeTask, "t1")
	assertSource("temperature", settings.ScopeManifest, "m1")
	assertSource("max_parallel", settings.ScopeProduct, "p1")

	// A knob with no override anywhere falls back to system.
	assertSource("approval_mode", settings.ScopeSystem, "")
}

func TestTool_SettingsResolve_UnknownTask_ReturnsError(t *testing.T) {
	h := newSettingsHarness(t)
	_, err := DoSettingsResolve(context.Background(), h.resolver, "t-does-not-exist")
	if err == nil {
		t.Fatal("expected resolver lookup error, got nil")
	}
}
