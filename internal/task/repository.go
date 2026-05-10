package task

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/relationships"
)

// ManifestReadinessChecker is the hook task.Store consults to decide
// whether a new task's manifest is currently satisfied (i.e. every one
// of its manifest-level dependencies is in a terminal status). If a
// manifest is unsatisfied, tasks created against it are seeded in
// 'waiting' with a populated block_reason so the operator sees which
// manifest is blocking them.
//
// Implemented by manifest.Store; wired via an adapter in node.go.
// Optional — a nil checker skips the check, which preserves pre-fix
// behavior and keeps unit tests standalone.
type ManifestReadinessChecker interface {
	IsSatisfied(ctx context.Context, manifestID string) (ok bool, unsatisfied []string, err error)
}

// SetManifestChecker wires a readiness checker. Safe to call once at
// startup; there's no lock because the setter runs before any Create
// call from the HTTP/MCP surfaces.
func (s *Store) SetManifestChecker(c ManifestReadinessChecker) {
	s.manifestChecker = c
}

// Create is a no-op stub. The tasks table has been retired; all task
// data lives in the entities table. Returns an error to surface
// misconfigured callers.
func (s *Store) Create(manifestID, title, description, schedule, agent, sourceNode, createdBy, dependsOn string) (*Task, error) {
	return nil, fmt.Errorf("task.Store.Create: tasks table has been retired; use entity.Store instead")
}

// joinIDs formats a list of full UUIDs as comma-separated for inclusion
// in a block_reason. Empty input → "(unknown)" so the reason is never
// ambiguous-looking to the operator.
func joinIDs(ids []string) string {
	if len(ids) == 0 {
		return "(unknown)"
	}
	return strings.Join(ids, ", ")
}

// Get is a no-op stub. The tasks table has been retired.
func (s *Store) Get(id string) (*Task, error) {
	return nil, fmt.Errorf("task.Store.Get: tasks table has been retired; use entity.Store instead")
}

// ListByManifest returns tasks owned by a manifest via the relationships
// store. The tasks table has been retired; this reads ownership edges
// but cannot hydrate task rows.
func (s *Store) ListByManifest(manifestID string, limit int) ([]*Task, error) {
	return nil, nil
}

// List is a no-op stub. The tasks table has been retired.
func (s *Store) List(status string, limit int) ([]*Task, error) {
	return nil, nil
}

// Update is a no-op stub. The tasks table has been retired.
func (s *Store) Update(id string, title, description *string) (*Task, error) {
	return nil, fmt.Errorf("task.Store.Update: tasks table has been retired; use entity.Store instead")
}

// SetManifest is a no-op stub. The tasks table has been retired.
func (s *Store) SetManifest(taskID, manifestID string) (*Task, error) {
	return nil, fmt.Errorf("task.Store.SetManifest: tasks table has been retired")
}

// UpdateStatus is a no-op stub. The tasks table has been retired.
func (s *Store) UpdateStatus(id, status string) error {
	return nil
}

// Delete is a no-op stub. The tasks table has been retired.
func (s *Store) Delete(id string) error {
	return nil
}

// ListDeleted is a no-op stub. The tasks table has been retired.
func (s *Store) ListDeleted(limit int) ([]*Task, error) {
	return nil, nil
}

// Restore is a no-op stub. The tasks table has been retired.
func (s *Store) Restore(id string) error {
	return nil
}

// SetBlockReason is a no-op stub. The tasks table has been retired.
func (s *Store) SetBlockReason(id, reason string) error {
	return nil
}

// ErrTaskDepCycle is returned when SetDependency would create a cycle in
// the task dependency graph.
var ErrTaskDepCycle = fmt.Errorf("task dependency: cycle detected")

// ErrTaskDepSelfLoop is returned when a task is asked to depend on itself.
var ErrTaskDepSelfLoop = fmt.Errorf("task dependency: a task cannot depend on itself")

// SetDependency is a no-op stub. The tasks table has been retired.
func (s *Store) SetDependency(id, dependsOn string) error {
	return s.SetDependencyWithAudit(id, dependsOn, "", "")
}

// SetDependencyWithAudit is a no-op stub. The tasks table has been retired.
func (s *Store) SetDependencyWithAudit(id, dependsOn, changedBy, reason string) error {
	return nil
}

