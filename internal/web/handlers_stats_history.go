package web

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/relationships"
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
	Models          []labelCount `json:"models"`
	Agents          []labelCount `json:"agents"`
	TerminalReasons []labelCount `json:"terminal_reasons"`
	TriggerSplit    []labelCount `json:"trigger_split"`
	// Tool call breakdown from tool_calls_json aggregated (all time)
	TopTools []labelCount `json:"top_tools"`
	// Daily system averages (from system_host_samples)
	SystemDaily []daySystemBucket `json:"system_daily"`
	// Summary totals
	Totals statsTotals `json:"totals"`
}

type daySystemBucket struct {
	Day              string  `json:"day"`
	AvgCPUPct        float64 `json:"avg_cpu_pct"`
	AvgNetRxMbps     float64 `json:"avg_net_rx_mbps"`
	AvgNetTxMbps     float64 `json:"avg_net_tx_mbps"`
	AvgDiskReadMBps  float64 `json:"avg_disk_read_mbps"`
	AvgDiskWriteMBps float64 `json:"avg_disk_write_mbps"`
}

type dayRunBucket struct {
	Day       string  `json:"day"`
	Completed int     `json:"completed"`
	Failed    int     `json:"failed"`
	AvgDurSec float64 `json:"avg_dur_sec"`
	MaxDurSec float64 `json:"max_dur_sec"`
	AvgRunNum float64 `json:"avg_run_number"` // retry indicator
}

type dayEfficiencyBucket struct {
	Day               string  `json:"day"`
	AvgTurns          float64 `json:"avg_turns"`
	AvgActions        float64 `json:"avg_actions"`
	AvgActionsPerTurn float64 `json:"avg_actions_per_turn"`
	AvgContextPct     float64 `json:"avg_context_pct"`
	AvgTokensPerTurn  float64 `json:"avg_tokens_per_turn"`
	AvgCacheHitPct    float64 `json:"avg_cache_hit_pct"`
	TotalCompactions  int64   `json:"total_compactions"`
	TotalErrors       int64   `json:"total_errors"`
	AvgTTFBMs         float64 `json:"avg_ttfb_ms"`
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
	TotalRuns         int     `json:"total_runs"`
	TotalFailed       int     `json:"total_failed"`
	TotalTurns        int64   `json:"total_turns"`
	TotalActions      int64   `json:"total_actions"`
	TotalCompactions  int64   `json:"total_compactions"`
	TotalErrors       int64   `json:"total_errors"`
	TotalInputTok     int64   `json:"total_input_tokens"`
	TotalOutputTok    int64   `json:"total_output_tokens"`
	TotalCacheRead    int64   `json:"total_cache_read_tokens"`
	TotalCacheCreate  int64   `json:"total_cache_create_tokens"`
	TotalLinesAdded   int64   `json:"total_lines_added"`
	TotalLinesRemoved int64   `json:"total_lines_removed"`
	TotalFilesChanged int64   `json:"total_files_changed"`
	TotalCommits      int64   `json:"total_commits"`
	TotalTestsRun     int64   `json:"total_tests_run"`
	TotalTestsPassed  int64   `json:"total_tests_passed"`
	TotalTestsFailed  int64   `json:"total_tests_failed"`
	AvgCacheHitPct    float64 `json:"avg_cache_hit_pct"`
	AvgTurns          float64 `json:"avg_turns"`
	AvgDurSec         float64 `json:"avg_dur_sec"`
	AvgContextPct     float64 `json:"avg_context_pct"`
}

