package execution

// ModelInfo describes a model's provider and maximum context window.
type ModelInfo struct {
	Provider          string
	ContextWindowSize int
}

// modelRegistry maps known model IDs to their provider and context window.
// Seeded from the default_model catalog values plus common Anthropic/Google/OpenAI entries.
var modelRegistry = map[string]ModelInfo{
	// Anthropic
	"claude-opus-4-7":         {Provider: "anthropic", ContextWindowSize: 1_000_000},
	"claude-sonnet-4-6":       {Provider: "anthropic", ContextWindowSize: 200_000},
	"claude-haiku-4-5":        {Provider: "anthropic", ContextWindowSize: 200_000},
	"claude-haiku-4-5-20251001": {Provider: "anthropic", ContextWindowSize: 200_000},

	// Google — Gemini 3 series (in catalog)
	"gemini-3-pro":   {Provider: "google", ContextWindowSize: 2_097_152},
	"gemini-3-flash": {Provider: "google", ContextWindowSize: 1_048_576},
	"gemini-3-ultra": {Provider: "google", ContextWindowSize: 2_097_152},

	// Google — Gemini 2.5 series
	"gemini-2.5-pro":   {Provider: "google", ContextWindowSize: 2_097_152},
	"gemini-2.5-flash": {Provider: "google", ContextWindowSize: 1_048_576},

	// Google — Gemini 2.0 series
	"gemini-2.0-flash":          {Provider: "google", ContextWindowSize: 1_048_576},
	"gemini-2.0-flash-exp":      {Provider: "google", ContextWindowSize: 1_048_576},
	"gemini-2.0-pro-exp-02-05":  {Provider: "google", ContextWindowSize: 1_048_576},

	// OpenAI
	"gpt-4o": {Provider: "openai", ContextWindowSize: 128_000},
}

// LookupModel returns the ModelInfo for a given model ID.
// Returns {Provider:"unknown", ContextWindowSize:200_000} for unrecognised models.
func LookupModel(modelID string) ModelInfo {
	if info, ok := modelRegistry[modelID]; ok {
		return info
	}
	return ModelInfo{Provider: "unknown", ContextWindowSize: 200_000}
}
