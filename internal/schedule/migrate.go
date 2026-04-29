package schedule

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
)

// BackfillFromTasks imports legacy tasks.schedule + tasks.next_run_at into
// the central schedules table as one CURRENT row per task. Idempotent —
// the WHERE NOT EXISTS guard skips any task that already has a current
// schedule row, so re-runs (every boot, by design) are no-ops once the
// initial import lands.
//
// Why on every boot:
//   - Fresh deploys (no schedules table yet) need the import too.
//   - Operator-driven imports of historical task rows (CSV / sync) get
//     picked up automatically without a separate one-shot command.
//
// Source columns (legacy task scheduler fields, still authoritative
// until the runner is cut over to read from `schedules`):
//   - tasks.schedule    → schedules.cron_expr   (cron expression; '' = one-shot)
//   - tasks.next_run_at → schedules.run_at      (next fire timestamp)
//   - tasks.run_count   → schedules.runs_so_far (counter passthrough)
//   - tasks.created_at  → schedules.valid_from  (lineage anchor — when this
//                          schedule first existed; falls back to now())
//
// Filter: WHERE schedule != '' OR next_run_at != ''. A task with neither
// has nothing to import — it's never been scheduled.
//
// Defaults applied:
//   - timezone   = 'UTC'  (legacy tasks had no tz; UTC matches the runner)
//   - max_runs   = 0      (unbounded — legacy had no cap field)
//   - stop_at    = ''     (never expires; matches legacy semantics)
//   - enabled    = 1      (legacy tasks were always live unless cancelled —
//                          cancelled tasks have next_run_at = '' so they
//                          fall out of the filter naturally)
//   - created_by = 'backfill'
//   - reason     = 'one-shot import from tasks.schedule + tasks.next_run_at'
//
// Returns the number of rows inserted. Logs a single line at INFO when n>0;
// silent when n=0 to avoid every-boot log noise after the import lands.
func BackfillFromTasks(ctx context.Context, db *sql.DB) (int, error) {
	// One INSERT … SELECT, gated by NOT EXISTS so the backfill never
	// double-writes. valid_to = '' on the existence check is the
	// canonical "current row" predicate (matches every read path).
	res, err := db.ExecContext(ctx, `
		INSERT INTO schedules (
			entity_kind, entity_id,
			run_at, cron_expr, timezone, max_runs, runs_so_far, stop_at, enabled,
			valid_from, valid_to, created_by, reason, created_at
		)
		SELECT
			'task', t.id,
			COALESCE(NULLIF(t.next_run_at, ''), datetime('now')),
			COALESCE(t.schedule, ''),
			'UTC',
			0,
			COALESCE(t.run_count, 0),
			'',
			1,
			COALESCE(NULLIF(t.created_at, ''), datetime('now')),
			'',
			'backfill',
			'one-shot import from tasks.schedule + tasks.next_run_at',
			datetime('now')
		FROM tasks t
		WHERE (t.schedule != '' OR t.next_run_at != '')
		  AND NOT EXISTS (
			SELECT 1 FROM schedules s
			WHERE s.entity_kind = 'task'
			  AND s.entity_id = t.id
			  AND s.valid_to = ''
		  )
	`)
	if err != nil {
		// tasks table may not exist yet on some bootstrap paths
		// (e.g. tests that init schedule.Store with a fresh DB).
		// Treat "no such table: tasks" as a no-op rather than fatal.
		if isNoTasksTable(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("backfill schedules from tasks: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("backfill rows affected: %w", err)
	}
	if n > 0 {
		slog.Info(fmt.Sprintf("backfilled %d task schedules into schedules table", n))
	}
	return int(n), nil
}

// isNoTasksTable detects the SQLite "no such table: tasks" error so the
// backfill is a soft no-op when run before the task store has migrated
// (e.g. unit tests that wire only the schedule store). Any other error
// surfaces normally.
func isNoTasksTable(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "no such table: tasks")
}
