package execution

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrRunNotFound   = errors.New("execution: run not found")
	ErrEmptyEntityID = errors.New("execution: entity_id must be non-empty")
)

const (
	EventStarted   = "started"
	EventSample    = "sample"
	EventCompleted = "completed"
	EventFailed    = "failed"
)

// Row is one append-only event row in execution_log.
type Row struct {
	ID                 string   `json:"id"`
	RunUID             string   `json:"run_uid"`
	EntityUID          string   `json:"entity_uid"`
	Event              string   `json:"event"`
	RunNumber          int      `json:"run_number"`
	Trigger            string   `json:"trigger"`
	NodeID             string   `json:"node_id"`
	TerminalReason     string   `json:"terminal_reason"`
	StartedAt          int64    `json:"started_at"`
	CompletedAt        int64    `json:"completed_at"`
	CancelledAt        int64    `json:"cancelled_at"`
	CancelledBy        string   `json:"cancelled_by"`
	DurationMS         int64    `json:"duration_ms"`
	TTFBMS             int64    `json:"ttfb_ms"`
	ExitCode           *int     `json:"exit_code"`
	Error              string   `json:"error"`
	Provider           string   `json:"provider"`
	Model              string   `json:"model"`
	ModelContextSize   int      `json:"model_context_size"`
	AgentRuntime       string   `json:"agent_runtime"`
	AgentVersion       string   `json:"agent_version"`
	PricingVersion     string   `json:"pricing_version"`
	InputTokens        int64    `json:"input_tokens"`
	OutputTokens       int64    `json:"output_tokens"`
	CacheReadTokens    int64    `json:"cache_read_tokens"`
	CacheCreateTokens  int64    `json:"cache_create_tokens"`
	ReasoningTokens    int64    `json:"reasoning_tokens"`
	ToolUseTokens      int64    `json:"tool_use_tokens"`
	CostUSD            float64  `json:"cost_usd"`
	EstimatedCostUSD   float64  `json:"estimated_cost_usd"`
	CacheSavingsUSD    float64  `json:"cache_savings_usd"`
	CacheHitRatePct    float64  `json:"cache_hit_rate_pct"`
	ContextWindowPct   float64  `json:"context_window_pct"`
	CostPerTurn        float64  `json:"cost_per_turn"`
	CostPerAction      float64  `json:"cost_per_action"`
	TokensPerTurn      float64  `json:"tokens_per_turn"`
	Turns              int      `json:"turns"`
	Actions            int      `json:"actions"`
	Errors             int      `json:"errors"`
	Compactions        int      `json:"compactions"`
	ParallelTasks      int      `json:"parallel_tasks"`
	ToolCallsJSON      string   `json:"tool_calls_json"`
	LinesAdded         int      `json:"lines_added"`
	LinesRemoved       int      `json:"lines_removed"`
	FilesChanged       int      `json:"files_changed"`
	Commits            int      `json:"commits"`
	PRNumber           *int     `json:"pr_number"`
	Branch             string   `json:"branch"`
	CommitSHA          string   `json:"commit_sha"`
	WorktreePath       string   `json:"worktree_path"`
	CPUPct             float64  `json:"cpu_pct"`
	RSSMB              float64  `json:"rss_mb"`
	DiskUsedGB         float64  `json:"disk_used_gb"`
	PeakCPUPct         float64  `json:"peak_cpu_pct"`
	AvgCPUPct          float64  `json:"avg_cpu_pct"`
	PeakRSSMB          float64  `json:"peak_rss_mb"`
	AvgRSSMB           float64  `json:"avg_rss_mb"`
	CreatedBy          string   `json:"created_by"`
	CreatedAt          string   `json:"created_at"`
	SessionID          string   `json:"session_id"` // Claude Code / MCP session that generated this row
	TestsRun           int      `json:"tests_run"`
	TestsPassed        int      `json:"tests_passed"`
	TestsFailed        int      `json:"tests_failed"`
}

// Store owns the execution_log table.
type Store struct{ db *sql.DB }

