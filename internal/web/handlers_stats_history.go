package web

import (
	"net/http"
	"strconv"

	"github.com/k8nstantin/OpenPraxis/internal/node"
)

// statsHistory is the full history payload for GET /api/stats/history.
// Every series is daily-bucketed over the requested window (default: all time).
// All data from execution_log — no joins.
type statsHistory struct {
	// Daily run activity
	Runs []dayRunBucket `json:"runs"`
	// Daily efficiency metrics
	Efficiency []dayEfficiencyBucket `json:"efficiency"`
	// Daily token volumes
	Tokens []dayTokenBucket `json:"tokens"`
	// Daily code productivity
	Productivity []dayProductivityBucket `json:"productivity"`
	// Model/agent distribution (all time totals)
	Models        []labelCount `json:"models"`
	Agents        []labelCount `json:"agents"`
	TerminalReasons []labelCount `json:"terminal_reasons"`
	TriggerSplit  []labelCount `json:"trigger_split"`
	// Tool call breakdown from tool_calls_json aggregated (all time)
	TopTools []labelCount `json:"top_tools"`
	// Daily system averages (from system_host_samples)
	SystemDaily []daySystemBucket `json:"system_daily"`
	// Summary totals
	Totals statsTotals `json:"totals"`
}

type daySystemBucket struct {
	Day             string  `json:"day"`
	AvgCPUPct       float64 `json:"avg_cpu_pct"`
	AvgNetRxMbps    float64 `json:"avg_net_rx_mbps"`
	AvgNetTxMbps    float64 `json:"avg_net_tx_mbps"`
	AvgDiskReadMBps float64 `json:"avg_disk_read_mbps"`
	AvgDiskWriteMBps float64 `json:"avg_disk_write_mbps"`
}

type dayRunBucket struct {
	Day        string  `json:"day"`
	Completed  int     `json:"completed"`
	Failed     int     `json:"failed"`
	AvgDurSec  float64 `json:"avg_dur_sec"`
	MaxDurSec  float64 `json:"max_dur_sec"`
	AvgRunNum  float64 `json:"avg_run_number"` // retry indicator
}

type dayEfficiencyBucket struct {
	Day              string  `json:"day"`
	AvgTurns         float64 `json:"avg_turns"`
	AvgActions       float64 `json:"avg_actions"`
	AvgActionsPerTurn float64 `json:"avg_actions_per_turn"`
	AvgContextPct    float64 `json:"avg_context_pct"`
	AvgTokensPerTurn float64 `json:"avg_tokens_per_turn"`
	AvgCacheHitPct   float64 `json:"avg_cache_hit_pct"`
	TotalCompactions int64   `json:"total_compactions"`
	TotalErrors      int64   `json:"total_errors"`
	AvgTTFBMs        float64 `json:"avg_ttfb_ms"`
}

type dayTokenBucket struct {
	Day               string `json:"day"`
	InputTokens       int64  `json:"input_tokens"`
	OutputTokens      int64  `json:"output_tokens"`
	CacheReadTokens   int64  `json:"cache_read_tokens"`
	CacheCreateTokens int64  `json:"cache_create_tokens"`
	ReasoningTokens   int64  `json:"reasoning_tokens"`
	ToolUseTokens     int64  `json:"tool_use_tokens"`
}

type dayProductivityBucket struct {
	Day          string `json:"day"`
	LinesAdded   int64  `json:"lines_added"`
	LinesRemoved int64  `json:"lines_removed"`
	FilesChanged int64  `json:"files_changed"`
	Commits      int64  `json:"commits"`
	TestsRun     int64  `json:"tests_run"`
	TestsPassed  int64  `json:"tests_passed"`
	TestsFailed  int64  `json:"tests_failed"`
	PRsOpened    int64  `json:"prs_opened"`
}

type labelCount struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

