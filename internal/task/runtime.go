package task

import (
	"fmt"
	"log/slog"
	"time"
)

// RuntimeState represents a persisted snapshot of a running task.
type RuntimeState struct {
	TaskID    string    `json:"task_id"`
	Marker    string    `json:"marker"`
	Title     string    `json:"title"`
	Manifest  string    `json:"manifest"`
	Agent     string    `json:"agent"`
	PID       int       `json:"pid"`
	Paused    bool      `json:"paused"`
	Actions   int       `json:"actions"`
	Lines     int       `json:"lines"`
	LastLine  string    `json:"last_line"`
	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SaveRuntimeState persists a running task's state to SQLite.
func (s *Store) SaveRuntimeState(taskID, marker, title, manifest, agent string, pid int, paused bool, actions, lines int, lastLine string, startedAt time.Time) error {
	now := time.Now().UTC().Format(time.RFC3339)
	pausedInt := 0
	if paused {
		pausedInt = 1
	}
	_, err := s.db.Exec(`INSERT OR REPLACE INTO task_runtime_state (task_id, marker, title, manifest, agent, pid, paused, actions, lines, last_line, started_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		taskID, marker, title, manifest, agent, pid, pausedInt, actions, lines, lastLine,
		startedAt.UTC().Format(time.RFC3339), now)
	return err
}

// UpdateRuntimeState updates action/line counts and last line for a running task.
func (s *Store) UpdateRuntimeState(taskID string, actions, lines int, lastLine string, paused bool) error {
	now := time.Now().UTC().Format(time.RFC3339)
	pausedInt := 0
	if paused {
		pausedInt = 1
	}
	_, err := s.db.Exec(`UPDATE task_runtime_state SET actions = ?, lines = ?, last_line = ?, paused = ?, updated_at = ? WHERE task_id = ?`,
		actions, lines, lastLine, pausedInt, now, taskID)
	return err
}

// DeleteRuntimeState removes a task's runtime state (called on completion).
func (s *Store) DeleteRuntimeState(taskID string) error {
	_, err := s.db.Exec(`DELETE FROM task_runtime_state WHERE task_id = ?`, taskID)
	return err
}

// ListRuntimeState returns all persisted runtime states (for recovery on startup).
func (s *Store) ListRuntimeState() ([]RuntimeState, error) {
	rows, err := s.db.Query(`SELECT task_id, marker, title, manifest, agent, pid, paused, actions, lines, last_line, started_at, updated_at FROM task_runtime_state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []RuntimeState
	for rows.Next() {
		var rs RuntimeState
		var startedStr, updatedStr string
		var paused int
		if err := rows.Scan(&rs.TaskID, &rs.Marker, &rs.Title, &rs.Manifest, &rs.Agent, &rs.PID, &paused, &rs.Actions, &rs.Lines, &rs.LastLine, &startedStr, &updatedStr); err != nil {
			return nil, err
		}
		rs.Paused = paused != 0
		rs.StartedAt, _ = time.Parse(time.RFC3339, startedStr)
		rs.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
		result = append(result, rs)
	}
	return result, rows.Err()
}

// ClearRuntimeState drops every task_runtime_state row. Used by the
// RC/M5 RecoverInFlight path after it has classified each orphan — a
// restarted task will re-save its own state when it fires again.
func (s *Store) ClearRuntimeState() error {
	_, err := s.db.Exec(`DELETE FROM task_runtime_state`)
	return err
}

// listOrphanRunningTasks returns tasks left in running/paused status from
// a prior serve process. Used by RecoverInFlight to catch orphans whose
// task_runtime_state row never got written (historical crash path).
func (s *Store) listOrphanRunningTasks() ([]*Task, error) {
	rows, err := s.db.Query(`SELECT ` + taskColumns + ` FROM tasks WHERE status IN ('running', 'paused') AND deleted_at = ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTasks(rows)
}

// RecoverAsFailed marks a task failed with last_output carrying the
// failure reason. Called by RecoverInFlight's stop / fail branches.
// Uses last_output (not a dedicated failure_reason column) so the
// dashboard's existing "last output" surface renders it without a
// schema change.
func (s *Store) RecoverAsFailed(taskID, reason string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE tasks SET status = 'failed', last_output = ?, updated_at = ? WHERE id = ? AND status IN ('running', 'paused')`,
		reason, now, taskID)
	return err
}

// RecoverAsScheduled resets a running/paused task to scheduled with
// next_run_at=now so the scheduler picks it up on its next tick. Called
// by RecoverInFlight's restart branch. last_output records why the reset
// happened so the dashboard shows the operator what the runner did.
func (s *Store) RecoverAsScheduled(taskID, reason string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE tasks SET status = 'scheduled', next_run_at = ?, last_output = ?, updated_at = ? WHERE id = ? AND status IN ('running', 'paused')`,
		now, reason, now, taskID)
	return err
}

// CleanupOrphaned marks any tasks stuck in "running" as "failed" on startup.
func (s *Store) CleanupOrphaned() {
	now := time.Now().UTC().Format(time.RFC3339)

	// Get runtime state of orphaned tasks for better error messages
	rows, err := s.db.Query(`SELECT task_id, marker, pid, actions, lines, started_at FROM task_runtime_state`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var taskID, marker, startedAt string
			var pid, actions, lines int
			if err := rows.Scan(&taskID, &marker, &pid, &actions, &lines, &startedAt); err == nil {
				msg := fmt.Sprintf("Process terminated — orphaned on restart (PID %d, %d actions, %d lines, started %s)", pid, actions, lines, startedAt)
				s.db.Exec(`UPDATE tasks SET status = 'failed', last_output = ?, updated_at = ? WHERE id = ? AND status IN ('running', 'paused')`, msg, now, taskID)
				slog.Info("cleanup orphaned task", "marker", marker, "pid", pid, "actions", actions)
			}
		}
	}

	// Catch any remaining running/paused tasks without runtime state
	s.db.Exec(`UPDATE tasks SET status = 'failed', last_output = 'Process terminated — orphaned on restart', updated_at = ? WHERE status IN ('running', 'paused')`, now)

	// Clear all runtime state — we're starting fresh
	s.db.Exec(`DELETE FROM task_runtime_state`)
}
