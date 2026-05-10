package task

import "time"

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
// a prior serve process. The tasks table has been retired; returns empty.
func (s *Store) listOrphanRunningTasks() ([]*Task, error) {
	return nil, nil
}

// RecoverAsFailed is a no-op. The tasks table has been retired.
func (s *Store) RecoverAsFailed(taskID, reason string) error {
	return nil
}

// RecoverAsScheduled is a no-op. The tasks table has been retired.
func (s *Store) RecoverAsScheduled(taskID, reason string) error {
	return nil
}

// CleanupOrphaned is a no-op. The tasks table has been retired.
func (s *Store) CleanupOrphaned() {}
