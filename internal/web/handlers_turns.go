package web

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/k8nstantin/OpenPraxis/internal/node"
)

// turnTimelineRow is one entry in the turn-timeline series.
type turnTimelineRow struct {
	Turn       int    `json:"turn"`
	StartedAt  string `json:"started_at"`
	DurationMs int64  `json:"duration_ms"`
}

// turnToolCount is one tool/count pair within a turn bucket.
type turnToolCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// turnToolsRow groups tool counts by turn.
type turnToolsRow struct {
	Turn  int             `json:"turn"`
	Tools []turnToolCount `json:"tools"`
}

// turnActivityRow is one bucket of /api/stats/turn-activity.
type turnActivityRow struct {
	Hour  string `json:"hour"`
	Turns int64  `json:"turns"`
}

// costPerTurnRow is one row of /api/entities/{id}/cost-per-turn.
type costPerTurnRow struct {
	TurnNumber       int     `json:"turn_number"`
	TurnStartedAt    string  `json:"turn_started_at"`
	CostPerTurnAvg   float64 `json:"cost_per_turn_avg"`
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
}

// apiTurnTimeline returns per-turn duration timing for a run.
// GET /api/entities/{id}/turn-timeline?run_uid=<runUID>
//
// Each row's duration is the wall-clock gap between this turn's
// boundary and the next turn's boundary; the final row's duration
// is the gap to the run's terminal event (or 0 if still running).
func apiTurnTimeline(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entityID := mux.Vars(r)["id"]
		runUID := r.URL.Query().Get("run_uid")
		if entityID == "" || runUID == "" {
			writeError(w, "entity id and run_uid are required", http.StatusBadRequest)
			return
		}

		rows, err := n.DB().QueryContext(r.Context(), `
			SELECT turns, created_at
			  FROM execution_log
			 WHERE entity_uid = ? AND run_uid = ? AND event = 'turn'
			 ORDER BY turns ASC, created_at ASC`, entityID, runUID)
		if err != nil {
			writeError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type turnRaw struct {
			turn int
			ts   time.Time
		}
		var raws []turnRaw
		for rows.Next() {
			var turn int
			var createdAt string
			if err := rows.Scan(&turn, &createdAt); err != nil {
				continue
			}
			t, perr := time.Parse(time.RFC3339Nano, createdAt)
			if perr != nil {
				t, _ = time.Parse(time.RFC3339, createdAt)
			}
			raws = append(raws, turnRaw{turn: turn, ts: t})
		}

		// Find the run's terminal event timestamp to bound the last turn.
		var terminal time.Time
		var terminalStr string
		_ = n.DB().QueryRowContext(r.Context(), `
			SELECT created_at
			  FROM execution_log
			 WHERE run_uid = ? AND event IN ('completed','failed')
			 ORDER BY created_at DESC LIMIT 1`, runUID).Scan(&terminalStr)
		if terminalStr != "" {
			if t, err := time.Parse(time.RFC3339Nano, terminalStr); err == nil {
				terminal = t
			} else if t, err := time.Parse(time.RFC3339, terminalStr); err == nil {
				terminal = t
			}
		}

		out := make([]turnTimelineRow, 0, len(raws))
		for i, r0 := range raws {
			var next time.Time
			if i+1 < len(raws) {
				next = raws[i+1].ts
			} else {
				next = terminal
			}
			var dur int64
			if !next.IsZero() && !r0.ts.IsZero() && next.After(r0.ts) {
				dur = next.Sub(r0.ts).Milliseconds()
			}
			out = append(out, turnTimelineRow{
				Turn:       r0.turn,
				StartedAt:  r0.ts.UTC().Format(time.RFC3339),
				DurationMs: dur,
			})
		}
		writeJSON(w, out)
	}
}

