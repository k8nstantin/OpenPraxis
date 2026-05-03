package task

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	gpcpu "github.com/shirou/gopsutil/v3/cpu"
	gpdisk "github.com/shirou/gopsutil/v3/disk"
	gpload "github.com/shirou/gopsutil/v3/load"
	gpmem "github.com/shirou/gopsutil/v3/mem"
	gpnet "github.com/shirou/gopsutil/v3/net"

	executionlog "github.com/k8nstantin/OpenPraxis/internal/execution"
)

// HostMetricsSample is one CPU/RSS reading of the serve process. The
// sampler stamps a sample into every currently-attached task, so the
// per-run Run Stats card can overlay host load on the same timeline as
// the model's own turn/cost/actions lines.
type HostMetricsSample struct {
	TS      time.Time `json:"ts"`
	CPUPct  float64   `json:"cpu_pct"`
	RSSMB   float64   `json:"rss_mb"`
	// Disk capacity captured at the same tick — surfaces an agent
	// ballooning the worktree on the per-run timeline.
	DiskUsedGB  float64 `json:"disk_used_gb"`
	DiskTotalGB float64 `json:"disk_total_gb"`
	// Live task counters captured at the same tick as host CPU/RSS.
	// Feeds the Run Stats card's 5-aligned-sparkline layout — same X-axis
	// as CPU/RSS. Populated by a runner-supplied TaskStatFn closure.
	CostUSD float64 `json:"cost_usd"`
	Turns   int     `json:"turns"`
	Actions int     `json:"actions"`
}

// TaskStatFn pulls the attached task's live counters at sample time. The
// runner supplies it at Attach (closes over the RunningTask pointer so
// it sees live updates).
type TaskStatFn func() (costUSD float64, turns int, actions int)

// HostMetrics is the summary rolled up from a task's samples on Detach.
// Persisted as denormalised columns on task_runs so dashboards can sort
// / filter without scanning the samples table.
type HostMetrics struct {
	PeakCPUPct float64
	AvgCPUPct  float64
	PeakRSSMB  float64
	AvgRSSMB   float64
}

// HostSampler polls the serve process for CPU% + RSS every Interval and
// attributes each reading to every attached task. Tasks Attach at spawn
// and Detach at completion; Detach returns the accumulated samples and
// the rolled-up HostMetrics for persistence.
//
// Cheap by design: one `ps -o %cpu=,rss= -p <pid>` fork every Interval
// (default 5s). Zero when no tasks are attached — fanout is a no-op.
// attachedTask bundles the per-task sample buffer with the callback the
// sampler invokes at each tick to read live cost/turns/actions.
type attachedTask struct {
	samples []HostMetricsSample
	statFn  TaskStatFn
}

type HostSampler struct {
	mu       sync.Mutex
	attached map[string]*attachedTask

	interval time.Duration
	pid      int
	stopCh   chan struct{}
	started  bool

	// EL/M2-T3: per-tick fan-out also writes a row into
	// execution_log_samples for any attached task that has been
	// registered with an execution_log run id. Both fields are nil/empty
	// pre-wire and the per-tick path no-ops on them.
	execStore  *executionlog.Store
	execRunIDs map[string]string
}

// NewHostSampler returns a sampler that polls every interval. interval
// <= 0 → 5s default. Call Start to kick the polling goroutine.
func NewHostSampler(interval time.Duration) *HostSampler {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &HostSampler{
		attached:   make(map[string]*attachedTask),
		interval:   interval,
		pid:        os.Getpid(),
		stopCh:     make(chan struct{}),
		execRunIDs: make(map[string]string),
	}
}

// SetExecLogStore wires the unified execution_log store onto the sampler so
// the per-tick fanout can also append a row to execution_log_samples for
// every attached task that has been registered via RegisterExecLogRun.
// Nil is safe — the fanout simply skips the execution_log write.
func (s *HostSampler) SetExecLogStore(store *executionlog.Store) {
	s.mu.Lock()
	s.execStore = store
	s.mu.Unlock()
}

// RegisterExecLogRun associates an attached task with the execution_log
// row id minted at run start (EL/M2-T1). Only attached tasks with a
// registered run id participate in the per-tick execution_log_samples
// write. Re-registering the same taskID overwrites the prior id.
func (s *HostSampler) RegisterExecLogRun(taskID, execLogID string) {
	if taskID == "" || execLogID == "" {
		return
	}
	s.mu.Lock()
	if s.execRunIDs == nil {
		s.execRunIDs = make(map[string]string)
	}
	s.execRunIDs[taskID] = execLogID
	s.mu.Unlock()
}

