package task

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// parseFinalResultUsage scans the tail of a run's combined output for the
// terminal `type:"result"` event and returns the usage block from it. Used
// for pricing calibration — the result event carries the cumulative usage
// the billing engine actually charged on.
func parseFinalResultUsage(output string) (Usage, bool) {
	if output == "" {
		return Usage{}, false
	}
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0 && i >= len(lines)-20; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		t, _ := event["type"].(string)
		if t != "result" {
			continue
		}
		usage, _ := event["usage"].(map[string]any)
		if usage == nil {
			return Usage{}, false
		}
		return Usage{
			InputTokens:         intFromAny(usage["input_tokens"]),
			OutputTokens:        intFromAny(usage["output_tokens"]),
			CacheCreationTokens: intFromAny(usage["cache_creation_input_tokens"]),
			CacheReadTokens:     intFromAny(usage["cache_read_input_tokens"]),
		}, true
	}
	return Usage{}, false
}

// Usage holds the token counts we bill on. Fields mirror the stream-json
// message.usage shape emitted by Claude Code.
type Usage struct {
	InputTokens         int
	OutputTokens        int
	CacheCreationTokens int
	CacheReadTokens     int
}

// Add returns the sum of two Usage values — handy for aggregating per-message
// usage into a run total.
func (u Usage) Add(o Usage) Usage {
	return Usage{
		InputTokens:         u.InputTokens + o.InputTokens,
		OutputTokens:        u.OutputTokens + o.OutputTokens,
		CacheCreationTokens: u.CacheCreationTokens + o.CacheCreationTokens,
		CacheReadTokens:     u.CacheReadTokens + o.CacheReadTokens,
	}
}

// ModelRates holds the per-million-token rates for a model family. Values are
// the list-price defaults; real billing is calibrated per model by
// Store.CalibrateModelPricing against the authoritative total_cost_usd
// emitted by Claude Code in the terminal result event.
type ModelRates struct {
	InputPerMTok      float64
	OutputPerMTok     float64
	CacheWritePerMTok float64
	CacheReadPerMTok  float64
}

// DefaultRates returns the list-price rates for the given model id. Used as
// the cold-start estimate before the pricing multiplier has been calibrated
// from a real run. Substring match — picks the Claude family by keyword.
func DefaultRates(model string) ModelRates {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "opus"):
		return ModelRates{InputPerMTok: 15, OutputPerMTok: 75, CacheWritePerMTok: 18.75, CacheReadPerMTok: 1.5}
	case strings.Contains(m, "sonnet"):
		return ModelRates{InputPerMTok: 3, OutputPerMTok: 15, CacheWritePerMTok: 3.75, CacheReadPerMTok: 0.3}
	case strings.Contains(m, "haiku"):
		return ModelRates{InputPerMTok: 1, OutputPerMTok: 5, CacheWritePerMTok: 1.25, CacheReadPerMTok: 0.1}
	}
	return ModelRates{InputPerMTok: 15, OutputPerMTok: 75, CacheWritePerMTok: 18.75, CacheReadPerMTok: 1.5}
}

// ComputeCost applies rates to usage. Result is USD for this usage block.
func ComputeCost(r ModelRates, u Usage) float64 {
	return float64(u.InputTokens)*r.InputPerMTok/1_000_000 +
		float64(u.OutputTokens)*r.OutputPerMTok/1_000_000 +
		float64(u.CacheCreationTokens)*r.CacheWritePerMTok/1_000_000 +
		float64(u.CacheReadTokens)*r.CacheReadPerMTok/1_000_000
}

// EstimateCost is the live-run estimator: default rates × the model's
// calibration multiplier (defaults to 1.0 before any calibration).
func EstimateCost(model string, multiplier float64, u Usage) float64 {
	if multiplier <= 0 {
		multiplier = 1.0
	}
	return ComputeCost(DefaultRates(model), u) * multiplier
}

// ModelPricing is what we persist per model — a single scalar that scales
// the default rate table to match the actual total_cost_usd reported by the
// terminal result event. EMA-updated on every completed run.
type ModelPricing struct {
	Model      string
	Multiplier float64
	Samples    int
	UpdatedAt  time.Time
}

