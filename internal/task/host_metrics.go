package task

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HostMetricsSample is one CPU/RSS reading of the serve process. The
// sampler stamps a sample into every currently-attached task, so the
// per-run Run Stats card can overlay host load on the same timeline as
// the model's own turn/cost/actions lines.
type HostMetricsSample struct {
	TS     time.Time `json:"ts"`
	CPUPct float64   `json:"cpu_pct"`
	RSSMB  float64   `json:"rss_mb"`
}

// HostMetrics is the summary rolled up from a task's samples on Detach.
// Persisted as denormalised columns on task_runs so dashboards can sort
// / filter without scanning the samples table.
type HostMetrics struct {
	PeakCPUPct float64
	AvgCPUPct  float64
	PeakRSSMB  float64
}

// HostSampler polls the serve process for CPU% + RSS every Interval and
// attributes each reading to every attached task. Tasks Attach at spawn
// and Detach at completion; Detach returns the accumulated samples and
// the rolled-up HostMetrics for persistence.
//
// Cheap by design: one `ps -o %cpu=,rss= -p <pid>` fork every Interval
// (default 5s). Zero when no tasks are attached — fanout is a no-op.
type HostSampler struct {
	mu       sync.Mutex
	attached map[string]*[]HostMetricsSample

	interval time.Duration
	pid      int
	stopCh   chan struct{}
	started  bool
}

// NewHostSampler returns a sampler that polls every interval. interval
// <= 0 → 5s default. Call Start to kick the polling goroutine.
func NewHostSampler(interval time.Duration) *HostSampler {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &HostSampler{
		attached: make(map[string]*[]HostMetricsSample),
		interval: interval,
		pid:      os.Getpid(),
		stopCh:   make(chan struct{}),
	}
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

func (s *HostSampler) fanout(sample HostMetricsSample) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, slot := range s.attached {
		*slot = append(*slot, sample)
	}
}

// Attach registers a task to receive samples. Starting-empty — reattach
// clears any prior state.
func (s *HostSampler) Attach(taskID string) {
	s.mu.Lock()
	buf := make([]HostMetricsSample, 0, 64)
	s.attached[taskID] = &buf
	s.mu.Unlock()
}

// Detach returns the samples accumulated for a task + the rolled-up
// summary, and removes the task from tracking. Safe to call for an
// unknown task ID — returns empty.
func (s *HostSampler) Detach(taskID string) ([]HostMetricsSample, HostMetrics) {
	s.mu.Lock()
	slot := s.attached[taskID]
	delete(s.attached, taskID)
	s.mu.Unlock()
	if slot == nil {
		return nil, HostMetrics{}
	}
	return *slot, rollupSamples(*slot)
}

func rollupSamples(samples []HostMetricsSample) HostMetrics {
	var m HostMetrics
	if len(samples) == 0 {
		return m
	}
	var cpuSum float64
	for _, s := range samples {
		cpuSum += s.CPUPct
		if s.CPUPct > m.PeakCPUPct {
			m.PeakCPUPct = s.CPUPct
		}
		if s.RSSMB > m.PeakRSSMB {
			m.PeakRSSMB = s.RSSMB
		}
	}
	m.AvgCPUPct = cpuSum / float64(len(samples))
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
	return HostMetricsSample{
		TS:     time.Now().UTC(),
		CPUPct: cpu,
		RSSMB:  rssKB / 1024.0,
	}, nil
}
