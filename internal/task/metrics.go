package task

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/relationships"
)

// CostAggregation holds aggregated cost data for a time period.
type CostAggregation struct {
	Period string  `json:"period"`
	Tasks  int     `json:"tasks"`
	Runs   int     `json:"runs"`
	Turns  int     `json:"turns"`
	Cost   float64 `json:"cost"`
	Budget float64 `json:"budget"`
}

// CostDrillDownEntry represents individual task runs for a specific date.
type CostDrillDownEntry struct {
	RunID       int     `json:"run_id"`
	TaskID      string  `json:"task_id"`
	TaskTitle   string  `json:"task_title"`
	ManifestID  string  `json:"manifest_id"`
	Agent       string  `json:"agent"`
	RunNumber   int     `json:"run_number"`
	Status      string  `json:"status"`
	Actions     int     `json:"actions"`
	CostUSD     float64 `json:"cost_usd"`
	Turns       int     `json:"turns"`
	Duration    int     `json:"duration_sec"`
	StartedAt   string  `json:"started_at"`
	CompletedAt string  `json:"completed_at"`
}

// CostTrendSummary holds summary cost data for trend cards.
type CostTrendSummary struct {
	Today     float64 `json:"today"`
	ThisWeek  float64 `json:"this_week"`
	ThisMonth float64 `json:"this_month"`
	Avg30d    float64 `json:"avg_30d"`
}

