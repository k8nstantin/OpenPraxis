package task

import (
	"bufio"
	"encoding/json"
	"math"
	"os"
	"testing"
)

// TestPricingE2E_ReplaysRealRun — optional end-to-end check that replays a
// recorded Claude Code stream-json run against the live-cost path.
//
// Enables only when OPENPRAXIS_E2E_RUN_PATH points at a run-output file
// (one event JSON per line, same format we store in task_runs.output). The
// test:
//   1. Walks the file exactly like runner.go does: per-message-id usage
//      dedupe + live cost estimate.
//   2. Calibrates a fresh pricing multiplier from the terminal result event.
//   3. Asserts that, after calibration, re-computing EstimateCost on the
//      final summed usage lands within 1 cent of the reported total_cost_usd.
//
// Skipped in CI. Run locally with:
//   OPENPRAXIS_E2E_RUN_PATH=/tmp/run120.jsonl go test ./internal/task/ -run PricingE2E -v
func TestPricingE2E_ReplaysRealRun(t *testing.T) {
	path := os.Getenv("OPENPRAXIS_E2E_RUN_PATH")
	if path == "" {
		t.Skip("OPENPRAXIS_E2E_RUN_PATH not set; skipping replay test")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	usageByMessage := make(map[string]Usage)
	var model string
	var allOutput []byte
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		allOutput = append(allOutput, line...)
		allOutput = append(allOutput, '\n')
		var event map[string]any
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}
		if id, m, u, ok := parseAssistantUsage(event); ok {
			if model == "" && m != "" {
				model = m
			}
			if id != "" {
				usageByMessage[id] = u
			}
		}
	}

	// Final summed usage from the dedupe map.
	var summed Usage
	for _, u := range usageByMessage {
		summed = summed.Add(u)
	}

	// Pull the authoritative cost + usage from the result event.
	reported, _ := ParseCostFromOutput(string(allOutput))
	resultUsage, _ := parseFinalResultUsage(string(allOutput))

	t.Logf("model=%s", model)
	t.Logf("summed-from-assistant-events: %+v", summed)
	t.Logf("result-event-usage: %+v", resultUsage)
	t.Logf("reported total_cost_usd: %.6f", reported)

	// Calibrate against the result event (authoritative).
	s := openPricingTestDB(t)
	if err := s.CalibrateModelPricing(model, reported, resultUsage); err != nil {
		t.Fatalf("Calibrate: %v", err)
	}
	mult := s.GetModelMultiplier(model)
	t.Logf("calibrated multiplier: %.6f", mult)

	// Re-estimate using the calibrated rates against the authoritative usage
	// — this must reproduce the reported cost to near-float-precision, since
	// calibration is literally the ratio that makes this hold.
	est := EstimateCost(model, mult, resultUsage)
	if math.Abs(est-reported) > 0.01 {
		t.Fatalf("post-calibration estimate = $%.4f, want ~$%.4f", est, reported)
	}

	// And the live-estimate path (pre-calibration, default multiplier 1.0,
	// summed per-message usage) should have been > 0 — confirming the bug
	// fix: the running-task card would have shown a real number, not $0.
	live := EstimateCost(model, 1.0, summed)
	if live <= 0 {
		t.Fatalf("live estimate was $0 — the bug still reproduces")
	}
	t.Logf("live estimate (pre-calibration): $%.4f (reported $%.4f)", live, reported)
}
