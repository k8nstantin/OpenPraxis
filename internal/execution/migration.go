package execution

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// BackfillFromTaskRuns copies all existing task_runs rows into execution_log.
// Idempotent: returns 0 immediately if any task-kind rows already exist.
func BackfillFromTaskRuns(ctx context.Context, db *sql.DB) (int, error) {
	var existing int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM execution_log WHERE entity_kind='task' AND run_number > 0`,
	).Scan(&existing); err != nil {
		return 0, fmt.Errorf("backfill: check existing: %w", err)
	}
	if existing > 0 {
		return 0, nil
	}

	rows, err := db.QueryContext(ctx, `SELECT
		task_id, run_number, output, status, actions, lines,
		cost_usd, turns, started_at, completed_at,
		input_tokens, output_tokens, cache_read_tokens, cache_create_tokens,
		model, pricing_version, duration_ms, agent_runtime, agent_version,
		peak_cpu_pct, avg_cpu_pct, peak_rss_mb, avg_rss_mb,
		errors, compactions, files_changed, lines_added, lines_removed,
		exit_code, cancelled_at, cancelled_by,
		branch, commit_sha, commits, pr_number, worktree_path
	FROM task_runs`)
	if err != nil {
		return 0, fmt.Errorf("backfill: query task_runs: %w", err)
	}
	defer rows.Close()

	type taskRunRow struct {
		taskID, output, status, model, pricingVersion   string
		agentRuntime, agentVersion                      string
		startedAt, completedAt, cancelledAt, cancelledBy string
		branch, commitSHA, worktreePath                  string
		runNumber, actions, lines, turns                 int
		inputTokens, outputTokens                        int
		cacheReadTokens, cacheCreateTokens               int
		errors, compactions, filesChanged                int
		linesAdded, linesRemoved, exitCode, commits      int
		prNumber                                         int
		costUSD                                          float64
		durationMS                                       int64
		peakCPUPct, avgCPUPct, peakRSSMB, avgRSSMB      float64
	}

	var batch []taskRunRow
	for rows.Next() {
		var r taskRunRow
		if err := rows.Scan(
			&r.taskID, &r.runNumber, &r.output, &r.status, &r.actions, &r.lines,
			&r.costUSD, &r.turns, &r.startedAt, &r.completedAt,
			&r.inputTokens, &r.outputTokens, &r.cacheReadTokens, &r.cacheCreateTokens,
			&r.model, &r.pricingVersion, &r.durationMS, &r.agentRuntime, &r.agentVersion,
			&r.peakCPUPct, &r.avgCPUPct, &r.peakRSSMB, &r.avgRSSMB,
			&r.errors, &r.compactions, &r.filesChanged, &r.linesAdded, &r.linesRemoved,
			&r.exitCode, &r.cancelledAt, &r.cancelledBy,
			&r.branch, &r.commitSHA, &r.commits, &r.prNumber, &r.worktreePath,
		); err != nil {
			return 0, fmt.Errorf("backfill: scan row: %w", err)
		}
		batch = append(batch, r)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("backfill: iterate rows: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("backfill: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	inserted := 0
	for _, tr := range batch {
		id, _ := uuid.NewV7()

		startedAtMS := parseTimeMS(tr.startedAt)
		completedAtMS := parseTimeMS(tr.completedAt)
		cancelledAtMS := parseTimeMS(tr.cancelledAt)

		info := LookupModel(tr.model)

		exitCode := tr.exitCode
		var prNumber *int
		if tr.prNumber != 0 {
			n := tr.prNumber
			prNumber = &n
		}

		r := Row{
			ID:                id.String(),
			EntityKind:        KindTask,
			EntityID:          tr.taskID,
			RunNumber:         tr.runNumber,
			Status:            tr.status,
			TerminalReason:    terminalReason(tr.status),
			StartedAt:         startedAtMS,
			CompletedAt:       completedAtMS,
			DurationMS:        tr.durationMS,
			ExitCode:          &exitCode,
			CancelledAt:       cancelledAtMS,
			CancelledBy:       tr.cancelledBy,
			Provider:          info.Provider,
			Model:             tr.model,
			ModelContextSize:  info.ContextWindowSize,
			AgentRuntime:      tr.agentRuntime,
			AgentVersion:      tr.agentVersion,
			PricingVersion:    tr.pricingVersion,
			InputTokens:       tr.inputTokens,
			OutputTokens:      tr.outputTokens,
			CacheReadTokens:   tr.cacheReadTokens,
			CacheCreateTokens: tr.cacheCreateTokens,
			CostUSD:           tr.costUSD,
			Turns:             tr.turns,
			Actions:           tr.actions,
			Errors:            tr.errors,
			Compactions:       tr.compactions,
			FilesChanged:      tr.filesChanged,
			LinesAdded:        tr.linesAdded,
			LinesRemoved:      tr.linesRemoved,
			Commits:           tr.commits,
			PRNumber:          prNumber,
			Branch:            tr.branch,
			CommitSHA:         tr.commitSHA,
			WorktreePath:      tr.worktreePath,
			PeakCPUPct:        tr.peakCPUPct,
			AvgCPUPct:         tr.avgCPUPct,
			PeakRSSMB:         tr.peakRSSMB,
			AvgRSSMB:          tr.avgRSSMB,
			ToolCallsJSON:     "{}",
			LastOutput:        tr.output,
		}
		ComputeDerived(&r)

		_, err := tx.ExecContext(ctx, `INSERT INTO execution_log (
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
		if err != nil {
			return 0, fmt.Errorf("backfill: insert row: %w", err)
		}
		inserted++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("backfill: commit: %w", err)
	}

	slog.Info("backfill: migrated N rows from task_runs", "count", inserted)
	return inserted, nil
}

// ComputeDerived fills in ratio/derived fields on r from its raw counters.
func ComputeDerived(r *Row) {
	if r.DurationMS == 0 && r.CompletedAt > r.StartedAt {
		r.DurationMS = r.CompletedAt - r.StartedAt
	}

	totalInput := r.InputTokens + r.CacheReadTokens
	if totalInput > 0 {
		r.CacheHitRatePct = float64(r.CacheReadTokens) / float64(totalInput) * 100
	}

	if r.ModelContextSize > 0 {
		total := r.InputTokens + r.OutputTokens + r.CacheReadTokens + r.CacheCreateTokens
		r.ContextWindowPct = float64(total) / float64(r.ModelContextSize) * 100
	}

	if r.Turns > 0 {
		r.CostPerTurn = r.CostUSD / float64(r.Turns)
		r.TokensPerTurn = float64(r.InputTokens+r.OutputTokens) / float64(r.Turns)
	}

	if r.Actions > 0 {
		r.CostPerAction = r.CostUSD / float64(r.Actions)
	}
}

func terminalReason(status string) string {
	switch status {
	case "completed":
		return "success"
	case "failed":
		return "error"
	case "cancelled":
		return "cancelled"
	default:
		return status
	}
}

// parseTimeMS parses an RFC3339 string and returns unix milliseconds.
// Returns 0 for empty or unparseable strings.
func parseTimeMS(s string) int64 {
	if s == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return 0
	}
	return t.UnixMilli()
}
