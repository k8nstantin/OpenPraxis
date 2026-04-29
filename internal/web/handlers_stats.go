package web

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/task"
)

// GET /api/run-stats?entity_kind=...&entity_id=...&as_of=<rfc3339>
//
// Returns the task_runs rows + per-run host samples for an entity. The
// entity_kind dispatches the join:
//   - product:  recursive descend product_dependencies → all task_runs
//                under any descendant manifest's tasks (mirrors
//                EnrichRecursiveCosts).
//   - manifest: all task_runs WHERE task.manifest_id = entity_id.
//   - task:     all task_runs WHERE task_id = entity_id.
//
// `as_of` filters all reads to WHERE started_at <= as_of (or ts <= as_of
// for samples). Powers the Stats tab Cumulative + Per-run panels.
type runStatsResponse struct {
	Runs          []task.TaskRun                    `json:"runs"`
	SamplesByRun  map[string][]task.HostMetricsSample `json:"samples_by_run"`
}

func apiRunStats(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=5")
		q := r.URL.Query()
		kind := q.Get("entity_kind")
		entityID := q.Get("entity_id")
		asOfStr := q.Get("as_of")

		if kind == "" || entityID == "" {
			writeError(w, "entity_kind and entity_id required", 400)
			return
		}

		asOfClause := ""
		var args []any
		if asOfStr != "" {
			if _, err := time.Parse(time.RFC3339, asOfStr); err != nil {
				writeError(w, "as_of must be RFC3339", 400)
				return
			}
			asOfClause = " AND started_at <= ?"
		}

		db := n.Tasks.DB()
		runs, err := selectRunsForEntity(db, kind, entityID, asOfClause, asOfStr, args)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		if runs == nil {
			runs = []task.TaskRun{}
		}

		samples := map[string][]task.HostMetricsSample{}
		for _, run := range runs {
			s, err := loadSamples(db, run.ID, asOfStr)
			if err != nil {
				continue
			}
			if s == nil {
				s = []task.HostMetricsSample{}
			}
			samples[itoa(run.ID)] = s
		}

		writeJSON(w, runStatsResponse{Runs: runs, SamplesByRun: samples})
	}
}

