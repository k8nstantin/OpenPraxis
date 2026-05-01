// Package schedule is the SCD-2 (slowly-changing-dimension type-2) store
// for entity firing schedules. One row per active or historical schedule
// version; "replacing" a schedule = Close(old) + Create(new) so history
// stays intact and the dashboard can render an audit trail.
//
// Conventions match internal/relationships/store.go: TEXT timestamps,
// empty-string '' (not NULL) for "still current", partial indexes on
// valid_to = '' for the hot read path. No CHECK constraints / triggers
// — schema stays portable across SQL engines (SQLite today; Postgres /
// Iceberg-via-Trino likely on future fleet-scale peers).
//
// The runner currently reads scheduling fields directly from the task
// row (next_run_at, schedule_cron, etc). A follow-up PR cuts the runner
// over to read from this table; this PR only adds the substrate + the
// UI surface so the central table is real, persisted, and exercised.
package schedule

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// ErrInvalidKind is returned when entity_kind is not one of the
// enumerated entity kinds.
var ErrInvalidKind = errors.New("schedule: invalid entity_kind")

// ErrEmptyEntityID is returned when entity_id is empty.
var ErrEmptyEntityID = errors.New("schedule: entity_id must be non-empty")

// ErrEmptyRunAt is returned when run_at is empty on Create. Even cron
// schedules need an initial fire timestamp for the scheduler to pick up.
var ErrEmptyRunAt = errors.New("schedule: run_at must be non-empty")

// Standard entity kinds for schedules. Stored as TEXT in entity_kind.
// Add a new kind: extend allEntityKinds (which the validator iterates)
// AND add the const; the schema needs no migration.
const (
	KindProduct  = "product"
	KindManifest = "manifest"
	KindTask     = "task"
)

var allEntityKinds = []string{KindProduct, KindManifest, KindTask}

// validKind returns true if k is one of the enumerated entity kinds.
func validKind(k string) bool {
	for _, v := range allEntityKinds {
		if k == v {
			return true
		}
	}
	return false
}

// nowUTC returns the current time as RFC3339Nano in UTC. Composite-PK
// concerns mirror internal/relationships/store.go — back-to-back
// Create/Close pairs at second precision can collide under tight loops.
func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// Schedule is one row in the schedules table.
//
// Field semantics:
//   - ID: assigned by store on Create.
//   - EntityKind, EntityID: which entity this schedule fires.
//   - RunAt: next fire time (one-shot) OR the seed-now for cron schedules.
//   - CronExpr: '' = one-shot fire at RunAt; non-empty = recurrence.
//   - Timezone: IANA tz name; defaults to "UTC" on Create when blank.
//   - MaxRuns: 0 = unbounded; otherwise stop after N runs.
//   - RunsSoFar: counter the runner increments. Owned by the runner;
//     the store accepts it as a passthrough for restoration / fixtures.
//   - StopAt: '' = no end date; otherwise terminate after this RFC3339 ts.
//   - Enabled: 0/1 — operator can disable without closing the row, so
//     the audit trail keeps the cron expr + history intact.
//   - Metadata: opaque JSON blob for forward-compat fields.
//
// SCD-2 columns:
//   - ValidFrom: '' on Create input → defaults to now(). Otherwise
//     caller-controlled (used by hypothetical backfills).
//   - ValidTo: '' on Create (always); set by Close to mark the row
//     superseded. Read path treats '' as "this is the current version."
//   - CreatedBy / Reason: optional audit attribution.
//   - CreatedAt: WRITE-PATH IGNORES INPUT — set by store on insert.
type Schedule struct {
	ID         int64  `json:"id"`
	EntityKind string `json:"entity_kind"`
	EntityID   string `json:"entity_id"`
	RunAt      string `json:"run_at"`
	CronExpr   string `json:"cron_expr"`
	Timezone   string `json:"timezone"`
	MaxRuns    int    `json:"max_runs"`
	RunsSoFar  int    `json:"runs_so_far"`
	StopAt     string `json:"stop_at"`
	Enabled    bool   `json:"enabled"`
	Metadata   string `json:"metadata"`

	ValidFrom string `json:"valid_from"`
	ValidTo   string `json:"valid_to"`
	CreatedBy string `json:"created_by"`
	Reason    string `json:"reason"`
	CreatedAt string `json:"created_at"`
}

