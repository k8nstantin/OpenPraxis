package web

import (
	"net/http"
	"strconv"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/node"
)

// chartsData is the wire shape for GET /api/stats/charts.
// All buckets cover the rolling 24-hour window, one row per hour.
type chartsData struct {
	// Hourly activity — runs per hour split by outcome
	Activity []hourBucket `json:"activity"`
	// Hourly productivity — code output per hour
	Productivity []productivityBucket `json:"productivity"`
	// Hourly efficiency — cache hit rate, turns/run, actions/turn
	Efficiency []efficiencyBucket `json:"efficiency"`
	// Hourly token economics — cache read vs write ratio
	Tokens []tokenBucket `json:"tokens"`

	// Productivity totals for the 24h window
	TotalCommits      int64 `json:"total_commits"`
	TotalLinesAdded   int64 `json:"total_lines_added"`
	TotalLinesRemoved int64 `json:"total_lines_removed"`
	TotalFilesChanged int64 `json:"total_files_changed"`
	TotalPRsOpened    int64 `json:"total_prs_opened"`
	TotalTestsRun     int64 `json:"total_tests_run"`
	TotalTestsPassed  int64 `json:"total_tests_passed"`
	TotalTestsFailed  int64 `json:"total_tests_failed"`
	ReposTouched      int64 `json:"repos_touched"`

	// Split totals
	InteractiveRuns int64 `json:"interactive_runs"`
	AutonomousRuns  int64 `json:"autonomous_runs"`

	// Terminal reason breakdown
	TerminalReasons []reasonCount `json:"terminal_reasons"`

	// Hourly system averages — from system_host_samples
	System []systemHourBucket `json:"system"`
}

type systemHourBucket struct {
	Hour        string  `json:"hour"`
	AvgCPUPct   float64 `json:"avg_cpu_pct"`
	AvgMemUsedMB float64 `json:"avg_mem_used_mb"`
	AvgNetRxMbps float64 `json:"avg_net_rx_mbps"`
	AvgNetTxMbps float64 `json:"avg_net_tx_mbps"`
	AvgDiskReadMBps  float64 `json:"avg_disk_read_mbps"`
	AvgDiskWriteMBps float64 `json:"avg_disk_write_mbps"`
}

type hourBucket struct {
	Hour      string `json:"hour"` // "2026-05-04T14:00:00Z"
	Completed int    `json:"completed"`
	Failed    int    `json:"failed"`
	Running   int    `json:"running"` // started but no terminal event yet
}

type productivityBucket struct {
	Hour         string `json:"hour"`
	LinesAdded   int64  `json:"lines_added"`
	LinesRemoved int64  `json:"lines_removed"`
	FilesChanged int64  `json:"files_changed"`
	Commits      int64  `json:"commits"`
	TestsRun     int64  `json:"tests_run"`
	TestsPassed  int64  `json:"tests_passed"`
	TestsFailed  int64  `json:"tests_failed"`
}

type efficiencyBucket struct {
	Hour            string  `json:"hour"`
	AvgTurns        float64 `json:"avg_turns"`
	AvgActionsPerTurn float64 `json:"avg_actions_per_turn"`
	CacheHitRatePct float64 `json:"cache_hit_rate_pct"`
}

type tokenBucket struct {
	Hour              string `json:"hour"`
	CacheReadTokens   int64  `json:"cache_read_tokens"`
	CacheCreateTokens int64  `json:"cache_create_tokens"`
	InputTokens       int64  `json:"input_tokens"`
	OutputTokens      int64  `json:"output_tokens"`
}

type reasonCount struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

// easternTZ is America/New_York — charts always show Eastern time.
var easternTZ, _ = time.LoadLocation("America/New_York")

