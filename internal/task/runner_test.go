package task

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/k8nstantin/OpenPraxis/internal/relationships"
	"github.com/k8nstantin/OpenPraxis/internal/settings"
)

// Fake lookups — minimal stubs mirroring those in internal/settings/resolver_test.go.
// We can't reuse those directly (they're package-private to settings), so restate
// the smallest possible version here.

type fakeTaskLookup struct {
	tasks map[string]settings.TaskRec
}

func (f *fakeTaskLookup) GetTaskForSettings(_ context.Context, taskID string) (settings.TaskRec, error) {
	rec, ok := f.tasks[taskID]
	if !ok {
		// Unknown tasks are treated as standalone — zero manifest — so the
		// resolver falls through to the system default. This matches the
		// semantics the Runner's dispatch gate expects for ad-hoc tasks.
		return settings.TaskRec{ID: taskID, ManifestID: ""}, nil
	}
	return rec, nil
}

type fakeManifestLookup struct {
	manifests map[string]settings.ManifestRec
}

func (f *fakeManifestLookup) GetManifestForSettings(_ context.Context, manifestID string) (settings.ManifestRec, error) {
	rec, ok := f.manifests[manifestID]
	if !ok {
		return settings.ManifestRec{ID: manifestID, ProductID: ""}, nil
	}
	return rec, nil
}

// newRunnerHarness opens a sqlite DB with the settings schema applied, wires a
// settings.Resolver backed by fake lookups, and returns a Runner with that
// resolver. The task Store is also attached so Execute's downstream paths
// (status updates etc.) don't NPE — tests do not exercise those paths.
func newRunnerHarness(t *testing.T) (*Runner, *settings.Store, *fakeTaskLookup, *fakeManifestLookup) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "runner.db")
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

	if err := settings.InitSchema(db); err != nil {
		t.Fatalf("settings.InitSchema: %v", err)
	}

	settingsStore := settings.NewStore(db)
	tasks := &fakeTaskLookup{tasks: map[string]settings.TaskRec{}}
	manifests := &fakeManifestLookup{manifests: map[string]settings.ManifestRec{}}
	resolver := settings.NewResolver(settingsStore, tasks, manifests)

	taskStore, err := NewStore(db)
	if err != nil {
		t.Fatalf("task.NewStore: %v", err)
	}

	r := NewRunner(taskStore, nil, resolver, "", nil)
	return r, settingsStore, tasks, manifests
}

// addRunning pre-populates r.running with a placeholder entry scoped to a
// given product. Used to force the dispatch gate to fire without spawning an
// actual agent process. The zero cancel/cmd are fine — no code under test
// touches them for these checks.
func addRunning(r *Runner, taskID, productID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running[taskID] = &RunningTask{
		TaskID:    taskID,
		ProductID: productID,
		StartedAt: time.Now(),
	}
}

// setProductMaxParallel stores an explicit max_parallel value at product scope.
func setProductMaxParallel(t *testing.T, store *settings.Store, productID string, n int) {
	t.Helper()
	raw, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("marshal max_parallel: %v", err)
	}
	if err := store.Set(context.Background(), settings.ScopeProduct, productID, "max_parallel", string(raw), "test"); err != nil {
		t.Fatalf("store.Set product max_parallel: %v", err)
	}
}

// wireTask registers a task in the fake task lookup so NormalizeScope finds
// its manifest, which in turn resolves to a product via the manifest lookup.
// This is the same graph shape the real adapters build from the DB.
func wireTask(tasks *fakeTaskLookup, manifests *fakeManifestLookup, taskID, manifestID, productID string) {
	tasks.tasks[taskID] = settings.TaskRec{ID: taskID, ManifestID: manifestID}
	if manifestID != "" {
		manifests.manifests[manifestID] = settings.ManifestRec{ID: manifestID, ProductID: productID}
	}
}