// apiTurnTools returns tool-call counts per turn for a run.
// GET /api/entities/{id}/turn-tools?run_uid=<runUID>
//
// run_uid is currently unused at the SQL level — actions are keyed by
// task_id (entity uuid) and turn_number, not by run_uid. We accept it
// for API symmetry; future work may scope per-run once actions carry run_uid.
func apiTurnTools(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entityID := mux.Vars(r)["id"]
		if entityID == "" {
			writeError(w, "entity id is required", http.StatusBadRequest)
			return
		}

		rows, err := n.DB().QueryContext(r.Context(), `
			SELECT turn_number, tool_name, COUNT(*) AS cnt
			  FROM actions
			 WHERE task_id = ?
			   AND turn_number > 0
			 GROUP BY turn_number, tool_name
			 ORDER BY turn_number ASC, cnt DESC`, entityID)
		if err != nil {
			writeError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		grouped := make(map[int][]turnToolCount)
		var order []int
		for rows.Next() {
			var turn int
			var tool string
			var cnt int64
			if err := rows.Scan(&turn, &tool, &cnt); err != nil {
				continue
			}
			if _, ok := grouped[turn]; !ok {
				order = append(order, turn)
			}
			grouped[turn] = append(grouped[turn], turnToolCount{Name: tool, Count: cnt})
		}

		out := make([]turnToolsRow, 0, len(order))
		for _, t := range order {
			out = append(out, turnToolsRow{Turn: t, Tools: grouped[t]})
		}
		writeJSON(w, out)
	}
}

// apiTurnActivity returns hourly turn counts across all runs for the
// trailing `since` hours (default 24, max 720).
// GET /api/stats/turn-activity?since=<hours>
func apiTurnActivity(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hours := 24
		if s := r.URL.Query().Get("since"); s != "" {
			if v, err := strconv.Atoi(s); err == nil && v > 0 && v <= 720 {
				hours = v
			}
		}
		cutoff := time.Now().UTC().Add(-time.Duration(hours) * time.Hour).Format(time.RFC3339)

		rows, err := n.DB().QueryContext(r.Context(), `
			SELECT strftime('%Y-%m-%dT%H:00:00Z', created_at) AS hour,
			       COUNT(*) AS turns
			  FROM execution_log
			 WHERE event = 'turn' AND created_at >= ?
			 GROUP BY hour
			 ORDER BY hour ASC`, cutoff)
		if err != nil {
			writeError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		raw := make(map[string]int64)
		for rows.Next() {
			var hour string
			var turns int64
			if err := rows.Scan(&hour, &turns); err != nil {
				continue
			}
			raw[hour] = turns
		}

		// Pad every hour bucket so the chart x-axis is continuous.
		out := make([]turnActivityRow, 0, hours)
		now := time.Now().UTC().Truncate(time.Hour)
		for i := hours - 1; i >= 0; i-- {
			h := now.Add(-time.Duration(i) * time.Hour).Format("2006-01-02T15:00:00Z")
			out = append(out, turnActivityRow{Hour: h, Turns: raw[h]})
		}
		writeJSON(w, out)
	}
}

// apiCostPerTurn returns turn boundaries with the run's amortised
// per-turn cost (cost_usd / turns from the run's `completed` row).
// GET /api/entities/{id}/cost-per-turn?run_uid=<runUID>
func apiCostPerTurn(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entityID := mux.Vars(r)["id"]
		runUID := r.URL.Query().Get("run_uid")
		if entityID == "" || runUID == "" {
			writeError(w, "entity id and run_uid are required", http.StatusBadRequest)
			return
		}

		rows, err := n.DB().QueryContext(r.Context(), `
			SELECT
			  el_turn.turns AS turn_number,
			  el_turn.created_at AS turn_started_at,
			  CASE WHEN el_comp.turns > 0 THEN el_comp.cost_usd / el_comp.turns ELSE 0 END AS cost_per_turn_avg,
			  COALESCE(el_comp.input_tokens, 0)  AS input_tokens,
			  COALESCE(el_comp.output_tokens, 0) AS output_tokens
			FROM execution_log el_turn
			LEFT JOIN execution_log el_comp
			  ON el_comp.run_uid = el_turn.run_uid
			 AND el_comp.event = 'completed'
			WHERE el_turn.entity_uid = ?
			  AND el_turn.run_uid = ?
			  AND el_turn.event = 'turn'
			ORDER BY el_turn.turns ASC`, entityID, runUID)
		if err != nil {
			writeError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		out := make([]costPerTurnRow, 0)
		for rows.Next() {
			var row costPerTurnRow
			if err := rows.Scan(&row.TurnNumber, &row.TurnStartedAt,
				&row.CostPerTurnAvg, &row.InputTokens, &row.OutputTokens); err != nil {
				continue
			}
			out = append(out, row)
		}
		writeJSON(w, out)
	}
}
