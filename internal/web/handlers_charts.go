package web

import (
	"net/http"
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
}

type hourBucket struct {
	Hour      string `json:"hour"` // "2026-05-04T14:00:00Z"
	Completed int    `json:"completed"`
	Failed    int    `json:"failed"`
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

// pad24Hours fills in missing hours so every series always has 24 buckets.
func pad24ActivityHours(data []hourBucket) []hourBucket {
	m := make(map[string]hourBucket, len(data))
	for _, b := range data { m[b.Hour] = b }
	out := make([]hourBucket, 0, 24)
	now := time.Now().UTC()
	for i := 23; i >= 0; i-- {
		h := now.Add(-time.Duration(i)*time.Hour).Format("2006-01-02T15:00:00Z")
		if b, ok := m[h]; ok { out = append(out, b) } else { out = append(out, hourBucket{Hour: h}) }
	}
	return out
}
func pad24ProductivityHours(data []productivityBucket) []productivityBucket {
	m := make(map[string]productivityBucket, len(data))
	for _, b := range data { m[b.Hour] = b }
	out := make([]productivityBucket, 0, 24)
	now := time.Now().UTC()
	for i := 23; i >= 0; i-- {
		h := now.Add(-time.Duration(i)*time.Hour).Format("2006-01-02T15:00:00Z")
		if b, ok := m[h]; ok { out = append(out, b) } else { out = append(out, productivityBucket{Hour: h}) }
	}
	return out
}
func pad24EfficiencyHours(data []efficiencyBucket) []efficiencyBucket {
	m := make(map[string]efficiencyBucket, len(data))
	for _, b := range data { m[b.Hour] = b }
	out := make([]efficiencyBucket, 0, 24)
	now := time.Now().UTC()
	for i := 23; i >= 0; i-- {
		h := now.Add(-time.Duration(i)*time.Hour).Format("2006-01-02T15:00:00Z")
		if b, ok := m[h]; ok { out = append(out, b) } else { out = append(out, efficiencyBucket{Hour: h}) }
	}
	return out
}
func pad24TokenHours(data []tokenBucket) []tokenBucket {
	m := make(map[string]tokenBucket, len(data))
	for _, b := range data { m[b.Hour] = b }
	out := make([]tokenBucket, 0, 24)
	now := time.Now().UTC()
	for i := 23; i >= 0; i-- {
		h := now.Add(-time.Duration(i)*time.Hour).Format("2006-01-02T15:00:00Z")
		if b, ok := m[h]; ok { out = append(out, b) } else { out = append(out, tokenBucket{Hour: h}) }
	}
	return out
}

// apiStatsCharts handles GET /api/stats/charts.
// Returns 24 hourly buckets across activity, productivity, efficiency,
// and token economics — all from execution_log, no joins.
func apiStatsCharts(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		since := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)

		var d chartsData

		// ── Hourly activity (completed/failed per hour) ───────────────────
		rows, err := n.DB().QueryContext(r.Context(), `
			SELECT strftime('%Y-%m-%dT%H:00:00Z', created_at) as hour,
			       SUM(CASE WHEN event='completed' THEN 1 ELSE 0 END),
			       SUM(CASE WHEN event='failed'    THEN 1 ELSE 0 END)
			FROM execution_log
			WHERE event IN ('completed','failed') AND created_at >= ?
			GROUP BY hour ORDER BY hour ASC`, since)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var b hourBucket
				if rows.Scan(&b.Hour, &b.Completed, &b.Failed) == nil {
					d.Activity = append(d.Activity, b)
				}
			}
		}

		// ── Hourly productivity ────────────────────────────────────────────
		rows2, err := n.DB().QueryContext(r.Context(), `
			SELECT strftime('%Y-%m-%dT%H:00:00Z', created_at) as hour,
			       COALESCE(SUM(lines_added),0), COALESCE(SUM(lines_removed),0),
			       COALESCE(SUM(files_changed),0), COALESCE(SUM(commits),0),
			       COALESCE(SUM(tests_run),0), COALESCE(SUM(tests_passed),0),
			       COALESCE(SUM(tests_failed),0)
			FROM execution_log
			WHERE event IN ('completed','failed') AND created_at >= ?
			GROUP BY hour ORDER BY hour ASC`, since)
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

		// ── Hourly efficiency ──────────────────────────────────────────────
		rows3, err := n.DB().QueryContext(r.Context(), `
			SELECT strftime('%Y-%m-%dT%H:00:00Z', created_at) as hour,
			       AVG(turns),
			       AVG(CASE WHEN turns>0 THEN CAST(actions AS REAL)/turns ELSE NULL END),
			       AVG(cache_hit_rate_pct)
			FROM execution_log
			WHERE event IN ('completed','failed') AND created_at >= ?
			GROUP BY hour ORDER BY hour ASC`, since)
		if err == nil {
			defer rows3.Close()
			for rows3.Next() {
				var b efficiencyBucket
				if rows3.Scan(&b.Hour, &b.AvgTurns, &b.AvgActionsPerTurn, &b.CacheHitRatePct) == nil {
					d.Efficiency = append(d.Efficiency, b)
				}
			}
		}

		// ── Hourly token economics ─────────────────────────────────────────
		rows4, err := n.DB().QueryContext(r.Context(), `
			SELECT strftime('%Y-%m-%dT%H:00:00Z', created_at) as hour,
			       COALESCE(SUM(cache_read_tokens),0), COALESCE(SUM(cache_create_tokens),0),
			       COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0)
			FROM execution_log
			WHERE event IN ('completed','failed','sample') AND created_at >= ?
			GROUP BY hour ORDER BY hour ASC`, since)
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

		// ── Productivity totals ────────────────────────────────────────────
		n.DB().QueryRowContext(r.Context(), `
			SELECT COALESCE(SUM(commits),0), COALESCE(SUM(lines_added),0),
			       COALESCE(SUM(lines_removed),0), COALESCE(SUM(files_changed),0),
			       COUNT(DISTINCT CASE WHEN pr_number>0 THEN pr_number END),
			       COALESCE(SUM(tests_run),0), COALESCE(SUM(tests_passed),0),
			       COALESCE(SUM(tests_failed),0),
			       COUNT(DISTINCT CASE WHEN worktree_path!='' THEN worktree_path END)
			FROM execution_log
			WHERE event IN ('completed','failed') AND created_at >= ?`, since).
			Scan(&d.TotalCommits, &d.TotalLinesAdded, &d.TotalLinesRemoved,
				&d.TotalFilesChanged, &d.TotalPRsOpened,
				&d.TotalTestsRun, &d.TotalTestsPassed, &d.TotalTestsFailed,
				&d.ReposTouched)

		// ── Interactive vs autonomous split ───────────────────────────────
		n.DB().QueryRowContext(r.Context(), `
			SELECT COALESCE(SUM(CASE WHEN trigger='interactive' THEN 1 ELSE 0 END),0),
			       COALESCE(SUM(CASE WHEN trigger!='interactive' THEN 1 ELSE 0 END),0)
			FROM execution_log
			WHERE event IN ('completed','failed') AND created_at >= ?`, since).
			Scan(&d.InteractiveRuns, &d.AutonomousRuns)

		// ── Terminal reasons ──────────────────────────────────────────────
		rows5, err := n.DB().QueryContext(r.Context(), `
			SELECT COALESCE(terminal_reason,'success') as reason, COUNT(*) as cnt
			FROM execution_log
			WHERE event IN ('completed','failed') AND created_at >= ?
			GROUP BY reason ORDER BY cnt DESC LIMIT 10`, since)
		if err == nil {
			defer rows5.Close()
			for rows5.Next() {
				var rc reasonCount
				if rows5.Scan(&rc.Reason, &rc.Count) == nil {
					d.TerminalReasons = append(d.TerminalReasons, rc)
				}
			}
		}

		// Pad all series to full 24-hour grids so charts span the complete day.
		d.Activity     = pad24ActivityHours(d.Activity)
		d.Productivity = pad24ProductivityHours(d.Productivity)
		d.Efficiency   = pad24EfficiencyHours(d.Efficiency)
		d.Tokens       = pad24TokenHours(d.Tokens)

		writeJSON(w, d)
	}
}
