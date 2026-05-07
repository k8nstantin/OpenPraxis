package web

import (
	"net/http"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/node"
)

// GET /api/stats/aggregate
//
// Aggregates execution_log WHERE event IN ('completed','failed').
// Returns cumulative rollups plus cost_today (completed rows from today UTC).
func apiStatsAggregate(n *node.Node) http.HandlerFunc {
	type response struct {
		RunsTotal           int64   `json:"runs_total"`
		CostTotal           float64 `json:"cost_total"`
		TurnsTotal          int64   `json:"turns_total"`
		ActionsTotal        int64   `json:"actions_total"`
		ErrorsTotal         int64   `json:"errors_total"`
		InputTokensTotal    int64   `json:"input_tokens_total"`
		OutputTokensTotal   int64   `json:"output_tokens_total"`
		CacheReadTotal      int64   `json:"cache_read_tokens_total"`
		CacheCreateTotal    int64   `json:"cache_create_tokens_total"`
		AvgCacheHitRatePct  float64 `json:"avg_cache_hit_rate_pct"`
		AvgCostPerTurn      float64 `json:"avg_cost_per_turn"`
		CostToday           float64 `json:"cost_today"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=10")
		db := n.DB()

		// Aggregate over all terminal rows.
		const aggSQL = `
SELECT
  COUNT(*)                                  AS runs_total,
  COALESCE(SUM(cost_usd),             0)   AS cost_total,
  COALESCE(SUM(turns),                0)   AS turns_total,
  COALESCE(SUM(actions),              0)   AS actions_total,
  COALESCE(SUM(errors),               0)   AS errors_total,
  COALESCE(SUM(input_tokens),         0)   AS input_tokens_total,
  COALESCE(SUM(output_tokens),        0)   AS output_tokens_total,
  COALESCE(SUM(cache_read_tokens),    0)   AS cache_read_tokens_total,
  COALESCE(SUM(cache_create_tokens),  0)   AS cache_create_tokens_total,
  COALESCE(AVG(CASE WHEN cache_hit_rate_pct > 0 THEN cache_hit_rate_pct END), 0) AS avg_cache_hit_rate_pct,
  COALESCE(AVG(CASE WHEN cost_per_turn  > 0 THEN cost_per_turn  END), 0) AS avg_cost_per_turn
FROM execution_log
WHERE event IN ('completed', 'failed')`

		var resp response
		row := db.QueryRowContext(r.Context(), aggSQL)
		if err := row.Scan(
			&resp.RunsTotal,
			&resp.CostTotal,
			&resp.TurnsTotal,
			&resp.ActionsTotal,
			&resp.ErrorsTotal,
			&resp.InputTokensTotal,
			&resp.OutputTokensTotal,
			&resp.CacheReadTotal,
			&resp.CacheCreateTotal,
			&resp.AvgCacheHitRatePct,
			&resp.AvgCostPerTurn,
		); err != nil {
			writeError(w, err.Error(), 500)
			return
		}

		// cost_today: completed rows where DATE(created_at) = DATE('now')
		const todaySQL = `
SELECT COALESCE(SUM(cost_usd), 0)
FROM execution_log
WHERE event = 'completed'
  AND DATE(created_at) = DATE('now')`

		if err := db.QueryRowContext(r.Context(), todaySQL).Scan(&resp.CostToday); err != nil {
			writeError(w, err.Error(), 500)
			return
		}

		writeJSON(w, resp)
	}
}

// GET /api/stats/trend?since=<iso>&bucket=hour
//
// Buckets terminal rows by hour. Default since = 24 hours ago.
func apiStatsTrend(n *node.Node) http.HandlerFunc {
	type sample struct {
		TS      string  `json:"ts"`
		Cost    float64 `json:"cost"`
		Turns   int64   `json:"turns"`
		Actions int64   `json:"actions"`
	}
	type response struct {
		Samples []sample `json:"samples"`
		Period  string   `json:"period"`
		Bucket  string   `json:"bucket"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=30")
		q := r.URL.Query()

		sinceStr := q.Get("since")
		var since time.Time
		period := "24h"
		if sinceStr != "" {
			t, err := time.Parse(time.RFC3339, sinceStr)
			if err != nil {
				writeError(w, "since must be RFC3339", 400)
				return
			}
			since = t
			period = sinceStr
		} else {
			since = time.Now().UTC().Add(-24 * time.Hour)
		}

		bucket := q.Get("bucket")
		if bucket == "" {
			bucket = "hour"
		}

		// Only 'hour' bucket is supported for now; reject others explicitly.
		if bucket != "hour" {
			writeError(w, "bucket must be 'hour'", 400)
			return
		}

		const trendSQL = `
SELECT
  strftime('%Y-%m-%dT%H:00:00Z', created_at) AS ts,
  COALESCE(SUM(cost_usd), 0)   AS cost,
  COALESCE(SUM(turns),    0)   AS turns,
  COALESCE(SUM(actions),  0)   AS actions
FROM execution_log
WHERE event IN ('completed', 'failed')
  AND created_at >= ?
GROUP BY ts
ORDER BY ts ASC`

		rows, err := n.DB().QueryContext(r.Context(), trendSQL,
			since.UTC().Format(time.RFC3339Nano))
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		defer rows.Close()

		samples := []sample{}
		for rows.Next() {
			var s sample
			if err := rows.Scan(&s.TS, &s.Cost, &s.Turns, &s.Actions); err != nil {
				writeError(w, err.Error(), 500)
				return
			}
			samples = append(samples, s)
		}
		if err := rows.Err(); err != nil {
			writeError(w, err.Error(), 500)
			return
		}

		writeJSON(w, response{Samples: samples, Period: period, Bucket: bucket})
	}
}