// writeSCDDepRow writes a dependency revision using the unified
// relationships SCD-2 store.
func (s *Store) writeSCDDepRow(taskID, dependsOn, now, changedBy, reason string) error {
	if s.rels == nil {
		return fmt.Errorf("task_dependency: relationships backend not wired")
	}
	ctx := context.Background()
	prior, err := s.rels.ListOutgoing(ctx, taskID, relationships.EdgeDependsOn)
	if err != nil {
		return fmt.Errorf("read prior dep: %w", err)
	}
	for _, p := range prior {
		if p.DstKind != relationships.KindTask {
			continue
		}
		if err := s.rels.Remove(ctx, p.SrcID, p.DstID,
			relationships.EdgeDependsOn, changedBy, "superseded by SetDependency"); err != nil {
			return fmt.Errorf("close prior dep edge: %w", err)
		}
	}
	if dependsOn == "" {
		return nil
	}
	if err := s.rels.Create(ctx, relationships.Edge{
		SrcKind:   relationships.KindTask,
		SrcID:     taskID,
		DstKind:   relationships.KindTask,
		DstID:     dependsOn,
		Kind:      relationships.EdgeDependsOn,
		CreatedBy: changedBy,
		Reason:    reason,
	}); err != nil {
		return fmt.Errorf("insert new dep edge: %w", err)
	}
	return nil
}

// DepHistoryEntry is one row of the SCD Type 2 task_dependency table.
type DepHistoryEntry struct {
	ID         int    `json:"id"`
	TaskID     string `json:"task_id"`
	DependsOn  string `json:"depends_on"`
	ValidFrom  string `json:"valid_from"`
	ValidTo    string `json:"valid_to"`
	ChangedBy  string `json:"changed_by"`
	Reason     string `json:"reason"`
}