// pad24*Hours fills in all 24 hourly buckets (oldest → newest) so charts
// always have a full day of x-axis slots even when some hours have no data.
func pad24ActivityHours(data []hourBucket) []hourBucket {
	m := make(map[string]hourBucket, len(data))
	for _, b := range data { m[b.Hour] = b }
	var out []hourBucket
	now := time.Now().UTC()
	for i := 23; i >= 0; i-- {
		h := now.Add(-time.Duration(i) * time.Hour).Truncate(time.Hour).Format("2006-01-02T15:00:00Z")
		if b, ok := m[h]; ok { out = append(out, b) } else { out = append(out, hourBucket{Hour: h}) }
	}
	return out
}
func pad24ProductivityHours(data []productivityBucket) []productivityBucket {
	m := make(map[string]productivityBucket, len(data))
	for _, b := range data { m[b.Hour] = b }
	var out []productivityBucket
	now := time.Now().UTC()
	for i := 23; i >= 0; i-- {
		h := now.Add(-time.Duration(i) * time.Hour).Truncate(time.Hour).Format("2006-01-02T15:00:00Z")
		if b, ok := m[h]; ok { out = append(out, b) } else { out = append(out, productivityBucket{Hour: h}) }
	}
	return out
}
func pad24EfficiencyHours(data []efficiencyBucket) []efficiencyBucket {
	m := make(map[string]efficiencyBucket, len(data))
	for _, b := range data { m[b.Hour] = b }
	var out []efficiencyBucket
	now := time.Now().UTC()
	for i := 23; i >= 0; i-- {
		h := now.Add(-time.Duration(i) * time.Hour).Truncate(time.Hour).Format("2006-01-02T15:00:00Z")
		if b, ok := m[h]; ok { out = append(out, b) } else { out = append(out, efficiencyBucket{Hour: h}) }
	}
	return out
}
func pad24TokenHours(data []tokenBucket) []tokenBucket {
	m := make(map[string]tokenBucket, len(data))
	for _, b := range data { m[b.Hour] = b }
	var out []tokenBucket
	now := time.Now().UTC()
	for i := 23; i >= 0; i-- {
		h := now.Add(-time.Duration(i) * time.Hour).Truncate(time.Hour).Format("2006-01-02T15:00:00Z")
		if b, ok := m[h]; ok { out = append(out, b) } else { out = append(out, tokenBucket{Hour: h}) }
	}
	return out
}

// todayHours returns UTC-formatted hour buckets from 00:00 ET today through
// the current hour ET, inclusive. This gives a "today so far" x-axis that
// starts at midnight and ends at the current hour.
func todayHours() []string {
	now := time.Now().In(easternTZ)
	// Midnight of today in Eastern time, expressed as UTC.
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, easternTZ).UTC()
	// How many hours from midnight to current local hour?
	hoursToday := int(now.Hour()) + 1 // +1 to include current hour
	hours := make([]string, hoursToday)
	for i := 0; i < hoursToday; i++ {
		hours[i] = midnight.Add(time.Duration(i) * time.Hour).Format("2006-01-02T15:00:00Z")
	}
	return hours
}

func padTodayActivityHours(data []hourBucket) []hourBucket {
	m := make(map[string]hourBucket, len(data))
	for _, b := range data { m[b.Hour] = b }
	var out []hourBucket
	for _, h := range todayHours() {
		if b, ok := m[h]; ok { out = append(out, b) } else { out = append(out, hourBucket{Hour: h}) }
	}
	return out
}
func padTodayProductivityHours(data []productivityBucket) []productivityBucket {
	m := make(map[string]productivityBucket, len(data))
	for _, b := range data { m[b.Hour] = b }
	var out []productivityBucket
	for _, h := range todayHours() {
		if b, ok := m[h]; ok { out = append(out, b) } else { out = append(out, productivityBucket{Hour: h}) }
	}
	return out
}
func padTodayEfficiencyHours(data []efficiencyBucket) []efficiencyBucket {
	m := make(map[string]efficiencyBucket, len(data))
	for _, b := range data { m[b.Hour] = b }
	var out []efficiencyBucket
	for _, h := range todayHours() {
		if b, ok := m[h]; ok { out = append(out, b) } else { out = append(out, efficiencyBucket{Hour: h}) }
	}
	return out
}
func padTodayTokenHours(data []tokenBucket) []tokenBucket {
	m := make(map[string]tokenBucket, len(data))
	for _, b := range data { m[b.Hour] = b }
	var out []tokenBucket
	for _, h := range todayHours() {
		if b, ok := m[h]; ok { out = append(out, b) } else { out = append(out, tokenBucket{Hour: h}) }
	}
	return out
}

// realTS returns a SQL expression that gives the real event timestamp as unix seconds.
// started_at is stored in unix ms and is the authoritative timestamp;
// created_at is a fallback for rows predating the started_at column.
const realTS = `CASE WHEN started_at>0 THEN started_at/1000 ELSE strftime('%s', created_at) END`

// hourExpr returns a SQL expression that buckets realTS into "YYYY-MM-DDTHH:00:00Z".
const hourExpr = `strftime('%Y-%m-%dT%H:00:00Z', datetime(` + realTS + `, 'unixepoch'))`

