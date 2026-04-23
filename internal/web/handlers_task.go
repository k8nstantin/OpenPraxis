package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/task"

	"github.com/gorilla/mux"
)

func apiTasksByPeer(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tasks, err := n.Tasks.List("", 200)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		type tItem struct {
			ID         string  `json:"id"`
			Marker     string  `json:"marker"`
			Title      string  `json:"title"`
			Schedule   string  `json:"schedule"`
			Status     string  `json:"status"`
			Agent      string  `json:"agent"`
			DependsOn  string  `json:"depends_on"`
			RunCount   int     `json:"run_count"`
			TotalTurns int     `json:"total_turns"`
			TotalCost  float64 `json:"total_cost"`
			NextRunAt  string  `json:"next_run_at"`
			LastRunAt  string  `json:"last_run_at"`
			UpdatedAt  string  `json:"updated_at"`
			CreatedAt  string  `json:"created_at"`
		}
		type manifestGroup struct {
			ManifestID     string  `json:"manifest_id"`
			ManifestMarker string  `json:"manifest_marker"`
			ManifestTitle  string  `json:"manifest_title"`
			Count          int     `json:"count"`
			Tasks          []tItem `json:"tasks"`
		}
		type peerGroup struct {
			PeerID    string          `json:"peer_id"`
			Count     int             `json:"count"`
			Manifests []manifestGroup `json:"manifests"`
		}

		// Build: peer -> manifest -> tasks
		type mData struct {
			title  string
			marker string
			tasks  []tItem
		}
		type pData struct {
			manifestOrder []string
			manifests     map[string]*mData
		}
		peers := make(map[string]*pData)
		peerOrder := []string{}

		// Cache manifest titles
		manifestCache := make(map[string]*mData)
		getManifest := func(mid string) *mData {
			if md, ok := manifestCache[mid]; ok {
				return md
			}
			if mid == "" {
				md := &mData{title: "Standalone", marker: ""}
				manifestCache[mid] = md
				return md
			}
			md := &mData{title: "Unknown", marker: mid}
			if len(mid) >= 12 {
				md.marker = mid[:12]
			}
			if m, _ := n.Manifests.Get(mid); m != nil {
				md.title = m.Title
				md.marker = m.Marker
			}
			manifestCache[mid] = md
			return md
		}

		for _, t := range tasks {
			pid := t.SourceNode
			if pid == "" {
				pid = n.PeerID()
			}
			pd, ok := peers[pid]
			if !ok {
				pd = &pData{manifests: make(map[string]*mData)}
				peers[pid] = pd
				peerOrder = append(peerOrder, pid)
			}
			if _, ok := pd.manifests[t.ManifestID]; !ok {
				md := getManifest(t.ManifestID)
				pd.manifests[t.ManifestID] = &mData{title: md.title, marker: md.marker}
				pd.manifestOrder = append(pd.manifestOrder, t.ManifestID)
			}
			pd.manifests[t.ManifestID].tasks = append(pd.manifests[t.ManifestID].tasks, tItem{
				ID: t.ID, Marker: t.Marker, Title: t.Title, Schedule: t.Schedule, DependsOn: t.DependsOn,
				Status: t.Status, Agent: t.Agent, RunCount: t.RunCount, TotalTurns: t.TotalTurns, TotalCost: t.TotalCost,
				NextRunAt: t.NextRunAt, LastRunAt: t.LastRunAt,
				UpdatedAt: t.UpdatedAt.Format(time.RFC3339), CreatedAt: t.CreatedAt.Format(time.RFC3339),
			})
		}

		var result []peerGroup
		for _, pid := range peerOrder {
			pd := peers[pid]
			var mgs []manifestGroup
			totalCount := 0
			for _, mid := range pd.manifestOrder {
				md := pd.manifests[mid]
				mgs = append(mgs, manifestGroup{
					ManifestID: mid, ManifestMarker: md.marker, ManifestTitle: md.title,
					Count: len(md.tasks), Tasks: md.tasks,
				})
				totalCount += len(md.tasks)
			}
			result = append(result, peerGroup{PeerID: pid, Count: totalCount, Manifests: mgs})
		}
		writeJSON(w, result)
	}
}

