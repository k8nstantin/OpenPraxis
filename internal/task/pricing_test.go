package task

import (
	"database/sql"
	"math"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func openPricingTestDB(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "p.db") + "?_journal_mode=WAL&_busy_timeout=5000"
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	s, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s
}

// TestDefaultRates_OpusSonnetHaiku — the family dispatch is substring-based;
// it must pick the right rate card even for versioned model ids like
// "claude-opus-4-7" and is case-insensitive.
func TestDefaultRates_OpusSonnetHaiku(t *testing.T) {
	cases := []struct {
		model    string
		wantIn   float64
		wantOut  float64
	}{
		{"claude-opus-4-7", 15, 75},
		{"Claude-Sonnet-4-6", 3, 15},
		{"claude-haiku-4-5-20251001", 1, 5},
		{"unknown-model", 15, 75}, // fallback = opus
	}
	for _, c := range cases {
		r := DefaultRates(c.model)
		if r.InputPerMTok != c.wantIn || r.OutputPerMTok != c.wantOut {
			t.Errorf("DefaultRates(%q) = {in:%.2f out:%.2f}, want {in:%.2f out:%.2f}",
				c.model, r.InputPerMTok, r.OutputPerMTok, c.wantIn, c.wantOut)
		}
	}
}

// TestComputeCost — sanity check the math. $15/M in × 1M tokens = $15, etc.
func TestComputeCost(t *testing.T) {
	r := ModelRates{InputPerMTok: 15, OutputPerMTok: 75, CacheWritePerMTok: 18.75, CacheReadPerMTok: 1.5}
	u := Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000, CacheCreationTokens: 1_000_000, CacheReadTokens: 1_000_000}
	got := ComputeCost(r, u)
	want := 15.0 + 75.0 + 18.75 + 1.5
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("ComputeCost = %.6f, want %.6f", got, want)
	}
}

// TestGetModelMultiplier_DefaultOne — an uncalibrated model must estimate at
// default rates (multiplier 1.0), not at 0.
func TestGetModelMultiplier_DefaultOne(t *testing.T) {
	s := openPricingTestDB(t)
	if m := s.GetModelMultiplier("claude-opus-4-7"); m != 1.0 {
		t.Fatalf("uncalibrated multiplier = %.4f, want 1.0", m)
	}
}

// TestCalibrateModelPricing_FirstSampleReplaces — the first calibration
// should snap the multiplier to the observed ratio with no EMA damping.
// Reproduces the real task 120 numbers from the production DB: 1.25625375
// final cost against the usage that default opus rates would price at ~3.77.
func TestCalibrateModelPricing_FirstSampleReplaces(t *testing.T) {
	s := openPricingTestDB(t)
	u := Usage{
		InputTokens:         38,
		OutputTokens:        9942,
		CacheCreationTokens: 58391,
		CacheReadTokens:     1278022,
	}
	const model = "claude-opus-4-7"
	const reported = 1.25625375

	if err := s.CalibrateModelPricing(model, reported, u); err != nil {
		t.Fatalf("Calibrate: %v", err)
	}
	mult := s.GetModelMultiplier(model)
	predicted := ComputeCost(DefaultRates(model), u)
	wantSample := reported / predicted
	if math.Abs(mult-wantSample) > 1e-9 {
		t.Fatalf("multiplier = %.6f, want first-sample %.6f", mult, wantSample)
	}
	// EstimateCost with the calibrated multiplier must reproduce the reported
	// cost to within float noise — this is the whole point of the calibration.
	if est := EstimateCost(model, mult, u); math.Abs(est-reported) > 1e-6 {
		t.Fatalf("EstimateCost = %.6f, want %.6f", est, reported)
	}
}

// TestCalibrateModelPricing_EMABlendsOnSecondSample — a second sample must
// be EMA-blended (not overwrite), so a single outlier cannot jerk the
// estimate. Concretely: prior 0.3, new sample 1.0, alpha 0.3 → 0.3*0.7 + 1.0*0.3 = 0.51.
func TestCalibrateModelPricing_EMABlendsOnSecondSample(t *testing.T) {
	s := openPricingTestDB(t)
	const model = "claude-opus-4-7"

	// First run: usage priced at default $1 (1M input tokens), reported $0.30 → multiplier 0.3.
	u1 := Usage{InputTokens: 66_667} // 66_667 * 15 / 1e6 ≈ $1.00005
	if err := s.CalibrateModelPricing(model, 0.30003, u1); err != nil {
		t.Fatalf("first calibrate: %v", err)
	}
	first := s.GetModelMultiplier(model)
	if math.Abs(first-0.3) > 1e-3 {
		t.Fatalf("first multiplier = %.4f, want ~0.3", first)
	}

	// Second run: usage priced at $1, reported $1.00 → sample multiplier 1.0.
	// Blended: 0.3 * 0.7 + 1.0 * 0.3 = 0.51.
	if err := s.CalibrateModelPricing(model, 1.00005, u1); err != nil {
		t.Fatalf("second calibrate: %v", err)
	}
	blended := s.GetModelMultiplier(model)
	if math.Abs(blended-0.51) > 0.01 {
		t.Fatalf("blended multiplier = %.4f, want ~0.51", blended)
	}
}

// TestParseAssistantUsage — confirms we read the four token fields off a
// realistic stream-json assistant event, and dedupe key is message.id.
func TestParseAssistantUsage(t *testing.T) {
	event := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"id":    "msg_abc",
			"model": "claude-opus-4-7",
			"usage": map[string]any{
				"input_tokens":                float64(5),
				"output_tokens":               float64(8),
				"cache_creation_input_tokens": float64(15955),
				"cache_read_input_tokens":     float64(16204),
			},
		},
	}
	id, model, u, ok := parseAssistantUsage(event)
	if !ok || id != "msg_abc" || model != "claude-opus-4-7" {
		t.Fatalf("got id=%q model=%q ok=%v", id, model, ok)
	}
	if u.InputTokens != 5 || u.OutputTokens != 8 || u.CacheCreationTokens != 15955 || u.CacheReadTokens != 16204 {
		t.Fatalf("usage = %+v", u)
	}
}

// TestParseFinalResultUsage — scans the tail for the result event and pulls
// its usage block. Mirrors the run-output format we persist.
func TestParseFinalResultUsage(t *testing.T) {
	output := `{"type":"system","subtype":"init"}
{"type":"assistant","message":{"id":"m1","usage":{"input_tokens":5}}}
{"type":"result","subtype":"success","total_cost_usd":1.25,"usage":{"input_tokens":38,"output_tokens":9942,"cache_creation_input_tokens":58391,"cache_read_input_tokens":1278022}}
`
	u, ok := parseFinalResultUsage(output)
	if !ok {
		t.Fatal("parseFinalResultUsage = !ok")
	}
	if u.InputTokens != 38 || u.OutputTokens != 9942 || u.CacheCreationTokens != 58391 || u.CacheReadTokens != 1278022 {
		t.Fatalf("result usage = %+v", u)
	}
}