// TestRunner_Execute_MaxParallelFromResolver —
// The per-product cap used by Execute comes from the settings resolver, not
// from a hardcoded field. Setting the cap via the Store changes the gate
// behavior on the very next Execute call, no restart required.
func TestRunner_Execute_MaxParallelFromResolver(t *testing.T) {
	r, store, tasks, manifests := newRunnerHarness(t)

	wireTask(tasks, manifests, "t-a1", "m-a", "prod-a")
	setProductMaxParallel(t, store, "prod-a", 2)

	// Pre-populate: two tasks already running under prod-a → at the cap.
	addRunning(r, "t-a0a", "prod-a")
	addRunning(r, "t-a0b", "prod-a")

	task := &Task{ID: "t-a1", Title: "cap test", ManifestID: "m-a"}
	err := r.Execute(task, "manifest A", "manifest content", "")
	if err == nil {
		t.Fatalf("Execute: want cap error, got nil")
	}
	want := "max parallel tasks reached for product prod-a (2)"
	if err.Error() != want {
		t.Fatalf("Execute error = %q, want %q", err.Error(), want)
	}
}

// TestRunner_Execute_MaxParallelPerProductIsolated —
// Two products with different caps do not bleed into each other. Tasks
// counted against product A must not affect product B's dispatch budget.
func TestRunner_Execute_MaxParallelPerProductIsolated(t *testing.T) {
	r, store, tasks, manifests := newRunnerHarness(t)

	wireTask(tasks, manifests, "t-a", "m-a", "prod-a")
	wireTask(tasks, manifests, "t-b", "m-b", "prod-b")
	setProductMaxParallel(t, store, "prod-a", 2)
	setProductMaxParallel(t, store, "prod-b", 5)

	// Fill prod-a to its cap; leave prod-b empty.
	addRunning(r, "t-a0", "prod-a")
	addRunning(r, "t-a1", "prod-a")

	if got := r.RunningCountForProduct("prod-a"); got != 2 {
		t.Fatalf("RunningCountForProduct(prod-a) = %d, want 2", got)
	}
	if got := r.RunningCountForProduct("prod-b"); got != 0 {
		t.Fatalf("RunningCountForProduct(prod-b) = %d, want 0", got)
	}

	// Product A has hit its cap — Execute for a task in A must reject.
	errA := r.Execute(&Task{ID: "t-a", Title: "A", ManifestID: "m-a"}, "", "", "")
	if errA == nil || !strings.Contains(errA.Error(), "prod-a") {
		t.Fatalf("Execute prod-a: want cap error containing prod-a, got %v", errA)
	}

	// Product B still has headroom — the gate resolves cap=5 and count=0, so
	// it does not reject here. We stop short of actually spawning the agent
	// by asserting the resolver's view directly.
	ctx := context.Background()
	scopeB, capB, err := r.resolveMaxParallel(ctx, "t-b")
	if err != nil {
		t.Fatalf("resolveMaxParallel(t-b): %v", err)
	}
	if scopeB.ProductID != "prod-b" {
		t.Fatalf("resolveMaxParallel(t-b) ProductID = %q, want prod-b", scopeB.ProductID)
	}
	if capB != 5 {
		t.Fatalf("resolveMaxParallel(t-b) cap = %d, want 5", capB)
	}
	if r.RunningCountForProduct(scopeB.ProductID) >= capB {
		t.Fatalf("prod-b bookkeeping says gate would reject (%d >= %d)", r.RunningCountForProduct(scopeB.ProductID), capB)
	}
}

// TestRunner_Execute_StandaloneTaskUsesSystemDefault —
// A task with no manifest/product resolves via the settings system default.
// The v1 catalog default for max_parallel is 3, so three concurrently-running
// standalone tasks saturate the pool and the next one is rejected.
func TestRunner_Execute_StandaloneTaskUsesSystemDefault(t *testing.T) {
	r, _, tasks, manifests := newRunnerHarness(t)

	// Deliberately no wireTask — the task lookup returns TaskRec with empty
	// ManifestID, which keeps ProductID empty.
	_ = tasks
	_ = manifests

	// System default for max_parallel is 3 (see internal/settings/catalog.go).
	addRunning(r, "s0", "")
	addRunning(r, "s1", "")
	addRunning(r, "s2", "")

	err := r.Execute(&Task{ID: "s3", Title: "standalone"}, "", "", "")
	if err == nil {
		t.Fatalf("Execute standalone: want cap error, got nil")
	}
	// Empty ProductID renders with %s as empty; the (3) group is the key
	// assertion that system default is in effect.
	if !strings.Contains(err.Error(), "(3)") {
		t.Fatalf("standalone error %q does not contain (3); want system-default cap", err.Error())
	}
	if !strings.Contains(err.Error(), "max parallel tasks reached for product") {
		t.Fatalf("standalone error %q missing per-product prefix", err.Error())
	}
}