func apiTaskList(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("status")
		tasks, err := n.Tasks.List(status, 50)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, tasks)
	}
}

func apiTaskCreate(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ManifestID  string `json:"manifest_id"`
			Title       string `json:"title"`
			Description string `json:"description"`
			Schedule    string `json:"schedule"`
			Agent       string `json:"agent"`
			// MaxTurns is accepted for backwards compatibility with legacy
			// callers, silently ignored with a warn log (M4-T14). Per-task
			// max_turns is now set via PUT /api/tasks/:id/settings.
			MaxTurns  *int   `json:"max_turns,omitempty"`
			DependsOn string `json:"depends_on"`
		}
		if !decodeBody(w, r, &req) {
			return
		}
		if req.MaxTurns != nil {
			slog.Warn("ignored deprecated max_turns field on POST /api/tasks; set per-task value via PUT /api/tasks/:id/settings",
				"endpoint", "POST /api/tasks",
				"value", *req.MaxTurns,
				"successor", "PUT /api/tasks/:id/settings",
				"retired_in", "M4-T14")
		}
		if req.Title == "" {
			if req.ManifestID != "" {
				req.Title = "Task for manifest " + req.ManifestID[:min(8, len(req.ManifestID))]
			} else {
				writeError(w, "title is required for standalone tasks", 400)
				return
			}
		}
		t, err := n.Tasks.Create(req.ManifestID, req.Title, req.Description, req.Schedule, req.Agent, n.PeerID(), "dashboard", req.DependsOn)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, t)
	}
}

func apiTaskGet(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		t, err := n.Tasks.Get(id)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		if t == nil {
			writeError(w, "not found", 404)
			return
		}
		writeJSON(w, EnrichWithHTML(t, map[string]string{"description": t.Description}))
	}
}

func apiTaskUpdate(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		var req struct {
			Title       *string `json:"title"`
			Description *string `json:"description"`
			// MaxTurns is accepted for backwards compatibility with legacy
			// callers, silently ignored with a warn log (M4-T14 retirement).
			// Per-task max_turns lives in the settings table now; callers
			// should use PUT /api/tasks/:id/settings.
			MaxTurns *int `json:"max_turns,omitempty"`
		}
		if !decodeBody(w, r, &req) {
			return
		}
		if req.MaxTurns != nil {
			slog.Warn("ignored deprecated max_turns field on PATCH /api/tasks/:id; use PUT /api/tasks/:id/settings instead",
				"endpoint", "PATCH /api/tasks/:id",
				"task_id", id,
				"value", *req.MaxTurns,
				"successor", "PUT /api/tasks/:id/settings",
				"retired_in", "M4-T14")
		}
		// Record append-only description_revision on instructions changes
		// before the UPDATE, so edit history is preserved (DV/M2).
		if req.Description != nil {
			if _, err := n.RecordDescriptionChange(r.Context(), comments.TargetTask, id, *req.Description, ""); err != nil {
				writeError(w, err.Error(), 500)
				return
			}
		}
		t, err := n.Tasks.Update(id, req.Title, req.Description)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		if t == nil {
			writeError(w, "not found", 404)
			return
		}
		writeJSON(w, t)
	}
}

func apiTaskDelete(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		if err := n.Tasks.Delete(id); err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": "deleted"})
	}
}

// apiTaskStart handles POST /tasks/{id}/start with optional schedule override.
func apiTaskStart(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		t, err := n.Tasks.Get(id)
		if err != nil || t == nil {
			writeError(w, "task not found", 404)
			return
		}

		// Check for schedule override in request body
		schedule := t.Schedule
		var body struct {
			Schedule string `json:"schedule"`
		}
		if r.Body != nil {
			json.NewDecoder(r.Body).Decode(&body) // best-effort: schedule override is optional
			if body.Schedule != "" {
				schedule = body.Schedule
			}
		}

		if err := n.Tasks.ScheduleTask(id, schedule); err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": "scheduled", "schedule": schedule})
	}
}

func apiTaskUpdateStatus(n *node.Node, newStatus string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		if err := n.Tasks.UpdateStatus(id, newStatus); err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": newStatus})
	}
}

