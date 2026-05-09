package web

import (
	"net/http"
	"strconv"

	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/relationships"
	"github.com/k8nstantin/OpenPraxis/internal/settings"
)

// defaultFrontierWindowDays is the lookback window applied when neither the
// query string nor the `frontier_window_days` settings knob supplies one.
const defaultFrontierWindowDays = 30

// frontierTaskView is the per-task aggregate returned by /execution/frontier.
// Mirrors execution.PassRateSummary with snake_case JSON tags so the wire
// shape matches the response shape contracted in the M2/T2 spec.
type frontierTaskView struct {
	EntityUID    string  `json:"entity_uid"`
	TotalRuns    int     `json:"total_runs"`
	SuccessRuns  int     `json:"success_runs"`
	FailedRuns   int     `json:"failed_runs"`
	MaxTurnsRuns int     `json:"max_turns_runs"`
	TimeoutRuns  int     `json:"timeout_runs"`
	AvgTurns     float64 `json:"avg_turns"`
	AvgCostUSD   float64 `json:"avg_cost_usd"`
	PassRate     float64 `json:"pass_rate"`
}

// frontierBest names the highest-pass-rate task among those with runs.
// Null in the response when the manifest has no terminal runs.
type frontierBest struct {
	TaskID   string  `json:"task_id"`
	PassRate float64 `json:"pass_rate"`
}

type frontierResponse struct {
	ManifestID  string                      `json:"manifest_id"`
	WindowDays  int                         `json:"window_days"`
	Tasks       map[string]frontierTaskView `json:"tasks"`
	Best        *frontierBest               `json:"best"`
	AvgPassRate float64                     `json:"avg_pass_rate"`
}

// resolveFrontierWindowDays reads `frontier_window_days` at system scope.
// Falls back to defaultFrontierWindowDays when the resolver is wired but
// the key is missing, the value is non-positive, or the type is unexpected.
// A nil resolver also yields the default — keeps tests building a minimal
// Node viable.
func resolveFrontierWindowDays(r *http.Request, n *node.Node) int {
	if n == nil || n.SettingsResolver == nil {
		return defaultFrontierWindowDays
	}
	res, err := n.SettingsResolver.Resolve(r.Context(), settings.Scope{}, "frontier_window_days")
	if err != nil {
		return defaultFrontierWindowDays
	}
	switch v := res.Value.(type) {
	case int:
		if v > 0 {
			return v
		}
	case int64:
		if v > 0 {
			return int(v)
		}
	case float64:
		if v > 0 {
			return int(v)
		}
	}
	return defaultFrontierWindowDays
}

// apiExecutionFrontier returns per-task pass-rate aggregates for every task
// owned by `manifest_id`, plus a manifest-wide average and the best task.
// Tasks with zero terminal runs are omitted from the `tasks` map; the
// avg_pass_rate denominator excludes them too. `best` is null when no task
// has any runs.
func apiExecutionFrontier(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		manifestID := r.URL.Query().Get("manifest_id")
		if manifestID == "" {
			writeError(w, "manifest_id is required", http.StatusBadRequest)
			return
		}

		windowDays := resolveFrontierWindowDays(r, n)
		if v := r.URL.Query().Get("window_days"); v != "" {
			if d, err := strconv.Atoi(v); err == nil && d > 0 {
				windowDays = d
			}
		}

		ctx := r.Context()

		// Walk owns-edges out of the manifest to gather task ids. Filtering
		// by DstKind keeps the set type-correct even if a future edge variant
		// puts non-task children under a manifest.
		var taskIDs []string
		if n != nil && n.Relationships != nil {
			edges, err := n.Relationships.ListOutgoing(ctx, manifestID, relationships.EdgeOwns)
			if err != nil {
				writeError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			for _, e := range edges {
				if e.DstKind == relationships.KindTask {
					taskIDs = append(taskIDs, e.DstID)
				}
			}
		}

		summaries, err := n.ExecutionLog.FrontierByManifest(ctx, taskIDs, windowDays)
		if err != nil {
			writeError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		tasks := make(map[string]frontierTaskView, len(summaries))
		var (
			best    *frontierBest
			passSum float64
			counted int
		)
		for taskID, sum := range summaries {
			if sum.TotalRuns == 0 {
				continue
			}
			tasks[taskID] = frontierTaskView{
				EntityUID:    sum.EntityUID,
				TotalRuns:    sum.TotalRuns,
				SuccessRuns:  sum.SuccessRuns,
				FailedRuns:   sum.FailedRuns,
				MaxTurnsRuns: sum.MaxTurnsRuns,
				TimeoutRuns:  sum.TimeoutRuns,
				AvgTurns:     sum.AvgTurns,
				AvgCostUSD:   sum.AvgCostUSD,
				PassRate:     sum.PassRate,
			}
			passSum += sum.PassRate
			counted++
			if best == nil || sum.PassRate > best.PassRate {
				best = &frontierBest{TaskID: taskID, PassRate: sum.PassRate}
			}
		}

		var avg float64
		if counted > 0 {
			avg = passSum / float64(counted)
		}

		writeJSON(w, frontierResponse{
			ManifestID:  manifestID,
			WindowDays:  windowDays,
			Tasks:       tasks,
			Best:        best,
			AvgPassRate: avg,
		})
	}
}