// TestRunner_Execute_ExceedsProductCap_ReturnsError —
// Explicit check that the error message uses the per-product format spelled
// out in the M4-T11 spec: "max parallel tasks reached for product <id> (<n>)".
// Guards against regressing to the old "(%d)" one-size-fits-all message.
func TestRunner_Execute_ExceedsProductCap_ReturnsError(t *testing.T) {
	r, store, tasks, manifests := newRunnerHarness(t)

	wireTask(tasks, manifests, "t-x", "m-x", "prod-x")
	setProductMaxParallel(t, store, "prod-x", 1)

	addRunning(r, "existing", "prod-x")

	err := r.Execute(&Task{ID: "t-x", Title: "blocked", ManifestID: "m-x"}, "", "", "")
	if err == nil {
		t.Fatalf("Execute: want cap error, got nil")
	}
	want := fmt.Sprintf("max parallel tasks reached for product %s (%d)", "prod-x", 1)
	if err.Error() != want {
		t.Fatalf("Execute error = %q, want %q", err.Error(), want)
	}
}

// setProductKnob stores an explicit JSON-encoded value for any knob at
// product scope. Generalizes setProductMaxParallel so each M4-T12 test can
// pin arbitrary knobs without duplicating the marshal boilerplate.
func setProductKnob(t *testing.T, store *settings.Store, productID, key string, value interface{}) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %s: %v", key, err)
	}
	if err := store.Set(context.Background(), settings.ScopeProduct, productID, key, string(raw), "test"); err != nil {
		t.Fatalf("store.Set product %s: %v", key, err)
	}
}


// insertTaskRow creates a real tasks row + the EdgeOwns(manifest →
// task) row that ownership-aware queries (SumCostSince, etc.) join
// against. PR/M3 dropped the legacy tasks.manifest_id column; tests
// must wire ownership through the relationships store now.
func insertTaskRow(t *testing.T, db *sql.DB, taskID, manifestID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`INSERT INTO tasks (id, title, description, schedule, status, agent, source_node, created_by, created_at, updated_at)
		VALUES (?, 'test task', '', 'once', 'pending', 'claude-code', '', 'test', ?, ?)`,
		taskID, now, now)
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}
	if manifestID != "" {
		rels, err := relationships.New(db)
		if err != nil {
			t.Fatalf("init rels for owns edge: %v", err)
		}
		if err := rels.Create(context.Background(), relationships.Edge{
			SrcKind: relationships.KindManifest, SrcID: manifestID,
			DstKind: relationships.KindTask, DstID: taskID,
			Kind: relationships.EdgeOwns, CreatedBy: "test", Reason: "test fixture",
		}); err != nil {
			t.Fatalf("write owns edge: %v", err)
		}
	}
}

// insertManifestRow creates an EdgeOwns(product → manifest) row in the
// relationships store. The legacy manifests.project_id column was
// dropped in PR/M3; ownership-aware joins (SumCostSince, etc.) read
// from relationships.
func insertManifestRow(t *testing.T, db *sql.DB, manifestID, productID string) {
	t.Helper()
	rels, err := relationships.New(db)
	if err != nil {
		t.Fatalf("init rels: %v", err)
	}
	if err := rels.Create(context.Background(), relationships.Edge{
		SrcKind: relationships.KindProduct, SrcID: productID,
		DstKind: relationships.KindManifest, DstID: manifestID,
		Kind: relationships.EdgeOwns, CreatedBy: "test", Reason: "test fixture",
	}); err != nil {
		t.Fatalf("write owns edge: %v", err)
	}
}