// apiTaskRuns returns run history for a task.
func apiTaskRuns(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskID := mux.Vars(r)["id"]
		runs, err := n.Tasks.ListRuns(taskID, 50)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		if runs == nil {
			runs = []task.TaskRun{}
		}
		writeJSON(w, runs)
	}
}

// apiTaskRunGet returns a single run by ID.
func apiTaskRunGet(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runIDStr := mux.Vars(r)["runId"]
		var runID int
		if _, err := fmt.Sscanf(runIDStr, "%d", &runID); err != nil {
			writeError(w, "invalid run ID", 400)
			return
		}
		run, err := n.Tasks.GetRun(runID)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		if run == nil {
			writeError(w, "run not found", 404)
			return
		}
		writeJSON(w, run)
	}
}

func apiTaskLinkManifest(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskID := mux.Vars(r)["id"]
		var req struct {
			ManifestID string `json:"manifest_id"`
		}
		if !decodeBody(w, r, &req) {
			return
		}
		if req.ManifestID == "" {
			writeError(w, "manifest_id is required", 400)
			return
		}
		t, err := n.Tasks.Get(taskID)
		if err != nil || t == nil {
			writeError(w, "task not found", 404)
			return
		}
		if err := n.Tasks.LinkManifest(t.ID, req.ManifestID); err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": "linked"})
	}
}

func apiTaskUnlinkManifest(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskID := mux.Vars(r)["id"]
		var req struct {
			ManifestID string `json:"manifest_id"`
		}
		if !decodeBody(w, r, &req) {
			return
		}
		if req.ManifestID == "" {
			writeError(w, "manifest_id is required", 400)
			return
		}
		t, err := n.Tasks.Get(taskID)
		if err != nil || t == nil {
			writeError(w, "task not found", 404)
			return
		}
		if err := n.Tasks.UnlinkManifest(t.ID, req.ManifestID); err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": "unlinked"})
	}
}

func apiTaskSetDependency(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		var req struct {
			DependsOn string `json:"depends_on"`
		}
		if !decodeBody(w, r, &req) {
			return
		}

		t, err := n.Tasks.Get(id)
		if err != nil || t == nil {
			writeError(w, "task not found", 404)
			return
		}

		// Resolve the dep target to its full ID if the caller passed a
		// marker. 404 here is a real error — the UI picker shouldn't
		// offer something that doesn't exist, but guard defensively.
		depID := req.DependsOn
		if depID != "" {
			dep, err := n.Tasks.Get(depID)
			if err != nil || dep == nil {
				writeError(w, "dependency task not found", http.StatusNotFound)
				return
			}
			depID = dep.ID
		}

		// Store handles cycle detection, self-loop rejection,
		// parent-status-aware seeding, and block_reason population.
		// The handler just maps the typed errors to HTTP codes and
		// returns the refreshed row.
		if err := n.Tasks.SetDependency(t.ID, depID); err != nil {
			switch {
			case errors.Is(err, task.ErrTaskDepCycle):
				writeError(w, err.Error(), http.StatusConflict)
			case errors.Is(err, task.ErrTaskDepSelfLoop):
				writeError(w, err.Error(), http.StatusBadRequest)
			default:
				writeError(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		updated, _ := n.Tasks.Get(t.ID)
		writeJSON(w, updated)
	}
}

func apiTaskManifests(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskID := mux.Vars(r)["id"]
		t, err := n.Tasks.Get(taskID)
		if err != nil || t == nil {
			writeError(w, "task not found", 404)
			return
		}
		ids, err := n.Tasks.ListLinkedManifests(t.ID)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, ids)
	}
}

func apiRunningTasks(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if n.GetRunner() == nil {
			writeJSON(w, []any{})
			return
		}
		writeJSON(w, n.GetRunner().ListRunning())
	}
}

