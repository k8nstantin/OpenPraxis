package task

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/relationships"
)

// Task represents scheduled work against a manifest.
type Task struct {
	ID          string    `json:"id"`
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
	Agent       string    `json:"agent"`      // agent type: gemini-cli, claude-code, cursor, etc.
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
	// rels is the unified relationships SCD-2 store. After PR/M3 every
	// task→task dependency mutation lands here AND on the legacy
	// tasks.depends_on cache column (out of scope for this PR — kept
	// because the scheduler + runner read it directly). The legacy
	// task_dependency SCD audit table is dormant.
	rels         *relationships.Store
	reviewWriter ReviewWriter
	reviewReader ReviewReader
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
	// The tasks table has been retired — all task data lives in the
	// entities table. No CREATE TABLE tasks here.

	// system_host_samples — continuous capacity stream from the
	// SystemSampler started in cmd/serve.go. One row per tick (default
	// host_sampler_tick_seconds=5s). Independent of any task; powers the
	// System Capacity panel on the Stats tab.
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS system_host_samples (
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

	if err := s.initPricingSchema(); err != nil {
		return fmt.Errorf("create model_pricing table: %w", err)
	}

	return nil
}

// taskColumns is the standard column list for task SELECT queries.
// PR/M3 dropped manifest_id; Task.ManifestID is now populated post-scan
// via populateOwnership using the relationships store.
const taskColumns = `id, title, description, schedule, status, agent, source_node, created_by, depends_on, block_reason, run_count, last_run_at, next_run_at, last_output, created_at, updated_at`

func scanTask(row *sql.Row) (*Task, error) {
	var t Task
	var createdStr, updatedStr string
	err := row.Scan(&t.ID, &t.Title, &t.Description, &t.Schedule, &t.Status, &t.Agent, &t.SourceNode, &t.CreatedBy,
		&t.DependsOn, &t.BlockReason, &t.RunCount, &t.LastRunAt, &t.NextRunAt, &t.LastOutput, &createdStr, &updatedStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	return &t, nil
}

func scanTasks(rows *sql.Rows) ([]*Task, error) {
	var tasks []*Task
	for rows.Next() {
		var t Task
		var createdStr, updatedStr string
		err := rows.Scan(&t.ID, &t.Title, &t.Description, &t.Schedule, &t.Status, &t.Agent, &t.SourceNode, &t.CreatedBy,
			&t.DependsOn, &t.BlockReason, &t.RunCount, &t.LastRunAt, &t.NextRunAt, &t.LastOutput, &createdStr, &updatedStr)
		if err != nil {
			return nil, err
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
		tasks = append(tasks, &t)
	}
	return tasks, rows.Err()
}

// populateOwnership fills Task.ManifestID via the relationships store
// using a single batched ListIncomingForMany call. Used by every read
// path so Task consumers continue to see ManifestID populated even
// though the column was dropped in PR/M3.
func (s *Store) populateOwnership(tasks []*Task) {
	if s.rels == nil || len(tasks) == 0 {
		return
	}
	ctx := context.Background()
	ids := make([]string, 0, len(tasks))
	mapByID := make(map[string]*Task, len(tasks))
	for _, t := range tasks {
		if t == nil {
			continue
		}
		ids = append(ids, t.ID)
		mapByID[t.ID] = t
	}
	if len(ids) == 0 {
		return
	}
	byDst, err := s.rels.ListIncomingForMany(ctx, ids, relationships.EdgeOwns)
	if err != nil {
		return
	}
	for tid, edges := range byDst {
		t, ok := mapByID[tid]
		if !ok {
			continue
		}
		for _, e := range edges {
			if e.SrcKind == relationships.KindManifest {
				t.ManifestID = e.SrcID
				break
			}
		}
	}
}

// lookupOwner returns the current manifest owner of a task (or "").
func (s *Store) lookupOwner(ctx context.Context, taskID string) string {
	if s.rels == nil {
		return ""
	}
	edges, err := s.rels.ListIncoming(ctx, taskID, relationships.EdgeOwns)
	if err != nil {
		return ""
	}
	for _, e := range edges {
		if e.SrcKind == relationships.KindManifest {
			return e.SrcID
		}
	}
	return ""
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