// ListDepHistory returns every dep revision for a task from the unified
// relationships table.
func (s *Store) ListDepHistory(taskID string, limit int) ([]DepHistoryEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	if s.rels == nil {
		return nil, nil
	}
	rows, err := s.db.Query(
		`SELECT src_id, dst_id, valid_from, valid_to, created_by, reason
		 FROM relationships
		 WHERE src_id = ? AND src_kind = 'task' AND kind = 'depends_on'
		 ORDER BY valid_from DESC LIMIT ?`,
		taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DepHistoryEntry
	for rows.Next() {
		var e DepHistoryEntry
		if err := rows.Scan(&e.TaskID, &e.DependsOn,
			&e.ValidFrom, &e.ValidTo, &e.ChangedBy, &e.Reason); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// BackfillTaskDepSCD is a no-op. The tasks table has been retired;
// dependency history lives in the relationships table.
func (s *Store) BackfillTaskDepSCD() (int, error) {
	return 0, nil
}

// deriveDepState picks the status + block_reason for a task whose dep is
// being set.
func deriveDepState(currentStatus, parentStatus, parentID string) (string, string) {
	if currentStatus == string(StatusRunning) || currentStatus == string(StatusPaused) ||
		currentStatus == string(StatusCompleted) || currentStatus == string(StatusFailed) ||
		currentStatus == string(StatusCancelled) {
		return currentStatus, ""
	}
	if parentStatus == string(StatusCompleted) {
		return string(StatusScheduled), ""
	}
	return string(StatusWaiting), fmt.Sprintf("task %s not completed (is %s)",
		parentID, parentStatus)
}

// ActivateDependents is a no-op stub. The tasks table has been retired.
func (s *Store) ActivateDependents(completedTaskID string) (int, error) {
	return 0, nil
}

// TaskDep is the denormalised row the dashboard renders for both
// directions of /api/tasks/{id}/dependencies.
type TaskDep struct {
	ID     string `json:"id"`
	Marker string `json:"marker"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// ListDependents is a no-op stub. The tasks table has been retired.
func (s *Store) ListDependents(ctx context.Context, taskID string) ([]TaskDep, error) {
	return nil, nil
}

// RequestAction is a no-op stub. The tasks table has been retired.
func (s *Store) RequestAction(id, action string) error {
	return nil
}

// PendingActionRequest holds a task ID + requested action.
type PendingActionRequest struct {
	TaskID string
	Action string
}

// ListActionRequests is a no-op stub. The tasks table has been retired.
func (s *Store) ListActionRequests() ([]PendingActionRequest, error) {
	return nil, nil
}

// ClearActionRequest is a no-op stub. The tasks table has been retired.
func (s *Store) ClearActionRequest(id string) error {
	return nil
}

// PricingVersion stamps the pricing table identity used to compute cost_usd.
const PricingVersion = "v1-2026-04"

// updateTaskRunStats is a no-op. The tasks table has been retired.
func (s *Store) updateTaskRunStats(id, output, status string) {}

// RecordRun is a no-op stub retained for backward-compatibility.
func (s *Store) RecordRun(id, output, status string, actions, lines int, costUSD float64, turns int, startedAt time.Time, usage Usage, model string) (int64, error) {
	return 0, nil
}

// RecordHostMetrics is a no-op.
func (s *Store) RecordHostMetrics(runID int64, samples []HostMetricsSample, metrics HostMetrics) error {
	return nil
}

// ListHostSamples returns the time-series host samples for a run.
func (s *Store) ListHostSamples(runID int64) ([]HostMetricsSample, error) {
	rows, err := s.db.Query(
		`SELECT ts, cpu_pct, rss_mb, cost_usd, turns, actions, disk_used_gb, disk_total_gb
		 FROM task_run_host_samples
		 WHERE run_id = ? ORDER BY ts ASC`,
		runID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HostMetricsSample
	for rows.Next() {
		var tsStr string
		var smp HostMetricsSample
		if err := rows.Scan(&tsStr, &smp.CPUPct, &smp.RSSMB, &smp.CostUSD, &smp.Turns, &smp.Actions,
			&smp.DiskUsedGB, &smp.DiskTotalGB); err != nil {
			return nil, err
		}
		if t, err := time.Parse(time.RFC3339Nano, tsStr); err == nil {
			smp.TS = t
		}
		out = append(out, smp)
	}
	return out, rows.Err()
}

// ListRuns returns run history for a task, most recent first.
func (s *Store) ListRuns(taskID string, limit int) ([]TaskRun, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT `+taskRunsColumns+` FROM task_runs WHERE task_id = ? ORDER BY run_number DESC LIMIT ?`, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRuns(rows)
}

// GetRun returns a single task run by ID.
func (s *Store) GetRun(runID int) (*TaskRun, error) {
	rows, err := s.db.Query(`SELECT `+taskRunsColumns+` FROM task_runs WHERE id = ?`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs, err := scanRuns(rows)
	if err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return nil, nil
	}
	r := runs[0]
	return &r, nil
}

// ListAllRuns returns all task runs since the given time, ordered by started_at desc.
func (s *Store) ListAllRuns(since time.Time, limit int) ([]TaskRun, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := s.db.Query(`SELECT `+taskRunsColumns+` FROM task_runs WHERE started_at >= ? ORDER BY started_at DESC LIMIT ?`,
		since.UTC().Format(time.RFC3339), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRuns(rows)
}

// LinkManifest creates a manifest→task EdgeOwns edge in the relationships store.
func (s *Store) LinkManifest(taskID, manifestID string) error {
	if s.rels == nil {
		return fmt.Errorf("link manifest: relationships backend not wired")
	}
	ctx := context.Background()
	_, found, err := s.rels.Get(ctx, manifestID, taskID, relationships.EdgeOwns)
	if err != nil {
		return fmt.Errorf("link manifest: check existing edge: %w", err)
	}
	if found {
		return nil
	}
	return s.rels.Create(ctx, relationships.Edge{
		SrcKind:   relationships.KindManifest,
		SrcID:     manifestID,
		DstKind:   relationships.KindTask,
		DstID:     taskID,
		Kind:      relationships.EdgeOwns,
		CreatedBy: "task.Store.LinkManifest",
		Reason:    "task linked to manifest",
	})
}

// UnlinkManifest removes the manifest→task EdgeOwns edge.
func (s *Store) UnlinkManifest(taskID, manifestID string) error {
	if s.rels == nil {
		return fmt.Errorf("unlink manifest: relationships backend not wired")
	}
	return s.rels.Remove(context.Background(), manifestID, taskID,
		relationships.EdgeOwns, "task.Store.UnlinkManifest", "task unlinked from manifest")
}

// ListLinkedManifests returns manifest IDs that own a task.
func (s *Store) ListLinkedManifests(taskID string) ([]string, error) {
	if s.rels == nil {
		return nil, nil
	}
	edges, err := s.rels.ListIncoming(context.Background(), taskID, relationships.EdgeOwns)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range edges {
		if e.SrcKind == relationships.KindManifest {
			ids = append(ids, e.SrcID)
		}
	}
	return ids, nil
}

// ListTasksByLinkedManifest returns tasks owned by a manifest via the
// relationships store.
func (s *Store) ListTasksByLinkedManifest(manifestID string, limit int) ([]*Task, error) {
	return s.ListByManifest(manifestID, limit)
}