type statsTotals struct {
	TotalRuns        int     `json:"total_runs"`
	TotalFailed      int     `json:"total_failed"`
	TotalTurns       int64   `json:"total_turns"`
	TotalActions     int64   `json:"total_actions"`
	TotalCompactions int64   `json:"total_compactions"`
	TotalErrors      int64   `json:"total_errors"`
	TotalInputTok    int64   `json:"total_input_tokens"`
	TotalOutputTok   int64   `json:"total_output_tokens"`
	TotalCacheRead   int64   `json:"total_cache_read_tokens"`
	TotalCacheCreate int64   `json:"total_cache_create_tokens"`
	TotalLinesAdded  int64   `json:"total_lines_added"`
	TotalLinesRemoved int64  `json:"total_lines_removed"`
	TotalFilesChanged int64  `json:"total_files_changed"`
	TotalCommits     int64   `json:"total_commits"`
	TotalTestsRun    int64   `json:"total_tests_run"`
	TotalTestsPassed int64   `json:"total_tests_passed"`
	TotalTestsFailed int64   `json:"total_tests_failed"`
	AvgCacheHitPct   float64 `json:"avg_cache_hit_pct"`
	AvgTurns         float64 `json:"avg_turns"`
	AvgDurSec        float64 `json:"avg_dur_sec"`
	AvgContextPct    float64 `json:"avg_context_pct"`
}

