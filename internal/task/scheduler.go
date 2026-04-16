package task

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// ManifestDepChecker checks if a manifest's dependencies are satisfied.
// Implemented by node.Node to avoid circular imports.
type ManifestDepChecker interface {
	// CheckManifestDeps returns true if all dependency manifests are closed/archive.
	// Returns (satisfied bool, blockReason string).
	CheckManifestDeps(manifestID string) (bool, string)
}

// Scheduler checks for due tasks and fires them on a timer.
type Scheduler struct {
	store    *Store
	interval time.Duration
	stopCh   chan struct{}
	onFire   func(t *Task) // callback when a task fires
	depCheck ManifestDepChecker
}

// NewScheduler creates a task scheduler.
func NewScheduler(store *Store, checkInterval time.Duration, onFire func(t *Task), depCheck ManifestDepChecker) *Scheduler {
	return &Scheduler{
		store:    store,
		interval: checkInterval,
		stopCh:   make(chan struct{}),
		onFire:   onFire,
		depCheck: depCheck,
	}
}

// Start begins the scheduler loop.
func (s *Scheduler) Start() {
	go func() {
		// Initial check after short delay
		time.Sleep(2 * time.Second)
		s.check()

		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.check()
			case <-s.stopCh:
				return
			}
		}
	}()
}

// Stop halts the scheduler.
func (s *Scheduler) Stop() {
	close(s.stopCh)
}

func (s *Scheduler) check() {
	tasks, err := s.store.ListDue()
	if err != nil {
		slog.Error("list due tasks failed", "component", "scheduler", "error", err)
		return
	}

	for _, t := range tasks {
		// Check manifest dependency blocking
		if t.ManifestID != "" && s.depCheck != nil {
			satisfied, reason := s.depCheck.CheckManifestDeps(t.ManifestID)
			if !satisfied {
				slog.Info("task blocked by manifest dependency", "component", "scheduler", "marker", t.Marker, "reason", reason)
				// Put task back to waiting status so it doesn't re-fire every tick
				if err := s.store.UpdateStatus(t.ID, "waiting"); err != nil {
					slog.Error("update status to waiting failed", "component", "scheduler", "marker", t.Marker, "error", err)
				}
				if err := s.store.SetBlockReason(t.ID, reason); err != nil {
					slog.Error("set block reason failed", "component", "scheduler", "marker", t.Marker, "error", err)
				}
				continue
			}
		}

		slog.Info("firing task", "component", "scheduler", "marker", t.Marker, "title", t.Title, "schedule", t.Schedule)

		// Clear any previous block reason
		if err := s.store.SetBlockReason(t.ID, ""); err != nil {
			slog.Error("clear block reason failed", "component", "scheduler", "marker", t.Marker, "error", err)
		}

		// Mark as running
		if err := s.store.UpdateStatus(t.ID, "running"); err != nil {
			slog.Error("update status to running failed", "component", "scheduler", "marker", t.Marker, "error", err)
		}

		// Fire the callback
		if s.onFire != nil {
			go s.onFire(t)
		}

		// Compute next run for recurring tasks
		if t.Schedule != "once" {
			nextRun := ComputeNextRun(t.Schedule)
			if !nextRun.IsZero() {
				if err := s.store.SetNextRun(t.ID, nextRun.Format(time.RFC3339)); err != nil {
					slog.Error("set next run failed", "component", "scheduler", "marker", t.Marker, "error", err)
				}
			}
		}
	}
}

// ComputeNextRun parses a schedule string and returns the next fire time from now.
// Supports: "once", "in:30m" (one-shot delay), "at:ISO8601" (one-shot absolute), "5m", "1h" (recurring)
func ComputeNextRun(schedule string) time.Time {
	schedule = strings.TrimSpace(schedule)
	lower := strings.ToLower(schedule)
	if lower == "once" || lower == "" {
		return time.Time{}
	}

	// Absolute time: "at:2026-04-11T15:00:00Z" — fires once at that time
	if strings.HasPrefix(lower, "at:") {
		ts := schedule[3:]
		t, err := time.Parse(time.RFC3339, ts)
		if err == nil {
			return t
		}
		// Try without timezone
		t, err = time.Parse("2006-01-02T15:04:05", ts)
		if err == nil {
			return t.UTC()
		}
		return time.Time{}
	}

	// One-shot delay: "in:30m", "in:1h", "in:5s" — fires once after delay
	if strings.HasPrefix(lower, "in:") {
		return parseDuration(lower[3:])
	}

	// Recurring duration-style schedules: "5m", "1h", "30s", "24h"
	return parseDuration(lower)
}

