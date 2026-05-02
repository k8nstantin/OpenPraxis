// Package execution owns the execution_log SCD-2 store. One row per run
// of any entity kind (task, manifest, product). Replaces task_runs +
// the run-state aggregate columns on the entity tables (EL/M5 drops those).
// Schema is append-only: rows are inserted at run start, updated at
// completion — never deleted (soft-delete not needed; runs are permanent
// audit history).
package execution

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var (
	ErrRunNotFound   = errors.New("execution: run not found")
	ErrEmptyEntityID = errors.New("execution: entity_id must be non-empty")
	ErrInvalidKind   = errors.New("execution: invalid entity_kind")
)

const (
	KindTask     = "task"
	KindManifest = "manifest"
	KindProduct  = "product"
)

// Row is one row in execution_log.
type Row struct {
	ID                string  `json:"id"`
	EntityKind        string  `json:"entity_kind"`
	EntityID          string  `json:"entity_id"`
	RunNumber         int     `json:"run_number"`
	Trigger           string  `json:"trigger"`
	NodeID            string  `json:"node_id"`
	Status            string  `json:"status"`
	TerminalReason    string  `json:"terminal_reason"`
	StartedAt         int64   `json:"started_at"`        // unix ms
	CompletedAt       int64   `json:"completed_at"`      // unix ms
	DurationMS        int64   `json:"duration_ms"`
	TTFBMS            int64   `json:"ttfb_ms"`
	ExitCode          *int    `json:"exit_code"`
	Error             string  `json:"error"`
	CancelledAt       int64   `json:"cancelled_at"`
	CancelledBy       string  `json:"cancelled_by"`
	Provider          string  `json:"provider"`
	Model             string  `json:"model"`
	ModelContextSize  int     `json:"model_context_size"`
	AgentRuntime      string  `json:"agent_runtime"`
	AgentVersion      string  `json:"agent_version"`
	PricingVersion    string  `json:"pricing_version"`
	InputTokens       int     `json:"input_tokens"`
	OutputTokens      int     `json:"output_tokens"`
	CacheReadTokens   int     `json:"cache_read_tokens"`
	CacheCreateTokens int     `json:"cache_create_tokens"`
	ReasoningTokens   int     `json:"reasoning_tokens"`
	ToolUseTokens     int     `json:"tool_use_tokens"`
	CostUSD           float64 `json:"cost_usd"`
	EstimatedCostUSD  float64 `json:"estimated_cost_usd"`
	CacheSavingsUSD   float64 `json:"cache_savings_usd"`
	CacheHitRatePct   float64 `json:"cache_hit_rate_pct"`
	ContextWindowPct  float64 `json:"context_window_pct"`
	CostPerTurn       float64 `json:"cost_per_turn"`
	CostPerAction     float64 `json:"cost_per_action"`
	TokensPerTurn     float64 `json:"tokens_per_turn"`
	Turns             int     `json:"turns"`
	Actions           int     `json:"actions"`
	Errors            int     `json:"errors"`
	Compactions       int     `json:"compactions"`
	ParallelTasks     int     `json:"parallel_tasks"`
	ToolCallsJSON     string  `json:"tool_calls_json"`
	LinesAdded        int     `json:"lines_added"`
	LinesRemoved      int     `json:"lines_removed"`
	FilesChanged      int     `json:"files_changed"`
	Commits           int     `json:"commits"`
	PRNumber          *int    `json:"pr_number"`
	Branch            string  `json:"branch"`
	CommitSHA         string  `json:"commit_sha"`
	WorktreePath      string  `json:"worktree_path"`
	PeakCPUPct        float64 `json:"peak_cpu_pct"`
	AvgCPUPct         float64 `json:"avg_cpu_pct"`
	PeakRSSMB         float64 `json:"peak_rss_mb"`
	AvgRSSMB          float64 `json:"avg_rss_mb"`
	DiskUsedGB        float64 `json:"disk_used_gb"`
	LastOutput        string  `json:"last_output"`
}

// Sample is one row in execution_log_samples.
type Sample struct {
	ID         int64   `json:"id"`
	RunID      string  `json:"run_id"`
	TS         int64   `json:"ts"` // unix ms
	CPUPct     float64 `json:"cpu_pct"`
	RSSMB      float64 `json:"rss_mb"`
	DiskUsedGB float64 `json:"disk_used_gb"`
	CostUSD    float64 `json:"cost_usd"`
	Turns      int     `json:"turns"`
	Actions    int     `json:"actions"`
}

