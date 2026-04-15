package task

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
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
	TaskMarker  string  `json:"task_marker"`
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

	query := fmt.Sprintf(`SELECT %s as period, COUNT(DISTINCT r.task_id) as tasks, COUNT(*) as runs,
		COALESCE(SUM(r.turns), 0) as turns, COALESCE(SUM(r.cost_usd), 0) as cost
		FROM task_runs r LEFT JOIN tasks t ON r.task_id = t.id
		WHERE r.started_at >= ?`, groupExpr)
	args := []any{since.Format(time.RFC3339)}
	if agent != "" {
		query += ` AND t.agent = ?`
		args = append(args, agent)
	}
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
func (s *Store) CostDrillDown(date string, agent string) ([]CostDrillDownEntry, error) {
	query := `SELECT r.id, r.task_id, t.title, t.manifest_id, t.agent, r.run_number, r.status, r.actions, r.cost_usd, r.turns, r.started_at, r.completed_at
		FROM task_runs r LEFT JOIN tasks t ON r.task_id = t.id
		WHERE strftime('%Y-%m-%d', r.started_at) = ?`
	args := []any{date}
	if agent != "" {
		query += ` AND t.agent = ?`
		args = append(args, agent)
	}
	query += ` ORDER BY r.cost_usd DESC`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []CostDrillDownEntry
	for rows.Next() {
		var e CostDrillDownEntry
		var title, manifestID, agentStr sql.NullString
		var startedStr, completedStr string
		if err := rows.Scan(&e.RunID, &e.TaskID, &title, &manifestID, &agentStr, &e.RunNumber, &e.Status, &e.Actions, &e.CostUSD, &e.Turns, &startedStr, &completedStr); err != nil {
			return nil, err
		}
		if title.Valid {
			e.TaskTitle = title.String
		}
		if manifestID.Valid {
			e.ManifestID = manifestID.String
		}
		if agentStr.Valid {
			e.Agent = agentStr.String
		}
		if len(e.TaskID) >= 12 {
			e.TaskMarker = e.TaskID[:12]
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
		results = append(results, e)
	}
	return results, rows.Err()
}

// DistinctAgents returns the list of distinct agent names that have task runs.
func (s *Store) DistinctAgents() ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT t.agent FROM task_runs r JOIN tasks t ON r.task_id = t.id WHERE t.agent != '' ORDER BY t.agent`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var agents []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

// GetCostTrendSummary returns cost totals for today, this week, this month, and 30d average.
func (s *Store) GetCostTrendSummary(agent string) (*CostTrendSummary, error) {
	now := time.Now().UTC()
	today := now.Format("2006-01-02")
	weekStart := now.AddDate(0, 0, -int(now.Weekday())).Format("2006-01-02")
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	days30 := now.AddDate(0, 0, -30).Format(time.RFC3339)

	hasAgent := agent != ""
	agentJoin := ""
	agentWhere := ""
	if hasAgent {
		agentJoin = " JOIN tasks t ON r.task_id = t.id"
		agentWhere = " AND t.agent = ?"
	}

	var ts CostTrendSummary

	buildArgs := func(dateArg string) []any {
		if hasAgent {
			return []any{dateArg, agent}
		}
		return []any{dateArg}
	}

	q := fmt.Sprintf(`SELECT COALESCE(SUM(r.cost_usd), 0) FROM task_runs r%s WHERE strftime('%%Y-%%m-%%d', r.started_at) = ?%s`, agentJoin, agentWhere)
	if err := s.db.QueryRow(q, buildArgs(today)...).Scan(&ts.Today); err != nil {
		slog.Warn("query cost failed", "period", "today", "error", err)
	}

	q = fmt.Sprintf(`SELECT COALESCE(SUM(r.cost_usd), 0) FROM task_runs r%s WHERE strftime('%%Y-%%m-%%d', r.started_at) >= ?%s`, agentJoin, agentWhere)
	if err := s.db.QueryRow(q, buildArgs(weekStart)...).Scan(&ts.ThisWeek); err != nil {
		slog.Warn("query cost failed", "period", "this_week", "error", err)
	}

	q = fmt.Sprintf(`SELECT COALESCE(SUM(r.cost_usd), 0) FROM task_runs r%s WHERE strftime('%%Y-%%m-%%d', r.started_at) >= ?%s`, agentJoin, agentWhere)
	if err := s.db.QueryRow(q, buildArgs(monthStart)...).Scan(&ts.ThisMonth); err != nil {
		slog.Warn("query cost failed", "period", "this_month", "error", err)
	}

	// 30-day average: total over 30 days / 30
	var total30 float64
	q = fmt.Sprintf(`SELECT COALESCE(SUM(r.cost_usd), 0) FROM task_runs r%s WHERE r.started_at >= ?%s`, agentJoin, agentWhere)
	if err := s.db.QueryRow(q, buildArgs(days30)...).Scan(&total30); err != nil {
		slog.Warn("query cost failed", "period", "30d", "error", err)
	}
	ts.Avg30d = total30 / 30.0

	return &ts, nil
}

// TodayCost returns the total cost and turns for today from task_runs.
func (s *Store) TodayCost() (cost float64, turns int, taskCount int, err error) {
	today := time.Now().UTC().Format("2006-01-02")
	err = s.db.QueryRow(`SELECT COALESCE(SUM(cost_usd), 0), COALESCE(SUM(turns), 0), COUNT(DISTINCT task_id)
		FROM task_runs WHERE strftime('%Y-%m-%d', started_at) = ?`, today).Scan(&cost, &turns, &taskCount)
	return
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

