package task

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
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

// Create stores a new task. Per-task max_turns is set via the settings
// resolver (PUT /api/tasks/:id/settings) after creation — the legacy
// max_turns column was retired in M4-T14, so callers that still want to
// pin a value must write a settings row.
func (s *Store) Create(manifestID, title, description, schedule, agent, sourceNode, createdBy, dependsOn string) (*Task, error) {
	if schedule == "" {
		schedule = "once"
	}
	if agent == "" {
		agent = "claude-code"
	}

	id := uuid.Must(uuid.NewV7()).String()
	now := time.Now().UTC()

	// Initial status derives from the presence + state of depends_on.
	//
	//   - no dep                             → pending  (manual-only, won't auto-fire)
	//   - dep set, parent not yet completed  → waiting  (ActivateDependents will flip to scheduled)
	//   - dep set, parent already completed  → pending  (caller can schedule or manually start;
	//                                                    we don't auto-schedule on create because
	//                                                    there's no next_run_at to honor yet)
	//
	// This is the half of the state-machine fix that #67 couldn't reach:
	// #67 loosened ActivateDependents to also match 'pending' children as
	// a safety net, but the correct invariant is that dep-bearing tasks
	// start in 'waiting' so the sort order + UI filters tell the truth.
	initialStatus := StatusPending
	blockReason := ""

	// Task-level dep wins as the first blocker: if the parent task isn't
	// completed, the task is waiting regardless of manifest state.
	if dependsOn != "" {
		var parentStatus string
		err := s.db.QueryRow(`SELECT status FROM tasks WHERE (id = ? OR id LIKE ?) AND deleted_at = ''`,
			dependsOn, dependsOn+"%").Scan(&parentStatus)
		if err == nil && parentStatus != string(StatusCompleted) {
			initialStatus = StatusWaiting
			blockReason = fmt.Sprintf("task %s not completed", firstN(dependsOn, 12))
		}
		// sql.ErrNoRows or any other lookup failure: keep pending, operator
		// can correct by attaching the dep later via Edit.
	}

	// Manifest-level dep check only runs when the task-level gate is
	// already clear. Reason: if both blockers apply, the operator cares
	// about the closer one (task dep) first — once that clears, the
	// manifest-level check re-runs at activation time.
	if initialStatus == StatusPending && manifestID != "" && s.manifestChecker != nil {
		ok, unsatisfied, err := s.manifestChecker.IsSatisfied(context.Background(), manifestID)
		if err != nil {
			// Lookup failure shouldn't block task creation. Log via the
			// caller's slog (we don't have one here) by returning the
			// task normally; operator can re-trigger activation later.
			// Keep pending.
		} else if !ok {
			initialStatus = StatusWaiting
			blockReason = fmt.Sprintf("manifest not satisfied — blocked by: %s",
				joinMarkers(unsatisfied))
		}
	}

	// Product-level dep check runs only if task-level + manifest-level
	// gates are both clear. Same precedence as the manifest check:
	// innermost blocker is named in block_reason; outer checks re-run
	// at activation time as inner blockers clear.
	//
	// productID is derived from manifests.project_id. Kept as a local
	// SELECT rather than a dedicated cross-package store method so the
	// task package doesn't need to import product.
	if initialStatus == StatusPending && manifestID != "" && s.productChecker != nil {
		var productID string
		_ = s.db.QueryRow(`SELECT project_id FROM manifests WHERE id = ? AND deleted_at = ''`,
			manifestID).Scan(&productID)
		if productID != "" {
			ok, unsatisfied, err := s.productChecker.IsSatisfied(context.Background(), productID)
			if err == nil && !ok {
				initialStatus = StatusWaiting
				blockReason = fmt.Sprintf("product not satisfied — blocked by: %s",
					joinMarkers(unsatisfied))
			}
		}
	}

	_, err := s.db.Exec(`INSERT INTO tasks (id, manifest_id, title, description, schedule, status, agent, source_node, created_by, depends_on, block_reason, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, manifestID, title, description, schedule, string(initialStatus), agent, sourceNode, createdBy, dependsOn, blockReason,
		now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}

	return &Task{
		ID: id, Marker: id[:12], ManifestID: manifestID,
		Title: title, Description: description, Schedule: schedule,
		Status: string(initialStatus), Agent: agent, SourceNode: sourceNode,
		CreatedBy: createdBy, DependsOn: dependsOn, BlockReason: blockReason,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

// firstN returns the first n chars of s. Used for logging task/manifest
// markers in block_reason strings without pulling the whole UUID.
func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// joinMarkers formats a list of manifest ids as comma-separated markers
// for inclusion in a block_reason. Empty input → "(unknown)" so the
// reason is never ambiguous-looking to the operator.
func joinMarkers(ids []string) string {
	if len(ids) == 0 {
		return "(unknown)"
	}
	markers := make([]string, 0, len(ids))
	for _, id := range ids {
		markers = append(markers, firstN(id, 12))
	}
	return strings.Join(markers, ", ")
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

// Update modifies optional fields on a task (title, description). Only
// non-nil fields are updated. Per-task max_turns is no longer a task-row
// field — callers who want to set it go through the settings resolver
// (PUT /api/tasks/:id/settings). Retired in M4-T14.
func (s *Store) Update(id string, title, description *string) (*Task, error) {
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

// SetManifest swaps a task's primary manifest_id. Used to re-home a task
// when an operator splits / restructures manifests after the task was
// originally created (e.g. ELS atomic split moved 12 tasks under 6 new
// implementation manifests). The id may be a marker or full UUID. Returns
// the task as it stands after the update.
func (s *Store) SetManifest(taskID, manifestID string) (*Task, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		"UPDATE tasks SET manifest_id = ?, updated_at = ? WHERE (id = ? OR id LIKE ?) AND deleted_at = ''",
		manifestID, now, taskID, taskID+"%",
	)
	if err != nil {
		return nil, fmt.Errorf("set manifest: %w", err)
	}
	return s.Get(taskID)
}

// UpdateStatus changes a task's status, validating the transition against
// the canonical state machine in status.go. Returns the same validation
// error surface ValidateTransition produces when the move is illegal — the
// caller gets the exact reason and which next states would have been
// legal. A no-op (current == target) is silently allowed so idempotent
// writers don't see spurious errors.
func (s *Store) UpdateStatus(id, status string) error {
	// Resolve the row so we can read the current status for the
	// transition check. Marker-prefix match is preserved.
	var current string
	if err := s.db.QueryRow(`SELECT status FROM tasks WHERE (id = ? OR id LIKE ?) AND deleted_at = ''`,
		id, id+"%").Scan(&current); err != nil {
		return err
	}
	if err := ValidateTransition(Status(current), Status(status)); err != nil {
		return err
	}
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

// ErrTaskDepCycle is returned when SetDependency would create a cycle in
// the task dependency graph. Handlers translate this to HTTP 409 / an
// MCP tool error.
var ErrTaskDepCycle = errors.New("task dependency: cycle detected")

// ErrTaskDepSelfLoop is returned when a task is asked to depend on itself.
var ErrTaskDepSelfLoop = errors.New("task dependency: a task cannot depend on itself")

// SetDependency sets or clears the depends_on field for a task, and
// transitions the task's status + block_reason to match the new
// dependency state. Centralizes the four invariants that previously
// leaked into the HTTP handler at handlers_task.go:
//
//   1. Cycle detection. DFS from the proposed parent following the
//      single-parent depends_on column; if we ever hit taskID, reject.
//      Single-parent makes this O(depth), effectively O(1) for real
//      graphs.
//   2. Self-loop rejection with ErrTaskDepSelfLoop, so the HTTP / MCP
//      surfaces can surface a 400 rather than raw storage error text.
//   3. Parent-status-aware seeding. If the parent is already
//      StatusCompleted the dep is already satisfied — status flips
//      to Scheduled (ready to fire), not Waiting. Matches the
//      post-#77 invariant for Create so both paths agree.
//   4. block_reason is populated when parking in Waiting and cleared
//      on every other transition. The #85 UI renders a "Blocked:"
//      bar from this field; leaving it stale or empty breaks that
//      signal.
//
// Clearing the dep (dependsOn=="") is symmetric: column blanked, and
// if the task was Waiting on a task-level block, it flips to Pending
// with block_reason cleared (Option B symmetry with manifest-dep
// removal from #79 — operator arms explicitly).
func (s *Store) SetDependency(id, dependsOn string) error {
	return s.SetDependencyWithAudit(id, dependsOn, "", "")
}

// SetDependencyWithAudit writes the new dep via SCD Type 2 semantics.
//
//   - The currently-active row (valid_to='') in task_dependency is
//     closed by stamping valid_to=now.
//   - A fresh row is inserted with valid_from=now, valid_to='', the
//     new depends_on value, changedBy + reason for attribution.
//   - tasks.depends_on is updated as a cache of the new active value
//     so existing queries (scanTask, the scheduler) don't need an
//     SCD join on every read.
//
// Result: the dep history for a task is just the rows in
// task_dependency ordered by valid_from. No separate audit log —
// history and current state share one table.
//
// changedBy identifies the caller ("http-api", session id, operator
// name, etc.); reason is free-form. Both are optional but worth
// populating on every write because the table can't be purged without
// losing the decision trail.
//
// Cycle / self-loop rejection happens before any write so a refused
// change never produces an SCD row.
func (s *Store) SetDependencyWithAudit(id, dependsOn, changedBy, reason string) error {
	// Resolve the full row so prefix inputs still work + we see the
	// current status for the state transition.
	var (
		fullID, currentStatus string
	)
	if err := s.db.QueryRow(
		`SELECT id, status FROM tasks WHERE (id = ? OR id LIKE ?) AND deleted_at = ''`,
		id, id+"%").Scan(&fullID, &currentStatus); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Clear-path: no dep. Writes an SCD row with depends_on=''
	// so "dep cleared" is visible in the history stream.
	if dependsOn == "" {
		nextStatus := currentStatus
		if currentStatus == string(StatusWaiting) {
			nextStatus = string(StatusPending)
		}
		if err := s.writeSCDDepRow(fullID, "", now, changedBy, reason); err != nil {
			return err
		}
		_, err := s.db.Exec(
			`UPDATE tasks SET depends_on = '', block_reason = '', status = ?, updated_at = ? WHERE id = ?`,
			nextStatus, now, fullID)
		return err
	}

	// Resolve + validate the proposed parent.
	var parentID, parentStatus string
	if err := s.db.QueryRow(
		`SELECT id, status FROM tasks WHERE (id = ? OR id LIKE ?) AND deleted_at = ''`,
		dependsOn, dependsOn+"%").Scan(&parentID, &parentStatus); err != nil {
		return err
	}
	if parentID == fullID {
		return ErrTaskDepSelfLoop
	}
	if reaches, err := s.taskDepPathExists(parentID, fullID); err != nil {
		return fmt.Errorf("cycle check: %w", err)
	} else if reaches {
		return fmt.Errorf("%w: %s → %s would close a cycle",
			ErrTaskDepCycle, shortID(fullID), shortID(parentID))
	}

	nextStatus, blockReason := deriveDepState(currentStatus, parentStatus, parentID)

	if err := s.writeSCDDepRow(fullID, parentID, now, changedBy, reason); err != nil {
		return err
	}
	// When deriveDepState returns StatusScheduled (parent already
	// completed), set next_run_at too — the scheduler's dequeue query
	// requires next_run_at != ''. Bug #114: previously this UPDATE
	// wrote only the status column, leaving the task armed-but-
	// uncallable ("scheduled but never picked up").
	nextRunAt := ""
	if nextStatus == string(StatusScheduled) {
		nextRunAt = now
	}
	_, err := s.db.Exec(
		`UPDATE tasks SET depends_on = ?, status = ?, block_reason = ?, next_run_at = ?, updated_at = ? WHERE id = ?`,
		parentID, nextStatus, blockReason, nextRunAt, now, fullID)
	return err
}

// writeSCDDepRow closes the currently-active row (if any) and inserts
// a new active row. Uses two statements in sequence — SQLite
// transactions would add correctness if the two statements could race
// against concurrent writers for the same task_id, but SetDependency
// is operator-initiated and per-task serialization is implicit.
func (s *Store) writeSCDDepRow(taskID, dependsOn, now, changedBy, reason string) error {
	if _, err := s.db.Exec(
		`UPDATE task_dependency SET valid_to = ? WHERE task_id = ? AND valid_to = ''`,
		now, taskID); err != nil {
		return fmt.Errorf("close prior dep row: %w", err)
	}
	if _, err := s.db.Exec(
		`INSERT INTO task_dependency (task_id, depends_on, valid_from, valid_to, changed_by, reason)
		 VALUES (?, ?, ?, '', ?, ?)`,
		taskID, dependsOn, now, changedBy, reason); err != nil {
		return fmt.Errorf("insert new dep row: %w", err)
	}
	return nil
}

// DepHistoryEntry is one row of the SCD Type 2 task_dependency table.
// Rows are the full history of the task's dep relationship in
// chronological order. The current/active row has ValidTo == "".
type DepHistoryEntry struct {
	ID         int    `json:"id"`
	TaskID     string `json:"task_id"`
	DependsOn  string `json:"depends_on"` // empty = dep cleared in this revision
	ValidFrom  string `json:"valid_from"`
	ValidTo    string `json:"valid_to"`   // empty = currently active
	ChangedBy  string `json:"changed_by"`
	Reason     string `json:"reason"`
}

// ListDepHistory returns every dep revision for a task, newest
// valid_from first. limit<=0 defaults to 50.
func (s *Store) ListDepHistory(taskID string, limit int) ([]DepHistoryEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT id, task_id, depends_on, valid_from, valid_to, changed_by, reason
		 FROM task_dependency WHERE task_id = ? ORDER BY valid_from DESC, id DESC LIMIT ?`,
		taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DepHistoryEntry
	for rows.Next() {
		var e DepHistoryEntry
		if err := rows.Scan(&e.ID, &e.TaskID, &e.DependsOn,
			&e.ValidFrom, &e.ValidTo, &e.ChangedBy, &e.Reason); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// BackfillTaskDepSCD seeds task_dependency for tasks that had a
// non-empty tasks.depends_on cache but no SCD row yet. Runs on
// startup; idempotent via the "already has an active row" check.
// Legacy rows land with valid_from = the task's updated_at so the
// history doesn't all collapse to "now".
func (s *Store) BackfillTaskDepSCD() (int, error) {
	rows, err := s.db.Query(`
		SELECT t.id, t.depends_on, t.updated_at
		FROM tasks t
		WHERE t.depends_on != '' AND t.deleted_at = ''
		  AND NOT EXISTS (SELECT 1 FROM task_dependency d
		                  WHERE d.task_id = t.id AND d.valid_to = '')`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	type pair struct{ id, dep, when string }
	var pending []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.id, &p.dep, &p.when); err != nil {
			return 0, err
		}
		pending = append(pending, p)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	n := 0
	for _, p := range pending {
		if _, err := s.db.Exec(
			`INSERT INTO task_dependency (task_id, depends_on, valid_from, valid_to, changed_by, reason)
			 VALUES (?, ?, ?, '', 'scd-backfill', 'seeded from legacy tasks.depends_on')`,
			p.id, p.dep, p.when); err == nil {
			n++
		}
	}
	return n, nil
}

// deriveDepState picks the status + block_reason for a task whose dep is
// being set. Extracted so tests can exercise the classification without
// touching the DB.
func deriveDepState(currentStatus, parentStatus, parentID string) (string, string) {
	// If the current status is Running or Paused we leave it alone —
	// the task is mid-flight, changing its dep shouldn't derail it.
	// block_reason stays whatever it was.
	if currentStatus == string(StatusRunning) || currentStatus == string(StatusPaused) ||
		currentStatus == string(StatusCompleted) || currentStatus == string(StatusFailed) ||
		currentStatus == string(StatusCancelled) {
		return currentStatus, "" // block_reason cleared; dep is recorded but status stays
	}
	if parentStatus == string(StatusCompleted) {
		// Parent is already done — task is armed.
		return string(StatusScheduled), ""
	}
	// Parent is in some non-terminal state — task parks in Waiting
	// with a populated block_reason that the UI surfaces.
	return string(StatusWaiting), fmt.Sprintf("task %s not completed (is %s)",
		shortID(parentID), parentStatus)
}

// shortID returns the first 12 chars of an ID (the marker convention).
// Defensive for ids shorter than 12 (which shouldn't happen in prod).
func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

// taskDepPathExists runs a bounded DFS from src following single-parent
// depends_on edges. Returns true if dst is reachable. O(chain depth) in
// a well-formed graph; visited set caps work if the data contains a
// pre-existing cycle.
func (s *Store) taskDepPathExists(src, dst string) (bool, error) {
	visited := map[string]bool{}
	cur := src
	for cur != "" {
		if cur == dst {
			return true, nil
		}
		if visited[cur] {
			return false, nil // pre-existing cycle — treat as no-reach
		}
		visited[cur] = true
		var next string
		err := s.db.QueryRow(
			`SELECT depends_on FROM tasks WHERE id = ? AND deleted_at = ''`, cur).Scan(&next)
		if err == sql.ErrNoRows {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		cur = next
	}
	return false, nil
}

// ActivateDependents finds tasks that depend on the given task ID and
// schedules them. Historically the WHERE matched only status='waiting', but
// the task create path sets initial status='pending' even when depends_on
// is non-empty — so no dependent has ever auto-activated in production, and
// the whole dependency chain had to be fired by hand. Accepting both
// statuses fixes that without regressing any path that currently relies on
// 'waiting' (which remains valid and is still flipped here). The
// depends_on = ? filter guarantees we only touch direct children of the
// completed task, so loosening the status predicate is safe.
func (s *Store) ActivateDependents(completedTaskID string) (int, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`UPDATE tasks SET status = 'scheduled', next_run_at = ?, updated_at = ?
		WHERE depends_on = ? AND status IN ('waiting', 'pending') AND deleted_at = ''`, now, now, completedTaskID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// TaskDep is the denormalised row the dashboard renders for both
// directions of /api/tasks/{id}/dependencies. Same {id, marker, title,
// status} contract products + manifests use.
type TaskDep struct {
	ID     string `json:"id"`
	Marker string `json:"marker"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// ListDependents returns every task whose `depends_on` is taskID.
// Mirrors manifest.Store.ListDependents — used by the portal-v2
// Dependencies tab when the operator clicks "in" direction. Marker is
// the 12-char ID prefix per project convention.
func (s *Store) ListDependents(ctx context.Context, taskID string) ([]TaskDep, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, status FROM tasks WHERE depends_on = ? AND deleted_at = ''
		 ORDER BY created_at ASC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TaskDep{}
	for rows.Next() {
		var d TaskDep
		if err := rows.Scan(&d.ID, &d.Title, &d.Status); err != nil {
			return nil, err
		}
		if len(d.ID) >= 12 {
			d.Marker = d.ID[:12]
		}
		out = append(out, d)
	}
	return out, rows.Err()
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

// PricingVersion stamps the pricing table identity used to compute cost_usd
// for a task_runs row. Increment whenever internal/task/pricing.go's rate
// table changes so the future Unified Cost Tracking product (019dab45-d8f)
// can re-cost historical rows under the right table — or flag drift when
// today's rates differ from when the row was originally written.
const PricingVersion = "v1-2026-04"

// RecordRun updates run stats after a task executes and saves to run history.
// usage carries the per-token counts parsed from the agent's stream-json
// output; they're denormalised onto task_runs so dashboards don't have to
// re-parse the full output blob on every read. The blob remains in
// task_runs.output as the source of truth.
// RecordRun inserts a completed run into task_runs and returns the
// inserted row id. Caller passes the run_id to RecordHostMetrics to
// persist the host CPU/RSS sparkline samples + rollup columns.
func (s *Store) RecordRun(id, output, status string, actions, lines int, costUSD float64, turns int, startedAt time.Time, usage Usage, model string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	// No truncation — capture full output
	_, err := s.db.Exec(`UPDATE tasks SET run_count = run_count + 1, last_run_at = ?, last_output = ?, status = ?, updated_at = ? WHERE id = ?`,
		now, output, status, now, id)
	if err != nil {
		return 0, err
	}

	// Get current run_count for run_number
	var runCount int
	if err := s.db.QueryRow(`SELECT run_count FROM tasks WHERE id = ?`, id).Scan(&runCount); err != nil {
		slog.Warn("query run_count failed", "task_id", id, "error", err)
	}

	// Resolve the task's agent so the run row carries an agent_runtime
	// stamp (used by the Stats tab summary card). Best-effort — empty
	// string is fine if the task was deleted between RecordRun args
	// being prepared and this query.
	var agentRuntime string
	_ = s.db.QueryRow(`SELECT agent FROM tasks WHERE id = ?`, id).Scan(&agentRuntime)

	durationMS := time.Since(startedAt).Milliseconds()
	if durationMS < 0 {
		durationMS = 0
	}

	// Insert into task_runs history with full denorm (tokens + model + pricing version).
	res, err := s.db.Exec(`INSERT INTO task_runs
		(task_id, run_number, output, status, actions, lines, cost_usd, turns, started_at, completed_at,
		 input_tokens, output_tokens, cache_read_tokens, cache_create_tokens, model, pricing_version,
		 duration_ms, agent_runtime)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, runCount, output, status, actions, lines, costUSD, turns, startedAt.UTC().Format(time.RFC3339), now,
		usage.InputTokens, usage.OutputTokens, usage.CacheReadTokens, usage.CacheCreationTokens, model, PricingVersion,
		durationMS, agentRuntime)
	if err != nil {
		return 0, err
	}
	runID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return runID, nil
}

// RecordHostMetrics persists the HostSampler output for a run: rollup
// columns on task_runs + every sample into task_run_host_samples. Best-
// effort — errors log-and-swallow because losing host metrics must not
// fail the task completion path.
func (s *Store) RecordHostMetrics(runID int64, samples []HostMetricsSample, metrics HostMetrics) error {
	if runID <= 0 {
		return nil
	}
	if _, err := s.db.Exec(
		`UPDATE task_runs SET peak_cpu_pct = ?, avg_cpu_pct = ?, peak_rss_mb = ?, avg_rss_mb = ? WHERE id = ?`,
		metrics.PeakCPUPct, metrics.AvgCPUPct, metrics.PeakRSSMB, metrics.AvgRSSMB, runID,
	); err != nil {
		return fmt.Errorf("update task_runs host summary: %w", err)
	}
	if len(samples) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin host-samples tx: %w", err)
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO task_run_host_samples
		(run_id, ts, cpu_pct, rss_mb, cost_usd, turns, actions, disk_used_gb, disk_total_gb)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare host-samples insert: %w", err)
	}
	defer stmt.Close()
	for _, smp := range samples {
		if _, err := stmt.Exec(runID, smp.TS.UTC().Format(time.RFC3339Nano),
			smp.CPUPct, smp.RSSMB, smp.CostUSD, smp.Turns, smp.Actions,
			smp.DiskUsedGB, smp.DiskTotalGB); err != nil {
			return fmt.Errorf("insert host sample: %w", err)
		}
	}
	return tx.Commit()
}

// ListHostSamples returns the time-series host samples for a run.
// Ordered by ts ASC so the frontend can render left-to-right directly.
// Reads all 5 metrics captured at each sample: host CPU/RSS + live task
// counters (cost/turns/actions) — they share the same time axis so the
// Run Stats card can overlay all 5 sparklines aligned.
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
	rows, err := s.db.Query(`SELECT t.id, t.manifest_id, t.title, t.description, t.schedule, t.status, t.agent, t.source_node, t.created_by, t.depends_on, t.block_reason, t.run_count, t.last_run_at, t.next_run_at, t.last_output, t.created_at, t.updated_at
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
