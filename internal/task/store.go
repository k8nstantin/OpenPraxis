package task

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/relationships"
)

// Task represents scheduled work against a manifest.
type Task struct {
	ID          string    `json:"id"`
	Marker      string    `json:"marker"`
	ManifestID  string    `json:"manifest_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Schedule    string    `json:"schedule"`   // "once", "5m", "1h", cron expression
	// Status is one of the 8 canonical lifecycle states defined in
	// status.go: pending, waiting, scheduled, running, paused, completed,
	// failed, cancelled. See status.go's validTransitions map for the
	// allowed edges of the state machine. Writes go through
	// Store.UpdateStatus, which validates transitions and rejects
	// illegal moves.
	Status      string    `json:"status"`
	Agent       string    `json:"agent"`      // agent type: claude-code, cursor, etc.
	SourceNode  string    `json:"source_node"`
	CreatedBy   string    `json:"created_by"` // session or dashboard
	DependsOn   string    `json:"depends_on"`    // task ID that must complete before this runs
	BlockReason string    `json:"block_reason"`  // reason task is blocked (e.g. manifest dependency)
	RunCount    int       `json:"run_count"`
	LastRunAt   string    `json:"last_run_at"`
	NextRunAt   string    `json:"next_run_at"`
	LastOutput  string    `json:"last_output"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Computed from task_runs — populated by enrichWithCosts() (cheap
	// turns + cost) or EnrichRunStats() (single-get path: also actions
	// + tokens). Tasks are leaves in the product → manifest → task tree
	// so the sum is over this task's runs only — no recursive walk.
	TotalTurns   int     `json:"total_turns"`
	TotalCost    float64 `json:"total_cost"`
	TotalActions int     `json:"total_actions"`
	TotalTokens  int     `json:"total_tokens"`
}

// TaskRun represents a single execution of a task, preserving history.
type TaskRun struct {
	ID                int       `json:"id"`
	TaskID            string    `json:"task_id"`
	RunNumber         int       `json:"run_number"`
	Output            string    `json:"output"`
	Status            string    `json:"status"`
	Actions           int       `json:"actions"`
	Lines             int       `json:"lines"`
	CostUSD           float64   `json:"cost_usd"`
	Turns             int       `json:"turns"`
	InputTokens       int       `json:"input_tokens"`
	OutputTokens      int       `json:"output_tokens"`
	CacheReadTokens   int       `json:"cache_read_tokens"`
	CacheCreateTokens int       `json:"cache_create_tokens"`
	Model             string    `json:"model"`
	PricingVersion    string    `json:"pricing_version"`
	// Host-metrics summary. Full time-series on task_run_host_samples,
	// fetched separately for the Run Stats sparkline overlay.
	PeakCPUPct float64   `json:"peak_cpu_pct"`
	AvgCPUPct  float64   `json:"avg_cpu_pct"`
	PeakRSSMB  float64   `json:"peak_rss_mb"`
	StartedAt  time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`

	// Stats-tab columns. Powers the per-entity Stats panel without
	// re-parsing output blobs at read time. Populated by RecordRun
	// + post-completion fillers (errors / compactions / files /
	// commits / pr_number / branch / commit_sha).
	Errors        int     `json:"errors"`
	Compactions   int     `json:"compactions"`
	FilesChanged  int     `json:"files_changed"`
	ExitCode      int     `json:"exit_code"`
	CancelledAt   string  `json:"cancelled_at"`
	CancelledBy   string  `json:"cancelled_by"`
	DurationMS    int64   `json:"duration_ms"`
	AvgRSSMB      float64 `json:"avg_rss_mb"`
	Branch        string  `json:"branch"`
	CommitSHA     string  `json:"commit_sha"`
	Commits       int     `json:"commits"`
	PRNumber      int     `json:"pr_number"`
	WorktreePath  string  `json:"worktree_path"`
	AgentRuntime  string  `json:"agent_runtime"`
	AgentVersion  string  `json:"agent_version"`

	// Per-run code-churn split. Backfill is impossible for legacy rows
	// (data wasn't captured) — they stay 0; new runs populate going
	// forward when the runner extracts these from the agent's git diff
	// stat. The pre-existing `Lines` column above stays as the total
	// (= LinesAdded + LinesRemoved for new runs).
	LinesAdded   int `json:"lines_added"`
	LinesRemoved int `json:"lines_removed"`
}

// Store manages task persistence.
type Store struct {
	db              *sql.DB
	manifestChecker ManifestReadinessChecker // nil = skip manifest-level satisfaction check
	productChecker  ProductReadinessChecker  // nil = skip product-level satisfaction check
	reviewWriter    ReviewWriter             // nil = reject/approve return ErrReviewNotAvailable
	reviewReader    ReviewReader             // nil = TaskReviewStatus returns empty without error
	// rels is the unified relationships SCD-2 store. After PR/M3 every
	// task→task dependency mutation lands here AND on the legacy
	// tasks.depends_on cache column (out of scope for this PR — kept
	// because the scheduler + runner read it directly). The legacy
	// task_dependency SCD audit table is dormant.
	rels *relationships.Store
}

// SetRelationshipsBackend wires the unified relationships SCD-2 store
// for task→task dependency edges. Call once at startup before any
// SetDependency mutation runs.
func (s *Store) SetRelationshipsBackend(r *relationships.Store) {
	s.rels = r
}

// NewStore creates a task store. Auto-wires a default relationships
// backend against the same DB handle so tests + any caller that
// doesn't explicitly call SetRelationshipsBackend get a working dep
// API. node-level wiring overrides this with the shared singleton.
func NewStore(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	// Init the relationships backend BEFORE init() so the
	// BackfillTaskDepSCD call inside init() can see the unified store
	// and seed it from the legacy tasks.depends_on cache column.
	rels, err := relationships.New(db)
	if err != nil {
		return nil, fmt.Errorf("init relationships backend: %w", err)
	}
	s.rels = rels
	if err := s.init(); err != nil {
		return nil, err
	}
	return s, nil
}

// DB returns the underlying database connection for cross-store queries.
func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) init() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		manifest_id TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		schedule TEXT NOT NULL DEFAULT 'once',
		status TEXT NOT NULL DEFAULT 'pending',
		agent TEXT NOT NULL DEFAULT 'claude-code',
		source_node TEXT NOT NULL DEFAULT '',
		created_by TEXT NOT NULL DEFAULT '',
		run_count INTEGER NOT NULL DEFAULT 0,
		last_run_at TEXT NOT NULL DEFAULT '',
		next_run_at TEXT NOT NULL DEFAULT '',
		last_output TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		deleted_at TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		return fmt.Errorf("create tasks table: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_tasks_manifest ON tasks(manifest_id)`)
	if err != nil {
		return fmt.Errorf("create tasks manifest index: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status)`)
	if err != nil {
		return fmt.Errorf("create tasks status index: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_tasks_next_run ON tasks(next_run_at)`)
	if err != nil {
		return err
	}
	s.db.Exec(`ALTER TABLE tasks ADD COLUMN deleted_at TEXT NOT NULL DEFAULT ''`)
	// NOTE: max_turns column is retired in M4-T14. The migration routine
	// (task.MigrateMaxTurnsToSettings + task.DropMaxTurnsColumn in
	// node.New) copies prior column values into settings rows at task scope
	// and then drops the column. No ADD COLUMN here — fresh installs never
	// see the column, and upgrades have it removed before any Store query
	// that references the new taskColumns list runs.
	s.db.Exec(`ALTER TABLE tasks ADD COLUMN depends_on TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE tasks ADD COLUMN block_reason TEXT NOT NULL DEFAULT ''`)
	// Cross-process action signal: 'pause' | 'resume' | 'cancel'.
	// MCP sets this; serve's runner watches and acts on the task process it owns.
	s.db.Exec(`ALTER TABLE tasks ADD COLUMN action_request TEXT NOT NULL DEFAULT ''`)

	// Task runs history table
	_, err = s.db.Exec(`CREATE TABLE IF NOT EXISTS task_runs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id TEXT NOT NULL,
		run_number INTEGER NOT NULL,
		output TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT '',
		actions INTEGER NOT NULL DEFAULT 0,
		lines INTEGER NOT NULL DEFAULT 0,
		started_at TEXT NOT NULL,
		completed_at TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		return fmt.Errorf("create task_runs table: %w", err)
	}
	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_task_runs_task ON task_runs(task_id)`)
	if err != nil {
		return fmt.Errorf("create task_runs index: %w", err)
	}

	// Migrate: add cost_usd and turns columns to task_runs
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN cost_usd REAL NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN turns INTEGER NOT NULL DEFAULT 0`)
	// Token + model + pricing-version denorm. Source of truth stays the
	// raw stream-json in task_runs.output; these columns let dashboards
	// avoid re-parsing on every read and let the future Unified Cost
	// Tracking product (019dab45-d8f) recompute cost retroactively under
	// a different pricing table.
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN input_tokens INTEGER NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN output_tokens INTEGER NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN cache_read_tokens INTEGER NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN cache_create_tokens INTEGER NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN model TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN pricing_version TEXT NOT NULL DEFAULT ''`)

	// Host-metrics denorm. The full time-series lives in
	// task_run_host_samples below; these columns are the rollup used by
	// list views and the Run Stats card summary numbers.
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN peak_cpu_pct REAL NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN avg_cpu_pct REAL NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN peak_rss_mb REAL NOT NULL DEFAULT 0`)

	// Stats-tab denorm columns (PR/M-Stats). Each ALTER is wrapped in a
	// best-effort Exec because SQLite's ADD COLUMN is idempotent only via
	// "duplicate column" failure — same pattern as the columns above.
	// Powers /api/run-stats Cumulative + Per-run panels without re-parsing
	// the raw output blob on every read.
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN errors INTEGER NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN compactions INTEGER NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN files_changed INTEGER NOT NULL DEFAULT 0`)
	// Per-run code-churn split. Existing `lines` column stays as the total
	// (= lines_added + lines_removed for new runs). Backfill is impossible
	// for legacy rows — they keep 0; new runs populate going forward when
	// the runner extracts these from the agent's git diff stat. Powers the
	// Stats tab Per-run "Git + output" card (+added / −removed next to
	// files_changed) and the Cumulative panel's "Code churn per run" chart.
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN lines_added INTEGER NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN lines_removed INTEGER NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN exit_code INTEGER NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN cancelled_at TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN cancelled_by TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN duration_ms INTEGER NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN avg_rss_mb REAL NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN branch TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN commit_sha TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN commits INTEGER NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN pr_number INTEGER NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN worktree_path TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN agent_runtime TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE task_runs ADD COLUMN agent_version TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_task_runs_started ON task_runs(started_at DESC)`)

	// Backfill duration_ms for legacy completed rows so the Stats tab
	// has values to plot for runs written before this migration. Idempotent —
	// the WHERE clause skips rows already populated.
	s.db.Exec(`UPDATE task_runs SET duration_ms = CAST((julianday(completed_at) - julianday(started_at)) * 86400000 AS INTEGER) WHERE duration_ms = 0 AND completed_at != '' AND started_at != ''`)

	// Per-run host-metrics time-series. One row per sample (default 5s
	// cadence). Feeds the CPU/RSS sparklines overlaid on the Run Stats
	// card — same timeline as turn/actions/cost.
	_, err = s.db.Exec(`CREATE TABLE IF NOT EXISTS task_run_host_samples (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		run_id INTEGER NOT NULL,
		ts TEXT NOT NULL,
		cpu_pct REAL NOT NULL DEFAULT 0,
		rss_mb REAL NOT NULL DEFAULT 0
	)`)
	if err != nil {
		return fmt.Errorf("create task_run_host_samples table: %w", err)
	}
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_task_run_host_samples_run ON task_run_host_samples(run_id)`)

	// Stats-tab additions: per-run disk capture. The HostSampler now reads
	// disk usage at each tick alongside CPU/RSS so the per-run timeline can
	// plot disk-pressure overlays (e.g. an agent ballooning the worktree).
	s.db.Exec(`ALTER TABLE task_run_host_samples ADD COLUMN disk_used_gb REAL NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE task_run_host_samples ADD COLUMN disk_total_gb REAL NOT NULL DEFAULT 0`)

	// system_host_samples — continuous capacity stream from the
	// SystemSampler started in cmd/serve.go. One row per tick (default
	// host_sampler_tick_seconds=5s). Independent of any task; powers the
	// System Capacity panel on the Stats tab.
	_, err = s.db.Exec(`CREATE TABLE IF NOT EXISTS system_host_samples (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		ts            TEXT NOT NULL,
		cpu_pct       REAL NOT NULL DEFAULT 0,
		load_1m       REAL NOT NULL DEFAULT 0,
		load_5m       REAL NOT NULL DEFAULT 0,
		load_15m      REAL NOT NULL DEFAULT 0,
		mem_used_mb   REAL NOT NULL DEFAULT 0,
		mem_total_mb REAL NOT NULL DEFAULT 0,
		swap_used_mb REAL NOT NULL DEFAULT 0,
		disk_used_gb  REAL NOT NULL DEFAULT 0,
		disk_total_gb REAL NOT NULL DEFAULT 0,
		net_rx_mbps   REAL NOT NULL DEFAULT 0,
		net_tx_mbps   REAL NOT NULL DEFAULT 0,
		disk_read_mbps  REAL NOT NULL DEFAULT 0,
		disk_write_mbps REAL NOT NULL DEFAULT 0
	)`)
	if err != nil {
		return fmt.Errorf("create system_host_samples table: %w", err)
	}
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_sys_samples_ts ON system_host_samples(ts DESC)`)
	// Idempotent ALTERs for older DBs that pre-date the disk-IO columns.
	s.db.Exec(`ALTER TABLE system_host_samples ADD COLUMN disk_read_mbps REAL NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE system_host_samples ADD COLUMN disk_write_mbps REAL NOT NULL DEFAULT 0`)

	// Running task runtime state — persists in-memory RunningTask data to survive restarts
	_, err = s.db.Exec(`CREATE TABLE IF NOT EXISTS task_runtime_state (
		task_id TEXT PRIMARY KEY,
		marker TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL DEFAULT '',
		manifest TEXT NOT NULL DEFAULT '',
		agent TEXT NOT NULL DEFAULT '',
		pid INTEGER NOT NULL DEFAULT 0,
		paused INTEGER NOT NULL DEFAULT 0,
		actions INTEGER NOT NULL DEFAULT 0,
		lines INTEGER NOT NULL DEFAULT 0,
		last_line TEXT NOT NULL DEFAULT '',
		started_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("create task_runtime_state table: %w", err)
	}

	// Many-to-many: task ↔ manifest link table
	_, err = s.db.Exec(`CREATE TABLE IF NOT EXISTS task_manifests (
		task_id TEXT NOT NULL,
		manifest_id TEXT NOT NULL,
		linked_at TEXT NOT NULL,
		PRIMARY KEY (task_id, manifest_id)
	)`)
	if err != nil {
		return fmt.Errorf("create task_manifests table: %w", err)
	}

	// SCD Type 2 for the task→task dependency relationship. Every
	// SetDependency call closes the current row (sets valid_to) and
	// inserts a fresh row (valid_from=now, valid_to=NULL). The
	// "current" dep is whatever row has valid_to IS NULL for that
	// task; any prior state is recoverable by querying rows ordered
	// by valid_from. tasks.depends_on stays as a cache of the current
	// row so existing queries keep working without the SCD join.
	//
	// depends_on='' represents "no dep" — a cleared dep writes a row
	// with depends_on='' rather than leaving the table unchanged, so
	// an operator can see that a dep was deliberately removed (vs.
	// never set).
	_, err = s.db.Exec(`CREATE TABLE IF NOT EXISTS task_dependency (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id TEXT NOT NULL,
		depends_on TEXT NOT NULL DEFAULT '',
		valid_from TEXT NOT NULL,
		valid_to TEXT NOT NULL DEFAULT '',
		changed_by TEXT NOT NULL DEFAULT '',
		reason TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		return fmt.Errorf("create task_dependency table: %w", err)
	}
	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_task_dep_task ON task_dependency(task_id, valid_from DESC)`)
	if err != nil {
		return fmt.Errorf("create task_dep index: %w", err)
	}
	// Partial index for the current-dep lookup path. valid_to='' means
	// "still active" in our SCD encoding (we use empty string not NULL
	// for consistency with the rest of the schema).
	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_task_dep_current ON task_dependency(task_id) WHERE valid_to = ''`)
	if err != nil {
		return fmt.Errorf("create task_dep current index: %w", err)
	}

	if err := s.initPricingSchema(); err != nil {
		return fmt.Errorf("create model_pricing table: %w", err)
	}

	// Legacy tasks with a non-empty depends_on but no SCD row yet get
	// one seeded here so the dep history stream isn't blank on
	// upgrade. Idempotent — the NOT EXISTS guard in the backfill
	// makes repeat runs a no-op.
	// Idempotent migration: rewrite legacy block_reason prefixes to
	// the canonical form. Rows written before #97 normalized
	// node.go:615 carry "blocked by manifest <marker> (<title>)".
	// The activation walker's filter accepts both, but normalizing
	// in place lets us drop the compatibility clause in a later
	// release and keeps operator-visible text consistent.
	//
	// SQLite's REPLACE doesn't support prefix-rewrites directly;
	// use a CASE expression gated on LIKE so rows without the
	// legacy prefix aren't touched (idempotent on repeat boot).
	if _, err := s.db.Exec(`
		UPDATE tasks
		SET block_reason = 'manifest not satisfied — blocked by: ' ||
		    substr(block_reason, length('blocked by manifest ') + 1),
		    updated_at = ?
		WHERE block_reason LIKE 'blocked by manifest %'
		  AND deleted_at = ''`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("normalize legacy block_reason prefixes: %w", err)
	}

	if _, err := s.BackfillTaskDepSCD(); err != nil {
		return fmt.Errorf("backfill task dep SCD: %w", err)
	}

	return nil
}

// taskColumns is the standard column list for task SELECT queries.
const taskColumns = `id, manifest_id, title, description, schedule, status, agent, source_node, created_by, depends_on, block_reason, run_count, last_run_at, next_run_at, last_output, created_at, updated_at`

func scanTask(row *sql.Row) (*Task, error) {
	var t Task
	var createdStr, updatedStr string
	err := row.Scan(&t.ID, &t.ManifestID, &t.Title, &t.Description, &t.Schedule, &t.Status, &t.Agent, &t.SourceNode, &t.CreatedBy,
		&t.DependsOn, &t.BlockReason, &t.RunCount, &t.LastRunAt, &t.NextRunAt, &t.LastOutput, &createdStr, &updatedStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	if len(t.ID) >= 12 {
		t.Marker = t.ID[:12]
	}
	return &t, nil
}

func scanTasks(rows *sql.Rows) ([]*Task, error) {
	var tasks []*Task
	for rows.Next() {
		var t Task
		var createdStr, updatedStr string
		err := rows.Scan(&t.ID, &t.ManifestID, &t.Title, &t.Description, &t.Schedule, &t.Status, &t.Agent, &t.SourceNode, &t.CreatedBy,
			&t.DependsOn, &t.BlockReason, &t.RunCount, &t.LastRunAt, &t.NextRunAt, &t.LastOutput, &createdStr, &updatedStr)
		if err != nil {
			return nil, err
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
		if len(t.ID) >= 12 {
			t.Marker = t.ID[:12]
		}
		tasks = append(tasks, &t)
	}
	return tasks, rows.Err()
}

// taskRunsColumns is the canonical column list for task_runs SELECTs.
// Kept in one place so adding a denormalised column doesn't need a sweep
// across ListRuns / GetRun / ListAllRuns; scanRuns / scanRun read in
// the same order.
const taskRunsColumns = `id, task_id, run_number, output, status, actions, lines, cost_usd, turns, started_at, completed_at,
	input_tokens, output_tokens, cache_read_tokens, cache_create_tokens, model, pricing_version,
	peak_cpu_pct, avg_cpu_pct, peak_rss_mb,
	errors, compactions, files_changed, exit_code, cancelled_at, cancelled_by,
	duration_ms, avg_rss_mb, branch, commit_sha, commits, pr_number,
	worktree_path, agent_runtime, agent_version,
	lines_added, lines_removed`

func scanRuns(rows *sql.Rows) ([]TaskRun, error) {
	var runs []TaskRun
	for rows.Next() {
		var r TaskRun
		var startedStr, completedStr string
		if err := rows.Scan(&r.ID, &r.TaskID, &r.RunNumber, &r.Output, &r.Status, &r.Actions, &r.Lines, &r.CostUSD, &r.Turns, &startedStr, &completedStr,
			&r.InputTokens, &r.OutputTokens, &r.CacheReadTokens, &r.CacheCreateTokens, &r.Model, &r.PricingVersion,
			&r.PeakCPUPct, &r.AvgCPUPct, &r.PeakRSSMB,
			&r.Errors, &r.Compactions, &r.FilesChanged, &r.ExitCode, &r.CancelledAt, &r.CancelledBy,
			&r.DurationMS, &r.AvgRSSMB, &r.Branch, &r.CommitSHA, &r.Commits, &r.PRNumber,
			&r.WorktreePath, &r.AgentRuntime, &r.AgentVersion,
			&r.LinesAdded, &r.LinesRemoved); err != nil {
			return nil, err
		}
		r.StartedAt, _ = time.Parse(time.RFC3339, startedStr)
		r.CompletedAt, _ = time.Parse(time.RFC3339, completedStr)
		runs = append(runs, r)
	}
	return runs, rows.Err()
}