// buildStatsWhereClause builds the WHERE fragment + arg slice for the given
// scope. since is the days-back filter (empty for all time). entityUIDs, when
// non-nil, restricts to rows where entity_uid IN (...). For the system sample
// query (event='sample'), pass terminalEvents=false; otherwise true.
func buildStatsWhereClause(since string, entityUIDs []string, terminalEvents bool) (string, []any) {
	var clauses []string
	var args []any

	if terminalEvents {
		clauses = append(clauses, "event IN ('completed','failed')")
	} else {
		clauses = append(clauses, "event = 'sample'")
	}

	if since != "" {
		if terminalEvents {
			clauses = append(clauses, "(CASE WHEN started_at>0 THEN datetime(started_at/1000,'unixepoch') ELSE created_at END) >= datetime('now', '-"+since+" days')")
		} else {
			clauses = append(clauses, "date(created_at) >= date('now', '-"+since+" days')")
		}
	}

	if entityUIDs != nil {
		if len(entityUIDs) == 0 {
			// Empty allow-list — match nothing.
			clauses = append(clauses, "1=0")
		} else {
			placeholders := make([]string, len(entityUIDs))
			for i, uid := range entityUIDs {
				placeholders[i] = "?"
				args = append(args, uid)
			}
			clauses = append(clauses, "entity_uid IN ("+strings.Join(placeholders, ",")+")")
		}
	}

	return strings.Join(clauses, " AND "), args
}