// GET /api/execution/live
//
// Returns currently running executions (started but not yet terminal)
// with their latest sample metrics joined to entity titles.
func apiExecutionLive(n *node.Node) http.HandlerFunc {
	type liveTask struct {
		RunUID           string  `json:"run_uid"`
		EntityUID        string  `json:"entity_uid"`
		EntityTitle      string  `json:"entity_title"`
		EntityType       string  `json:"entity_type"`
		StartedAt        int64   `json:"started_at"`
		ElapsedSec       int64   `json:"elapsed_sec"`
		Turns            int     `json:"turns"`
		Actions          int     `json:"actions"`
		CostUSD          float64 `json:"cost_usd"`
		CPUPCT           float64 `json:"cpu_pct"`
		RSSMB            float64 `json:"rss_mb"`
		LinesAdded       int     `json:"lines_added"`
		LinesRemoved     int     `json:"lines_removed"`
		InputTokens      int64   `json:"input_tokens"`
		OutputTokens     int64   `json:"output_tokens"`
		CacheReadTokens  int64   `json:"cache_read_tokens"`
		CacheCreateTokens int64  `json:"cache_create_tokens"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		db := n.DB()

		// Find run_uids for executions that have a 'started' row but no
		// 'completed' or 'failed' row.
		const runningSQL = `
SELECT DISTINCT s.run_uid
FROM execution_log s
WHERE s.event = 'started'
  AND s.run_uid NOT IN (
    SELECT run_uid FROM execution_log
    WHERE event IN ('completed', 'failed')
  )`

		runRows, err := db.QueryContext(r.Context(), runningSQL)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		defer runRows.Close()

		var runUIDs []string
		for runRows.Next() {
			var uid string
			if err := runRows.Scan(&uid); err != nil {
				writeError(w, err.Error(), 500)
				return
			}
			runUIDs = append(runUIDs, uid)
		}
		if err := runRows.Err(); err != nil {
			writeError(w, err.Error(), 500)
			return
		}

		result := []liveTask{}
		now := time.Now().Unix()

		for _, runUID := range runUIDs {
			// Get the latest sample row for this run, falling back to the
			// started row if no samples have arrived yet.
			const latestSQL = `
SELECT
  el.run_uid,
  el.entity_uid,
  el.started_at,
  el.turns,
  el.actions,
  el.cost_usd,
  el.cpu_pct,
  el.rss_mb,
  el.lines_added,
  el.lines_removed,
  el.input_tokens,
  el.output_tokens,
  el.cache_read_tokens,
  el.cache_create_tokens
FROM execution_log el
WHERE el.run_uid = ?
  AND el.event IN ('sample', 'started')
ORDER BY el.created_at DESC
LIMIT 1`

			var t liveTask
			row := db.QueryRowContext(r.Context(), latestSQL, runUID)
			if err := row.Scan(
				&t.RunUID,
				&t.EntityUID,
				&t.StartedAt,
				&t.Turns,
				&t.Actions,
				&t.CostUSD,
				&t.CPUPCT,
				&t.RSSMB,
				&t.LinesAdded,
				&t.LinesRemoved,
				&t.InputTokens,
				&t.OutputTokens,
				&t.CacheReadTokens,
				&t.CacheCreateTokens,
			); err != nil {
				// Row may have disappeared — skip.
				continue
			}

			// Compute elapsed seconds from started_at (unix ms).
			if t.StartedAt > 0 {
				startSec := t.StartedAt / 1000
				t.ElapsedSec = now - startSec
				if t.ElapsedSec < 0 {
					t.ElapsedSec = 0
				}
			}

			// Join entity title + type from entities table (current row only).
			const entitySQL = `
SELECT COALESCE(title, ''), COALESCE(type, 'task')
FROM entities
WHERE entity_uid = ? AND valid_to = ''
LIMIT 1`
			var title, etype string
			if err := db.QueryRowContext(r.Context(), entitySQL, t.EntityUID).Scan(&title, &etype); err != nil {
				title = t.EntityUID
				etype = "task"
			}
			t.EntityTitle = title
			t.EntityType = etype

			// Overlay the live in-memory turn count from the task runner. The
			// execution_log sample row only refreshes every 5s, but the runner
			// increments its turn counter immediately at each turn boundary
			// (before firing task_turn_started). Looking it up here makes the
			// /api/execution/live response reflect the current turn within ~1s.
			if runner := n.GetRunner(); runner != nil {
				if live, ok := runner.LiveTurnCount(t.EntityUID); ok && live > t.Turns {
					t.Turns = live
				}
			}

			result = append(result, t)
		}

		writeJSON(w, result)
	}
}