// NewStore returns a Store backed by db. Call InitSchema once before use.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// migrateOldSchema renames the legacy execution_log table (entity_kind+entity_id)
// to execution_log_legacy and drops its indexes so the new schema can be created
// fresh. Idempotent — no-op when run_uid already exists.
func migrateOldSchema(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(execution_log)`)
	if err != nil {
		return nil // table doesn't exist yet, nothing to migrate
	}
	defer rows.Close()
	hasRunUID, hasEntityKind := false, false
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			continue
		}
		if name == "run_uid" {
			hasRunUID = true
		}
		if name == "entity_kind" {
			hasEntityKind = true
		}
	}
	if hasRunUID || !hasEntityKind {
		return nil // already on new schema or empty DB
	}
	// Old schema detected — rename to legacy and drop indexes so CREATE TABLE succeeds.
	for _, stmt := range []string{
		`DROP INDEX IF EXISTS idx_execlog_entity`,
		`DROP INDEX IF EXISTS idx_execlog_running`,
		`DROP INDEX IF EXISTS idx_execlog_daily`,
		`DROP INDEX IF EXISTS idx_execlog_model`,
		`DROP INDEX IF EXISTS idx_execlog_terminal`,
		`DROP INDEX IF EXISTS idx_execlog_trigger`,
		`ALTER TABLE execution_log RENAME TO execution_log_legacy`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate old execution_log: %w", err)
		}
	}
	return nil
}

// addSessionIDColumn adds session_id to an existing execution_log table that
// predates the column. Idempotent — SQLite returns "duplicate column" on
// repeated runs which we swallow.
func addSessionIDColumn(db *sql.DB) {
	db.Exec(`ALTER TABLE execution_log ADD COLUMN session_id TEXT NOT NULL DEFAULT ''`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_execlog_session ON execution_log(session_id, created_at DESC)`)
}

// addProductivityColumns adds test tracking columns introduced after the initial
// schema. Idempotent — duplicate column errors are swallowed.
func addProductivityColumns(db *sql.DB) {
	for _, col := range []string{
		`ALTER TABLE execution_log ADD COLUMN tests_run     INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE execution_log ADD COLUMN tests_passed  INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE execution_log ADD COLUMN tests_failed  INTEGER NOT NULL DEFAULT 0`,
	} {
		db.Exec(col)
	}
}