// computeStatsHistory runs every aggregation query for the given scope and
// returns the assembled statsHistory. entityUIDs=nil means "global" (no entity
// filter); entityUIDs=[]string{} means "explicit empty set, return zeros."
func computeStatsHistory(ctx context.Context, db *sql.DB, since string, entityUIDs []string) statsHistory {
	var h statsHistory
	whereClause, args := buildStatsWhereClause(since, entityUIDs, true)

	// ── Daily run activity ─────────────────────────────────────────────
	if rows, err := db.QueryContext(ctx, `
		SELECT date(datetime(CASE WHEN started_at>0 THEN started_at/1000 ELSE strftime('%s', created_at) END, 'unixepoch')) as day,
		       SUM(CASE WHEN event='completed' THEN 1 ELSE 0 END),
		       SUM(CASE WHEN event='failed' THEN 1 ELSE 0 END),
		       COALESCE(AVG(CASE WHEN duration_ms>0 THEN duration_ms/1000.0 END),0),
		       COALESCE(MAX(duration_ms)/1000.0,0),
		       COALESCE(AVG(CASE WHEN run_number>0 THEN run_number END),0)
		FROM execution_log WHERE `+whereClause+`
		GROUP BY day ORDER BY day ASC`, args...); err == nil {
		defer rows.Close()
		for rows.Next() {
			var b dayRunBucket
			rows.Scan(&b.Day, &b.Completed, &b.Failed, &b.AvgDurSec, &b.MaxDurSec, &b.AvgRunNum)
			h.Runs = append(h.Runs, b)
		}
	}

	// ── Daily efficiency ───────────────────────────────────────────────
	if rows, err := db.QueryContext(ctx, `
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
		GROUP BY day ORDER BY day ASC`, args...); err == nil {
		defer rows.Close()
		for rows.Next() {
			var b dayEfficiencyBucket
			rows.Scan(&b.Day, &b.AvgTurns, &b.AvgActions, &b.AvgActionsPerTurn,
				&b.AvgContextPct, &b.AvgTokensPerTurn, &b.AvgCacheHitPct,
				&b.TotalCompactions, &b.TotalErrors, &b.AvgTTFBMs)
			h.Efficiency = append(h.Efficiency, b)
		}
	}

	// ── Daily tokens ───────────────────────────────────────────────────
	if rows, err := db.QueryContext(ctx, `
		SELECT date(datetime(CASE WHEN started_at>0 THEN started_at/1000 ELSE strftime('%s', created_at) END, 'unixepoch')) as day,
		       COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0),
		       COALESCE(SUM(cache_read_tokens),0), COALESCE(SUM(cache_create_tokens),0),
		       COALESCE(SUM(reasoning_tokens),0), COALESCE(SUM(tool_use_tokens),0)
		FROM execution_log WHERE `+whereClause+`
		GROUP BY day ORDER BY day ASC`, args...); err == nil {
		defer rows.Close()
		for rows.Next() {
			var b dayTokenBucket
			rows.Scan(&b.Day, &b.InputTokens, &b.OutputTokens,
				&b.CacheReadTokens, &b.CacheCreateTokens,
				&b.ReasoningTokens, &b.ToolUseTokens)
			h.Tokens = append(h.Tokens, b)
		}
	}

	// ── Daily productivity ─────────────────────────────────────────────
	if rows, err := db.QueryContext(ctx, `
		SELECT date(datetime(CASE WHEN started_at>0 THEN started_at/1000 ELSE strftime('%s', created_at) END, 'unixepoch')) as day,
		       COALESCE(SUM(lines_added),0), COALESCE(SUM(lines_removed),0),
		       COALESCE(SUM(files_changed),0), COALESCE(SUM(commits),0),
		       COALESCE(SUM(tests_run),0), COALESCE(SUM(tests_passed),0),
		       COALESCE(SUM(tests_failed),0),
		       COUNT(DISTINCT CASE WHEN pr_number>0 THEN pr_number END)
		FROM execution_log WHERE `+whereClause+`
		GROUP BY day ORDER BY day ASC`, args...); err == nil {
		defer rows.Close()
		for rows.Next() {
			var b dayProductivityBucket
			rows.Scan(&b.Day, &b.LinesAdded, &b.LinesRemoved, &b.FilesChanged, &b.Commits,
				&b.TestsRun, &b.TestsPassed, &b.TestsFailed, &b.PRsOpened)
			h.Productivity = append(h.Productivity, b)
		}
	}

	// ── Models ─────────────────────────────────────────────────────────
	if rows, _ := db.QueryContext(ctx, `
		SELECT COALESCE(NULLIF(model,''),'unknown') as label, COUNT(*) as cnt
		FROM execution_log WHERE `+whereClause+` GROUP BY label ORDER BY cnt DESC`, args...); rows != nil {
		defer rows.Close()
		for rows.Next() {
			var lc labelCount
			rows.Scan(&lc.Label, &lc.Count)
			h.Models = append(h.Models, lc)
		}
	}

	// ── Agent runtimes ─────────────────────────────────────────────────
	if rows, _ := db.QueryContext(ctx, `
		SELECT COALESCE(NULLIF(agent_runtime,''),'unknown') as label, COUNT(*) as cnt
		FROM execution_log WHERE `+whereClause+` GROUP BY label ORDER BY cnt DESC`, args...); rows != nil {
		defer rows.Close()
		for rows.Next() {
			var lc labelCount
			rows.Scan(&lc.Label, &lc.Count)
			h.Agents = append(h.Agents, lc)
		}
	}

	// ── Terminal reasons ───────────────────────────────────────────────
	if rows, _ := db.QueryContext(ctx, `
		SELECT COALESCE(NULLIF(terminal_reason,''),'success') as label, COUNT(*) as cnt
		FROM execution_log WHERE `+whereClause+` GROUP BY label ORDER BY cnt DESC`, args...); rows != nil {
		defer rows.Close()
		for rows.Next() {
			var lc labelCount
			rows.Scan(&lc.Label, &lc.Count)
			h.TerminalReasons = append(h.TerminalReasons, lc)
		}
	}

	// ── Trigger split ──────────────────────────────────────────────────
	if rows, _ := db.QueryContext(ctx, `
		SELECT COALESCE(NULLIF(trigger,''),'unknown') as label, COUNT(*) as cnt
		FROM execution_log WHERE `+whereClause+` GROUP BY label ORDER BY cnt DESC`, args...); rows != nil {
		defer rows.Close()
		for rows.Next() {
			var lc labelCount
			rows.Scan(&lc.Label, &lc.Count)
			h.TriggerSplit = append(h.TriggerSplit, lc)
		}
	}

	// ── Totals ─────────────────────────────────────────────────────────
	db.QueryRowContext(ctx, `
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
		FROM execution_log WHERE `+whereClause, args...).
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
	// Source: execution_log sample rows (system_host_samples was dropped
	// in commit 79a8ca5). System samples are not entity-scoped, so when
	// an entity filter is in effect we skip this series entirely.
	if entityUIDs == nil {
		sysWhere, sysArgs := buildStatsWhereClause(since, nil, false)
		if rows, _ := db.QueryContext(ctx, `
			SELECT date(created_at) as day,
			       AVG(cpu_pct), AVG(net_rx_mbps), AVG(net_tx_mbps),
			       AVG(disk_read_mbps), AVG(disk_write_mbps)
			FROM execution_log WHERE `+sysWhere+`
			GROUP BY day ORDER BY day ASC`, sysArgs...); rows != nil {
			defer rows.Close()
			for rows.Next() {
				var b daySystemBucket
				if rows.Scan(&b.Day, &b.AvgCPUPct, &b.AvgNetRxMbps, &b.AvgNetTxMbps,
					&b.AvgDiskReadMBps, &b.AvgDiskWriteMBps) == nil {
					h.SystemDaily = append(h.SystemDaily, b)
				}
			}
		}
	}

	return h
}