// Start launches the poll loop. Idempotent — the second call is a no-op.
func (s *HostSampler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()
	go s.loop(ctx)
}

// Stop halts the poll loop. Detach-after-Stop still returns any samples
// collected before shutdown.
func (s *HostSampler) Stop() {
	select {
	case <-s.stopCh:
		// already closed
	default:
		close(s.stopCh)
	}
}

func (s *HostSampler) loop(ctx context.Context) {
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-t.C:
			s.mu.Lock()
			active := len(s.attached) > 0
			s.mu.Unlock()
			if !active {
				continue
			}
			sample, err := readProcMetrics(s.pid)
			if err != nil {
				slog.Warn("host metrics sample failed", "error", err)
				continue
			}
			s.fanout(sample)
		}
	}
}

// fanout snaps host CPU/RSS into each attached task's sample buffer,
// enriching per-task fields from the Attach-time callback. Host values
// are identical across all attached tasks in one tick.
//
// EL/M2-T3: in the same pass, append a row to execution_log_samples for
// every attached task that has been registered via RegisterExecLogRun.
// The execution_log write is best-effort — failures are logged at warn
// and never block the in-memory buffer that backs the legacy
// task_run_host_samples flush on Detach.
func (s *HostSampler) fanout(hostOnly HostMetricsSample) {
	s.mu.Lock()
	store := s.execStore
	type pendingExecSample struct {
		runID string
		smp   executionlog.Sample
	}
	var pending []pendingExecSample
	for taskID, at := range s.attached {
		sample := hostOnly
		if at.statFn != nil {
			sample.CostUSD, sample.Turns, sample.Actions = at.statFn()
		}
		at.samples = append(at.samples, sample)
		if store != nil {
			if runID, ok := s.execRunIDs[taskID]; ok && runID != "" {
				pending = append(pending, pendingExecSample{
					runID: runID,
					smp: executionlog.Sample{
						RunID:      runID,
						TS:         sample.TS.UnixMilli(),
						CPUPct:     sample.CPUPct,
						RSSMB:      sample.RSSMB,
						DiskUsedGB: sample.DiskUsedGB,
						CostUSD:    sample.CostUSD,
						Turns:      sample.Turns,
						Actions:    sample.Actions,
					},
				})
			}
		}
	}
	s.mu.Unlock()

	if store == nil || len(pending) == 0 {
		return
	}
	ctx := context.Background()
	for _, p := range pending {
		if err := store.InsertSample(ctx, p.smp); err != nil {
			slog.Warn("execution_log_samples insert failed",
				"component", "host_sampler", "run_id", p.runID, "error", err)
		}
	}
}

// Attach registers a task to receive samples + supplies a callback the
// sampler invokes at each tick to read live cost/turns/actions.
// statFn may be nil; per-task counters default to zero in that case.
// Starting-empty — reattach clears prior state.
func (s *HostSampler) Attach(taskID string, statFn TaskStatFn) {
	s.mu.Lock()
	s.attached[taskID] = &attachedTask{
		samples: make([]HostMetricsSample, 0, 64),
		statFn:  statFn,
	}
	s.mu.Unlock()
}

// Detach returns the samples accumulated for a task + the rolled-up
// summary, and removes the task from tracking. Safe to call for an
// unknown task ID — returns empty.
func (s *HostSampler) Detach(taskID string) ([]HostMetricsSample, HostMetrics) {
	s.mu.Lock()
	at := s.attached[taskID]
	delete(s.attached, taskID)
	delete(s.execRunIDs, taskID)
	s.mu.Unlock()
	if at == nil {
		return nil, HostMetrics{}
	}
	return at.samples, rollupSamples(at.samples)
}

func rollupSamples(samples []HostMetricsSample) HostMetrics {
	var m HostMetrics
	if len(samples) == 0 {
		return m
	}
	var cpuSum, rssSum float64
	for _, s := range samples {
		cpuSum += s.CPUPct
		rssSum += s.RSSMB
		if s.CPUPct > m.PeakCPUPct {
			m.PeakCPUPct = s.CPUPct
		}
		if s.RSSMB > m.PeakRSSMB {
			m.PeakRSSMB = s.RSSMB
		}
	}
	m.AvgCPUPct = cpuSum / float64(len(samples))
	m.AvgRSSMB = rssSum / float64(len(samples))
	return m
}

