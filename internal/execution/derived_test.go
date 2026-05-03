package execution

import (
	"math"
	"testing"
)

// TestComputeDerived_values exercises every ratio formula with realistic
// Sonnet-shaped inputs. Hand-computed expected values keep the test
// independent of the implementation under review.
func TestComputeDerived_values(t *testing.T) {
	in := DerivedInput{
		InputTokens:       2_000,
		OutputTokens:      1_000,
		CacheReadTokens:   8_000,
		CacheCreateTokens: 500,
		CostUSD:           0.05,
		Turns:             4,
		Actions:           20,
		Model:             "claude-sonnet-4-6", // 200_000 ctx, sonnet rates
	}
	got := ComputeDerived(in)

	// CacheHitRate = 8000 / (2000 + 8000) * 100 = 80
	if !approx(got.CacheHitRatePct, 80) {
		t.Errorf("CacheHitRatePct = %v, want 80", got.CacheHitRatePct)
	}
	// ContextWindowPct = (2000 + 1000 + 8000 + 500) / 200000 * 100 = 5.75
	if !approx(got.ContextWindowPct, 5.75) {
		t.Errorf("ContextWindowPct = %v, want 5.75", got.ContextWindowPct)
	}
	// CostPerTurn = 0.05 / 4 = 0.0125
	if !approx(got.CostPerTurn, 0.0125) {
		t.Errorf("CostPerTurn = %v, want 0.0125", got.CostPerTurn)
	}
	// CostPerAction = 0.05 / 20 = 0.0025
	if !approx(got.CostPerAction, 0.0025) {
		t.Errorf("CostPerAction = %v, want 0.0025", got.CostPerAction)
	}
	// TokensPerTurn = (2000 + 1000) / 4 = 750
	if !approx(got.TokensPerTurn, 750) {
		t.Errorf("TokensPerTurn = %v, want 750", got.TokensPerTurn)
	}
	// CacheSavings = 8000 * (3 - 0.3) / 1_000_000 = 0.0216
	if !approx(got.CacheSavingsUSD, 0.0216) {
		t.Errorf("CacheSavingsUSD = %v, want 0.0216", got.CacheSavingsUSD)
	}
}

// TestComputeDerived_zero — every denominator is zero; no NaN, no Inf.
func TestComputeDerived_zero(t *testing.T) {
	got := ComputeDerived(DerivedInput{})
	for _, f := range []float64{
		got.CacheHitRatePct, got.ContextWindowPct,
		got.CostPerTurn, got.CostPerAction, got.TokensPerTurn,
		got.CacheSavingsUSD,
	} {
		if math.IsNaN(f) || math.IsInf(f, 0) || f != 0 {
			t.Errorf("expected 0 for empty input, got %v", f)
		}
	}
}

// TestComputeDerived_unknownModel — a model id outside the registry falls
// back to the default 200k context window so ContextWindowPct still computes.
// CacheSavings uses the unknown-model fallback rates (opus list price).
func TestComputeDerived_unknownModel(t *testing.T) {
	got := ComputeDerived(DerivedInput{
		InputTokens:     1_000,
		OutputTokens:    1_000,
		CacheReadTokens: 1_000,
		Model:           "totally-fake-model",
	})
	if got.ContextWindowPct == 0 {
		t.Errorf("expected non-zero ContextWindowPct on unknown model fallback, got 0")
	}
	// fallback rates = opus: input 15, cache_read 1.5
	// savings = 1000 * (15 - 1.5) / 1_000_000 = 0.0135
	if !approx(got.CacheSavingsUSD, 0.0135) {
		t.Errorf("CacheSavingsUSD = %v, want 0.0135 (opus fallback rates)", got.CacheSavingsUSD)
	}
}

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