// TestRunner_Execute_UsesResolvedMaxTurns —
// The resolver snapshot that feeds Execute decodes max_turns from the
// product-scope row. Asserted against the decoded runtimeKnobs struct rather
// than the raw resolver output so any type-coercion regressions surface here.
func TestRunner_Execute_UsesResolvedMaxTurns(t *testing.T) {
	r, store, tasks, manifests := newRunnerHarness(t)

	wireTask(tasks, manifests, "t-mt", "m-mt", "prod-mt")
	setProductKnob(t, store, "prod-mt", "max_turns", 237)

	ctx := context.Background()
	scope, _, err := r.resolveMaxParallel(ctx, "t-mt")
	if err != nil {
		t.Fatalf("resolveMaxParallel: %v", err)
	}
	all, err := r.resolver.ResolveAll(ctx, scope)
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}
	knobs, err := decodeRuntimeKnobs(all)
	if err != nil {
		t.Fatalf("decodeRuntimeKnobs: %v", err)
	}
	if knobs.MaxTurns != 237 {
		t.Fatalf("max_turns = %d, want 237", knobs.MaxTurns)
	}
	// Sanity: the source field must point at the product tier we set.
	if all["max_turns"].Source != settings.ScopeProduct {
		t.Fatalf("max_turns source = %s, want product", all["max_turns"].Source)
	}
}

// TestRunner_Execute_UsesResolvedTimeout —
// Mirrors TestRunner_Execute_UsesResolvedMaxTurns for timeout_minutes. The
// Execute wiring converts this int into time.Duration via * time.Minute;
// verified in code (no hardcoded 30min literal remains).
func TestRunner_Execute_UsesResolvedTimeout(t *testing.T) {
	r, store, tasks, manifests := newRunnerHarness(t)

	wireTask(tasks, manifests, "t-to", "m-to", "prod-to")
	setProductKnob(t, store, "prod-to", "timeout_minutes", 15)

	ctx := context.Background()
	scope, _, err := r.resolveMaxParallel(ctx, "t-to")
	if err != nil {
		t.Fatalf("resolveMaxParallel: %v", err)
	}
	all, err := r.resolver.ResolveAll(ctx, scope)
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}
	knobs, err := decodeRuntimeKnobs(all)
	if err != nil {
		t.Fatalf("decodeRuntimeKnobs: %v", err)
	}
	if knobs.TimeoutMinutes != 15 {
		t.Fatalf("timeout_minutes = %d, want 15", knobs.TimeoutMinutes)
	}
	// Ensure the conversion to Duration still produces the expected wall-clock.
	d := time.Duration(knobs.TimeoutMinutes) * time.Minute
	if d != 15*time.Minute {
		t.Fatalf("timeout duration = %v, want 15m", d)
	}
}

// TestRunner_Execute_PerTaskAgentOverridesResolved —
// The t.Agent column still wins over the resolver default. Tested via the
// chooseAgent helper so the override rule is exercised without spawning a
// process; Execute's only use of it is one line referencing the same helper.
func TestRunner_Execute_PerTaskAgentOverridesResolved(t *testing.T) {
	cases := []struct {
		name, taskAgent, resolved, want string
	}{
		{"override wins", "codex", "claude-code", "codex"},
		{"fallback when blank", "", "claude-code", "claude-code"},
		{"both blank is allowed", "", "", ""},
		{"override beats another non-default", "cursor", "windsurf", "cursor"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := chooseAgent(tc.taskAgent, tc.resolved)
			if got != tc.want {
				t.Fatalf("chooseAgent(%q,%q) = %q, want %q", tc.taskAgent, tc.resolved, got, tc.want)
			}
		})
	}
}



// TestRunner_Retry_TransientFailure_Requeues —
// shouldRetry returns true for each reason isTransientFailure classifies as
// transient, provided the attempts budget has headroom and the cap is > 0.
func TestRunner_Retry_TransientFailure_Requeues(t *testing.T) {
	transient := []string{"timeout", "build_fail", "process_error", "unknown_reason"}
	for _, reason := range transient {
		t.Run(reason, func(t *testing.T) {
			if !shouldRetry("failed", reason, 0, 3) {
				t.Fatalf("shouldRetry(failed, %q, 0, 3) = false, want true", reason)
			}
		})
	}
}