// InitSchema creates execution_log (idempotent).
func InitSchema(db *sql.DB) error {
	if err := migrateOldSchema(db); err != nil {
		return err
	}
	// Add columns that postdate the initial schema.
	addSessionIDColumn(db)
	addProductivityColumns(db)
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS execution_log (
    id                  TEXT PRIMARY KEY,
    run_uid             TEXT    NOT NULL,
    entity_uid          TEXT    NOT NULL,
    event               TEXT    NOT NULL,
    run_number          INTEGER NOT NULL DEFAULT 0,
    trigger             TEXT    NOT NULL DEFAULT '',
    node_id             TEXT    NOT NULL DEFAULT '',
    terminal_reason     TEXT    NOT NULL DEFAULT '',
    started_at          INTEGER NOT NULL DEFAULT 0,
    completed_at        INTEGER NOT NULL DEFAULT 0,
    cancelled_at        INTEGER NOT NULL DEFAULT 0,
    cancelled_by        TEXT    NOT NULL DEFAULT '',
    duration_ms         INTEGER NOT NULL DEFAULT 0,
    ttfb_ms             INTEGER NOT NULL DEFAULT 0,
    exit_code           INTEGER,
    error               TEXT    NOT NULL DEFAULT '',
    provider            TEXT    NOT NULL DEFAULT '',
    model               TEXT    NOT NULL DEFAULT '',
    model_context_size  INTEGER NOT NULL DEFAULT 0,
    agent_runtime       TEXT    NOT NULL DEFAULT '',
    agent_version       TEXT    NOT NULL DEFAULT '',
    pricing_version     TEXT    NOT NULL DEFAULT '',
    input_tokens        INTEGER NOT NULL DEFAULT 0,
    output_tokens       INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens   INTEGER NOT NULL DEFAULT 0,
    cache_create_tokens INTEGER NOT NULL DEFAULT 0,
    reasoning_tokens    INTEGER NOT NULL DEFAULT 0,
    tool_use_tokens     INTEGER NOT NULL DEFAULT 0,
    cost_usd            REAL    NOT NULL DEFAULT 0,
    estimated_cost_usd  REAL    NOT NULL DEFAULT 0,
    cache_savings_usd   REAL    NOT NULL DEFAULT 0,
    cache_hit_rate_pct  REAL    NOT NULL DEFAULT 0,
    context_window_pct  REAL    NOT NULL DEFAULT 0,
    cost_per_turn       REAL    NOT NULL DEFAULT 0,
    cost_per_action     REAL    NOT NULL DEFAULT 0,
    tokens_per_turn     REAL    NOT NULL DEFAULT 0,
    turns               INTEGER NOT NULL DEFAULT 0,
    actions             INTEGER NOT NULL DEFAULT 0,
    errors              INTEGER NOT NULL DEFAULT 0,
    compactions         INTEGER NOT NULL DEFAULT 0,
    parallel_tasks      INTEGER NOT NULL DEFAULT 0,
    tool_calls_json     TEXT    NOT NULL DEFAULT '{}',
    lines_added         INTEGER NOT NULL DEFAULT 0,
    lines_removed       INTEGER NOT NULL DEFAULT 0,
    files_changed       INTEGER NOT NULL DEFAULT 0,
    commits             INTEGER NOT NULL DEFAULT 0,
    pr_number           INTEGER,
    branch              TEXT    NOT NULL DEFAULT '',
    commit_sha          TEXT    NOT NULL DEFAULT '',
    worktree_path       TEXT    NOT NULL DEFAULT '',
    cpu_pct             REAL    NOT NULL DEFAULT 0,
    rss_mb              REAL    NOT NULL DEFAULT 0,
    disk_used_gb        REAL    NOT NULL DEFAULT 0,
    peak_cpu_pct        REAL    NOT NULL DEFAULT 0,
    avg_cpu_pct         REAL    NOT NULL DEFAULT 0,
    peak_rss_mb         REAL    NOT NULL DEFAULT 0,
    avg_rss_mb          REAL    NOT NULL DEFAULT 0,
    created_by          TEXT    NOT NULL DEFAULT '',
    created_at          TEXT    NOT NULL,
    session_id          TEXT    NOT NULL DEFAULT '',
    tests_run           INTEGER NOT NULL DEFAULT 0,
    tests_passed        INTEGER NOT NULL DEFAULT 0,
    tests_failed        INTEGER NOT NULL DEFAULT 0
)`,
		`CREATE INDEX IF NOT EXISTS idx_execlog_run
  ON execution_log(run_uid, created_at ASC)`,
		`CREATE INDEX IF NOT EXISTS idx_execlog_entity
  ON execution_log(entity_uid, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_execlog_running
  ON execution_log(run_uid) WHERE event = 'started'`,
		`CREATE INDEX IF NOT EXISTS idx_execlog_daily
  ON execution_log(created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_execlog_model
  ON execution_log(model, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_execlog_trigger
  ON execution_log(trigger, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_execlog_session
  ON execution_log(session_id, created_at DESC)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("execution InitSchema: %w", err)
		}
	}
	return nil
}

// Insert writes any event row. All events — started, sample, completed, failed — use this.
func (s *Store) Insert(ctx context.Context, r Row) error {
	if r.EntityUID == "" {
		return ErrEmptyEntityID
	}
	if r.CreatedAt == "" {
		r.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if r.ID == "" {
		id, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("execution: insert: generate id: %w", err)
		}
		r.ID = id.String()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO execution_log (
		id, run_uid, entity_uid, event, run_number, trigger, node_id,
		terminal_reason, started_at, completed_at, cancelled_at, cancelled_by,
		duration_ms, ttfb_ms, exit_code, error, provider, model,
		model_context_size, agent_runtime, agent_version, pricing_version,
		input_tokens, output_tokens, cache_read_tokens, cache_create_tokens,
		reasoning_tokens, tool_use_tokens, cost_usd, estimated_cost_usd,
		cache_savings_usd, cache_hit_rate_pct, context_window_pct,
		cost_per_turn, cost_per_action, tokens_per_turn, turns, actions,
		errors, compactions, parallel_tasks, tool_calls_json,
		lines_added, lines_removed, files_changed, commits, pr_number,
		branch, commit_sha, worktree_path,
		cpu_pct, rss_mb, disk_used_gb,
		peak_cpu_pct, avg_cpu_pct, peak_rss_mb, avg_rss_mb,
		created_by, created_at, session_id,
		tests_run, tests_passed, tests_failed
	) VALUES (
		?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?
	)`,
		r.ID, r.RunUID, r.EntityUID, r.Event, r.RunNumber, r.Trigger, r.NodeID,
		r.TerminalReason, r.StartedAt, r.CompletedAt, r.CancelledAt, r.CancelledBy,
		r.DurationMS, r.TTFBMS, r.ExitCode, r.Error, r.Provider, r.Model,
		r.ModelContextSize, r.AgentRuntime, r.AgentVersion, r.PricingVersion,
		r.InputTokens, r.OutputTokens, r.CacheReadTokens, r.CacheCreateTokens,
		r.ReasoningTokens, r.ToolUseTokens, r.CostUSD, r.EstimatedCostUSD,
		r.CacheSavingsUSD, r.CacheHitRatePct, r.ContextWindowPct,
		r.CostPerTurn, r.CostPerAction, r.TokensPerTurn, r.Turns, r.Actions,
		r.Errors, r.Compactions, r.ParallelTasks, r.ToolCallsJSON,
		r.LinesAdded, r.LinesRemoved, r.FilesChanged, r.Commits, r.PRNumber,
		r.Branch, r.CommitSHA, r.WorktreePath,
		r.CPUPct, r.RSSMB, r.DiskUsedGB,
		r.PeakCPUPct, r.AvgCPUPct, r.PeakRSSMB, r.AvgRSSMB,
		r.CreatedBy, r.CreatedAt, r.SessionID,
		r.TestsRun, r.TestsPassed, r.TestsFailed,
	)
	if err != nil {
		return fmt.Errorf("execution: insert: %w", err)
	}
	return nil
}

// ListByRun returns all rows for a run in chronological order.
func (s *Store) ListByRun(ctx context.Context, runUID string) ([]Row, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+rowColumns+` FROM execution_log
		 WHERE run_uid = ?
		 ORDER BY created_at ASC`,
		runUID,
	)
	if err != nil {
		return nil, fmt.Errorf("execution: list by run: %w", err)
	}
	defer rows.Close()
	return collectRows(rows)
}

// LatestByRun returns the most recent row for a run (current state).
func (s *Store) LatestByRun(ctx context.Context, runUID string) (*Row, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+rowColumns+` FROM execution_log
		 WHERE run_uid = ?
		 ORDER BY created_at DESC LIMIT 1`,
		runUID,
	)
	if err != nil {
		return nil, fmt.Errorf("execution: latest by run: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if rerr := rows.Err(); rerr != nil {
			return nil, rerr
		}
		return nil, ErrRunNotFound
	}
	r, err := scanRow(rows)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ListByEntity returns completed/failed rows for an entity, newest first.
func (s *Store) ListByEntity(ctx context.Context, entityUID string, limit int) ([]Row, error) {
	if entityUID == "" {
		return nil, ErrEmptyEntityID
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+rowColumns+` FROM execution_log
		 WHERE entity_uid = ?
		 ORDER BY created_at DESC LIMIT ?`,
		entityUID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("execution: list by entity: %w", err)
	}
	defer rows.Close()
	return collectRows(rows)
}

// MigrateFromLegacy copies rows from the old execution_log format
// (entity_kind+entity_id+status) into the new format (entity_uid+event).
// Idempotent — skips rows where id already exists. Maps entity_id → entity_uid
// directly (same UUID). Returns migrated count.
//
// The legacy schema is detected by checking for the entity_kind column;
// if it doesn't exist the function returns 0 with no error.
func (s *Store) MigrateFromLegacy(ctx context.Context) (int, error) {
	// Probe for the legacy column without failing if it's absent.
	var hasLegacy bool
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(execution_log)`)
	if err != nil {
		return 0, fmt.Errorf("execution: migrate legacy: pragma: %w", err)
	}
	func() {
		defer rows.Close()
		for rows.Next() {
			var cid int
			var name, colType, notNull, dflt sql.NullString
			var pk int
			if serr := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); serr != nil {
				return
			}
			if name.String == "entity_kind" {
				hasLegacy = true
			}
		}
	}()
	if !hasLegacy {
		return 0, nil
	}

	legacyRows, err := s.db.QueryContext(ctx, `SELECT
		id, entity_id, run_number, trigger, node_id, status,
		terminal_reason, started_at, completed_at, cancelled_at, cancelled_by,
		duration_ms, ttfb_ms, exit_code, error, provider, model,
		model_context_size, agent_runtime, agent_version, pricing_version,
		input_tokens, output_tokens, cache_read_tokens, cache_create_tokens,
		reasoning_tokens, tool_use_tokens, cost_usd, estimated_cost_usd,
		cache_savings_usd, cache_hit_rate_pct, context_window_pct,
		cost_per_turn, cost_per_action, tokens_per_turn, turns, actions,
		errors, compactions, parallel_tasks, tool_calls_json,
		lines_added, lines_removed, files_changed, commits, pr_number,
		branch, commit_sha, worktree_path,
		peak_cpu_pct, avg_cpu_pct, peak_rss_mb, avg_rss_mb, disk_used_gb
	FROM execution_log`)
	if err != nil {
		return 0, fmt.Errorf("execution: migrate legacy: query: %w", err)
	}

	type legacyRow struct {
		id, entityID, trigger, nodeID, status                  string
		terminalReason, cancelledBy, error_                    string
		provider, model, agentRuntime, agentVersion            string
		pricingVersion, toolCallsJSON                          string
		branch, commitSHA, worktreePath                        string
		runNumber, modelContextSize                            int
		turns, actions, errors_, compactions, parallelTasks    int
		linesAdded, linesRemoved, filesChanged, commits        int
		startedAt, completedAt, cancelledAt                    int64
		durationMS, ttfbMS                                     int64
		inputTokens, outputTokens                              int64
		cacheReadTokens, cacheCreateTokens                     int64
		reasoningTokens, toolUseTokens                         int64
		costUSD, estimatedCostUSD, cacheSavingsUSD             float64
		cacheHitRatePct, contextWindowPct                      float64
		costPerTurn, costPerAction, tokensPerTurn               float64
		peakCPUPct, avgCPUPct, peakRSSMB, avgRSSMB, diskUsedGB float64
		exitCode                                               *int
		prNumber                                               *int
	}

	var batch []legacyRow
	func() {
		defer legacyRows.Close()
		for legacyRows.Next() {
			var lr legacyRow
			if serr := legacyRows.Scan(
				&lr.id, &lr.entityID, &lr.runNumber, &lr.trigger, &lr.nodeID, &lr.status,
				&lr.terminalReason, &lr.startedAt, &lr.completedAt, &lr.cancelledAt, &lr.cancelledBy,
				&lr.durationMS, &lr.ttfbMS, &lr.exitCode, &lr.error_,
				&lr.provider, &lr.model, &lr.modelContextSize,
				&lr.agentRuntime, &lr.agentVersion, &lr.pricingVersion,
				&lr.inputTokens, &lr.outputTokens, &lr.cacheReadTokens, &lr.cacheCreateTokens,
				&lr.reasoningTokens, &lr.toolUseTokens,
				&lr.costUSD, &lr.estimatedCostUSD, &lr.cacheSavingsUSD,
				&lr.cacheHitRatePct, &lr.contextWindowPct,
				&lr.costPerTurn, &lr.costPerAction, &lr.tokensPerTurn,
				&lr.turns, &lr.actions, &lr.errors_, &lr.compactions, &lr.parallelTasks,
				&lr.toolCallsJSON, &lr.linesAdded, &lr.linesRemoved, &lr.filesChanged,
				&lr.commits, &lr.prNumber,
				&lr.branch, &lr.commitSHA, &lr.worktreePath,
				&lr.peakCPUPct, &lr.avgCPUPct, &lr.peakRSSMB, &lr.avgRSSMB, &lr.diskUsedGB,
			); serr != nil {
				return
			}
			batch = append(batch, lr)
		}
	}()

	inserted := 0
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, lr := range batch {
		var alreadyExists int
		if serr := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM execution_log WHERE id = ? AND run_uid != ''`,
			lr.id,
		).Scan(&alreadyExists); serr != nil || alreadyExists > 0 {
			continue
		}

		event := legacyStatusToEvent(lr.status)

		_, ierr := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO execution_log (
			id, run_uid, entity_uid, event, run_number, trigger, node_id,
			terminal_reason, started_at, completed_at, cancelled_at, cancelled_by,
			duration_ms, ttfb_ms, exit_code, error, provider, model,
			model_context_size, agent_runtime, agent_version, pricing_version,
			input_tokens, output_tokens, cache_read_tokens, cache_create_tokens,
			reasoning_tokens, tool_use_tokens, cost_usd, estimated_cost_usd,
			cache_savings_usd, cache_hit_rate_pct, context_window_pct,
			cost_per_turn, cost_per_action, tokens_per_turn, turns, actions,
			errors, compactions, parallel_tasks, tool_calls_json,
			lines_added, lines_removed, files_changed, commits, pr_number,
			branch, commit_sha, worktree_path,
			cpu_pct, rss_mb, disk_used_gb,
			peak_cpu_pct, avg_cpu_pct, peak_rss_mb, avg_rss_mb,
			created_by, created_at
		) SELECT
			?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?
		WHERE NOT EXISTS (SELECT 1 FROM execution_log WHERE id = ?)`,
			lr.id, lr.id, lr.entityID, event, lr.runNumber, lr.trigger, lr.nodeID,
			lr.terminalReason, lr.startedAt, lr.completedAt, lr.cancelledAt, lr.cancelledBy,
			lr.durationMS, lr.ttfbMS, lr.exitCode, lr.error_,
			lr.provider, lr.model, lr.modelContextSize,
			lr.agentRuntime, lr.agentVersion, lr.pricingVersion,
			lr.inputTokens, lr.outputTokens, lr.cacheReadTokens, lr.cacheCreateTokens,
			lr.reasoningTokens, lr.toolUseTokens,
			lr.costUSD, lr.estimatedCostUSD, lr.cacheSavingsUSD,
			lr.cacheHitRatePct, lr.contextWindowPct,
			lr.costPerTurn, lr.costPerAction, lr.tokensPerTurn,
			lr.turns, lr.actions, lr.errors_, lr.compactions, lr.parallelTasks,
			lr.toolCallsJSON, lr.linesAdded, lr.linesRemoved, lr.filesChanged,
			lr.commits, lr.prNumber,
			lr.branch, lr.commitSHA, lr.worktreePath,
			0.0, 0.0, lr.diskUsedGB,
			lr.peakCPUPct, lr.avgCPUPct, lr.peakRSSMB, lr.avgRSSMB,
			"", now,
			// WHERE NOT EXISTS guard
			lr.id,
		)
		if ierr != nil {
			return inserted, fmt.Errorf("execution: migrate legacy: insert: %w", ierr)
		}
		inserted++
	}
	return inserted, nil
}