// Store owns the schedules table.
//
// createMu serializes Create + Close. Same rationale as
// relationships.Store.createMu: SQLite WAL gives each connection a
// snapshot at BEGIN time; without the mutex two concurrent Create calls
// for the same entity could each see "no current row" and both insert,
// breaking the read-path assumption that current rows are unique per
// entity. Future Postgres backend can drop the mutex.
type Store struct {
	db       *sql.DB
	createMu sync.Mutex
}

// New opens the store and runs the idempotent schema migration. Pass
// the same *sql.DB used by the rest of the OpenPraxis stores so this
// table lives in the canonical openpraxis.db.
func New(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.init(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) init() error {
	// CREATE TABLE / INDEXes are IF NOT EXISTS so init() runs every boot
	// and is a no-op after the first migration.
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schedules (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		entity_kind TEXT    NOT NULL,
		entity_id   TEXT    NOT NULL,
		run_at      TEXT    NOT NULL,
		cron_expr   TEXT    NOT NULL DEFAULT '',
		timezone    TEXT    NOT NULL DEFAULT 'UTC',
		max_runs    INTEGER NOT NULL DEFAULT 0,
		runs_so_far INTEGER NOT NULL DEFAULT 0,
		stop_at     TEXT    NOT NULL DEFAULT '',
		enabled     INTEGER NOT NULL DEFAULT 1,
		metadata    TEXT    NOT NULL DEFAULT '',

		valid_from  TEXT    NOT NULL,
		valid_to    TEXT    NOT NULL DEFAULT '',
		created_by  TEXT    NOT NULL DEFAULT '',
		reason      TEXT    NOT NULL DEFAULT '',
		created_at  TEXT    NOT NULL
	)`); err != nil {
		return fmt.Errorf("create schedules table: %w", err)
	}

	// Hot path: "what schedules apply to this entity right now?"
	if _, err := s.db.Exec(
		`CREATE INDEX IF NOT EXISTS idx_sched_entity_current ON schedules(entity_id, entity_kind) WHERE valid_to = ''`,
	); err != nil {
		return fmt.Errorf("create idx_sched_entity_current: %w", err)
	}
	// Scheduler tick driver — "what's due to fire?"
	if _, err := s.db.Exec(
		`CREATE INDEX IF NOT EXISTS idx_sched_due_current ON schedules(run_at) WHERE valid_to = '' AND enabled = 1`,
	); err != nil {
		return fmt.Errorf("create idx_sched_due_current: %w", err)
	}
	// Time-travel reader — "what schedule applied at time T?"
	if _, err := s.db.Exec(
		`CREATE INDEX IF NOT EXISTS idx_sched_asof ON schedules(entity_kind, valid_from, valid_to)`,
	); err != nil {
		return fmt.Errorf("create idx_sched_asof: %w", err)
	}
	// History reader — chronological audit trail per entity.
	if _, err := s.db.Exec(
		`CREATE INDEX IF NOT EXISTS idx_sched_history ON schedules(entity_id, valid_from DESC)`,
	); err != nil {
		return fmt.Errorf("create idx_sched_history: %w", err)
	}

	// NOTE: legacy task scheduling fields (tasks.next_run_at, tasks.schedule)
	// stay live during this PR — the runner still reads them. The follow-up
	// PR cuts the runner over to read from the schedules table and runs a
	// one-time backfill at that point. This PR keeps init() side-effect-
	// free aside from the migration so an op-restart with no operator
	// activity is genuinely a no-op.
	return nil
}

// rowColumns is the SELECT projection shared by every reader. Centralised
// so a future column add lands in one place; matches the order in
// scanSchedule.
const rowColumns = `id, entity_kind, entity_id, run_at, cron_expr, timezone,
	max_runs, runs_so_far, stop_at, enabled, metadata,
	valid_from, valid_to, created_by, reason, created_at`

// scanSchedule decodes one row from the rowColumns projection. Stored as
// INTEGER; cast to bool in Go since SQLite has no native bool.
func scanSchedule(rows *sql.Rows) (*Schedule, error) {
	var s Schedule
	var enabledInt int
	if err := rows.Scan(
		&s.ID, &s.EntityKind, &s.EntityID, &s.RunAt, &s.CronExpr, &s.Timezone,
		&s.MaxRuns, &s.RunsSoFar, &s.StopAt, &enabledInt, &s.Metadata,
		&s.ValidFrom, &s.ValidTo, &s.CreatedBy, &s.Reason, &s.CreatedAt,
	); err != nil {
		return nil, err
	}
	s.Enabled = enabledInt != 0
	return &s, nil
}

// validateCreate enforces Go-side invariants. Schema is portable — no
// CHECK constraints — so this is the ONLY gate.
func validateCreate(in *Schedule) error {
	if in.EntityID == "" {
		return ErrEmptyEntityID
	}
	if !validKind(in.EntityKind) {
		return fmt.Errorf("%w: entity_kind=%q", ErrInvalidKind, in.EntityKind)
	}
	if strings.TrimSpace(in.RunAt) == "" {
		return ErrEmptyRunAt
	}
	return nil
}

// Create opens a new active schedule row for the given entity. Multiple
// active rows per entity ARE allowed (an entity can have several
// concurrent recurrence rules — e.g. "weekly on Mon AND every 4h on
// weekdays"); replacing a single rule is the operator pattern of
// Close(old) + Create(new).
//
// On input:
//   - ID is ignored (autoincremented).
//   - Timezone defaults to "UTC" if blank.
//   - Enabled defaults to true unless explicitly false.
//   - ValidFrom defaults to now() if blank.
//   - ValidTo is always written as '' (active).
//   - CreatedAt is always overwritten with now().
//
// Returns the persisted row (with ID + CreatedAt populated).
func (s *Store) Create(ctx context.Context, in *Schedule) (*Schedule, error) {
	if in == nil {
		return nil, fmt.Errorf("schedule: Create called with nil input")
	}
	if err := validateCreate(in); err != nil {
		return nil, err
	}

	s.createMu.Lock()
	defer s.createMu.Unlock()

	now := nowUTC()
	validFrom := in.ValidFrom
	if validFrom == "" {
		validFrom = now
	}
	tz := in.Timezone
	if tz == "" {
		tz = "UTC"
	}
	// Caller-controlled. The HTTP / MCP layer is responsible for
	// defaulting Enabled to true on create input — Go's zero-value bool
	// is false, and we don't want a "user posted a fresh JSON without
	// the field" to silently land disabled. The HTTP handler sets the
	// default before calling Create.
	enabledInt := 0
	if in.Enabled {
		enabledInt = 1
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO schedules
			(entity_kind, entity_id, run_at, cron_expr, timezone,
			 max_runs, runs_so_far, stop_at, enabled, metadata,
			 valid_from, valid_to, created_by, reason, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?)`,
		in.EntityKind, in.EntityID, in.RunAt, in.CronExpr, tz,
		in.MaxRuns, in.RunsSoFar, in.StopAt, enabledInt, in.Metadata,
		validFrom, in.CreatedBy, in.Reason, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert schedule: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("schedule LastInsertId: %w", err)
	}

	out := *in
	out.ID = id
	out.Timezone = tz
	out.Enabled = enabledInt != 0
	out.ValidFrom = validFrom
	out.ValidTo = ""
	out.CreatedAt = now
	return &out, nil
}