// parseDuration parses "30m", "1h", "5s", "24h", "7d" into a time from now.
func parseDuration(s string) time.Time {
	if len(s) < 2 {
		return time.Time{}
	}
	unit := s[len(s)-1]
	numStr := s[:len(s)-1]
	num, err := strconv.Atoi(numStr)
	if err == nil && num > 0 {
		switch unit {
		case 's':
			return time.Now().UTC().Add(time.Duration(num) * time.Second)
		case 'm':
			return time.Now().UTC().Add(time.Duration(num) * time.Minute)
		case 'h':
			return time.Now().UTC().Add(time.Duration(num) * time.Hour)
		case 'd':
			return time.Now().UTC().Add(time.Duration(num) * 24 * time.Hour)
		}
	}
	return time.Time{}
}

// IsOneShot returns true if the schedule fires only once (once, at:, in:).
func IsOneShot(schedule string) bool {
	s := strings.ToLower(strings.TrimSpace(schedule))
	return s == "once" || s == "" || strings.HasPrefix(s, "at:") || strings.HasPrefix(s, "in:")
}

// UpdateSchedule changes the schedule and next_run_at for a task.
func (s *Store) UpdateSchedule(id, schedule, nextRunAt string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE tasks SET schedule = ?, next_run_at = ?, status = 'scheduled', updated_at = ? WHERE (id = ? OR id LIKE ?) AND deleted_at = ''`,
		schedule, nextRunAt, now, id, id+"%")
	return err
}

// ScheduleTask sets the next_run_at for a task and marks it as scheduled.
// If the task has an unmet depends_on, it is parked in 'waiting' status instead —
// ActivateDependents will wake it up when the dependency completes.
func (s *Store) ScheduleTask(id, schedule string) error {
	now := time.Now().UTC()

	// Resolve the task's depends_on (if any) and its dep's status.
	var dependsOn string
	if err := s.db.QueryRow(`SELECT depends_on FROM tasks WHERE (id = ? OR id LIKE ?) AND deleted_at = '' LIMIT 1`,
		id, id+"%").Scan(&dependsOn); err != nil {
		return fmt.Errorf("lookup task: %w", err)
	}
	if dependsOn != "" {
		var depStatus string
		err := s.db.QueryRow(`SELECT status FROM tasks WHERE (id = ? OR id LIKE ?) AND deleted_at = '' LIMIT 1`,
			dependsOn, dependsOn+"%").Scan(&depStatus)
		if err == nil && depStatus != "completed" && depStatus != "max_turns" {
			_, werr := s.db.Exec(`UPDATE tasks SET status = 'waiting', next_run_at = '', schedule = ?, updated_at = ? WHERE (id = ? OR id LIKE ?) AND deleted_at = ''`,
				schedule, now.Format(time.RFC3339), id, id+"%")
			return werr
		}
	}

	nextRun := ComputeNextRun(schedule)
	if nextRun.IsZero() && schedule != "once" {
		return fmt.Errorf("invalid schedule: %s", schedule)
	}
	if schedule == "once" {
		nextRun = now
	}

	_, err := s.db.Exec(`UPDATE tasks SET status = 'scheduled', schedule = ?, next_run_at = ?, updated_at = ? WHERE id = ? OR id LIKE ?`,
		schedule, nextRun.Format(time.RFC3339), now.Format(time.RFC3339), id, id+"%")
	return err
}

// ListDue returns tasks whose next_run_at has passed and are in scheduled status.
func (s *Store) ListDue() ([]*Task, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	rows, err := s.db.Query(`SELECT `+taskColumns+` FROM tasks WHERE status = 'scheduled' AND next_run_at != '' AND next_run_at <= ? AND deleted_at = '' LIMIT 10`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTasks(rows)
}

// SetNextRun updates the next run time for a recurring task and resets to scheduled.
func (s *Store) SetNextRun(id, nextRunAt string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE tasks SET next_run_at = ?, status = 'scheduled', updated_at = ? WHERE id = ?`, nextRunAt, now, id)
	return err
}
