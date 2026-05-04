package web

import (
	"net/http"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/node"
)

// overviewStats is the wire shape for GET /api/stats/overview.
// All values are cumulative for the rolling 24-hour window ending now.
type overviewStats struct {
	Window string `json:"window"` // "24h"

	// Totals across all runs
	Runs              int     `json:"runs"`
	RunsFailed        int     `json:"runs_failed"`
	Turns             int64   `json:"turns"`
	Actions           int64   `json:"actions"`
	InputTokens       int64   `json:"input_tokens"`
	OutputTokens      int64   `json:"output_tokens"`
	CacheReadTokens   int64   `json:"cache_read_tokens"`
	CacheCreateTokens int64   `json:"cache_create_tokens"`
	CacheHitRatePct   float64 `json:"cache_hit_rate_pct"`

	// Split by trigger
	Interactive overviewTrigger `json:"interactive"`
	Autonomous  overviewTrigger `json:"autonomous"`
}

type overviewTrigger struct {
	Runs    int   `json:"runs"`
	Turns   int64 `json:"turns"`
	Actions int64 `json:"actions"`
}

// apiStatsOverview handles GET /api/stats/overview.
// Returns cumulative metrics for the rolling 24-hour window from execution_log.
// Only completed/failed rows are counted — sample and started rows are live state,
// not final values.
func apiStatsOverview(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		since := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)

		// For completed/failed runs: use the terminal row (final state).
		// For active runs (started but not yet completed): use the latest
		// sample row per run_uid so live interactive sessions show up.
		// For each run_uid, use the terminal row if available (completed/failed),
		// otherwise the latest sample row. This ensures in-progress sessions
		// (interactive or autonomous) appear in today's totals.
		rows, err := n.DB().QueryContext(r.Context(), `
			WITH terminal AS (
				SELECT event, trigger, turns, actions,
				       input_tokens, output_tokens, cache_read_tokens, cache_create_tokens,
				       cache_hit_rate_pct, run_uid
				FROM execution_log
				WHERE event IN ('completed', 'failed') AND created_at >= ?
			),
			active_latest AS (
				SELECT e.event, e.trigger, e.turns, e.actions,
				       e.input_tokens, e.output_tokens, e.cache_read_tokens, e.cache_create_tokens,
				       e.cache_hit_rate_pct, e.run_uid
				FROM execution_log e
				INNER JOIN (
					SELECT run_uid, MAX(created_at) as max_at
					FROM execution_log
					WHERE event IN ('sample','started') AND created_at >= ?
					GROUP BY run_uid
				) latest ON e.run_uid = latest.run_uid AND e.created_at = latest.max_at
				WHERE e.run_uid NOT IN (SELECT run_uid FROM terminal)
			),
			combined AS (SELECT * FROM terminal UNION ALL SELECT * FROM active_latest)
			SELECT
				CASE WHEN event = 'failed' THEN 'failed' ELSE 'active' END as state,
				trigger,
				COALESCE(SUM(turns), 0),
				COALESCE(SUM(actions), 0),
				COALESCE(SUM(input_tokens), 0),
				COALESCE(SUM(output_tokens), 0),
				COALESCE(SUM(cache_read_tokens), 0),
				COALESCE(SUM(cache_create_tokens), 0),
				COALESCE(AVG(cache_hit_rate_pct), 0),
				COUNT(DISTINCT run_uid)
			FROM combined
			GROUP BY state, trigger
		`, since, since)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()

		var s overviewStats
		s.Window = "24h"

		for rows.Next() {
			var (
				event, trigger                                       string
				turns, actions, inputTok, outputTok, cacheRead, cacheCreate int64
				cacheHit                                             float64
				count                                                int
			)
			if err := rows.Scan(&event, &trigger,
				&turns, &actions, &inputTok, &outputTok, &cacheRead, &cacheCreate,
				&cacheHit, &count); err != nil {
				continue
			}

			if event == "failed" {
				s.RunsFailed += count
			} else {
				s.Runs += count
			}

			s.Turns             += turns
			s.Actions           += actions
			s.InputTokens       += inputTok
			s.OutputTokens      += outputTok
			s.CacheReadTokens   += cacheRead
			s.CacheCreateTokens += cacheCreate

			if trigger == "interactive" {
				s.Interactive.Runs    += count
				s.Interactive.Turns   += turns
				s.Interactive.Actions += actions
			} else {
				s.Autonomous.Runs    += count
				s.Autonomous.Turns   += turns
				s.Autonomous.Actions += actions
			}
		}

		// Recompute cache hit rate from token totals (more accurate than averaging averages).
		totalTok := s.CacheReadTokens + s.CacheCreateTokens + s.InputTokens + s.OutputTokens
		if totalTok > 0 {
			s.CacheHitRatePct = float64(s.CacheReadTokens) / float64(totalTok) * 100
		}

		writeJSON(w, s)
	}
}
