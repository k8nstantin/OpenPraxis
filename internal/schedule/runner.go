// Runner registers every current+enabled row in the schedules table
// against an in-memory robfig/cron/v3 ticker. The library owns the
// next-fire computation (cron parser handles cron_expr; a one-shot
// adapter handles cron_expr=''); we own the SQLite-side persistence
// (rows live in `schedules`, MarkFired increments runs_so_far + flips
// enabled when max_runs is reached).
//
// Reload is the synchronisation seam: every HTTP/MCP path that mutates
// the schedules table should call Runner.Reload so the in-memory cron
// stays in sync. Reload is cheap (one indexed query + a Stop/Start
// inside the cron lib); calling it on every write is well within
// budget for OpenPraxis-scale workloads.

package schedule

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// DispatchFunc fires the given entity. Receives the same context the
// runner was started with, the entity_id, and the schedule row id (so
// the dispatcher can correlate with audit trails). Returns an error if
// the entity could not be enqueued / fired; on error the runner SKIPS
// MarkFired so the row remains pending for the next tick.
type DispatchFunc func(ctx context.Context, entityID string, scheduleID int64) error

// Runner owns a singleton cron.Cron instance and keeps it in sync with
// the schedules table.
type Runner struct {
	store       *Store
	dispatchers map[string]DispatchFunc

	cron *cron.Cron
	// entries maps schedule.id → cron.EntryID so Reload can Remove
	// entries that no longer have a current+enabled row.
	mu      sync.Mutex
	entries map[int64]cron.EntryID

	// ctx is captured at Start; jobs use it so a cancellation propagates
	// to in-flight dispatches (best-effort — a long-running dispatcher
	// must respect ctx itself).
	ctx context.Context
}

// NewRunner builds a runner with the given dispatcher map. The map is
// keyed on entity_kind ("task", "manifest", "product", …); a row whose
// kind has no dispatcher registered is logged + skipped at fire time.
func NewRunner(store *Store, dispatchers map[string]DispatchFunc) *Runner {
	if dispatchers == nil {
		dispatchers = map[string]DispatchFunc{}
	}
	// SkipIfStillRunning prevents a slow dispatcher from stacking up if
	// it takes longer than the cron interval. Logger forwards lib
	// internals to slog at INFO so we see "skipped because previous
	// instance still running" + parse errors in the operator log.
	c := cron.New(
		cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)),
		cron.WithLogger(cron.PrintfLogger(slogPrintfer{})),
	)
	return &Runner{
		store:       store,
		dispatchers: dispatchers,
		cron:        c,
		entries:     map[int64]cron.EntryID{},
	}
}

// Start kicks the cron ticker. Call once at server boot; the goroutine
// the lib spawns lives until ctx cancellation. Reload is invoked once
// synchronously before the ticker spins so the registered set matches
// the DB on first tick.
func (r *Runner) Start(ctx context.Context) error {
	r.ctx = ctx
	if err := r.Reload(ctx); err != nil {
		return fmt.Errorf("schedule.Runner: initial reload: %w", err)
	}
	r.cron.Start()
	go func() {
		<-ctx.Done()
		// cron.Stop returns a context that closes when in-flight jobs
		// drain; we don't block boot/shutdown on that here.
		_ = r.cron.Stop()
	}()
	return nil
}

