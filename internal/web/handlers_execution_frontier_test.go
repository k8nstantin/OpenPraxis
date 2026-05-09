package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/k8nstantin/OpenPraxis/internal/execution"
	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/relationships"

	_ "github.com/mattn/go-sqlite3"
)

// newFrontierNode wires the minimum stores apiExecutionFrontier touches:
// execution_log + relationships, both backed by an isolated SQLite file.
// SettingsResolver is left nil so the handler exercises the
// resolver-missing fallback to defaultFrontierWindowDays.
func newFrontierNode(t *testing.T) *node.Node {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "frontier.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("set WAL: %v", err)
	}
	if err := execution.InitSchema(db); err != nil {
		t.Fatalf("execution InitSchema: %v", err)
	}
	rels, err := relationships.New(db)
	if err != nil {
		t.Fatalf("relationships.New: %v", err)
	}
	return &node.Node{
		ExecutionLog:  execution.NewStore(db),
		Relationships: rels,
	}
}

// seedTerminalRun appends one terminal execution_log row for entityID.
// reason picks the bucket: success / max_turns / timeout / "" (counts as
// failed via FrontierByManifest's residual classification).
func seedTerminalRun(t *testing.T, store *execution.Store, entityID, runUID, reason string, turns int, cost float64) {
	t.Helper()
	event := execution.EventCompleted
	if reason != execution.TerminalReasonSuccess && reason != "" {
		event = execution.EventFailed
	}
	row := execution.Row{
		RunUID:         runUID,
		EntityUID:      entityID,
		Event:          event,
		TerminalReason: reason,
		Turns:          turns,
		CostUSD:        cost,
		CreatedBy:      "test",
	}
	if err := store.Insert(context.Background(), row); err != nil {
		t.Fatalf("insert run %s: %v", runUID, err)
	}
}

func addOwnsEdge(t *testing.T, rels *relationships.Store, manifestID, taskID string) {
	t.Helper()
	if err := rels.Create(context.Background(), relationships.Edge{
		SrcKind: relationships.KindManifest,
		SrcID:   manifestID,
		DstKind: relationships.KindTask,
		DstID:   taskID,
		Kind:    relationships.EdgeOwns,
	}); err != nil {
		t.Fatalf("create owns edge: %v", err)
	}
}

func decodeFrontier(t *testing.T, rec *httptest.ResponseRecorder) frontierResponse {
	t.Helper()
	var got frontierResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	return got
}

func TestApiExecutionFrontier_MissingManifestID(t *testing.T) {
	n := newFrontierNode(t)
	rec := doGET(t, apiExecutionFrontier(n), "/api/execution/frontier")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestApiExecutionFrontier_NoTasks(t *testing.T) {
	n := newFrontierNode(t)
	rec := doGET(t, apiExecutionFrontier(n), "/api/execution/frontier?manifest_id=mf-empty")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	got := decodeFrontier(t, rec)
	if got.ManifestID != "mf-empty" {
		t.Fatalf("manifest_id mismatch: %q", got.ManifestID)
	}
	if got.WindowDays != defaultFrontierWindowDays {
		t.Fatalf("window_days: want %d, got %d", defaultFrontierWindowDays, got.WindowDays)
	}
	if len(got.Tasks) != 0 {
		t.Fatalf("tasks: want empty, got %+v", got.Tasks)
	}
	if got.Best != nil {
		t.Fatalf("best: want nil, got %+v", got.Best)
	}
	if got.AvgPassRate != 0 {
		t.Fatalf("avg_pass_rate: want 0, got %v", got.AvgPassRate)
	}
}

func TestApiExecutionFrontier_TaskWithNoRunsAbsent(t *testing.T) {
	n := newFrontierNode(t)
	addOwnsEdge(t, n.Relationships, "mf-1", "tk-runs")
	addOwnsEdge(t, n.Relationships, "mf-1", "tk-noruns")
	seedTerminalRun(t, n.ExecutionLog, "tk-runs", "run-1aaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", execution.TerminalReasonSuccess, 4, 0.10)

	rec := doGET(t, apiExecutionFrontier(n), "/api/execution/frontier?manifest_id=mf-1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	got := decodeFrontier(t, rec)
	if _, ok := got.Tasks["tk-noruns"]; ok {
		t.Fatalf("task with zero runs should be absent from tasks map: %+v", got.Tasks)
	}
	if _, ok := got.Tasks["tk-runs"]; !ok {
		t.Fatalf("task with runs missing: %+v", got.Tasks)
	}
	if got.AvgPassRate != 1.0 {
		t.Fatalf("avg_pass_rate: want 1.0, got %v", got.AvgPassRate)
	}
	if got.Best == nil || got.Best.TaskID != "tk-runs" || got.Best.PassRate != 1.0 {
		t.Fatalf("best mismatch: %+v", got.Best)
	}
}

func TestApiExecutionFrontier_BestAndAvgAcrossTasks(t *testing.T) {
	n := newFrontierNode(t)
	addOwnsEdge(t, n.Relationships, "mf-2", "tk-A")
	addOwnsEdge(t, n.Relationships, "mf-2", "tk-B")
	// tk-A: 2 success, 2 failure → 0.5
	seedTerminalRun(t, n.ExecutionLog, "tk-A", "run-a1aaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", execution.TerminalReasonSuccess, 5, 0.20)
	seedTerminalRun(t, n.ExecutionLog, "tk-A", "run-a2aaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", execution.TerminalReasonSuccess, 5, 0.20)
	seedTerminalRun(t, n.ExecutionLog, "tk-A", "run-a3aaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", execution.TerminalReasonMaxTurns, 50, 1.50)
	seedTerminalRun(t, n.ExecutionLog, "tk-A", "run-a4aaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", execution.TerminalReasonTimeout, 30, 0.80)
	// tk-B: 1 success → 1.0
	seedTerminalRun(t, n.ExecutionLog, "tk-B", "run-b1aaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", execution.TerminalReasonSuccess, 3, 0.05)

	rec := doGET(t, apiExecutionFrontier(n), "/api/execution/frontier?manifest_id=mf-2&window_days=7")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	got := decodeFrontier(t, rec)
	if got.WindowDays != 7 {
		t.Fatalf("window_days override ignored: %d", got.WindowDays)
	}
	if len(got.Tasks) != 2 {
		t.Fatalf("tasks: want 2, got %d (%+v)", len(got.Tasks), got.Tasks)
	}
	a, ok := got.Tasks["tk-A"]
	if !ok {
		t.Fatalf("tk-A missing")
	}
	if a.TotalRuns != 4 || a.SuccessRuns != 2 || a.MaxTurnsRuns != 1 || a.TimeoutRuns != 1 || a.FailedRuns != 0 {
		t.Fatalf("tk-A buckets wrong: %+v", a)
	}
	if a.PassRate != 0.5 {
		t.Fatalf("tk-A pass_rate: want 0.5, got %v", a.PassRate)
	}
	if got.Best == nil || got.Best.TaskID != "tk-B" || got.Best.PassRate != 1.0 {
		t.Fatalf("best should be tk-B@1.0: %+v", got.Best)
	}
	wantAvg := (0.5 + 1.0) / 2.0
	if got.AvgPassRate != wantAvg {
		t.Fatalf("avg_pass_rate: want %v, got %v", wantAvg, got.AvgPassRate)
	}
}