func parseSinceDays(r *http.Request) string {
	if d := r.URL.Query().Get("days"); d != "" {
		if days, err := strconv.Atoi(d); err == nil && days > 0 {
			return strconv.Itoa(days)
		}
	}
	return ""
}

// apiStatsHistory handles GET /api/stats/history?days=90
// Returns full history daily-bucketed. Default: all time. No date limit enforced
// so operators see the full picture from day one.
func apiStatsHistory(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		since := parseSinceDays(r)
		h := computeStatsHistory(r.Context(), n.DB(), since, nil)
		writeJSON(w, h)
	}
}

// apiEntityStatsHistory handles GET /api/entities/{id}/stats?days=90
// Returns the same statsHistory shape as /api/stats/history, scoped to:
//   - task entity:     just this task's runs
//   - manifest entity: all linked tasks' runs (owns edges)
//   - product/other:   all descendant tasks' runs (two-hop owns walk)
func apiEntityStatsHistory(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id, ok := entityID(w, r)
		if !ok {
			return
		}
		since := parseSinceDays(r)

		uids := resolveEntityTaskUIDs(ctx, n, id)
		h := computeStatsHistory(ctx, n.DB(), since, uids)
		writeJSON(w, h)
	}
}

// resolveEntityTaskUIDs returns the set of entity_uids whose runs roll up to
// the given entity. Mirrors apiEntityExecutionLog's walk:
//   - task / unknown-no-relationships: just [id]
//   - manifest:                        owned tasks (1-hop owns walk)
//   - product / other:                 owned manifests' owned tasks (2-hop owns walk)
//                                      + the entity itself if it has direct runs
func resolveEntityTaskUIDs(ctx context.Context, n *node.Node, id string) []string {
	e, _ := n.Entities.Get(id)
	if e == nil || n.Relationships == nil {
		return []string{id}
	}
	if e.Type == relationships.KindTask {
		return []string{id}
	}

	if e.Type == relationships.KindManifest {
		edges, _ := n.Relationships.ListOutgoing(ctx, id, relationships.EdgeOwns)
		uids := []string{id}
		for _, edge := range edges {
			if edge.DstKind == relationships.KindTask {
				uids = append(uids, edge.DstID)
			}
		}
		return uids
	}

	// product or unknown kind: walk owned non-task entities, collect their
	// owned tasks. Include the entity itself in case it has direct runs.
	manifEdges, _ := n.Relationships.ListOutgoing(ctx, id, relationships.EdgeOwns)
	uids := []string{id}
	for _, me := range manifEdges {
		if me.DstKind == relationships.KindTask {
			uids = append(uids, me.DstID)
			continue
		}
		// non-task child — also count its own runs and walk its children.
		uids = append(uids, me.DstID)
		tEdges, _ := n.Relationships.ListOutgoing(ctx, me.DstID, relationships.EdgeOwns)
		for _, te := range tEdges {
			if te.DstKind == relationships.KindTask {
				uids = append(uids, te.DstID)
			}
		}
	}
	return uids
}
