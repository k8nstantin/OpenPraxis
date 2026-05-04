package task

import (
	"time"
)

// RuntimeState represents a persisted snapshot of a running task.
// Preserved for the RecoverInFlight API; no longer backed by a DB table —
// the execution_log EventStarted row is the durable record.
type RuntimeState struct {
	TaskID    string    `json:"task_id"`
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

// SaveRuntimeState is a no-op. The execution_log EventStarted row is the
// durable crash-recovery record; task_runtime_state has been retired.
func (s *Store) SaveRuntimeState(taskID, title, manifest, agent string, pid int, paused bool, actions, lines int, lastLine string, startedAt time.Time) error {
	return nil
}

// UpdateRuntimeState is a no-op. task_runtime_state has been retired.
func (s *Store) UpdateRuntimeState(taskID string, actions, lines int, lastLine string, paused bool) error {
	return nil
}

// DeleteRuntimeState is a no-op. task_runtime_state has been retired.
func (s *Store) DeleteRuntimeState(taskID string) error {
	return nil
}

// ListRuntimeState returns an empty slice. task_runtime_state has been
// retired; RecoverInFlight falls through to listOrphanRunningTasks.
func (s *Store) ListRuntimeState() ([]RuntimeState, error) {
	return nil, nil
}

// ClearRuntimeState is a no-op. task_runtime_state has been retired.
func (s *Store) ClearRuntimeState() error {
	return nil
}

// listOrphanRunningTasks returns tasks left in running/paused status from
// a prior serve process. Used by RecoverInFlight to catch orphans.
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
func (s *Store) RecoverAsFailed(taskID, reason string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE tasks SET status = 'failed', last_output = ?, updated_at = ? WHERE id = ? AND status IN ('running', 'paused')`,
		reason, now, taskID)
	return err
}

// RecoverAsScheduled resets a running/paused task to scheduled with
// next_run_at=now so the scheduler picks it up on its next tick. Called
// by RecoverInFlight's restart branch.
func (s *Store) RecoverAsScheduled(taskID, reason string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE tasks SET status = 'scheduled', next_run_at = ?, last_output = ?, updated_at = ? WHERE id = ? AND status IN ('running', 'paused')`,
		now, reason, now, taskID)
	return err
}

// CleanupOrphaned marks any tasks stuck in "running" as "failed" on startup.
// task_runtime_state is retired; orphans are detected via task status only.
func (s *Store) CleanupOrphaned() {
	now := time.Now().UTC().Format(time.RFC3339)
	s.db.Exec(`UPDATE tasks SET status = 'failed', last_output = 'Process terminated — orphaned on restart', updated_at = ? WHERE status IN ('running', 'paused')`, now)
}