// apiStatsCharts handles GET /api/stats/charts.
// Window: midnight Eastern time today → now. x-axis always starts at 0h.
func apiStatsCharts(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nowET := time.Now().In(easternTZ)
		midnightET := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), 0, 0, 0, 0, easternTZ)
		sinceUnix := midnightET.Unix()
		// WHERE clause uses realTS comparison (unix seconds)
		whereClause := `event IN ('completed','failed') AND (` + realTS + `) >= ` + strconv.FormatInt(sinceUnix, 10)

		var d chartsData

		// ── Hourly activity ───────────────────────────────────────────────
		// Activity query:
		// - completed/failed: bucketed by their own timestamp (when the run ended)
		// - running (no terminal): bucketed by LATEST event timestamp — the most
		//   recent sample row shows when the session was last active, not when it started.
		//   A session started 20h ago but sampled 5 min ago appears in the current hour.
		actWhere := `(` + realTS + `) >= ` + strconv.FormatInt(sinceUnix, 10)
		rows, err := n.DB().QueryContext(r.Context(), `
			SELECT `+hourExpr+` as hour,
			       SUM(CASE WHEN event='completed' THEN 1 ELSE 0 END),
			       SUM(CASE WHEN event='failed'    THEN 1 ELSE 0 END),
			       0 as running
			FROM execution_log WHERE `+actWhere+`
			  AND event IN ('completed','failed')
			GROUP BY hour
			UNION ALL
			-- Running sessions: bucket by latest event time for that run_uid
			SELECT `+hourExpr+` as hour,
			       0, 0, COUNT(DISTINCT run_uid) as running
			FROM execution_log el
			INNER JOIN (
			  SELECT run_uid, MAX(id) AS max_id FROM execution_log GROUP BY run_uid
			) latest ON el.id = latest.max_id
			WHERE el.run_uid NOT IN (
			  SELECT run_uid FROM execution_log WHERE event IN ('completed','failed')
			)
			AND (` + realTS + `) >= ` + strconv.FormatInt(sinceUnix, 10) + `
			GROUP BY hour
			ORDER BY hour ASC`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var b hourBucket
				if rows.Scan(&b.Hour, &b.Completed, &b.Failed, &b.Running) == nil {
					d.Activity = append(d.Activity, b)
				}
			}
		}

		// ── Hourly productivity ───────────────────────────────────────────
		rows2, err := n.DB().QueryContext(r.Context(), `
			SELECT `+hourExpr+` as hour,
			       COALESCE(SUM(lines_added),0), COALESCE(SUM(lines_removed),0),
			       COALESCE(SUM(files_changed),0), COALESCE(SUM(commits),0),
			       COALESCE(SUM(tests_run),0), COALESCE(SUM(tests_passed),0),
			       COALESCE(SUM(tests_failed),0)
			FROM execution_log WHERE `+whereClause+`
			GROUP BY hour ORDER BY hour ASC`)
		if err == nil {
			defer rows2.Close()
			for rows2.Next() {
				var b productivityBucket
				if rows2.Scan(&b.Hour, &b.LinesAdded, &b.LinesRemoved,
					&b.FilesChanged, &b.Commits,
					&b.TestsRun, &b.TestsPassed, &b.TestsFailed) == nil {
					d.Productivity = append(d.Productivity, b)
				}
			}
		}

		// ── Hourly efficiency ─────────────────────────────────────────────
		// Use the latest row per run_uid so both completed runs and still-running
		// interactive sessions (which have cache_hit_rate_pct on their sample rows)
		// contribute one data point each.
		effWhere := `(` + realTS + `) >= ` + strconv.FormatInt(sinceUnix, 10)
		rows3, err := n.DB().QueryContext(r.Context(), `
			SELECT `+hourExpr+` as hour,
			       COALESCE(AVG(CASE WHEN turns>0 THEN turns END),0),
			       COALESCE(AVG(CASE WHEN turns>0 AND actions>0 THEN CAST(actions AS REAL)/turns END),0),
			       COALESCE(AVG(CASE WHEN cache_hit_rate_pct>0 THEN cache_hit_rate_pct END),0)
			FROM execution_log el
			INNER JOIN (
			  SELECT run_uid, MAX(id) AS max_id FROM execution_log GROUP BY run_uid
			) latest ON el.id = latest.max_id
			WHERE `+effWhere+`
			GROUP BY hour ORDER BY hour ASC`)
		if err == nil {
			defer rows3.Close()
			for rows3.Next() {
				var b efficiencyBucket
				if rows3.Scan(&b.Hour, &b.AvgTurns, &b.AvgActionsPerTurn, &b.CacheHitRatePct) == nil {
					d.Efficiency = append(d.Efficiency, b)
				}
			}
		}

		// ── Hourly tokens ─────────────────────────────────────────────────
		tokWhere := `event IN ('completed','failed','sample') AND (` + realTS + `) >= ` + strconv.FormatInt(sinceUnix, 10)
		rows4, err := n.DB().QueryContext(r.Context(), `
			SELECT `+hourExpr+` as hour,
			       COALESCE(SUM(cache_read_tokens),0), COALESCE(SUM(cache_create_tokens),0),
			       COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0)
			FROM execution_log WHERE `+tokWhere+`
			GROUP BY hour ORDER BY hour ASC`)
		if err == nil {
			defer rows4.Close()
			for rows4.Next() {
				var b tokenBucket
				if rows4.Scan(&b.Hour, &b.CacheReadTokens, &b.CacheCreateTokens,
					&b.InputTokens, &b.OutputTokens) == nil {
					d.Tokens = append(d.Tokens, b)
				}
			}
		}

		// ── Totals ────────────────────────────────────────────────────────
		n.DB().QueryRowContext(r.Context(), `
			SELECT COALESCE(SUM(commits),0), COALESCE(SUM(lines_added),0),
			       COALESCE(SUM(lines_removed),0), COALESCE(SUM(files_changed),0),
			       COUNT(DISTINCT CASE WHEN pr_number>0 THEN pr_number END),
			       COALESCE(SUM(tests_run),0), COALESCE(SUM(tests_passed),0),
			       COALESCE(SUM(tests_failed),0),
			       COUNT(DISTINCT CASE WHEN commit_sha!='' THEN commit_sha END)
			FROM execution_log WHERE `+whereClause).
			Scan(&d.TotalCommits, &d.TotalLinesAdded, &d.TotalLinesRemoved,
				&d.TotalFilesChanged, &d.TotalPRsOpened,
				&d.TotalTestsRun, &d.TotalTestsPassed, &d.TotalTestsFailed,
				&d.ReposTouched)

		// ── Interactive vs autonomous ─────────────────────────────────────
		n.DB().QueryRowContext(r.Context(), `
			SELECT COALESCE(SUM(CASE WHEN trigger='interactive' OR agent_runtime='' THEN 1 ELSE 0 END),0),
			       COALESCE(SUM(CASE WHEN trigger!='interactive' AND agent_runtime!='' THEN 1 ELSE 0 END),0)
			FROM execution_log WHERE `+whereClause).
			Scan(&d.InteractiveRuns, &d.AutonomousRuns)

		// ── Terminal reasons ──────────────────────────────────────────────
		rows5, err := n.DB().QueryContext(r.Context(), `
			SELECT COALESCE(NULLIF(terminal_reason,''),'success') as reason, COUNT(*) as cnt
			FROM execution_log WHERE `+whereClause+`
			GROUP BY reason ORDER BY cnt DESC LIMIT 10`)
		if err == nil {
			defer rows5.Close()
			for rows5.Next() {
				var rc reasonCount
				if rows5.Scan(&rc.Reason, &rc.Count) == nil {
					d.TerminalReasons = append(d.TerminalReasons, rc)
				}
			}
		}

		// ── Hourly system averages ────────────────────────────────────────
		since24 := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
		sysRows, err := n.DB().QueryContext(r.Context(), `
			SELECT strftime('%Y-%m-%dT%H:00:00Z', ts) as hour,
			       AVG(cpu_pct), AVG(mem_used_mb),
			       AVG(net_rx_mbps), AVG(net_tx_mbps),
			       AVG(disk_read_mbps), AVG(disk_write_mbps)
			FROM system_host_samples
			WHERE ts >= ?
			GROUP BY hour ORDER BY hour ASC`, since24)
		if err == nil {
			defer sysRows.Close()
			for sysRows.Next() {
				var b systemHourBucket
				if sysRows.Scan(&b.Hour, &b.AvgCPUPct, &b.AvgMemUsedMB,
					&b.AvgNetRxMbps, &b.AvgNetTxMbps,
					&b.AvgDiskReadMBps, &b.AvgDiskWriteMBps) == nil {
					d.System = append(d.System, b)
				}
			}
		}

		d.Activity     = padTodayActivityHours(d.Activity)
		d.Productivity = padTodayProductivityHours(d.Productivity)
		d.Efficiency   = padTodayEfficiencyHours(d.Efficiency)
		d.Tokens       = padTodayTokenHours(d.Tokens)

		writeJSON(w, d)
	}
}