// ReadHostMetrics returns a single fresh sample of the current serve
// process's CPU% and RSS. Used by the /api/host/stats live-poll endpoint
// on the overview card; the sampler goroutine uses the same underlying
// call via readProcMetrics.
func ReadHostMetrics() (HostMetricsSample, error) {
	return readProcMetrics(os.Getpid())
}

// readProcMetrics shells out to `ps` to read the current process's
// %CPU + RSS. Cross-platform (macOS + Linux both support `-o %cpu=,rss=`)
// and cheap enough at 5s cadence that the fork cost is noise compared to
// per-tool-call hook work. `ps` reports RSS in KB on both platforms.
func readProcMetrics(pid int) (HostMetricsSample, error) {
	cmd := exec.Command("ps", "-o", "%cpu=,rss=", "-p", strconv.Itoa(pid))
	out, err := cmd.Output()
	if err != nil {
		return HostMetricsSample{}, fmt.Errorf("ps: %w", err)
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) < 2 {
		return HostMetricsSample{}, fmt.Errorf("ps output malformed: %q", string(out))
	}
	cpu, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return HostMetricsSample{}, fmt.Errorf("parse cpu: %w", err)
	}
	rssKB, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return HostMetricsSample{}, fmt.Errorf("parse rss: %w", err)
	}
	smp := HostMetricsSample{
		TS:     time.Now().UTC(),
		CPUPct: cpu,
		RSSMB:  rssKB / 1024.0,
	}
	if d, err := gpdisk.Usage("/"); err == nil && d != nil {
		smp.DiskUsedGB = float64(d.Used) / (1024 * 1024 * 1024)
		smp.DiskTotalGB = float64(d.Total) / (1024 * 1024 * 1024)
	}
	return smp, nil
}

// SystemHostSample is one capacity reading of the host (independent of
// any task). Persisted to system_host_samples; powers the Stats tab's
// System Capacity panel. Captured by SystemSampler at every tick.
type SystemHostSample struct {
	TS          time.Time `json:"ts"`
	CPUPct      float64   `json:"cpu_pct"`
	Load1m      float64   `json:"load_1m"`
	Load5m      float64   `json:"load_5m"`
	Load15m     float64   `json:"load_15m"`
	MemUsedMB   float64   `json:"mem_used_mb"`
	MemTotalMB  float64   `json:"mem_total_mb"`
	SwapUsedMB  float64   `json:"swap_used_mb"`
	DiskUsedGB  float64   `json:"disk_used_gb"`
	DiskTotalGB float64   `json:"disk_total_gb"`
	NetRxMbps   float64   `json:"net_rx_mbps"`
	NetTxMbps   float64   `json:"net_tx_mbps"`
	DiskReadMBps  float64 `json:"disk_read_mbps"`
	DiskWriteMBps float64 `json:"disk_write_mbps"`
}