// TestRunner_Retry_NonTransientFailure_DoesNotRequeue —
// Reasons the runner classifies as non-transient (max_turns, deliverable_missing,
// cost_cap, daily_budget) never trigger retry regardless of the cap.
func TestRunner_Retry_NonTransientFailure_DoesNotRequeue(t *testing.T) {
	nonTransient := []string{"max_turns", "deliverable_missing"}
	for _, reason := range nonTransient {
		t.Run(reason, func(t *testing.T) {
			if shouldRetry("failed", reason, 0, 5) {
				t.Fatalf("shouldRetry(failed, %q, 0, 5) = true, want false", reason)
			}
		})
	}
}

// TestRunner_Retry_ExhaustedCount_DoesNotRequeue —
// Attempts equal to or greater than the cap short-circuit retry even when
// the reason is transient. Also verifies the zero-cap case.
func TestRunner_Retry_ExhaustedCount_DoesNotRequeue(t *testing.T) {
	cases := []struct {
		name     string
		attempts int
		cap      int
		want     bool
	}{
		{"zero cap", 0, 0, false},
		{"at cap", 3, 3, false},
		{"above cap", 5, 3, false},
		{"headroom", 2, 3, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldRetry("failed", "timeout", tc.attempts, tc.cap)
			if got != tc.want {
				t.Fatalf("shouldRetry(failed, timeout, %d, %d) = %v, want %v", tc.attempts, tc.cap, got, tc.want)
			}
		})
	}
}

// TestResolvedHelpers_Float_Str_StrSlice —
// Unit tests for the three new resolved-value coercion helpers added
// alongside resolvedInt. Mirrors resolvedInt's defensive-coercion discipline:
// accept the canonical type AND the neighbours (float64 ↔ int64 etc.) that
// the resolver may return depending on whether a knob was explicit-storage
// decoded or falls through to a system default.
func TestResolvedHelpers_Float_Str_StrSlice(t *testing.T) {
	t.Run("resolvedFloat", func(t *testing.T) {
		cases := []struct {
			in   interface{}
			want float64
			ok   bool
		}{
			{float64(1.5), 1.5, true},
			{float32(2.5), 2.5, true},
			{int(3), 3.0, true},
			{int64(4), 4.0, true},
			{"bad", 0, false},
			{nil, 0, false},
		}
		for _, tc := range cases {
			got, err := resolvedFloat(tc.in)
			if tc.ok && err != nil {
				t.Errorf("resolvedFloat(%v) err = %v, want ok", tc.in, err)
			}
			if !tc.ok && err == nil {
				t.Errorf("resolvedFloat(%v) = %v, want error", tc.in, got)
			}
			if tc.ok && got != tc.want {
				t.Errorf("resolvedFloat(%v) = %v, want %v", tc.in, got, tc.want)
			}
		}
	})

	t.Run("resolvedStr", func(t *testing.T) {
		s, err := resolvedStr("claude-code")
		if err != nil || s != "claude-code" {
			t.Errorf("resolvedStr(\"claude-code\") = %q, %v; want \"claude-code\", nil", s, err)
		}
		s, err = resolvedStr(nil)
		if err != nil || s != "" {
			t.Errorf("resolvedStr(nil) = %q, %v; want empty, nil", s, err)
		}
		if _, err := resolvedStr(42); err == nil {
			t.Errorf("resolvedStr(42) err = nil, want type error")
		}
	})

	t.Run("resolvedStrSlice", func(t *testing.T) {
		got, err := resolvedStrSlice([]string{"Bash", "Read"})
		if err != nil || len(got) != 2 || got[0] != "Bash" {
			t.Errorf("[]string path: got %v, %v; want [Bash Read], nil", got, err)
		}

		got, err = resolvedStrSlice([]interface{}{"A", "B"})
		if err != nil || len(got) != 2 || got[1] != "B" {
			t.Errorf("[]interface{} path: got %v, %v", got, err)
		}

		got, err = resolvedStrSlice(nil)
		if err != nil || got != nil {
			t.Errorf("nil path: got %v, %v", got, err)
		}

		if _, err := resolvedStrSlice([]interface{}{1, 2}); err == nil {
			t.Errorf("mixed-type slice: err = nil, want type error")
		}
	})
}