func legacyStatusToEvent(status string) string {
	switch status {
	case "running":
		return EventStarted
	case "failed":
		return EventFailed
	case "completed":
		return EventCompleted
	default:
		return EventCompleted
	}
}

const rowColumns = `id, run_uid, entity_uid, event, run_number, trigger, node_id,
	terminal_reason, started_at, completed_at, cancelled_at, cancelled_by,
	duration_ms, ttfb_ms, exit_code, error, provider, model,
	model_context_size, agent_runtime, agent_version, pricing_version,
	input_tokens, output_tokens, cache_read_tokens, cache_create_tokens,
	reasoning_tokens, tool_use_tokens, cost_usd, estimated_cost_usd,
	cache_savings_usd, cache_hit_rate_pct, context_window_pct,
	cost_per_turn, cost_per_action, tokens_per_turn, turns, actions,
	errors, compactions, parallel_tasks, tool_calls_json,
	lines_added, lines_removed, files_changed, commits, pr_number,
	branch, commit_sha, worktree_path,
	cpu_pct, rss_mb, disk_used_gb,
	peak_cpu_pct, avg_cpu_pct, peak_rss_mb, avg_rss_mb,
	created_by, created_at`

func scanRow(rows *sql.Rows) (Row, error) {
	var r Row
	if err := rows.Scan(
		&r.ID, &r.RunUID, &r.EntityUID, &r.Event, &r.RunNumber, &r.Trigger, &r.NodeID,
		&r.TerminalReason, &r.StartedAt, &r.CompletedAt, &r.CancelledAt, &r.CancelledBy,
		&r.DurationMS, &r.TTFBMS, &r.ExitCode, &r.Error, &r.Provider, &r.Model,
		&r.ModelContextSize, &r.AgentRuntime, &r.AgentVersion, &r.PricingVersion,
		&r.InputTokens, &r.OutputTokens, &r.CacheReadTokens, &r.CacheCreateTokens,
		&r.ReasoningTokens, &r.ToolUseTokens, &r.CostUSD, &r.EstimatedCostUSD,
		&r.CacheSavingsUSD, &r.CacheHitRatePct, &r.ContextWindowPct,
		&r.CostPerTurn, &r.CostPerAction, &r.TokensPerTurn, &r.Turns, &r.Actions,
		&r.Errors, &r.Compactions, &r.ParallelTasks, &r.ToolCallsJSON,
		&r.LinesAdded, &r.LinesRemoved, &r.FilesChanged, &r.Commits, &r.PRNumber,
		&r.Branch, &r.CommitSHA, &r.WorktreePath,
		&r.CPUPct, &r.RSSMB, &r.DiskUsedGB,
		&r.PeakCPUPct, &r.AvgCPUPct, &r.PeakRSSMB, &r.AvgRSSMB,
		&r.CreatedBy, &r.CreatedAt,
	); err != nil {
		return Row{}, err
	}
	return r, nil
}

func collectRows(rows *sql.Rows) ([]Row, error) {
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