// readSystemMetrics returns a single fresh capacity sample for the
// whole host. Cheap (no shell-out — gopsutil reads sysctl / /proc
// directly). Network throughput is computed as a delta against
// previous-sample byte totals; the first call after start records the
// baseline and reports zeros so a misleading "all bytes since boot"
// spike doesn't land on the chart.
func readSystemMetrics(prev systemNetBaseline) (SystemHostSample, systemNetBaseline) {
	smp := SystemHostSample{TS: time.Now().UTC()}

	// CPU% — single 0-interval call returns avg-since-last-call. If
	// percent is unavailable we leave 0 rather than blocking the tick.
	if pcts, err := gpcpu.Percent(0, false); err == nil && len(pcts) > 0 {
		smp.CPUPct = pcts[0]
	}

	if l, err := gpload.Avg(); err == nil && l != nil {
		smp.Load1m, smp.Load5m, smp.Load15m = l.Load1, l.Load5, l.Load15
	}

	if v, err := gpmem.VirtualMemory(); err == nil && v != nil {
		smp.MemUsedMB = float64(v.Used) / (1024 * 1024)
		smp.MemTotalMB = float64(v.Total) / (1024 * 1024)
	}
	if sw, err := gpmem.SwapMemory(); err == nil && sw != nil {
		smp.SwapUsedMB = float64(sw.Used) / (1024 * 1024)
	}

	if d, err := gpdisk.Usage("/"); err == nil && d != nil {
		smp.DiskUsedGB = float64(d.Used) / (1024 * 1024 * 1024)
		smp.DiskTotalGB = float64(d.Total) / (1024 * 1024 * 1024)
	}

	next := prev
	if io, err := gpnet.IOCounters(false); err == nil && len(io) > 0 {
		now := time.Now()
		rx, tx := io[0].BytesRecv, io[0].BytesSent
		if !prev.At.IsZero() {
			dt := now.Sub(prev.At).Seconds()
			if dt > 0 {
				smp.NetRxMbps = float64(rx-prev.RxBytes) * 8 / 1e6 / dt
				smp.NetTxMbps = float64(tx-prev.TxBytes) * 8 / 1e6 / dt
				if smp.NetRxMbps < 0 {
					smp.NetRxMbps = 0
				}
				if smp.NetTxMbps < 0 {
					smp.NetTxMbps = 0
				}
			}
		}
		next = systemNetBaseline{At: now, RxBytes: rx, TxBytes: tx, DiskRead: prev.DiskRead, DiskWrite: prev.DiskWrite}
	}

	// Disk I/O — gopsutil/disk.IOCounters returns a map of per-device
	// counters. Sum across devices for a host-wide read/write rate.
	if iom, err := gpdisk.IOCounters(); err == nil {
		var rb, wb uint64
		for _, ioc := range iom {
			rb += ioc.ReadBytes
			wb += ioc.WriteBytes
		}
		now := next.At
		if now.IsZero() {
			now = time.Now()
		}
		if !prev.At.IsZero() && (prev.DiskRead != 0 || prev.DiskWrite != 0) {
			dt := now.Sub(prev.At).Seconds()
			if dt > 0 {
				smp.DiskReadMBps = float64(rb-prev.DiskRead) / (1024 * 1024) / dt
				smp.DiskWriteMBps = float64(wb-prev.DiskWrite) / (1024 * 1024) / dt
				if smp.DiskReadMBps < 0 {
					smp.DiskReadMBps = 0
				}
				if smp.DiskWriteMBps < 0 {
					smp.DiskWriteMBps = 0
				}
			}
		}
		next.DiskRead = rb
		next.DiskWrite = wb
		if next.At.IsZero() {
			next.At = now
		}
	}

	return smp, next
}

// systemNetBaseline holds the previous-tick network + disk-IO byte
// totals so the next tick can compute a per-second delta. Embedded in
// SystemSampler.
type systemNetBaseline struct {
	At         time.Time
	RxBytes    uint64
	TxBytes    uint64
	DiskRead   uint64
	DiskWrite  uint64
}

// SystemSampler runs continuously from server start, capturing one host
// capacity sample per tick to system_host_samples. Independent of any
// task — keeps a baseline visible even when no tasks are running.
//
// Tick rate is read from the `host_sampler_tick_seconds` knob at
// construction. Reload on knob change is out of scope; restarting serve
// applies a new value.
type SystemSampler struct {
	db       *sql.DB
	interval time.Duration
	stopCh   chan struct{}
	started  bool
	mu       sync.Mutex
}

// NewSystemSampler returns a sampler ticking every interval. interval
// <= 0 → 5s default. Call Start to begin writing rows.
func NewSystemSampler(db *sql.DB, interval time.Duration) *SystemSampler {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &SystemSampler{
		db:       db,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start launches the poll loop. Idempotent — second call is a no-op.
func (s *SystemSampler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()
	go s.loop(ctx)
}

// Stop halts the poll loop.
func (s *SystemSampler) Stop() {
	select {
	case <-s.stopCh:
	default:
		close(s.stopCh)
	}
}

func (s *SystemSampler) loop(ctx context.Context) {
	t := time.NewTicker(s.interval)
	defer t.Stop()
	var baseline systemNetBaseline
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-t.C:
			var smp SystemHostSample
			smp, baseline = readSystemMetrics(baseline)
			if _, err := s.db.Exec(
				`INSERT INTO system_host_samples
				(ts, cpu_pct, load_1m, load_5m, load_15m, mem_used_mb, mem_total_mb, swap_used_mb, disk_used_gb, disk_total_gb, net_rx_mbps, net_tx_mbps, disk_read_mbps, disk_write_mbps)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				smp.TS.UTC().Format(time.RFC3339Nano),
				smp.CPUPct, smp.Load1m, smp.Load5m, smp.Load15m,
				smp.MemUsedMB, smp.MemTotalMB, smp.SwapUsedMB,
				smp.DiskUsedGB, smp.DiskTotalGB,
				smp.NetRxMbps, smp.NetTxMbps,
				smp.DiskReadMBps, smp.DiskWriteMBps,
			); err != nil {
				slog.Warn("system sample insert failed", "error", err)
			}
		}
	}
}