func apiTaskStats(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tasks, err := n.Tasks.List("", 500)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}

		var runningCount int
		if n.GetRunner() != nil {
			runningCount = len(n.GetRunner().ListRunning())
		}

		// Get today's cost from task_runs DB (survives restart)
		costToday, turnsToday, _, _ := n.Tasks.TodayCost()
		costToday = math.Round(costToday*100) / 100

		// Build top tasks from today's drill-down
		type topTask struct {
			Marker string  `json:"marker"`
			Title  string  `json:"title"`
			Turns  int     `json:"turns"`
			Cost   float64 `json:"cost"`
			Status string  `json:"status"`
		}
		var topTasks []topTask
		today := time.Now().UTC().Format("2006-01-02")
		drillDown, _ := n.Tasks.CostDrillDown(today, "")
		// Aggregate by task_id for top tasks
		type taskAgg struct {
			marker, title, status string
			turns                 int
			cost                  float64
		}
		taskAggs := make(map[string]*taskAgg)
		for _, d := range drillDown {
			a, ok := taskAggs[d.TaskID]
			if !ok {
				a = &taskAgg{marker: d.TaskMarker, title: d.TaskTitle, status: d.Status}
				taskAggs[d.TaskID] = a
			}
			a.cost += d.CostUSD
			a.turns += d.Turns
		}
		for _, a := range taskAggs {
			// Skip zero-cost/zero-turn noise (killed/failed tasks that did nothing)
			if a.cost == 0 && a.turns == 0 {
				continue
			}
			topTasks = append(topTasks, topTask{
				Marker: a.marker, Title: a.title, Turns: a.turns,
				Cost: math.Round(a.cost*100) / 100, Status: a.status,
			})
		}
		// Sort by cost descending — no limit, show ALL tasks today
		for i := 0; i < len(topTasks); i++ {
			for j := i + 1; j < len(topTasks); j++ {
				if topTasks[j].Cost > topTasks[i].Cost {
					topTasks[i], topTasks[j] = topTasks[j], topTasks[i]
				}
			}
		}

		// Collect scheduled/waiting/pending tasks
		type pendingTask struct {
			Marker    string `json:"marker"`
			Title     string `json:"title"`
			Status    string `json:"status"`
			Schedule  string `json:"schedule"`
			NextRunAt string `json:"next_run_at"`
			DependsOn string `json:"depends_on"`
		}
		var pendingTasks []pendingTask
		for _, t := range tasks {
			if t.Status == "scheduled" || t.Status == "waiting" || t.Status == "pending" {
				dep := ""
				if len(t.DependsOn) >= 12 {
					dep = t.DependsOn[:12]
				}
				pendingTasks = append(pendingTasks, pendingTask{
					Marker: t.Marker, Title: t.Title, Status: t.Status,
					Schedule: t.Schedule, NextRunAt: t.NextRunAt, DependsOn: dep,
				})
			}
		}

		dailyBudget := parseDailyBudget(n)
		budgetPct := 0
		budgetExceeded := false
		if dailyBudget > 0 {
			budgetPct = int(math.Round(costToday / dailyBudget * 100))
			budgetExceeded = costToday >= dailyBudget
		}

		writeJSON(w, map[string]any{
			"running":         runningCount,
			"tasks_total":     len(tasks),
			"turns_today":     turnsToday,
			"cost_today":      costToday,
			"daily_budget":    dailyBudget,
			"budget_pct":      budgetPct,
			"budget_exceeded": budgetExceeded,
			"top_tasks":       topTasks,
			"pending_tasks":   pendingTasks,
		})
	}
}

