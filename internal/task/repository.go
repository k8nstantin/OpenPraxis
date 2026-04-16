package task

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Create stores a new task.
func (s *Store) Create(manifestID, title, description, schedule, agent, sourceNode, createdBy, dependsOn string, maxTurns int) (*Task, error) {
	if schedule == "" {
		schedule = "once"
	}
	if agent == "" {
		agent = "claude-code"
	}
	if maxTurns <= 0 {
		maxTurns = 100
	}

	id := uuid.Must(uuid.NewV7()).String()
	now := time.Now().UTC()

	_, err := s.db.Exec(`INSERT INTO tasks (id, manifest_id, title, description, schedule, status, agent, source_node, created_by, max_turns, depends_on, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'pending', ?, ?, ?, ?, ?, ?, ?)`,
		id, manifestID, title, description, schedule, agent, sourceNode, createdBy, maxTurns, dependsOn,
		now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}

	return &Task{
		ID: id, Marker: id[:12], ManifestID: manifestID,
		Title: title, Description: description, Schedule: schedule,
		Status: "pending", Agent: agent, SourceNode: sourceNode,
		CreatedBy: createdBy, MaxTurns: maxTurns, DependsOn: dependsOn, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// Get retrieves a task by ID or prefix.
func (s *Store) Get(id string) (*Task, error) {
	row := s.db.QueryRow(`SELECT `+taskColumns+` FROM tasks WHERE (id = ? OR id LIKE ?) AND deleted_at = ''`, id, id+"%")
	t, err := scanTask(row)
	if err == nil && t != nil {
		s.enrichWithCosts([]*Task{t})
	}
	return t, err
}

// ListByManifest returns tasks for a manifest.
func (s *Store) ListByManifest(manifestID string, limit int) ([]*Task, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT `+taskColumns+` FROM tasks WHERE (manifest_id = ? OR manifest_id LIKE ?) AND deleted_at = '' ORDER BY created_at DESC LIMIT ?`, manifestID, manifestID+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks, err := scanTasks(rows)
	if err == nil {
		s.enrichWithCosts(tasks)
	}
	return tasks, err
}

// List returns all tasks.
func (s *Store) List(status string, limit int) ([]*Task, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT ` + taskColumns + ` FROM tasks WHERE deleted_at = ''`
	var args []any
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY CASE status WHEN 'running' THEN 0 WHEN 'paused' THEN 0 WHEN 'scheduled' THEN 1 WHEN 'waiting' THEN 1 WHEN 'pending' THEN 2 WHEN 'completed' THEN 3 WHEN 'failed' THEN 4 WHEN 'cancelled' THEN 5 ELSE 6 END, updated_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks, err := scanTasks(rows)
	if err == nil {
		s.enrichWithCosts(tasks)
	}
	return tasks, err
}

// Update modifies optional fields on a task (title, description, max_turns).
// Only non-nil fields are updated.
func (s *Store) Update(id string, title, description *string, maxTurns *int) (*Task, error) {
	var sets []string
	var args []any
	now := time.Now().UTC().Format(time.RFC3339)

	if title != nil {
		sets = append(sets, "title = ?")
		args = append(args, *title)
	}
	if description != nil {
		sets = append(sets, "description = ?")
		args = append(args, *description)
	}
	if maxTurns != nil {
		sets = append(sets, "max_turns = ?")
		args = append(args, *maxTurns)
	}
	if len(sets) == 0 {
		return s.Get(id)
	}

	sets = append(sets, "updated_at = ?")
	args = append(args, now)
	args = append(args, id, id+"%")

	query := fmt.Sprintf("UPDATE tasks SET %s WHERE (id = ? OR id LIKE ?) AND deleted_at = ''", strings.Join(sets, ", "))
	_, err := s.db.Exec(query, args...)
	if err != nil {
		return nil, err
	}
	return s.Get(id)
}

// UpdateStatus changes a task's status.
func (s *Store) UpdateStatus(id, status string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE tasks SET status = ?, updated_at = ? WHERE id = ? OR id LIKE ?`, status, now, id, id+"%")
	return err
}

// Delete soft-deletes a task.
func (s *Store) Delete(id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE tasks SET deleted_at = ? WHERE (id = ? OR id LIKE ?) AND deleted_at = ''`, now, id, id+"%")
	return err
}

// ListDeleted returns soft-deleted tasks.
func (s *Store) ListDeleted(limit int) ([]*Task, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT `+taskColumns+` FROM tasks WHERE deleted_at != '' ORDER BY deleted_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTasks(rows)
}

// Restore un-deletes a soft-deleted task.
func (s *Store) Restore(id string) error {
	_, err := s.db.Exec(`UPDATE tasks SET deleted_at = '' WHERE (id = ? OR id LIKE ?) AND deleted_at != ''`, id, id+"%")
	return err
}

// SetBlockReason sets or clears the block_reason field for a task.
func (s *Store) SetBlockReason(id, reason string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE tasks SET block_reason = ?, updated_at = ? WHERE (id = ? OR id LIKE ?) AND deleted_at = ''`,
		reason, now, id, id+"%")
	return err
}

// SetDependency sets or clears the depends_on field for a task.
func (s *Store) SetDependency(id, dependsOn string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE tasks SET depends_on = ?, updated_at = ? WHERE (id = ? OR id LIKE ?) AND deleted_at = ''`,
		dependsOn, now, id, id+"%")
	return err
}

