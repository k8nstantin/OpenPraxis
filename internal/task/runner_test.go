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

	r := NewRunner(taskStore, nil, resolver, nil)
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
