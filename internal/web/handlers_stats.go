package web

import (
	"net/http"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/execution"
	"github.com/k8nstantin/OpenPraxis/internal/node"
)

// GET /api/run-stats?entity_kind=task&entity_id=...&as_of=<rfc3339>
//
// Returns execution_log rows for an entity plus per-run sample rows.
// `as_of` is accepted for API compatibility but filtering is done
// client-side by the Stats tab (execution_log rows carry unix timestamps).
//
// Powers the Stats tab Cumulative + Per-run panels.
type runStatsResponse struct {
	Runs         []execution.Row              `json:"runs"`
	SamplesByRun map[string][]execution.Row   `json:"samples_by_run"`
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

		// Verify entity exists.
		if n.Entities == nil {
			writeError(w, "entity store not ready", 503)
			return
		}
		if e, _ := n.Entities.Get(entityID); e == nil {
			writeError(w, "entity not found: "+entityID, 404)
			return
		}

		// Validate as_of when provided (accepted for API compat; not used for
		// DB-level filtering since execution_log uses unix epoch timestamps).
		if asOfStr != "" {
			if _, err := time.Parse(time.RFC3339, asOfStr); err != nil {
				writeError(w, "as_of must be RFC3339", 400)
				return
			}
		}

		ctx := r.Context()
		allRows, err := n.ExecutionLog.ListByEntity(ctx, entityID, 500)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}

		// Separate terminal/summary rows (completed/failed/started) from
		// sample rows. Terminal rows are the "runs" the Stats tab charts.
		// Sample rows are grouped by run_uid for the per-run overlay.
		var runs []execution.Row
		samplesByRun := map[string][]execution.Row{}

		for _, row := range allRows {
			if row.Event == execution.EventSample {
				samplesByRun[row.RunUID] = append(samplesByRun[row.RunUID], row)
			} else {
				runs = append(runs, row)
			}
		}

		if runs == nil {
			runs = []execution.Row{}
		}

		writeJSON(w, runStatsResponse{Runs: runs, SamplesByRun: samplesByRun})
	}
}

// GET /api/system-stats?from=<rfc3339>&to=<rfc3339>&as_of=<rfc3339>
//
// Returns rows from system_host_samples within the [from, to] window,
// further bounded by as_of when supplied. Powers the Stats tab System
// Capacity panel.
type systemStatsResponse struct {
	Samples []sysHostSample `json:"samples"`
}

// sysHostSample is the wire shape for one system_host_samples row.
// Mirrors the task.SystemHostSample fields without importing internal/task.
type sysHostSample struct {
	TS           time.Time `json:"ts"`
	CPUPct       float64   `json:"cpu_pct"`
	Load1m       float64   `json:"load_1m"`
	Load5m       float64   `json:"load_5m"`
	Load15m      float64   `json:"load_15m"`
	MemUsedMB    float64   `json:"mem_used_mb"`
	MemTotalMB   float64   `json:"mem_total_mb"`
	SwapUsedMB   float64   `json:"swap_used_mb"`
	DiskUsedGB   float64   `json:"disk_used_gb"`
	DiskTotalGB  float64   `json:"disk_total_gb"`
	NetRxMbps    float64   `json:"net_rx_mbps"`
	NetTxMbps    float64   `json:"net_tx_mbps"`
	DiskReadMBps float64   `json:"disk_read_mbps"`
	DiskWriteMBps float64  `json:"disk_write_mbps"`
}

func apiSystemStats(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=5")
		q := r.URL.Query()
		fromStr := q.Get("from")
		toStr := q.Get("to")

		now := time.Now().UTC()
		from := now.Add(-1 * time.Hour)
		to := now
		if fromStr != "" {
			if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
				from = t
			}
		}
		if toStr != "" {
			if t, err := time.Parse(time.RFC3339, toStr); err == nil {
				to = t
			}
		}

		// Source: execution_log sample rows — single source of truth.
		// Reads rows written by the runner's host sampler.
		//
		// execution_log only carries load_avg_1m, mem_used_mb, mem_total_mb
		// and disk_used_gb today. load_5m/load_15m/swap_used_mb/disk_total_gb
		// are returned as 0 rather than fabricating duplicate-of-load_1m
		// values; clients should show "—" rather than chart a flat line.
		db := n.DB()
		rows, err := db.Query(`
			SELECT created_at,
			       cpu_pct, load_avg_1m,
			       mem_used_mb, mem_total_mb,
			       disk_used_gb,
			       net_rx_mbps, net_tx_mbps,
			       disk_read_mbps, disk_write_mbps
			FROM execution_log
			WHERE event = 'sample'
			  AND created_at >= ? AND created_at <= ?
			ORDER BY created_at ASC LIMIT 5000`,
			from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339),
		)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		defer rows.Close()

		samples := []sysHostSample{}
		for rows.Next() {
			var s sysHostSample
			var tsStr string
			if err := rows.Scan(&tsStr, &s.CPUPct,
				&s.Load1m,
				&s.MemUsedMB, &s.MemTotalMB,
				&s.DiskUsedGB,
				&s.NetRxMbps, &s.NetTxMbps,
				&s.DiskReadMBps, &s.DiskWriteMBps); err != nil {
				continue
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
