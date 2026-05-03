package execution

import "strings"

// DerivedInput is the raw run-counter shape ComputeDerived needs to compute
// the per-run ratio fields stored on execution_log.
type DerivedInput struct {
	InputTokens       int
	OutputTokens      int
	CacheReadTokens   int
	CacheCreateTokens int
	CostUSD           float64
	Turns             int
	Actions           int
	Model             string
}

// DerivedOutput is the set of computed fields ComputeDerived returns. The
// runner UPDATEs each one onto the execution_log row at completion.
type DerivedOutput struct {
	CacheHitRatePct  float64
	ContextWindowPct float64
	CostPerTurn      float64
	CostPerAction    float64
	TokensPerTurn    float64
	CacheSavingsUSD  float64
}

// ComputeDerived computes ratio fields from raw counters. Zero-input safe:
// all outputs default to 0 when the relevant denominator is zero. Pure
// function — no I/O, no logging — so it is trivial to test exhaustively
// (TestComputeDerived_values).
func ComputeDerived(in DerivedInput) DerivedOutput {
	var out DerivedOutput

	totalInput := in.InputTokens + in.CacheReadTokens
	if totalInput > 0 {
		out.CacheHitRatePct = float64(in.CacheReadTokens) / float64(totalInput) * 100
	}

	if ctxSize := LookupModel(in.Model).ContextWindowSize; ctxSize > 0 {
		total := in.InputTokens + in.OutputTokens + in.CacheReadTokens + in.CacheCreateTokens
		out.ContextWindowPct = float64(total) / float64(ctxSize) * 100
	}

	if in.Turns > 0 {
		out.CostPerTurn = in.CostUSD / float64(in.Turns)
		out.TokensPerTurn = float64(in.InputTokens+in.OutputTokens) / float64(in.Turns)
	}

	if in.Actions > 0 {
		out.CostPerAction = in.CostUSD / float64(in.Actions)
	}

	rates := defaultRates(in.Model)
	out.CacheSavingsUSD = float64(in.CacheReadTokens) *
		(rates.InputPerMTok - rates.CacheReadPerMTok) / 1_000_000

	return out
}

// modelRates is a private mirror of internal/task.ModelRates. Replicated
// here (instead of imported) to avoid an import cycle: internal/task already
// depends on internal/execution, so the dependency cannot run the other way.
type modelRates struct {
	InputPerMTok      float64
	CacheReadPerMTok  float64
	OutputPerMTok     float64
	CacheWritePerMTok float64
}

// defaultRates returns the list-price rates for the given model id. Substring
// match on the model family — same algorithm as task.DefaultRates so that
// dual-write rows round-trip consistent cache_savings_usd values.
func defaultRates(model string) modelRates {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "opus"):
		return modelRates{InputPerMTok: 15, CacheReadPerMTok: 1.5, OutputPerMTok: 75, CacheWritePerMTok: 18.75}
	case strings.Contains(m, "sonnet"):
		return modelRates{InputPerMTok: 3, CacheReadPerMTok: 0.3, OutputPerMTok: 15, CacheWritePerMTok: 3.75}
	case strings.Contains(m, "haiku"):
		return modelRates{InputPerMTok: 1, CacheReadPerMTok: 0.1, OutputPerMTok: 5, CacheWritePerMTok: 1.25}
	}
	return modelRates{InputPerMTok: 15, CacheReadPerMTok: 1.5, OutputPerMTok: 75, CacheWritePerMTok: 18.75}
}