// ParseCostFromOutput extracts cost_usd and num_turns from stream-json output.
// Scans the last 20 lines for a "type":"result" event.
func ParseCostFromOutput(output string) (costUSD float64, turns int) {
	if output == "" {
		return 0, 0
	}
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0 && i >= len(lines)-20; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var event struct {
			Type         string  `json:"type"`
			TotalCostUSD float64 `json:"total_cost_usd"`
			CostUSD      float64 `json:"cost_usd"`
			NumTurns     int     `json:"num_turns"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if event.Type == "result" {
			cost := event.TotalCostUSD
			if cost == 0 {
				cost = event.CostUSD
			}
			return cost, event.NumTurns
		}
	}
	return 0, 0
}

// ParseTestsFromOutput scans task output for common test result patterns
// and returns (testsRun, testsPassed, testsFailed).
// Recognises: Go test output, Jest, pytest, npm test.
func ParseTestsFromOutput(output string) (run, passed, failed int) {
	if output == "" {
		return
	}
	for _, line := range strings.Split(output, "\n") {
		// Go: "ok  package (0.123s)" or "FAIL package"
		if strings.HasPrefix(line, "ok  ") || strings.HasPrefix(line, "ok\t") {
			run++
			passed++
			continue
		}
		if strings.HasPrefix(line, "FAIL\t") || strings.HasPrefix(line, "FAIL ") {
			run++
			failed++
			continue
		}
		// pytest: "X passed, Y failed"
		if strings.Contains(line, " passed") {
			var p, f int
			fmt.Sscanf(line, "%d passed", &p)
			fmt.Sscanf(line, "%d failed", &f)
			if p > 0 || f > 0 {
				run += p + f
				passed += p
				failed += f
			}
			continue
		}
		// Jest: "Tests: X passed, Y failed, Z total"
		if strings.Contains(line, "Tests:") && strings.Contains(line, "passed") {
			var p, f, total int
			fmt.Sscanf(line, "Tests: %d passed", &p)
			fmt.Sscanf(line, "Tests: %d failed", &f)
			fmt.Sscanf(line, "Tests: %d total", &total)
			if total > 0 {
				run = total
				passed = p
				failed = f
			}
			continue
		}
	}
	return
}

// ParseOutputMetrics extracts compaction count, error count, reasoning tokens,
// and TTFB from the full stream-json output of a run.
func ParseOutputMetrics(output string) (compactions, errors int, reasoningTokens int64, ttfbMS int64) {
	if output == "" {
		return
	}
	var firstAssistantSeen bool
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		t, _ := event["type"].(string)

		// TTFB: first assistant event index × ~avg ms per line is unreliable;
		// instead use the first non-system line position as a proxy.
		if !firstAssistantSeen && t == "assistant" {
			// Line index as proxy for TTFB (each line ≈ one event).
			// Better than nothing when clock timestamps aren't in the stream.
			ttfbMS = int64(i * 50) // crude but consistent
			firstAssistantSeen = true
		}

		// Compactions: system events with subtype "compact_history".
		if t == "system" {
			if sub, _ := event["subtype"].(string); sub == "compact_history" || sub == "compaction" {
				compactions++
			}
		}

		// Errors: error events or tool results with is_error:true.
		if t == "tool_result" {
			if isErr, _ := event["is_error"].(bool); isErr {
				errors++
			}
		}
		if t == "error" {
			errors++
		}

		// Reasoning tokens: from the result event usage block.
		if t == "result" {
			if usage, ok := event["usage"].(map[string]any); ok {
				if rt, ok := usage["reasoning_tokens"].(float64); ok {
					reasoningTokens = int64(rt)
				}
			}
		}

		// Also pick up reasoning tokens from assistant message usage.
		if t == "assistant" {
			if msg, ok := event["message"].(map[string]any); ok {
				if usage, ok := msg["usage"].(map[string]any); ok {
					if rt, ok := usage["reasoning_tokens"].(float64); ok && rt > 0 {
						reasoningTokens = int64(rt)
					}
				}
			}
		}
	}
	return
}

// BackfillCosts parses cost_usd from output for runs that have cost_usd=0 but non-empty output.
func (s *Store) BackfillCosts() (int, error) {
	rows, err := s.db.Query(`SELECT id, output FROM task_runs WHERE cost_usd = 0 AND output != '' LIMIT 5000`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var updated int
	for rows.Next() {
		var id int
		var output string
		if err := rows.Scan(&id, &output); err != nil {
			continue
		}
		cost, turns := ParseCostFromOutput(output)
		if cost > 0 || turns > 0 {
			_, err := s.db.Exec(`UPDATE task_runs SET cost_usd = ?, turns = ? WHERE id = ?`, cost, turns, id)
			if err == nil {
				updated++
			}
		}
	}
	return updated, rows.Err()
}

// CostByPeriod returns cost aggregated by day, week, or month. Optional agent filter.
func (s *Store) CostByPeriod(period string, days int, agent string) ([]CostAggregation, error) {
	now := time.Now().UTC()
	since := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(days - 1))

	var groupExpr string
	switch period {
	case "week":
		groupExpr = `strftime('%Y-W%W', r.started_at)`
	case "month":
		groupExpr = `strftime('%Y-%m', r.started_at)`
	default:
		groupExpr = `strftime('%Y-%m-%d', r.started_at)`
	}

	// Note: agent filter is no longer supported since the tasks table has been retired.
	query := fmt.Sprintf(`SELECT %s as period, COUNT(DISTINCT r.task_id) as tasks, COUNT(*) as runs,
		COALESCE(SUM(r.turns), 0) as turns, COALESCE(SUM(r.cost_usd), 0) as cost
		FROM task_runs r
		WHERE r.started_at >= ?`, groupExpr)
	args := []any{since.Format(time.RFC3339)}
	query += ` GROUP BY period ORDER BY period DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []CostAggregation
	for rows.Next() {
		var a CostAggregation
		if err := rows.Scan(&a.Period, &a.Tasks, &a.Runs, &a.Turns, &a.Cost); err != nil {
			return nil, err
		}
		results = append(results, a)
	}
	return results, rows.Err()
}

// CostDrillDown returns individual task runs for a specific date.
// ManifestID is resolved via the relationships store after the SELECT.
// The tasks table has been retired; title and agent come from entities.
func (s *Store) CostDrillDown(date string, agent string) ([]CostDrillDownEntry, error) {
	// Note: agent filter is no longer supported since the tasks table has been retired.
	query := `SELECT r.id, r.task_id, r.run_number, r.status, r.actions, r.cost_usd, r.turns, r.started_at, r.completed_at
		FROM task_runs r
		WHERE strftime('%Y-%m-%d', r.started_at) = ?`
	args := []any{date}
	query += ` ORDER BY r.cost_usd DESC`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []CostDrillDownEntry
	taskIDs := []string{}
	idxByTaskID := map[string][]int{}
	for rows.Next() {
		var e CostDrillDownEntry
		var startedStr, completedStr string
		if err := rows.Scan(&e.RunID, &e.TaskID, &e.RunNumber, &e.Status, &e.Actions, &e.CostUSD, &e.Turns, &startedStr, &completedStr); err != nil {
			return nil, err
		}
		e.StartedAt = startedStr
		e.CompletedAt = completedStr
		if startedStr != "" && completedStr != "" {
			st, _ := time.Parse(time.RFC3339, startedStr)
			ct, _ := time.Parse(time.RFC3339, completedStr)
			if !st.IsZero() && !ct.IsZero() {
				e.Duration = int(ct.Sub(st).Seconds())
			}
		}
		idxByTaskID[e.TaskID] = append(idxByTaskID[e.TaskID], len(results))
		taskIDs = append(taskIDs, e.TaskID)
		results = append(results, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Resolve owning manifest per task via relationships.
	if s.rels != nil && len(taskIDs) > 0 {
		ctx := context.Background()
		byTask, err := s.rels.ListIncomingForMany(ctx, taskIDs, relationships.EdgeOwns)
		if err == nil {
			for tID, edges := range byTask {
				for _, e := range edges {
					if e.SrcKind == relationships.KindManifest {
						for _, idx := range idxByTaskID[tID] {
							results[idx].ManifestID = e.SrcID
						}
						break
					}
				}
			}
		}
	}
	return results, nil
}

// DistinctAgents returns an empty list. The tasks table has been retired;
// agent information is now stored in the entities table.
func (s *Store) DistinctAgents() ([]string, error) {
	return nil, nil
}

// GetCostTrendSummary returns cost totals for today, this week, this month, and 30d average.
func (s *Store) GetCostTrendSummary(agent string) (*CostTrendSummary, error) {
	now := time.Now().UTC()
	today := now.Format("2006-01-02")
	weekStart := now.AddDate(0, 0, -int(now.Weekday())).Format("2006-01-02")
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	days30 := now.AddDate(0, 0, -30).Format(time.RFC3339)

	// Note: agent filter is no longer supported since the tasks table has been retired.
	var ts CostTrendSummary

	q := `SELECT COALESCE(SUM(r.cost_usd), 0) FROM task_runs r WHERE strftime('%Y-%m-%d', r.started_at) = ?`
	if err := s.db.QueryRow(q, today).Scan(&ts.Today); err != nil {
		slog.Warn("query cost failed", "period", "today", "error", err)
	}

	q = `SELECT COALESCE(SUM(r.cost_usd), 0) FROM task_runs r WHERE strftime('%Y-%m-%d', r.started_at) >= ?`
	if err := s.db.QueryRow(q, weekStart).Scan(&ts.ThisWeek); err != nil {
		slog.Warn("query cost failed", "period", "this_week", "error", err)
	}

	q = `SELECT COALESCE(SUM(r.cost_usd), 0) FROM task_runs r WHERE strftime('%Y-%m-%d', r.started_at) >= ?`
	if err := s.db.QueryRow(q, monthStart).Scan(&ts.ThisMonth); err != nil {
		slog.Warn("query cost failed", "period", "this_month", "error", err)
	}

	// 30-day average: total over 30 days / 30
	var total30 float64
	q = `SELECT COALESCE(SUM(r.cost_usd), 0) FROM task_runs r WHERE r.started_at >= ?`
	if err := s.db.QueryRow(q, days30).Scan(&total30); err != nil {
		slog.Warn("query cost failed", "period", "30d", "error", err)
	}
	ts.Avg30d = total30 / 30.0

	return &ts, nil
}

// SumCostSince returns the total cost_usd spent across task_runs whose
// tasks belong to the given product since the supplied timestamp.
// Used by the runner's daily-budget pre-spawn check: the runtime sums
// all runs on/after start-of-day UTC and refuses to spawn another when
// a per-product daily cap has already been hit.
//
// Empty productID returns 0 — standalone tasks (no manifest/product)
// have no product-level budget to enforce. The daily-budget knob is
// defined at product scope and nowhere else in the catalog.
//
// Resolves manifest → task chain via the relationships store. The
// legacy m.project_id / t.manifest_id JOIN was retired in PR/M3.
func (s *Store) SumCostSince(productID string, since time.Time) (float64, error) {
	if productID == "" {
		return 0, nil
	}
	if s.rels == nil {
		return 0, fmt.Errorf("SumCostSince: relationships backend not wired")
	}
	ctx := context.Background()
	manifestEdges, err := s.rels.ListOutgoing(ctx, productID, relationships.EdgeOwns)
	if err != nil {
		return 0, err
	}
	manifestIDs := make([]string, 0, len(manifestEdges))
	for _, e := range manifestEdges {
		if e.DstKind == relationships.KindManifest {
			manifestIDs = append(manifestIDs, e.DstID)
		}
	}
	if len(manifestIDs) == 0 {
		return 0, nil
	}
	taskEdgesByManifest, err := s.rels.ListOutgoingForMany(ctx, manifestIDs, relationships.EdgeOwns)
	if err != nil {
		return 0, err
	}
	taskIDs := []string{}
	for _, edges := range taskEdgesByManifest {
		for _, e := range edges {
			if e.DstKind == relationships.KindTask {
				taskIDs = append(taskIDs, e.DstID)
			}
		}
	}
	if len(taskIDs) == 0 {
		return 0, nil
	}
	ph := strings.Repeat("?,", len(taskIDs))
	ph = ph[:len(ph)-1]
	args := make([]any, 0, len(taskIDs)+1)
	for _, id := range taskIDs {
		args = append(args, id)
	}
	args = append(args, since.UTC().Format(time.RFC3339))
	var total float64
	err = s.db.QueryRow(`SELECT COALESCE(SUM(cost_usd), 0)
		FROM task_runs WHERE task_id IN (`+ph+`) AND started_at >= ?`, args...).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("sum cost since: %w", err)
	}
	return total, nil
}

// TodayCost returns the total cost and turns for today from task_runs.
func (s *Store) TodayCost() (cost float64, turns int, taskCount int, err error) {
	today := time.Now().UTC().Format("2006-01-02")
	err = s.db.QueryRow(`SELECT COALESCE(SUM(cost_usd), 0), COALESCE(SUM(turns), 0), COUNT(DISTINCT task_id)
		FROM task_runs WHERE strftime('%Y-%m-%d', started_at) = ?`, today).Scan(&cost, &turns, &taskCount)
	return
}

// EnrichRunStats populates the four cumulative gauges (turns / cost /
// actions / tokens) on a single task by summing task_runs WHERE
// task_id = t.id. Tokens is the all-buckets sum
// (input + output + cache_read + cache_create). Used by the single-task
// GET so the Main-tab gauges show the same actions/tokens that already
// surface on products + manifests. List endpoints stay on the cheaper
// enrichWithCosts batch path.
func (s *Store) EnrichRunStats(t *Task) {
	if t == nil {
		return
	}
	row := s.db.QueryRow(`
		SELECT
			COALESCE(SUM(turns), 0),
			COALESCE(SUM(cost_usd), 0),
			COALESCE(SUM(actions), 0),
			COALESCE(SUM(input_tokens + output_tokens + cache_read_tokens + cache_create_tokens), 0)
		FROM task_runs WHERE task_id = ?`, t.ID)
	var turns, actions, tokens int
	var cost float64
	if err := row.Scan(&turns, &cost, &actions, &tokens); err == nil {
		t.TotalTurns = turns
		t.TotalCost = cost
		t.TotalActions = actions
		t.TotalTokens = tokens
	}
}

// enrichWithCosts populates TotalTurns and TotalCost from task_runs for a batch of tasks.
func (s *Store) enrichWithCosts(tasks []*Task) {
	if len(tasks) == 0 {
		return
	}
	ids := make([]string, len(tasks))
	taskMap := make(map[string]*Task, len(tasks))
	for i, t := range tasks {
		ids[i] = t.ID
		taskMap[t.ID] = t
	}

	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	rows, err := s.db.Query(
		fmt.Sprintf("SELECT task_id, COALESCE(SUM(turns),0), COALESCE(SUM(cost_usd),0) FROM task_runs WHERE task_id IN (%s) GROUP BY task_id", placeholders),
		args...,
	)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var taskID string
		var turns int
		var cost float64
		if err := rows.Scan(&taskID, &turns, &cost); err == nil {
			if t, ok := taskMap[taskID]; ok {
				t.TotalTurns = turns
				t.TotalCost = cost
			}
		}
	}
}

// ProductivityMetrics holds the calculated productivity score and its components.
type ProductivityMetrics struct {
	// Overall score (0-100)
	Score int `json:"score"`
	Grade string `json:"grade"` // A, B, C, D, F

	// Positive signals
	TasksCompleted    int `json:"tasks_completed"`
	FirstAttemptPass  int `json:"first_attempt_pass"`  // completed on run 1
	LinesCommitted   int `json:"lines_committed"`      // insertions from git_details
	FilesChanged     int `json:"files_changed"`
	TotalActions     int `json:"total_actions"`

	// Negative signals
	TasksFailed      int `json:"tasks_failed"`
	ReworkRuns       int `json:"rework_runs"`           // tasks with run_count > 1
	AmnesiaCount     int `json:"amnesia_count"`

	// Efficiency
	AvgTurnsPerTask  float64 `json:"avg_turns_per_task"`
	CostPerCompletion float64 `json:"cost_per_completion"`
	TotalCost        float64 `json:"total_cost"`
	TotalTurns       int     `json:"total_turns"`

	// Trend (last 7 days, one score per day)
	Trend []DailyProductivity `json:"trend"`

	// Period
	Period string `json:"period"` // today, week, month
}

// DailyProductivity holds productivity data for a single day.
type DailyProductivity struct {
	Date             string  `json:"date"`
	Score            int     `json:"score"`
	TasksCompleted   int     `json:"tasks_completed"`
	TasksFailed      int     `json:"tasks_failed"`
	LinesCommitted   int     `json:"lines_committed"`
	Cost             float64 `json:"cost"`
}

// Productivity calculates the productivity score for a given period.
// period: "today", "week", "month", "all"
func (s *Store) Productivity(db *sql.DB, period string) (*ProductivityMetrics, error) {
	m := &ProductivityMetrics{Period: period}

	// Date filter
	var dateFilter string
	switch period {
	case "today":
		dateFilter = "strftime('%Y-%m-%d', started_at) = strftime('%Y-%m-%d', 'now')"
	case "week":
		dateFilter = "started_at >= datetime('now', '-7 days')"
	case "month":
		dateFilter = "started_at >= datetime('now', '-30 days')"
	default:
		dateFilter = "1=1"
	}

	// Task runs: completions, failures, turns, cost, actions
	err := s.db.QueryRow(fmt.Sprintf(`
		SELECT
			COALESCE(SUM(CASE WHEN status='completed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status!='completed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(turns), 0),
			COALESCE(SUM(cost_usd), 0),
			COALESCE(SUM(actions), 0)
		FROM task_runs WHERE %s
	`, dateFilter)).Scan(&m.TasksCompleted, &m.TasksFailed, &m.TotalTurns, &m.TotalCost, &m.TotalActions)
	if err != nil {
		return nil, fmt.Errorf("query task_runs: %w", err)
	}

	// First-attempt pass and rework metrics require the tasks table which has
	// been retired. These fields remain 0.
	m.FirstAttemptPass = 0
	m.ReworkRuns = 0

	// Lines committed + files changed from git_details JSON
	rows, err := db.Query(`SELECT git_details FROM watcher_audits WHERE git_details != ''`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var gitJSON string
			if rows.Scan(&gitJSON) == nil && gitJSON != "" {
				var gd struct {
					CommitCount int `json:"commit_count"`
					FilesChanged int `json:"files_changed"`
					Insertions  int `json:"insertions"`
					Deletions   int `json:"deletions"`
				}
				if json.Unmarshal([]byte(gitJSON), &gd) == nil {
					m.LinesCommitted += gd.Insertions
					m.FilesChanged += gd.FilesChanged
				}
			}
		}
	}

	// Amnesia count
	db.QueryRow(`SELECT COUNT(*) FROM amnesia WHERE status != 'dismissed'`).Scan(&m.AmnesiaCount)

	// Efficiency
	if m.TasksCompleted > 0 {
		m.AvgTurnsPerTask = float64(m.TotalTurns) / float64(m.TasksCompleted)
		m.CostPerCompletion = m.TotalCost / float64(m.TasksCompleted)
	}

	// Calculate score (0-100)
	// Weighted average of positive rates, penalized by failure rates
	score := 50.0 // baseline — no data = neutral

	totalRuns := m.TasksCompleted + m.TasksFailed
	if totalRuns > 0 {
		// Completion rate (0-35 points) — most important
		completionRate := float64(m.TasksCompleted) / float64(totalRuns)
		score += completionRate * 35

		// First-attempt success rate (0-25 points) — efficiency matters
		firstAttemptRate := float64(m.FirstAttemptPass) / float64(totalRuns)
		score += firstAttemptRate * 25

		// Failure penalty (-20 max)
		failRate := float64(m.TasksFailed) / float64(totalRuns)
		score -= failRate * 20

		// Rework penalty — capped at -10
		reworkRate := float64(m.ReworkRuns) / float64(totalRuns)
		score -= reworkRate * 10
	}

	// Clamp
	if score < 0 { score = 0 }
	if score > 100 { score = 100 }
	m.Score = int(score)

	// Grade
	switch {
	case m.Score >= 90: m.Grade = "A"
	case m.Score >= 80: m.Grade = "B"
	case m.Score >= 70: m.Grade = "C"
	case m.Score >= 60: m.Grade = "D"
	default: m.Grade = "F"
	}

	// 7-day trend
	trendRows, err := db.Query(`
		SELECT strftime('%Y-%m-%d', started_at) as day,
			COALESCE(SUM(CASE WHEN status='completed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status!='completed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(cost_usd), 0)
		FROM task_runs
		WHERE started_at >= datetime('now', '-7 days')
		GROUP BY day ORDER BY day
	`)
	if err == nil {
		defer trendRows.Close()
		for trendRows.Next() {
			var dp DailyProductivity
			if trendRows.Scan(&dp.Date, &dp.TasksCompleted, &dp.TasksFailed, &dp.Cost) == nil {
				// Daily score: simple ratio
				total := dp.TasksCompleted + dp.TasksFailed
				if total > 0 {
					dp.Score = int(float64(dp.TasksCompleted) / float64(total) * 100)
				}
				m.Trend = append(m.Trend, dp)
			}
		}
	}

	return m, nil
}