// selectRunsForEntity returns task_runs scoped to the entity. Walks the
// product DAG when kind=product so a parent product's stats include all
// descendants — mirrors the existing EnrichRecursiveCosts walk.
func selectRunsForEntity(db *sql.DB, kind, entityID, asOfClause, asOfStr string, _ []any) ([]task.TaskRun, error) {
	cols := taskRunColumnsForSelect("tr")

	var (
		query string
		args  []any
	)
	switch kind {
	case "task":
		query = `SELECT ` + cols + ` FROM task_runs tr WHERE tr.task_id = ?` + asOfClause + ` ORDER BY tr.started_at DESC LIMIT 500`
		args = append(args, entityID)
		if asOfStr != "" {
			args = append(args, asOfStr)
		}
	case "manifest":
		query = `SELECT ` + cols + ` FROM task_runs tr
			INNER JOIN tasks t ON t.id = tr.task_id AND t.deleted_at = ''
			WHERE t.manifest_id = ?` + asOfClause + ` ORDER BY tr.started_at DESC LIMIT 500`
		args = append(args, entityID)
		if asOfStr != "" {
			args = append(args, asOfStr)
		}
	case "product":
		// Mirror product.Store.EnrichRecursiveCosts — recursive descend
		// into descendant products via product_dependencies, then sum
		// task_runs under any descendant's manifests.
		query = `WITH RECURSIVE descendants(id) AS (
				SELECT ?
				UNION ALL
				SELECT pd.depends_on_product_id FROM product_dependencies pd
				INNER JOIN descendants d ON pd.product_id = d.id
			)
			SELECT ` + cols + ` FROM task_runs tr
			INNER JOIN tasks t ON t.id = tr.task_id AND t.deleted_at = ''
			INNER JOIN manifests m ON m.id = t.manifest_id AND m.deleted_at = ''
			INNER JOIN descendants d ON d.id = m.project_id
			WHERE 1=1` + asOfClause + ` ORDER BY tr.started_at DESC LIMIT 500`
		args = append(args, entityID)
		if asOfStr != "" {
			args = append(args, asOfStr)
		}
	default:
		return nil, errBadKind(kind)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanTaskRunsRows(rows)
}

// taskRunColumnsForSelect returns the canonical task_runs column list
// prefixed with the supplied alias. Mirrors task.taskRunsColumns shape so
// scanTaskRunsRows can decode the exact same fields.
func taskRunColumnsForSelect(alias string) string {
	cols := []string{
		"id", "task_id", "run_number", "output", "status", "actions", "lines",
		"cost_usd", "turns", "started_at", "completed_at",
		"input_tokens", "output_tokens", "cache_read_tokens", "cache_create_tokens",
		"model", "pricing_version",
		"peak_cpu_pct", "avg_cpu_pct", "peak_rss_mb",
		"errors", "compactions", "files_changed", "exit_code",
		"cancelled_at", "cancelled_by", "duration_ms", "avg_rss_mb",
		"branch", "commit_sha", "commits", "pr_number",
		"worktree_path", "agent_runtime", "agent_version",
		"lines_added", "lines_removed",
	}
	if alias == "" {
		return strings.Join(cols, ", ")
	}
	for i, c := range cols {
		cols[i] = alias + "." + c
	}
	return strings.Join(cols, ", ")
}

// scanTaskRunsRows decodes rows in the order of taskRunColumnsForSelect.
// Keeps the Stats endpoint independent of internal/task's unexported
// scanRuns helper while still reading the same column set.
func scanTaskRunsRows(rows *sql.Rows) ([]task.TaskRun, error) {
	out := []task.TaskRun{}
	for rows.Next() {
		var r task.TaskRun
		var startedStr, completedStr string
		if err := rows.Scan(&r.ID, &r.TaskID, &r.RunNumber, &r.Output, &r.Status,
			&r.Actions, &r.Lines, &r.CostUSD, &r.Turns, &startedStr, &completedStr,
			&r.InputTokens, &r.OutputTokens, &r.CacheReadTokens, &r.CacheCreateTokens,
			&r.Model, &r.PricingVersion,
			&r.PeakCPUPct, &r.AvgCPUPct, &r.PeakRSSMB,
			&r.Errors, &r.Compactions, &r.FilesChanged, &r.ExitCode,
			&r.CancelledAt, &r.CancelledBy, &r.DurationMS, &r.AvgRSSMB,
			&r.Branch, &r.CommitSHA, &r.Commits, &r.PRNumber,
			&r.WorktreePath, &r.AgentRuntime, &r.AgentVersion,
			&r.LinesAdded, &r.LinesRemoved); err != nil {
			return nil, err
		}
		r.StartedAt, _ = time.Parse(time.RFC3339, startedStr)
		r.CompletedAt, _ = time.Parse(time.RFC3339, completedStr)
		out = append(out, r)
	}
	return out, rows.Err()
}

// loadSamples fetches host samples for a single run, optionally bounded
// by as_of. Returns []HostMetricsSample (never nil).
func loadSamples(db *sql.DB, runID int, asOfStr string) ([]task.HostMetricsSample, error) {
	q := `SELECT ts, cpu_pct, rss_mb, cost_usd, turns, actions, disk_used_gb, disk_total_gb
		FROM task_run_host_samples WHERE run_id = ?`
	args := []any{runID}
	if asOfStr != "" {
		q += ` AND ts <= ?`
		args = append(args, asOfStr)
	}
	q += ` ORDER BY ts ASC`
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []task.HostMetricsSample{}
	for rows.Next() {
		var smp task.HostMetricsSample
		var tsStr string
		if err := rows.Scan(&tsStr, &smp.CPUPct, &smp.RSSMB, &smp.CostUSD,
			&smp.Turns, &smp.Actions, &smp.DiskUsedGB, &smp.DiskTotalGB); err != nil {
			return nil, err
		}
		if t, err := time.Parse(time.RFC3339Nano, tsStr); err == nil {
			smp.TS = t
		} else if t, err := time.Parse(time.RFC3339, tsStr); err == nil {
			smp.TS = t
		}
		out = append(out, smp)
	}
	return out, rows.Err()
}

// GET /api/system-stats?from=<rfc3339>&to=<rfc3339>&as_of=<rfc3339>
//
// Returns rows from system_host_samples within the [from, to] window,
// further bounded by as_of when supplied. Powers the Stats tab System
// Capacity panel.
type systemStatsResponse struct {
	Samples []task.SystemHostSample `json:"samples"`
}

func apiSystemStats(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=5")
		q := r.URL.Query()
		fromStr := q.Get("from")
		toStr := q.Get("to")
		asOfStr := q.Get("as_of")

		// Defaults: last 1 hour. Mirrors the Stats tab default window.
		now := time.Now().UTC()
		from := now.Add(-1 * time.Hour)
		to := now
		if fromStr != "" {
			t, err := time.Parse(time.RFC3339, fromStr)
			if err != nil {
				writeError(w, "from must be RFC3339", 400)
				return
			}
			from = t
		}
		if toStr != "" {
			t, err := time.Parse(time.RFC3339, toStr)
			if err != nil {
				writeError(w, "to must be RFC3339", 400)
				return
			}
			to = t
		}

		query := `SELECT ts, cpu_pct, load_1m, load_5m, load_15m,
			mem_used_mb, mem_total_mb, swap_used_mb,
			disk_used_gb, disk_total_gb, net_rx_mbps, net_tx_mbps
			FROM system_host_samples WHERE ts >= ? AND ts <= ?`
		args := []any{from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339)}
		if asOfStr != "" {
			if _, err := time.Parse(time.RFC3339, asOfStr); err != nil {
				writeError(w, "as_of must be RFC3339", 400)
				return
			}
			query += ` AND ts <= ?`
			args = append(args, asOfStr)
		}
		query += ` ORDER BY ts ASC LIMIT 5000`

		rows, err := n.Tasks.DB().Query(query, args...)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		defer rows.Close()

		samples := []task.SystemHostSample{}
		for rows.Next() {
			var s task.SystemHostSample
			var tsStr string
			if err := rows.Scan(&tsStr, &s.CPUPct,
				&s.Load1m, &s.Load5m, &s.Load15m,
				&s.MemUsedMB, &s.MemTotalMB, &s.SwapUsedMB,
				&s.DiskUsedGB, &s.DiskTotalGB,
				&s.NetRxMbps, &s.NetTxMbps); err != nil {
				writeError(w, err.Error(), 500)
				return
			}
			if t, err := time.Parse(time.RFC3339Nano, tsStr); err == nil {
				s.TS = t
			} else if t, err := time.Parse(time.RFC3339, tsStr); err == nil {
				s.TS = t
			}
			samples = append(samples, s)
		}
		writeJSON(w, systemStatsResponse{Samples: samples})
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	return intToStr(i)
}

func intToStr(i int) string {
	// Cheap int → string without strconv import; only used for map keys.
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

func errBadKind(kind string) error {
	return &kindError{kind: kind}
}

type kindError struct{ kind string }

func (e *kindError) Error() string {
	return "unknown entity_kind: " + e.kind + " (want product|manifest|task)"
}