// Reload re-syncs the in-memory cron set with the schedules table.
// Diff-based: rows that no longer exist (enabled=0 / closed / max_runs
// reached) get cron.Remove'd; new rows get registered; rows that are
// still present + unchanged keep their existing entry.
//
// Idempotent + safe to call on every schedule mutation. Cost on a
// tens-of-rows table: ~1 indexed query + a few map lookups, well under
// 1ms.
func (r *Runner) Reload(ctx context.Context) error {
	rows, err := r.store.ListAllCurrent(ctx)
	if err != nil {
		return fmt.Errorf("list all current: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Build the set of ids that should be present after this Reload.
	want := make(map[int64]*Schedule, len(rows))
	for _, row := range rows {
		want[row.ID] = row
	}

	// Remove entries whose row is no longer in the wanted set.
	for id, entryID := range r.entries {
		if _, ok := want[id]; !ok {
			r.cron.Remove(entryID)
			delete(r.entries, id)
		}
	}

	// Add entries that don't yet exist in-memory.
	for id, row := range want {
		if _, ok := r.entries[id]; ok {
			// Already registered. Skip; we don't restart entries on
			// no-op reloads to avoid resetting next-fire timing.
			continue
		}
		entryID, err := r.register(row)
		if err != nil {
			slog.Warn("schedule.Runner: register failed",
				"schedule_id", row.ID,
				"entity_kind", row.EntityKind,
				"entity_id", row.EntityID,
				"cron_expr", row.CronExpr,
				"run_at", row.RunAt,
				"error", err)
			continue
		}
		r.entries[id] = entryID
	}
	return nil
}

// register builds the cron.Schedule for one row and adds it to the
// cron with a closure that dispatches + marks fired.
func (r *Runner) register(row *Schedule) (cron.EntryID, error) {
	sched, err := buildSchedule(row)
	if err != nil {
		return 0, err
	}
	job := r.makeJob(row.ID, row.EntityKind, row.EntityID)
	return r.cron.Schedule(sched, cron.FuncJob(job)), nil
}

// makeJob is the per-row dispatch closure. Captured fields stay valid
// across Reload because cron's entry table holds the FuncJob; closing
// over only the immutable triple (id, kind, entityID) keeps the job
// serializable across schedule mutations.
func (r *Runner) makeJob(id int64, kind, entityID string) func() {
	return func() {
		ctx := r.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		dispatch, ok := r.dispatchers[kind]
		if !ok {
			slog.Warn("schedule.Runner: no dispatcher for kind",
				"schedule_id", id, "entity_kind", kind, "entity_id", entityID)
			return
		}
		if err := dispatch(ctx, entityID, id); err != nil {
			// Don't MarkFired on dispatch failure — leave the row to
			// retry on the next tick. The cron lib will fire us again
			// at the next scheduled time without our intervention.
			slog.Error("schedule.Runner: dispatch failed",
				"schedule_id", id, "entity_kind", kind, "entity_id", entityID,
				"error", err)
			return
		}
		if err := r.store.MarkFired(ctx, id); err != nil {
			slog.Warn("schedule.Runner: MarkFired failed (dispatched anyway)",
				"schedule_id", id, "entity_kind", kind, "entity_id", entityID,
				"error", err)
		}
		slog.Info("schedule.Runner: fired",
			"schedule_id", id, "entity_kind", kind, "entity_id", entityID)

		// One-shot semantics: if the row is now disabled (max_runs hit),
		// remove it from the in-memory cron immediately so a stuck
		// future tick can't re-fire before the next Reload. Cron's own
		// one-shot Schedule returns zero time on subsequent Next() calls,
		// which would also handle this — belt-and-braces here.
		r.mu.Lock()
		if entryID, ok := r.entries[id]; ok {
			r.cron.Remove(entryID)
			delete(r.entries, id)
		}
		r.mu.Unlock()
	}
}

// buildSchedule chooses the cron.Schedule implementation for a row.
// cron_expr="" or "once" → one-shot adapter firing at run_at exactly
// once. Any other value → standard 5-field cron parser.
//
// The task store writes schedule="once" which propagates to the
// schedules table as cron_expr="once"; the original code only checked
// for empty string and tried to parse "once" as a cron expression,
// producing a WARN log on every Reload and silently dropping the entry.
func buildSchedule(row *Schedule) (cron.Schedule, error) {
	if row.CronExpr != "" && row.CronExpr != "once" {
		// Use the standard 5-field cron parser (minute hour dom month dow).
		// Robfig defaults to a 6-field parser with seconds; standard is
		// what the schema implies and what UIs typically show.
		parser := cron.NewParser(
			cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow,
		)
		s, err := parser.Parse(row.CronExpr)
		if err != nil {
			return nil, fmt.Errorf("parse cron_expr %q: %w", row.CronExpr, err)
		}
		return s, nil
	}
	// One-shot — cron_expr is "" or the sentinel "once".
	if row.RunAt == "" {
		return nil, errors.New("buildSchedule: one-shot schedule requires run_at")
	}
	t, err := parseRunAt(row.RunAt)
	if err != nil {
		return nil, fmt.Errorf("parse run_at %q: %w", row.RunAt, err)
	}
	return &oneShot{runAt: t}, nil
}

// parseRunAt accepts either RFC3339Nano (UTC TEXT, what the store
// emits) or RFC3339 (older rows pre-nano). Falls back to the
// SQLite-default "2006-01-02 15:04:05" if both fail.
func parseRunAt(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("unrecognised run_at format: %q", s)
}

// oneShot is a cron.Schedule that fires exactly once at runAt. After
// the first Next() that returns a non-zero time, subsequent calls
// return zero — cron interprets that as "deregister this entry."
type oneShot struct {
	mu    sync.Mutex
	runAt time.Time
	fired bool
}

func (s *oneShot) Next(now time.Time) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fired {
		return time.Time{}
	}
	s.fired = true
	// If runAt is in the past at registration time (e.g. we restarted
	// after a missed fire), still return runAt — cron will fire
	// immediately on the next tick.
	return s.runAt
}

// slogPrintfer adapts cron's Printfer interface to slog. cron uses this
// for Info-level lifecycle messages ("scheduling next run", "skip due
// to still-running"); we forward at slog.Info so they land in the
// operator log alongside everything else.
type slogPrintfer struct{}

func (slogPrintfer) Printf(format string, args ...any) {
	slog.Info(fmt.Sprintf("cron: "+format, args...))
}