// ListCurrent returns every CURRENT schedule for the (entityKind, entityID)
// pair, ordered by run_at ASC so the dashboard can render the next-fire
// at the top.
func (s *Store) ListCurrent(ctx context.Context, entityKind, entityID string) ([]*Schedule, error) {
	if entityID == "" {
		return nil, ErrEmptyEntityID
	}
	if !validKind(entityKind) {
		return nil, fmt.Errorf("%w: entity_kind=%q", ErrInvalidKind, entityKind)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+rowColumns+` FROM schedules
		 WHERE entity_id = ? AND entity_kind = ? AND valid_to = ''
		 ORDER BY run_at ASC`,
		entityID, entityKind)
	if err != nil {
		return nil, fmt.Errorf("query current schedules: %w", err)
	}
	defer rows.Close()

	out := make([]*Schedule, 0, 4)
	for rows.Next() {
		row, err := scanSchedule(rows)
		if err != nil {
			return nil, fmt.Errorf("scan current: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// ListHistory returns every row (active + closed) for the entity,
// ordered by valid_from DESC so the most recent change is first. Drives
// the dashboard's per-entity audit trail.
func (s *Store) ListHistory(ctx context.Context, entityKind, entityID string) ([]*Schedule, error) {
	if entityID == "" {
		return nil, ErrEmptyEntityID
	}
	if !validKind(entityKind) {
		return nil, fmt.Errorf("%w: entity_kind=%q", ErrInvalidKind, entityKind)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+rowColumns+` FROM schedules
		 WHERE entity_id = ? AND entity_kind = ?
		 ORDER BY valid_from DESC`,
		entityID, entityKind)
	if err != nil {
		return nil, fmt.Errorf("query history schedules: %w", err)
	}
	defer rows.Close()

	out := make([]*Schedule, 0, 8)
	for rows.Next() {
		row, err := scanSchedule(rows)
		if err != nil {
			return nil, fmt.Errorf("scan history: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// Close marks the schedule with the given ID as superseded by setting
// valid_to to now(). Append-only — never DELETE; replacing a schedule
// is Close(old) + Create(new). Idempotent: closing an already-closed
// row is a no-op (zero rows affected; no error).
//
// by + reason populate the same columns as on Create. valid_to != ''
// disambiguates whether attribution describes the open or the close —
// matches relationships.Store.Remove's convention.
func (s *Store) Close(ctx context.Context, id int64, reason, by string) error {
	if id <= 0 {
		return fmt.Errorf("schedule: Close requires positive id, got %d", id)
	}
	now := nowUTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE schedules
		   SET valid_to = ?, created_by = ?, reason = ?
		 WHERE id = ? AND valid_to = ''`,
		now, by, reason, id)
	if err != nil {
		return fmt.Errorf("close schedule: %w", err)
	}
	return nil
}

// validateAsOf parses an ISO8601 timestamp; empty is allowed (means
// "now / current state"). Mirrors relationships.validateAsOf.
func validateAsOf(asOf string) error {
	if asOf == "" {
		return nil
	}
	if _, err := time.Parse(time.RFC3339Nano, asOf); err != nil {
		return fmt.Errorf("schedule: as_of must be ISO8601 (RFC3339), got %q", asOf)
	}
	return nil
}

// ListAsOf returns the schedule rows that were CURRENT at the given
// asOf timestamp. Predicate:
//
//	valid_from <= asOf AND (valid_to > asOf OR valid_to = '')
//
// Empty asOf delegates to ListCurrent for partial-index speed.
//
// Use case: dashboard time-slider — "what schedules applied to this
// task last Tuesday?" — same shape as relationships.ListOutgoingAt.
func (s *Store) ListAsOf(ctx context.Context, ts time.Time, entityKind, entityID string) ([]*Schedule, error) {
	if entityID == "" {
		return nil, ErrEmptyEntityID
	}
	if !validKind(entityKind) {
		return nil, fmt.Errorf("%w: entity_kind=%q", ErrInvalidKind, entityKind)
	}
	asOf := ""
	if !ts.IsZero() {
		asOf = ts.UTC().Format(time.RFC3339Nano)
	}
	if err := validateAsOf(asOf); err != nil {
		return nil, err
	}
	if asOf == "" {
		return s.ListCurrent(ctx, entityKind, entityID)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+rowColumns+` FROM schedules
		 WHERE entity_id = ? AND entity_kind = ?
		   AND valid_from <= ?
		   AND (valid_to > ? OR valid_to = '')
		 ORDER BY run_at ASC`,
		entityID, entityKind, asOf, asOf)
	if err != nil {
		return nil, fmt.Errorf("query as-of schedules: %w", err)
	}
	defer rows.Close()

	out := make([]*Schedule, 0, 4)
	for rows.Next() {
		row, err := scanSchedule(rows)
		if err != nil {
			return nil, fmt.Errorf("scan as-of: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// Get returns the schedule by ID, including closed rows. Returns
// (nil, false, nil) if no row matches. Used by Close-confirmation flows
// that need to display the row before/after closing.
func (s *Store) Get(ctx context.Context, id int64) (*Schedule, bool, error) {
	if id <= 0 {
		return nil, false, fmt.Errorf("schedule: Get requires positive id, got %d", id)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+rowColumns+` FROM schedules WHERE id = ? LIMIT 1`, id)
	if err != nil {
		return nil, false, fmt.Errorf("get schedule: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if rerr := rows.Err(); rerr != nil {
			return nil, false, fmt.Errorf("get iterate: %w", rerr)
		}
		return nil, false, nil
	}
	row, err := scanSchedule(rows)
	if err != nil {
		return nil, false, fmt.Errorf("scan get: %w", err)
	}
	if rerr := rows.Err(); rerr != nil {
		return nil, false, fmt.Errorf("get final iterate: %w", rerr)
	}
	return row, true, nil
}

// debug logs a slow query. Threshold matches relationships'.
const slowQueryThreshold = 100 * time.Millisecond

// timed wraps a query body and logs to slog if it exceeds the threshold.
// Currently unused — wire if the future scheduler tick reads ListCurrent
// at high QPS. Keeping the helper here so debugging follows the
// relationships package's conventions if anyone reaches for it.
func timed(label string, body func() error) error {
	start := time.Now()
	err := body()
	if elapsed := time.Since(start); elapsed > slowQueryThreshold {
		slog.Warn("schedule slow query", "op", label, "elapsed_ms", elapsed.Milliseconds())
	}
	return err
}

var _ = timed // suppress unused warning until the runner picks it up

// ListAllCurrent returns every CURRENT, ENABLED schedule across all
// entities whose RunsSoFar has not yet hit MaxRuns. Drives the
// in-memory cron registration on Runner.Reload.
//
// Hot path: filtered by the partial index `idx_sched_due_current` so
// the scan is bounded to current+enabled rows. No tick-time row filter
// — the cron library owns the "is this fire-due" decision in memory
// against each row's parsed cron expr / one-shot run_at.
func (s *Store) ListAllCurrent(ctx context.Context) ([]*Schedule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+rowColumns+` FROM schedules
		 WHERE valid_to = ''
		   AND enabled = 1
		   AND (max_runs = 0 OR runs_so_far < max_runs)
		 ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list all current: %w", err)
	}
	defer rows.Close()

	out := make([]*Schedule, 0, 16)
	for rows.Next() {
		row, err := scanSchedule(rows)
		if err != nil {
			return nil, fmt.Errorf("scan current: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// MarkFired increments runs_so_far for the given schedule id. If
// max_runs > 0 and the new runs_so_far reaches max_runs, also flips
// enabled to 0 so the next Reload skips it.
//
// One-shot schedules (cron_expr='') typically have max_runs=1 and
// self-disable on the first MarkFired. Recurring (cron_expr non-empty)
// schedules with max_runs=0 fire indefinitely.
func (s *Store) MarkFired(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("schedule: MarkFired requires positive id, got %d", id)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE schedules
		 SET runs_so_far = runs_so_far + 1,
		     enabled = CASE
		         WHEN max_runs > 0 AND runs_so_far + 1 >= max_runs THEN 0
		         ELSE enabled
		     END
		 WHERE id = ? AND valid_to = ''`,
		id)
	if err != nil {
		return fmt.Errorf("mark fired: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Already closed or id invalid — surface as a soft warning so
		// the consumer can ignore it without bubbling.
		slog.Debug("schedule MarkFired: zero rows affected", "id", id)
	}
	return nil
}