// ActivateDependents finds tasks that depend on the given task ID and schedules them.
func (s *Store) ActivateDependents(completedTaskID string) (int, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`UPDATE tasks SET status = 'scheduled', next_run_at = ?, updated_at = ?
		WHERE depends_on = ? AND status = 'waiting' AND deleted_at = ''`, now, now, completedTaskID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// RequestAction writes a cross-process action signal on the task row.
// The runner that owns the task's process (serve) polls this column and acts.
// action must be one of: "pause", "resume", "cancel".
func (s *Store) RequestAction(id, action string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE tasks SET action_request = ?, updated_at = ? WHERE (id = ? OR id LIKE ?) AND deleted_at = ''`,
		action, now, id, id+"%")
	return err
}

// PendingActionRequest holds a task ID + requested action.
type PendingActionRequest struct {
	TaskID string
	Action string
}

// ListActionRequests returns all tasks with a non-empty action_request.
// Used by the runner's watcher loop.
func (s *Store) ListActionRequests() ([]PendingActionRequest, error) {
	rows, err := s.db.Query(`SELECT id, action_request FROM tasks WHERE action_request != '' AND deleted_at = ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingActionRequest
	for rows.Next() {
		var p PendingActionRequest
		if err := rows.Scan(&p.TaskID, &p.Action); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// ClearActionRequest clears the action_request field after the runner has acted.
func (s *Store) ClearActionRequest(id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE tasks SET action_request = '', updated_at = ? WHERE id = ?`, now, id)
	return err
}

// RecordRun updates run stats after a task executes and saves to run history.
func (s *Store) RecordRun(id, output, status string, actions, lines int, costUSD float64, turns int, startedAt time.Time) error {
	now := time.Now().UTC().Format(time.RFC3339)
	// No truncation — capture full output
	_, err := s.db.Exec(`UPDATE tasks SET run_count = run_count + 1, last_run_at = ?, last_output = ?, status = ?, updated_at = ? WHERE id = ?`,
		now, output, status, now, id)
	if err != nil {
		return err
	}

	// Get current run_count for run_number
	var runCount int
	if err := s.db.QueryRow(`SELECT run_count FROM tasks WHERE id = ?`, id).Scan(&runCount); err != nil {
		slog.Warn("query run_count failed", "task_id", id, "error", err)
	}

	// Insert into task_runs history
	_, err = s.db.Exec(`INSERT INTO task_runs (task_id, run_number, output, status, actions, lines, cost_usd, turns, started_at, completed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, runCount, output, status, actions, lines, costUSD, turns, startedAt.UTC().Format(time.RFC3339), now)
	return err
}

// ListRuns returns run history for a task, most recent first.
func (s *Store) ListRuns(taskID string, limit int) ([]TaskRun, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id, task_id, run_number, output, status, actions, lines, cost_usd, turns, started_at, completed_at
		FROM task_runs WHERE task_id = ? ORDER BY run_number DESC LIMIT ?`, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRuns(rows)
}

// GetRun returns a single task run by ID.
func (s *Store) GetRun(runID int) (*TaskRun, error) {
	var r TaskRun
	var startedStr, completedStr string
	err := s.db.QueryRow(`SELECT id, task_id, run_number, output, status, actions, lines, cost_usd, turns, started_at, completed_at FROM task_runs WHERE id = ?`, runID).
		Scan(&r.ID, &r.TaskID, &r.RunNumber, &r.Output, &r.Status, &r.Actions, &r.Lines, &r.CostUSD, &r.Turns, &startedStr, &completedStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.StartedAt, _ = time.Parse(time.RFC3339, startedStr)
	r.CompletedAt, _ = time.Parse(time.RFC3339, completedStr)
	return &r, nil
}

// ListAllRuns returns all task runs since the given time, ordered by started_at desc.
func (s *Store) ListAllRuns(since time.Time, limit int) ([]TaskRun, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := s.db.Query(`SELECT id, task_id, run_number, output, status, actions, lines, cost_usd, turns, started_at, completed_at
		FROM task_runs WHERE started_at >= ? ORDER BY started_at DESC LIMIT ?`,
		since.UTC().Format(time.RFC3339), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRuns(rows)
}

// LinkManifest adds an association between a task and a manifest.
func (s *Store) LinkManifest(taskID, manifestID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`INSERT OR IGNORE INTO task_manifests (task_id, manifest_id, linked_at) VALUES (?, ?, ?)`,
		taskID, manifestID, now)
	return err
}

// UnlinkManifest removes an association between a task and a manifest.
func (s *Store) UnlinkManifest(taskID, manifestID string) error {
	_, err := s.db.Exec(`DELETE FROM task_manifests WHERE task_id = ? AND manifest_id = ?`, taskID, manifestID)
	return err
}

// ListLinkedManifests returns manifest IDs linked to a task via the link table.
func (s *Store) ListLinkedManifests(taskID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT manifest_id FROM task_manifests WHERE task_id = ? ORDER BY linked_at`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ListTasksByLinkedManifest returns tasks linked to a manifest via the link table.
func (s *Store) ListTasksByLinkedManifest(manifestID string, limit int) ([]*Task, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT t.id, t.manifest_id, t.title, t.description, t.schedule, t.status, t.agent, t.source_node, t.created_by, t.max_turns, t.depends_on, t.block_reason, t.run_count, t.last_run_at, t.next_run_at, t.last_output, t.created_at, t.updated_at
		FROM tasks t JOIN task_manifests tm ON t.id = tm.task_id
		WHERE tm.manifest_id = ? AND t.deleted_at = '' ORDER BY t.created_at DESC LIMIT ?`, manifestID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks, err := scanTasks(rows)
	if err == nil {
		s.enrichWithCosts(tasks)
	}
	return tasks, err
}