// Store owns the execution_log and execution_log_samples tables.
type Store struct{ db *sql.DB }

// NewStore returns a Store backed by db. Call InitSchema once before use.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// InitSchema creates execution_log and execution_log_samples (idempotent).
func InitSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS execution_log (
  id                  TEXT PRIMARY KEY,
  entity_kind         TEXT NOT NULL,
  entity_id           TEXT NOT NULL,
  run_number          INTEGER NOT NULL DEFAULT 0,
  trigger             TEXT NOT NULL DEFAULT '',
  node_id             TEXT NOT NULL DEFAULT '',
  status              TEXT NOT NULL DEFAULT '',
  terminal_reason     TEXT NOT NULL DEFAULT '',
  started_at          INTEGER NOT NULL DEFAULT 0,
  completed_at        INTEGER NOT NULL DEFAULT 0,
  duration_ms         INTEGER NOT NULL DEFAULT 0,
  ttfb_ms             INTEGER NOT NULL DEFAULT 0,
  exit_code           INTEGER,
  error               TEXT NOT NULL DEFAULT '',
  cancelled_at        INTEGER NOT NULL DEFAULT 0,
  cancelled_by        TEXT NOT NULL DEFAULT '',
  provider            TEXT NOT NULL DEFAULT '',
  model               TEXT NOT NULL DEFAULT '',
  model_context_size  INTEGER NOT NULL DEFAULT 0,
  agent_runtime       TEXT NOT NULL DEFAULT '',
  agent_version       TEXT NOT NULL DEFAULT '',
  pricing_version     TEXT NOT NULL DEFAULT '',
  input_tokens        INTEGER NOT NULL DEFAULT 0,
  output_tokens       INTEGER NOT NULL DEFAULT 0,
  cache_read_tokens   INTEGER NOT NULL DEFAULT 0,
  cache_create_tokens INTEGER NOT NULL DEFAULT 0,
  reasoning_tokens    INTEGER NOT NULL DEFAULT 0,
  tool_use_tokens     INTEGER NOT NULL DEFAULT 0,
  cost_usd            REAL NOT NULL DEFAULT 0,
  estimated_cost_usd  REAL NOT NULL DEFAULT 0,
  cache_savings_usd   REAL NOT NULL DEFAULT 0,
  cache_hit_rate_pct  REAL NOT NULL DEFAULT 0,
  context_window_pct  REAL NOT NULL DEFAULT 0,
  cost_per_turn       REAL NOT NULL DEFAULT 0,
  cost_per_action     REAL NOT NULL DEFAULT 0,
  tokens_per_turn     REAL NOT NULL DEFAULT 0,
  turns               INTEGER NOT NULL DEFAULT 0,
  actions             INTEGER NOT NULL DEFAULT 0,
  errors              INTEGER NOT NULL DEFAULT 0,
  compactions         INTEGER NOT NULL DEFAULT 0,
  parallel_tasks      INTEGER NOT NULL DEFAULT 0,
  tool_calls_json     TEXT NOT NULL DEFAULT '{}',
  lines_added         INTEGER NOT NULL DEFAULT 0,
  lines_removed       INTEGER NOT NULL DEFAULT 0,
  files_changed       INTEGER NOT NULL DEFAULT 0,
  commits             INTEGER NOT NULL DEFAULT 0,
  pr_number           INTEGER,
  branch              TEXT NOT NULL DEFAULT '',
  commit_sha          TEXT NOT NULL DEFAULT '',
  worktree_path       TEXT NOT NULL DEFAULT '',
  peak_cpu_pct        REAL NOT NULL DEFAULT 0,
  avg_cpu_pct         REAL NOT NULL DEFAULT 0,
  peak_rss_mb         REAL NOT NULL DEFAULT 0,
  avg_rss_mb          REAL NOT NULL DEFAULT 0,
  disk_used_gb        REAL NOT NULL DEFAULT 0,
  last_output         TEXT NOT NULL DEFAULT ''
)`,
		`CREATE INDEX IF NOT EXISTS idx_execlog_entity
  ON execution_log(entity_kind, entity_id, started_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_execlog_running
  ON execution_log(status) WHERE status = 'running'`,
		`CREATE INDEX IF NOT EXISTS idx_execlog_daily
  ON execution_log(started_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_execlog_model
  ON execution_log(model, started_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_execlog_terminal
  ON execution_log(terminal_reason, started_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_execlog_trigger
  ON execution_log(trigger, entity_kind, started_at DESC)`,
		`CREATE TABLE IF NOT EXISTS execution_log_samples (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id       TEXT NOT NULL,
  ts           INTEGER NOT NULL,
  cpu_pct      REAL NOT NULL DEFAULT 0,
  rss_mb       REAL NOT NULL DEFAULT 0,
  disk_used_gb REAL NOT NULL DEFAULT 0,
  cost_usd     REAL NOT NULL DEFAULT 0,
  turns        INTEGER NOT NULL DEFAULT 0,
  actions      INTEGER NOT NULL DEFAULT 0
)`,
		`CREATE INDEX IF NOT EXISTS idx_execlog_samples_run
  ON execution_log_samples(run_id, ts ASC)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("execution InitSchema: %w", err)
		}
	}
	return nil
}

// Insert writes a new run row. EL/M2 fills the body; stub returns nil.
func (s *Store) Insert(ctx context.Context, r Row) error {
	if r.EntityID == "" {
		return ErrEmptyEntityID
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO execution_log (
		id, entity_kind, entity_id, run_number, trigger, node_id, status,
		terminal_reason, started_at, completed_at, duration_ms, ttfb_ms,
		exit_code, error, cancelled_at, cancelled_by, provider, model,
		model_context_size, agent_runtime, agent_version, pricing_version,
		input_tokens, output_tokens, cache_read_tokens, cache_create_tokens,
		reasoning_tokens, tool_use_tokens, cost_usd, estimated_cost_usd,
		cache_savings_usd, cache_hit_rate_pct, context_window_pct,
		cost_per_turn, cost_per_action, tokens_per_turn, turns, actions,
		errors, compactions, parallel_tasks, tool_calls_json,
		lines_added, lines_removed, files_changed, commits, pr_number,
		branch, commit_sha, worktree_path,
		peak_cpu_pct, avg_cpu_pct, peak_rss_mb, avg_rss_mb, disk_used_gb,
		last_output
	) VALUES (
		?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?
	)`,
		r.ID, r.EntityKind, r.EntityID, r.RunNumber, r.Trigger, r.NodeID, r.Status,
		r.TerminalReason, r.StartedAt, r.CompletedAt, r.DurationMS, r.TTFBMS,
		r.ExitCode, r.Error, r.CancelledAt, r.CancelledBy, r.Provider, r.Model,
		r.ModelContextSize, r.AgentRuntime, r.AgentVersion, r.PricingVersion,
		r.InputTokens, r.OutputTokens, r.CacheReadTokens, r.CacheCreateTokens,
		r.ReasoningTokens, r.ToolUseTokens, r.CostUSD, r.EstimatedCostUSD,
		r.CacheSavingsUSD, r.CacheHitRatePct, r.ContextWindowPct,
		r.CostPerTurn, r.CostPerAction, r.TokensPerTurn, r.Turns, r.Actions,
		r.Errors, r.Compactions, r.ParallelTasks, r.ToolCallsJSON,
		r.LinesAdded, r.LinesRemoved, r.FilesChanged, r.Commits, r.PRNumber,
		r.Branch, r.CommitSHA, r.WorktreePath,
		r.PeakCPUPct, r.AvgCPUPct, r.PeakRSSMB, r.AvgRSSMB, r.DiskUsedGB,
		r.LastOutput,
	)
	return err
}

// UpdateCompletion applies a partial update to the row with the given id.
// EL/M2 will use this to write completion fields; stub validates id only.
func (s *Store) UpdateCompletion(ctx context.Context, id string, fields map[string]any) error {
	if id == "" {
		return ErrRunNotFound
	}
	if len(fields) == 0 {
		return nil
	}
	cols := make([]string, 0, len(fields))
	args := make([]any, 0, len(fields)+1)
	for k, v := range fields {
		cols = append(cols, k+" = ?")
		args = append(args, v)
	}
	args = append(args, id)
	q := "UPDATE execution_log SET "
	for i, c := range cols {
		if i > 0 {
			q += ", "
		}
		q += c
	}
	q += " WHERE id = ?"
	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("execution UpdateCompletion: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrRunNotFound
	}
	return nil
}

// Get returns the run row by id.
func (s *Store) Get(ctx context.Context, id string) (Row, error) {
	if id == "" {
		return Row{}, ErrRunNotFound
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+rowColumns+` FROM execution_log WHERE id = ? LIMIT 1`, id)
	if err != nil {
		return Row{}, fmt.Errorf("execution Get: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if rerr := rows.Err(); rerr != nil {
			return Row{}, rerr
		}
		return Row{}, ErrRunNotFound
	}
	return scanRow(rows)
}

// ListByEntity returns up to limit rows for the given entity, ordered started_at DESC.
func (s *Store) ListByEntity(ctx context.Context, kind, entityID string, limit int) ([]Row, error) {
	if entityID == "" {
		return nil, ErrEmptyEntityID
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+rowColumns+` FROM execution_log
		 WHERE entity_kind = ? AND entity_id = ?
		 ORDER BY started_at DESC LIMIT ?`,
		kind, entityID, limit)
	if err != nil {
		return nil, fmt.Errorf("execution ListByEntity: %w", err)
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		r, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// InsertSample appends a time-series sample for a run.
func (s *Store) InsertSample(ctx context.Context, sm Sample) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO execution_log_samples (run_id, ts, cpu_pct, rss_mb, disk_used_gb, cost_usd, turns, actions)
		 VALUES (?,?,?,?,?,?,?,?)`,
		sm.RunID, sm.TS, sm.CPUPct, sm.RSSMB, sm.DiskUsedGB, sm.CostUSD, sm.Turns, sm.Actions)
	return err
}

// ListSamples returns all samples for a run, ordered ts ASC.
func (s *Store) ListSamples(ctx context.Context, runID string) ([]Sample, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, run_id, ts, cpu_pct, rss_mb, disk_used_gb, cost_usd, turns, actions
		 FROM execution_log_samples WHERE run_id = ? ORDER BY ts ASC`,
		runID)
	if err != nil {
		return nil, fmt.Errorf("execution ListSamples: %w", err)
	}
	defer rows.Close()
	var out []Sample
	for rows.Next() {
		var sm Sample
		if err := rows.Scan(&sm.ID, &sm.RunID, &sm.TS, &sm.CPUPct, &sm.RSSMB, &sm.DiskUsedGB, &sm.CostUSD, &sm.Turns, &sm.Actions); err != nil {
			return nil, err
		}
		out = append(out, sm)
	}
	return out, rows.Err()
}

// rowColumns is the SELECT projection for execution_log rows.
const rowColumns = `id, entity_kind, entity_id, run_number, trigger, node_id, status,
	terminal_reason, started_at, completed_at, duration_ms, ttfb_ms,
	exit_code, error, cancelled_at, cancelled_by, provider, model,
	model_context_size, agent_runtime, agent_version, pricing_version,
	input_tokens, output_tokens, cache_read_tokens, cache_create_tokens,
	reasoning_tokens, tool_use_tokens, cost_usd, estimated_cost_usd,
	cache_savings_usd, cache_hit_rate_pct, context_window_pct,
	cost_per_turn, cost_per_action, tokens_per_turn, turns, actions,
	errors, compactions, parallel_tasks, tool_calls_json,
	lines_added, lines_removed, files_changed, commits, pr_number,
	branch, commit_sha, worktree_path,
	peak_cpu_pct, avg_cpu_pct, peak_rss_mb, avg_rss_mb, disk_used_gb,
	last_output`

func scanRow(rows *sql.Rows) (Row, error) {
	var r Row
	if err := rows.Scan(
		&r.ID, &r.EntityKind, &r.EntityID, &r.RunNumber, &r.Trigger, &r.NodeID, &r.Status,
		&r.TerminalReason, &r.StartedAt, &r.CompletedAt, &r.DurationMS, &r.TTFBMS,
		&r.ExitCode, &r.Error, &r.CancelledAt, &r.CancelledBy, &r.Provider, &r.Model,
		&r.ModelContextSize, &r.AgentRuntime, &r.AgentVersion, &r.PricingVersion,
		&r.InputTokens, &r.OutputTokens, &r.CacheReadTokens, &r.CacheCreateTokens,
		&r.ReasoningTokens, &r.ToolUseTokens, &r.CostUSD, &r.EstimatedCostUSD,
		&r.CacheSavingsUSD, &r.CacheHitRatePct, &r.ContextWindowPct,
		&r.CostPerTurn, &r.CostPerAction, &r.TokensPerTurn, &r.Turns, &r.Actions,
		&r.Errors, &r.Compactions, &r.ParallelTasks, &r.ToolCallsJSON,
		&r.LinesAdded, &r.LinesRemoved, &r.FilesChanged, &r.Commits, &r.PRNumber,
		&r.Branch, &r.CommitSHA, &r.WorktreePath,
		&r.PeakCPUPct, &r.AvgCPUPct, &r.PeakRSSMB, &r.AvgRSSMB, &r.DiskUsedGB,
		&r.LastOutput,
	); err != nil {
		return Row{}, err
	}
	return r, nil
}