// calibrationAlpha is the EMA weight applied to a new sample. 0.3 gives the
// latest run moderate influence without losing history — the multiplier
// settles within ~10 runs if pricing is stable.
const calibrationAlpha = 0.3

// initPricingSchema creates the model_pricing table. Called from Store.init().
func (s *Store) initPricingSchema() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS model_pricing (
		model TEXT PRIMARY KEY,
		multiplier REAL NOT NULL,
		samples INTEGER NOT NULL DEFAULT 0,
		updated_at INTEGER NOT NULL
	)`)
	return err
}

// GetModelMultiplier returns the calibration multiplier for model, or 1.0 if
// we have never calibrated it. Safe to call on any code path that needs a
// live-cost estimate.
func (s *Store) GetModelMultiplier(model string) float64 {
	if model == "" {
		return 1.0
	}
	var m float64
	err := s.db.QueryRow(`SELECT multiplier FROM model_pricing WHERE model = ?`, model).Scan(&m)
	if err == sql.ErrNoRows || err != nil || m <= 0 {
		return 1.0
	}
	return m
}

// GetModelPricing returns the full pricing row, or a zero-value row if
// absent. Used by the dashboard + tests to inspect calibration state.
func (s *Store) GetModelPricing(model string) (ModelPricing, error) {
	var p ModelPricing
	var ts int64
	err := s.db.QueryRow(`SELECT model, multiplier, samples, updated_at FROM model_pricing WHERE model = ?`, model).
		Scan(&p.Model, &p.Multiplier, &p.Samples, &ts)
	if err == sql.ErrNoRows {
		return ModelPricing{Model: model, Multiplier: 1.0}, nil
	}
	if err != nil {
		return ModelPricing{}, err
	}
	p.UpdatedAt = time.Unix(ts, 0).UTC()
	return p, nil
}

// CalibrateModelPricing blends a new observation into the stored multiplier.
// reportedCost must be the authoritative total_cost_usd from the result event;
// u must be the matching final usage. Called once per completed run.
//
// The new sample is reported / predicted_at_default_rates. We EMA-blend into
// the prior multiplier so a single outlier does not jerk the estimate around.
func (s *Store) CalibrateModelPricing(model string, reportedCost float64, u Usage) error {
	if model == "" || reportedCost <= 0 {
		return nil
	}
	predicted := ComputeCost(DefaultRates(model), u)
	if predicted <= 0 {
		return nil
	}
	sample := reportedCost / predicted

	prior, err := s.GetModelPricing(model)
	if err != nil {
		return err
	}
	var next float64
	if prior.Samples == 0 {
		next = sample
	} else {
		next = prior.Multiplier*(1-calibrationAlpha) + sample*calibrationAlpha
	}
	now := time.Now().UTC().Unix()
	_, err = s.db.Exec(`INSERT INTO model_pricing (model, multiplier, samples, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(model) DO UPDATE SET
			multiplier = excluded.multiplier,
			samples = model_pricing.samples + 1,
			updated_at = excluded.updated_at`,
		model, next, 1, now)
	return err
}

// parseAssistantUsage reads (messageID, model, usage) from the parsed event
// map of a stream-json assistant line. Returns ok=false if the event is not
// an assistant event with usage present. The caller dedupes by messageID
// because Claude Code may emit the same assistant message more than once as
// it streams — last-write-wins matches the terminal result event.
func parseAssistantUsage(event map[string]any) (id, model string, u Usage, ok bool) {
	msg, _ := event["message"].(map[string]any)
	if msg == nil {
		return "", "", Usage{}, false
	}
	id, _ = msg["id"].(string)
	model, _ = msg["model"].(string)
	usage, _ := msg["usage"].(map[string]any)
	if usage == nil {
		return id, model, Usage{}, false
	}
	u.InputTokens = intFromAny(usage["input_tokens"])
	u.OutputTokens = intFromAny(usage["output_tokens"])
	u.CacheCreationTokens = intFromAny(usage["cache_creation_input_tokens"])
	u.CacheReadTokens = intFromAny(usage["cache_read_input_tokens"])
	return id, model, u, true
}

func intFromAny(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return 0
}