// apiStatsHistory handles GET /api/stats/history?days=90
// Returns full history daily-bucketed. Default: all time. No date limit enforced
// so operators see the full picture from day one.
func apiStatsHistory(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var since string
		if d := r.URL.Query().Get("days"); d != "" {
			if days, err := strconv.Atoi(d); err == nil && days > 0 {
				since = strconv.Itoa(days)
			}
		}
		// Use started_at (real run timestamp) for date filtering when available.
		// Fall back to created_at for rows without started_at.
		whereClause := "event IN ('completed','failed')"
		if since != "" {
			whereClause += " AND (CASE WHEN started_at>0 THEN datetime(started_at/1000,'unixepoch') ELSE created_at END) >= datetime('now', '-" + since + " days')"
		}

		var h statsHistory
		db := n.DB()

		// ── Daily run activity ─────────────────────────────────────────────
		rows, err := db.QueryContext(r.Context(), `
			SELECT date(datetime(CASE WHEN started_at>0 THEN started_at/1000 ELSE strftime('%s', created_at) END, 'unixepoch')) as day,
			       SUM(CASE WHEN event='completed' THEN 1 ELSE 0 END),
			       SUM(CASE WHEN event='failed' THEN 1 ELSE 0 END),
			       COALESCE(AVG(CASE WHEN duration_ms>0 THEN duration_ms/1000.0 END),0),
			       COALESCE(MAX(duration_ms)/1000.0,0),
			       COALESCE(AVG(CASE WHEN run_number>0 THEN run_number END),0)
			FROM execution_log WHERE `+whereClause+`
			GROUP BY day ORDER BY day ASC`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var b dayRunBucket
				rows.Scan(&b.Day, &b.Completed, &b.Failed, &b.AvgDurSec, &b.MaxDurSec, &b.AvgRunNum)
				h.Runs = append(h.Runs, b)
			}
		}

		// ── Daily efficiency ───────────────────────────────────────────────
		rows2, err := db.QueryContext(r.Context(), `
			SELECT date(datetime(CASE WHEN started_at>0 THEN started_at/1000 ELSE strftime('%s', created_at) END, 'unixepoch')) as day,
			       COALESCE(SUM(turns),0),
			       COALESCE(SUM(actions),0),
			       COALESCE(AVG(CASE WHEN turns>0 AND actions>0 THEN CAST(actions AS REAL)/turns END),0),
			       COALESCE(AVG(CASE WHEN context_window_pct>0 THEN context_window_pct END),0),
			       COALESCE(AVG(CASE WHEN tokens_per_turn>0 THEN tokens_per_turn END),0),
			       COALESCE(AVG(CASE WHEN cache_hit_rate_pct>0 THEN cache_hit_rate_pct END),0),
			       COALESCE(SUM(compactions),0),
			       COALESCE(SUM(errors),0),
			       COALESCE(AVG(CASE WHEN ttfb_ms>0 THEN ttfb_ms END),0)
			FROM execution_log WHERE `+whereClause+`
			GROUP BY day ORDER BY day ASC`)
		if err == nil {
			defer rows2.Close()
			for rows2.Next() {
				var b dayEfficiencyBucket
				rows2.Scan(&b.Day, &b.AvgTurns, &b.AvgActions, &b.AvgActionsPerTurn,
					&b.AvgContextPct, &b.AvgTokensPerTurn, &b.AvgCacheHitPct,
					&b.TotalCompactions, &b.TotalErrors, &b.AvgTTFBMs)
				h.Efficiency = append(h.Efficiency, b)
			}
		}

		// ── Daily tokens ───────────────────────────────────────────────────
		rows3, err := db.QueryContext(r.Context(), `
			SELECT date(datetime(CASE WHEN started_at>0 THEN started_at/1000 ELSE strftime('%s', created_at) END, 'unixepoch')) as day,
			       COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0),
			       COALESCE(SUM(cache_read_tokens),0), COALESCE(SUM(cache_create_tokens),0),
			       COALESCE(SUM(reasoning_tokens),0), COALESCE(SUM(tool_use_tokens),0)
			FROM execution_log WHERE `+whereClause+`
			GROUP BY day ORDER BY day ASC`)
		if err == nil {
			defer rows3.Close()
			for rows3.Next() {
				var b dayTokenBucket
				rows3.Scan(&b.Day, &b.InputTokens, &b.OutputTokens,
					&b.CacheReadTokens, &b.CacheCreateTokens,
					&b.ReasoningTokens, &b.ToolUseTokens)
				h.Tokens = append(h.Tokens, b)
			}
		}

		// ── Daily productivity ─────────────────────────────────────────────
		rows4, err := db.QueryContext(r.Context(), `
			SELECT date(datetime(CASE WHEN started_at>0 THEN started_at/1000 ELSE strftime('%s', created_at) END, 'unixepoch')) as day,
			       COALESCE(SUM(lines_added),0), COALESCE(SUM(lines_removed),0),
			       COALESCE(SUM(files_changed),0), COALESCE(SUM(commits),0),
			       COALESCE(SUM(tests_run),0), COALESCE(SUM(tests_passed),0),
			       COALESCE(SUM(tests_failed),0),
			       COUNT(DISTINCT CASE WHEN pr_number>0 THEN pr_number END)
			FROM execution_log WHERE `+whereClause+`
			GROUP BY day ORDER BY day ASC`)
		if err == nil {
			defer rows4.Close()
			for rows4.Next() {
				var b dayProductivityBucket
				rows4.Scan(&b.Day, &b.LinesAdded, &b.LinesRemoved, &b.FilesChanged, &b.Commits,
					&b.TestsRun, &b.TestsPassed, &b.TestsFailed, &b.PRsOpened)
				h.Productivity = append(h.Productivity, b)
			}
		}

		// ── Models ─────────────────────────────────────────────────────────
		rows5, _ := db.QueryContext(r.Context(), `
			SELECT COALESCE(NULLIF(model,''),'unknown') as label, COUNT(*) as cnt
			FROM execution_log WHERE `+whereClause+` GROUP BY label ORDER BY cnt DESC`)
		if rows5 != nil {
			defer rows5.Close()
			for rows5.Next() {
				var lc labelCount
				rows5.Scan(&lc.Label, &lc.Count)
				h.Models = append(h.Models, lc)
			}
		}

		// ── Agent runtimes ─────────────────────────────────────────────────
		rows6, _ := db.QueryContext(r.Context(), `
			SELECT COALESCE(NULLIF(agent_runtime,''),'unknown') as label, COUNT(*) as cnt
			FROM execution_log WHERE `+whereClause+` GROUP BY label ORDER BY cnt DESC`)
		if rows6 != nil {
			defer rows6.Close()
			for rows6.Next() {
				var lc labelCount
				rows6.Scan(&lc.Label, &lc.Count)
				h.Agents = append(h.Agents, lc)
			}
		}

		// ── Terminal reasons ───────────────────────────────────────────────
		rows7, _ := db.QueryContext(r.Context(), `
			SELECT COALESCE(NULLIF(terminal_reason,''),'success') as label, COUNT(*) as cnt
			FROM execution_log WHERE `+whereClause+` GROUP BY label ORDER BY cnt DESC`)
		if rows7 != nil {
			defer rows7.Close()
			for rows7.Next() {
				var lc labelCount
				rows7.Scan(&lc.Label, &lc.Count)
				h.TerminalReasons = append(h.TerminalReasons, lc)
			}
		}

		// ── Trigger split ──────────────────────────────────────────────────
		rows8, _ := db.QueryContext(r.Context(), `
			SELECT COALESCE(NULLIF(trigger,''),'unknown') as label, COUNT(*) as cnt
			FROM execution_log WHERE `+whereClause+` GROUP BY label ORDER BY cnt DESC`)
		if rows8 != nil {
			defer rows8.Close()
			for rows8.Next() {
				var lc labelCount
				rows8.Scan(&lc.Label, &lc.Count)
				h.TriggerSplit = append(h.TriggerSplit, lc)
			}
		}

		// ── Totals ─────────────────────────────────────────────────────────
		db.QueryRowContext(r.Context(), `
			SELECT
			  SUM(CASE WHEN event='completed' THEN 1 ELSE 0 END),
			  SUM(CASE WHEN event='failed' THEN 1 ELSE 0 END),
			  COALESCE(SUM(turns),0), COALESCE(SUM(actions),0),
			  COALESCE(SUM(compactions),0), COALESCE(SUM(errors),0),
			  COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0),
			  COALESCE(SUM(cache_read_tokens),0), COALESCE(SUM(cache_create_tokens),0),
			  COALESCE(SUM(lines_added),0), COALESCE(SUM(lines_removed),0),
			  COALESCE(SUM(files_changed),0), COALESCE(SUM(commits),0),
			  COALESCE(SUM(tests_run),0), COALESCE(SUM(tests_passed),0), COALESCE(SUM(tests_failed),0),
			  COALESCE(AVG(CASE WHEN cache_hit_rate_pct>0 THEN cache_hit_rate_pct END),0),
			  COALESCE(AVG(CASE WHEN turns>0 THEN turns END),0),
			  COALESCE(AVG(CASE WHEN duration_ms>0 THEN duration_ms/1000.0 END),0),
			  COALESCE(AVG(CASE WHEN context_window_pct>0 THEN context_window_pct END),0)
			FROM execution_log WHERE `+whereClause).
			Scan(&h.Totals.TotalRuns, &h.Totals.TotalFailed,
				&h.Totals.TotalTurns, &h.Totals.TotalActions,
				&h.Totals.TotalCompactions, &h.Totals.TotalErrors,
				&h.Totals.TotalInputTok, &h.Totals.TotalOutputTok,
				&h.Totals.TotalCacheRead, &h.Totals.TotalCacheCreate,
				&h.Totals.TotalLinesAdded, &h.Totals.TotalLinesRemoved,
				&h.Totals.TotalFilesChanged, &h.Totals.TotalCommits,
				&h.Totals.TotalTestsRun, &h.Totals.TotalTestsPassed, &h.Totals.TotalTestsFailed,
				&h.Totals.AvgCacheHitPct, &h.Totals.AvgTurns,
				&h.Totals.AvgDurSec, &h.Totals.AvgContextPct)

		// ── Daily system averages ─────────────────────────────────────────
		sysWhere := "1=1"
		if since != "" {
			sysWhere = "date(ts) >= date('now', '-" + since + " days')"
		}
		sysRows, _ := db.QueryContext(r.Context(), `
			SELECT date(ts) as day,
			       AVG(cpu_pct), AVG(net_rx_mbps), AVG(net_tx_mbps),
			       AVG(disk_read_mbps), AVG(disk_write_mbps)
			FROM system_host_samples WHERE `+sysWhere+`
			GROUP BY day ORDER BY day ASC`)
		if sysRows != nil {
			defer sysRows.Close()
			for sysRows.Next() {
				var b daySystemBucket
				if sysRows.Scan(&b.Day, &b.AvgCPUPct, &b.AvgNetRxMbps, &b.AvgNetTxMbps,
					&b.AvgDiskReadMBps, &b.AvgDiskWriteMBps) == nil {
					h.SystemDaily = append(h.SystemDaily, b)
				}
			}
		}

		writeJSON(w, h)
	}
}