// parseTaskResultMetrics extracts num_turns, total_cost_usd, and terminal_reason
// from the JSON-lines output of a task.
func parseTaskResultMetrics(output string) (turns int, cost float64, reason string) {
	if output == "" {
		return 0, 0, ""
	}
	// Scan lines from the end — the result event is usually the last JSON object
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var event struct {
			Type           string  `json:"type"`
			NumTurns       int     `json:"num_turns"`
			TotalCostUSD   float64 `json:"total_cost_usd"`
			TerminalReason string  `json:"terminal_reason"`
			StopReason     string  `json:"stop_reason"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if event.Type == "result" {
			reason = event.TerminalReason
			if reason == "" {
				reason = event.StopReason
			}
			return event.NumTurns, event.TotalCostUSD, reason
		}
	}
	return 0, 0, ""
}

// apiProductivity returns productivity score and breakdown.
// GET /api/tasks/productivity?period=today|week|month|all
func apiProductivity(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		period := r.URL.Query().Get("period")
		if period == "" {
			period = "all"
		}
		metrics, err := n.Tasks.Productivity(n.Tasks.DB(), period)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, metrics)
	}
}

// apiCostHistory returns cost aggregations by day/week/month, or drill-down for a specific date.
// GET /api/tasks/cost-history?days=30&period=day|week|month&agent=claude-code
// GET /api/tasks/cost-history?date=2026-04-13&agent=claude-code  (drill-down)
func apiCostHistory(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dailyBudget := parseDailyBudget(n)
		agent := r.URL.Query().Get("agent")

		// Drill-down: individual tasks for a specific date
		dateParam := r.URL.Query().Get("date")
		if dateParam != "" {
			entries, err := n.Tasks.CostDrillDown(dateParam, agent)
			if err != nil {
				writeError(w, err.Error(), 500)
				return
			}
			writeJSON(w, map[string]any{
				"date":    dateParam,
				"entries": entries,
				"budget":  dailyBudget,
			})
			return
		}

		// Period aggregation
		period := r.URL.Query().Get("period")
		if period == "" {
			period = "day"
		}

		daysStr := r.URL.Query().Get("days")
		days := 30
		if daysStr != "" {
			if d, err := strconv.Atoi(daysStr); err == nil && d > 0 && d <= 365 {
				days = d
			}
		}
		if period == "week" && days < 56 {
			days = 56
		}
		if period == "month" && days < 180 {
			days = 180
		}

		aggs, err := n.Tasks.CostByPeriod(period, days, agent)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}

		// For day period, fill in gaps (dates with no runs)
		if period == "day" {
			now := time.Now().UTC()
			since := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(days - 1))
			existing := make(map[string]bool)
			for _, a := range aggs {
				existing[a.Period] = true
				a.Budget = dailyBudget
			}
			// Add missing days with zero cost
			var filled []task.CostAggregation
			for d := now; !d.Before(since); d = d.AddDate(0, 0, -1) {
				key := d.Format("2006-01-02")
				found := false
				for i := range aggs {
					if aggs[i].Period == key {
						aggs[i].Budget = dailyBudget
						filled = append(filled, aggs[i])
						found = true
						break
					}
				}
				if !found {
					filled = append(filled, task.CostAggregation{Period: key, Budget: dailyBudget})
				}
			}
			aggs = filled
		} else {
			for i := range aggs {
				aggs[i].Budget = dailyBudget
			}
		}

		writeJSON(w, aggs)
	}
}

// apiCostAgents returns distinct agent names that have task runs.
func apiCostAgents(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agents, err := n.Tasks.DistinctAgents()
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		if agents == nil {
			agents = []string{}
		}
		writeJSON(w, agents)
	}
}

// apiCostTrend returns summary trend data (today, this week, this month, 30d avg).
func apiCostTrend(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agent := r.URL.Query().Get("agent")
		summary, err := n.Tasks.GetCostTrendSummary(agent)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, summary)
	}
}

func apiTaskKill(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		if n.GetRunner() == nil {
			writeError(w, "runner not initialized", 500)
			return
		}
		if err := n.GetRunner().Kill(id); err != nil {
			writeError(w, err.Error(), 400)
			return
		}
		writeJSON(w, map[string]string{"status": "killed"})
	}
}

func apiTaskPause(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		if n.GetRunner() == nil {
			writeError(w, "runner not initialized", 500)
			return
		}
		if err := n.GetRunner().Pause(id); err != nil {
			writeError(w, err.Error(), 400)
			return
		}
		writeJSON(w, map[string]string{"status": "paused"})
	}
}

func apiTaskResume(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		if n.GetRunner() == nil {
			writeError(w, "runner not initialized", 500)
			return
		}
		if err := n.GetRunner().Resume(id); err != nil {
			writeError(w, err.Error(), 400)
			return
		}
		writeJSON(w, map[string]string{"status": "resumed"})
	}
}

func apiTaskReschedule(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		var req struct {
			Schedule  string `json:"schedule"`
			NextRunAt string `json:"next_run_at"`
		}
		if !decodeBody(w, r, &req) {
			return
		}
		if req.Schedule == "" && req.NextRunAt == "" {
			writeError(w, "schedule or next_run_at required", 400)
			return
		}

		t, err := n.Tasks.Get(id)
		if err != nil || t == nil {
			writeError(w, "task not found", 404)
			return
		}

		schedule := req.Schedule
		if schedule == "" {
			schedule = t.Schedule
		}
		nextRunAt := req.NextRunAt
		if nextRunAt == "" {
			nextRun := task.ComputeNextRun(schedule)
			if !nextRun.IsZero() {
				nextRunAt = nextRun.Format(time.RFC3339)
			}
		}

		if err := n.Tasks.UpdateSchedule(t.ID, schedule, nextRunAt); err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": "rescheduled", "schedule": schedule, "next_run_at": nextRunAt})
	}
}

func apiTaskOutput(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		if n.GetRunner() == nil {
			writeJSON(w, map[string]any{"lines": []string{}, "running": false})
			return
		}
		lines, running := n.GetRunner().GetOutput(id)
		writeJSON(w, map[string]any{"lines": lines, "running": running})
	}
}

func apiTaskActions(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskID := mux.Vars(r)["id"]
		actions, err := n.Actions.ListByTask(taskID, 100)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, actions)
	}
}

func apiTaskAmnesia(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskID := mux.Vars(r)["id"]
		events, err := n.Actions.ListAmnesiaByTask(taskID, 50)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, events)
	}
}

func apiTaskDelusions(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskID := mux.Vars(r)["id"]
		events, err := n.Manifests.ListDelusionsByTask(taskID, 50)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, events)
	}
}

// apiTaskReject — POST /api/tasks/{id}/reject with body {reason, reviewer?}.
//
// Flips status completed → scheduled and appends a review_rejection comment.
// 404 when the task doesn't exist, 409 when the task isn't currently
// completed, 400 on empty reason. Body on success echoes the updated task.
func apiTaskReject(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		t, err := n.Tasks.Get(id)
		if err != nil || t == nil {
			writeError(w, "task not found", http.StatusNotFound)
			return
		}
		var body struct {
			Reason   string `json:"reason"`
			Reviewer string `json:"reviewer"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		if body.Reason == "" {
			writeError(w, "reason is required", http.StatusBadRequest)
			return
		}
		reviewer := body.Reviewer
		if reviewer == "" {
			reviewer = "http-api"
		}
		if err := n.Tasks.RejectCompletedTask(r.Context(), t.ID, body.Reason, reviewer); err != nil {
			switch {
			case errors.Is(err, task.ErrTaskNotCompleted):
				writeError(w, err.Error(), http.StatusConflict)
			case errors.Is(err, task.ErrEmptyReviewReason):
				writeError(w, err.Error(), http.StatusBadRequest)
			default:
				writeError(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		updated, _ := n.Tasks.Get(t.ID)
		writeJSON(w, updated)
	}
}

// apiTaskApprove — POST /api/tasks/{id}/approve with optional body {reviewer}.
//
// Appends a review_approval comment. Status does NOT change; approval is a
// signal consumed by manifest-closure warnings. 404 / 409 / 500 error map.
func apiTaskApprove(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		t, err := n.Tasks.Get(id)
		if err != nil || t == nil {
			writeError(w, "task not found", http.StatusNotFound)
			return
		}
		var body struct {
			Reviewer string `json:"reviewer"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		reviewer := body.Reviewer
		if reviewer == "" {
			reviewer = "http-api"
		}
		if err := n.Tasks.ApproveCompletedTask(r.Context(), t.ID, reviewer); err != nil {
			switch {
			case errors.Is(err, task.ErrTaskNotCompleted):
				writeError(w, err.Error(), http.StatusConflict)
			default:
				writeError(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		writeJSON(w, map[string]string{"status": "approved", "task_id": t.ID, "reviewer": reviewer})
	}
}

// apiTaskReviewStatus — GET /api/tasks/{id}/review.
//
// Returns the derived TaskReviewStatus (NeedsRework / HasApproval +
// latest rejection/approval metadata). Drives the review badge on the
// task detail page.
func apiTaskReviewStatus(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		t, err := n.Tasks.Get(id)
		if err != nil || t == nil {
			writeError(w, "task not found", http.StatusNotFound)
			return
		}
		st, err := n.Tasks.TaskReviewStatus(r.Context(), t.ID)
		if err != nil {
			writeError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, st)
	}
}
