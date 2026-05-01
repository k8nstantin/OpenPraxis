package settings

import (
	"encoding/json"
	"errors"
	"fmt"
)

// KnobType identifies the declared value shape for a knob. The Store layer
// stores values as opaque JSON-encoded strings; the catalog owns type/range
// validation on the way in and decode on the way out.
type KnobType string

const (
	KnobInt         KnobType = "int"
	KnobFloat       KnobType = "float"
	KnobString      KnobType = "string"
	KnobEnum        KnobType = "enum"
	KnobMultiselect KnobType = "multiselect"
)

// KnobDef describes one knob in the v1 catalog. Slider fields are optional
// and used only by numeric knobs for UI hinting; EnumValues constrains enums.
// Default is the system-tier value returned when no explicit entry exists at
// task / manifest / product scope.
type KnobDef struct {
	Key         string      `json:"key"`
	Type        KnobType    `json:"type"`
	SliderMin   *float64    `json:"slider_min,omitempty"`
	SliderMax   *float64    `json:"slider_max,omitempty"`
	SliderStep  *float64    `json:"slider_step,omitempty"`
	EnumValues  []string    `json:"enum_values,omitempty"`
	Default     interface{} `json:"default"`
	Description string      `json:"description"`
	Unit        string      `json:"unit,omitempty"`
}

// Sentinel errors surfaced by ValidateValue. Callers should use errors.Is for
// matching. M2's API write layer imports these names — do not rename.
var (
	ErrUnknownKey     = errors.New("settings: unknown key")
	ErrTypeMismatch   = errors.New("settings: value type mismatch")
	ErrEnumOutOfRange = errors.New("settings: enum value not in allowed list")
)

// Catalog returns all v1 knobs in stable order. Order matches the issue #34
// v1 table so UIs that render in catalog order stay deterministic.
func Catalog() []KnobDef {
	return []KnobDef{
		{Key: "max_parallel", Type: KnobInt, SliderMin: f(1), SliderMax: f(100), SliderStep: f(1), Default: 3, Description: "Max tasks this product runs concurrently"},
		{Key: "max_turns", Type: KnobInt, SliderMin: f(10), SliderMax: f(10000), SliderStep: f(10), Default: 50, Description: "Agent turn ceiling per task run"},
		{Key: "max_cost_usd", Type: KnobFloat, SliderMin: f(0.01), SliderMax: f(1000), SliderStep: f(0.01), Default: 10.0, Unit: "USD", Description: "Max cost per single task run"},
		{Key: "daily_budget_usd", Type: KnobFloat, SliderMin: f(1), SliderMax: f(10000), SliderStep: f(1), Default: 100.0, Unit: "USD", Description: "Per-product daily budget; clamped by visceral rule"},
		{Key: "timeout_minutes", Type: KnobInt, SliderMin: f(1), SliderMax: f(1440), SliderStep: f(1), Default: 30, Unit: "minutes", Description: "Max wall time per task run"},
		{Key: "temperature", Type: KnobFloat, SliderMin: f(0), SliderMax: f(2), SliderStep: f(0.05), Default: 0.2, Description: "LLM sampling temperature"},
		{Key: "reasoning_effort", Type: KnobEnum, EnumValues: []string{"minimal", "low", "medium", "high"}, Default: "medium", Description: "Thinking budget for reasoning models"},
		{Key: "default_agent", Type: KnobEnum, EnumValues: []string{"claude-code", "gemini-cli", "codex", "cursor", "windsurf"}, Default: "claude-code", Description: "Agent runtime"},
		{Key: "default_model", Type: KnobEnum, EnumValues: []string{
		        "",                   // empty = let the agent runtime choose its own default
		        "claude-opus-4-7",    // Opus 4.7 — highest capability, 1M context
		        "claude-sonnet-4-6",  // Sonnet 4.6 — balanced
		        "claude-haiku-4-5",   // Haiku 4.5 — fast / cheap
		        "gemini-3-pro",
		        "gemini-3-flash",
		        "gemini-3-ultra",
		        "gemini-2.5-pro",

		        "gemini-2.5-flash",
		        "gemini-2.0-pro-exp-02-05",
		        "gemini-2.0-flash-exp",
		}, Default: "", Description: "Model ID passed to the agent as --model. Empty = agent default."},		{Key: "retry_on_failure", Type: KnobInt, SliderMin: f(0), SliderMax: f(10), SliderStep: f(1), Default: 0, Description: "Auto-retry count"},
		{Key: "approval_mode", Type: KnobEnum, EnumValues: []string{"auto", "manual", "on-failure"}, Default: "auto", Description: "Codex approval mode"},
		// Prompt-context knobs — resolve via the same task → manifest →
		// product → system inheritance chain as every other knob. Defaults
		// are heuristic (no empirical measurements) and will be calibrated
		// from prompt_build_stats once the instrumentation ships; see
		// idea 019db6ba-ba0 and memory 019db6bb-a4e.
		{Key: "prompt_max_comment_chars", Type: KnobInt, SliderMin: f(500), SliderMax: f(10000), SliderStep: f(500), Default: 2000, Description: "Max chars per comment when injected into the task-thread context block; longer comments are truncated with a pointer to the full text."},
		{Key: "prompt_max_context_pct", Type: KnobFloat, SliderMin: f(0.1), SliderMax: f(0.9), SliderStep: f(0.05), Default: 0.4, Description: "Max fraction of the resolved model's context window that the prior-runs + other-comments block may consume; older comments drop first when over budget."},
		{Key: "compliance_checks_enabled", Type: KnobEnum, EnumValues: []string{"true", "false"}, Default: "true", Description: "When true, PostToolUse hooks run visceral-compliance + manifest-delusion embedding checks per tool call. Disable for high-throughput agents — each tool call triggers up to N × M embedding calls (N rules, M open manifests) which can peg serve CPU. Defaults true; set false at product or task scope to opt out."},
		// RC/M5 operator knobs — cadence, restart behavior, branch prefix,
		// worktree base dir. All inherit via task → manifest → product →
		// system like every other catalog entry.
		{Key: "scheduler_tick_seconds", Type: KnobInt, SliderMin: f(2), SliderMax: f(300), SliderStep: f(1), Default: 10, Unit: "seconds", Description: "How often the scheduler wakes to check for due tasks. Lower = more responsive, more DB load."},
		{Key: "host_sampler_tick_seconds", Type: KnobInt, SliderMin: f(1), SliderMax: f(300), SliderStep: f(1), Default: 5, Unit: "seconds", Description: "Interval (seconds) for the per-run + system host samplers."},
		{Key: "on_restart_behavior", Type: KnobEnum, EnumValues: []string{"restart", "stop", "fail"}, Default: "stop", Description: "What to do with tasks left in running state when serve restarts. stop=mark failed with recovery hint; restart=re-fire immediately; fail=mark failed with no auto-recovery."},
		{Key: "branch_prefix", Type: KnobString, Default: "openpraxis", Description: "Prefix used in the agent's <git_workflow> branch name: <prefix>/<task_id>. Operators can set per-product for QA/staging branches."},
		{Key: "worktree_base_dir", Type: KnobString, Default: ".openpraxis-work", Description: "Directory under the repo root where per-task git worktrees are materialised. Absolute paths are supported for out-of-tree worktrees."},
		{Key: "allowed_tools", Type: KnobMultiselect, Default: []string{
			"Bash", "Read", "Write", "Edit", "Glob", "Grep",
			// MCP tools the runner's prompt template instructs agents to call.
			// Must stay in sync with internal/task/runner.go:defaultAllowedTools.
			// Missing any of these silently denies the agent — it can't post its
			// closing execution_review, load visceral rules, or read settings.
			"mcp__openpraxis__memory_store",
			"mcp__openpraxis__memory_search",
			"mcp__openpraxis__memory_recall",
			"mcp__openpraxis__visceral_rules",
			"mcp__openpraxis__visceral_confirm",
			"mcp__openpraxis__manifest_get",
			"mcp__openpraxis__conversation_save",
			"mcp__openpraxis__settings_get",
			"mcp__openpraxis__settings_resolve",
			"mcp__openpraxis__settings_catalog",
			"mcp__openpraxis__comment_add",
		}, Description: "Tool allowlist for agent. Includes MCP tools the runner prompts reference; removing any breaks the corresponding closing step."},
		// Frontend dashboard v2 cutover flags. Resolve at system scope; the
		// existing inheritance chain still applies if an operator wants to
		// override per-product (e.g. a QA product running on the new UI
		// before the rest of the org cuts over).
		{Key: "frontend_dashboard_v2", Type: KnobEnum, EnumValues: []string{"true", "false"}, Default: "false", Description: "When true, the legacy /products nav link redirects to /dashboard/products (React v2). Defaults false until parity is verified across all migrated tabs."},
		{Key: "frontend_dev_mode", Type: KnobEnum, EnumValues: []string{"true", "false"}, Default: "false", Description: "When true, dashboard v2 is allowed to talk to a Vite HMR proxy. Do NOT enable in production — prod always serves the embedded build."},
		// Per-tab cutover knobs — flipped on as each tab's React 2 manifest
		// reaches parity. Resolves at system scope; the inheritance chain
		// allows a per-product override (QA product on the new UI before the
		// rest of the org cuts over). The dashboard's lazy route registry
		// reads these at boot via /api/settings/resolve and decides whether
		// to redirect the legacy URL to /dashboard/<tab>.
		{Key: "frontend_dashboard_v2_products", Type: KnobEnum, EnumValues: []string{"true", "false"}, Default: "false", Description: "Per-tab cutover — when true, legacy /products redirects to /dashboard/products."},
		{Key: "frontend_dashboard_v2_manifests", Type: KnobEnum, EnumValues: []string{"true", "false"}, Default: "false", Description: "Per-tab cutover — when true, legacy /manifests redirects to /dashboard/manifests."},
		{Key: "frontend_dashboard_v2_tasks", Type: KnobEnum, EnumValues: []string{"true", "false"}, Default: "false", Description: "Per-tab cutover — when true, legacy /tasks redirects to /dashboard/tasks."},
		{Key: "frontend_dashboard_v2_memories", Type: KnobEnum, EnumValues: []string{"true", "false"}, Default: "false", Description: "Per-tab cutover — when true, legacy /memories redirects to /dashboard/memories."},
		{Key: "frontend_dashboard_v2_conversations", Type: KnobEnum, EnumValues: []string{"true", "false"}, Default: "false", Description: "Per-tab cutover — when true, legacy /conversations redirects to /dashboard/conversations."},
		{Key: "frontend_dashboard_v2_settings", Type: KnobEnum, EnumValues: []string{"true", "false"}, Default: "false", Description: "Per-tab cutover — when true, legacy /settings redirects to /dashboard/settings."},
		{Key: "frontend_dashboard_v2_compliance", Type: KnobEnum, EnumValues: []string{"true", "false"}, Default: "false", Description: "Per-tab cutover — when true, legacy /compliance redirects to /dashboard/compliance."},
		{Key: "frontend_dashboard_v2_overview", Type: KnobEnum, EnumValues: []string{"true", "false"}, Default: "false", Description: "Per-tab cutover — when true, legacy /overview redirects to /dashboard/overview."},
		// Comment attachment knobs (UB-2). Default 10 MiB cap matches the
		// M1 manifest spec; allowlist mirrors the canonical default in
		// internal/comments/attachments.go (image/*, text/*, plus a small
		// set of structured document mimes). Override per product /
		// manifest / task to widen or narrow the policy without a code
		// change.
		{Key: "comment_attachment_max_mb", Type: KnobInt, SliderMin: f(1), SliderMax: f(500), SliderStep: f(1), Default: 10, Unit: "MB", Description: "Max comment attachment size in megabytes. Uploads exceeding this cap are rejected with 413."},
		{Key: "comment_attachment_allowed_mimes", Type: KnobString, Default: "image/,text/,application/pdf,application/json,application/xml,application/zip", Description: "Comma-separated mime allowlist for comment attachments. Entries ending with `/` match by prefix (e.g. `image/` matches `image/png`); others match exactly."},
	}
}

// KnobByKey returns the catalog entry for a key and whether it was found.
// Linear scan over the 12-item catalog is cheap and avoids init-order traps.
func KnobByKey(key string) (KnobDef, bool) {
	for _, k := range Catalog() {
		if k.Key == key {
			return k, true
		}
	}
	return KnobDef{}, false
}

// SystemDefault returns the built-in system-tier default for a key.
// Equivalent to KnobByKey(key).Default with a presence flag.
func SystemDefault(key string) (interface{}, bool) {
	k, ok := KnobByKey(key)
	if !ok {
		return nil, false
	}
	return k.Default, true
}

// ValidateValue checks a JSON-encoded value against the knob's declared type.
// Returns (warnings, hardError). Warnings are soft — callers should surface
// them to the user but never block the save (the user explicitly chose no
// hard caps on slider values). Hard errors (unknown key, type mismatch,
// unknown enum member, bad JSON) always block.
//
// Visceral-rule clamping (e.g. daily_budget_usd ≤ 100) is NOT applied here;
// that belongs in M2's API write layer so catalog-level validation stays
// purely type/shape-focused.
func ValidateValue(key, jsonValue string) (warnings []string, hardError error) {
	knob, ok := KnobByKey(key)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownKey, key)
	}

	var decoded interface{}
	if err := json.Unmarshal([]byte(jsonValue), &decoded); err != nil {
		return nil, fmt.Errorf("%w: %q is not valid JSON: %v", ErrTypeMismatch, jsonValue, err)
	}

	switch knob.Type {
	case KnobInt:
		return validateInt(knob, decoded)
	case KnobFloat:
		return validateFloat(knob, decoded)
	case KnobString:
		return validateString(knob, decoded)
	case KnobEnum:
		return validateEnum(knob, decoded)
	case KnobMultiselect:
		return validateMultiselect(knob, decoded)
	default:
		return nil, fmt.Errorf("%w: knob %q has unsupported type %q", ErrTypeMismatch, key, knob.Type)
	}
}

func validateInt(knob KnobDef, decoded interface{}) ([]string, error) {
	n, ok := decoded.(float64)
	if !ok {
		return nil, fmt.Errorf("%w: %q expects int, got %T", ErrTypeMismatch, knob.Key, decoded)
	}
	if n != float64(int64(n)) {
		return nil, fmt.Errorf("%w: %q expects whole number, got %v", ErrTypeMismatch, knob.Key, n)
	}
	return sliderWarnings(knob, n), nil
}

func validateFloat(knob KnobDef, decoded interface{}) ([]string, error) {
	n, ok := decoded.(float64)
	if !ok {
		return nil, fmt.Errorf("%w: %q expects float, got %T", ErrTypeMismatch, knob.Key, decoded)
	}
	return sliderWarnings(knob, n), nil
}

func validateString(knob KnobDef, decoded interface{}) ([]string, error) {
	if _, ok := decoded.(string); !ok {
		return nil, fmt.Errorf("%w: %q expects string, got %T", ErrTypeMismatch, knob.Key, decoded)
	}
	return nil, nil
}

func validateEnum(knob KnobDef, decoded interface{}) ([]string, error) {
	s, ok := decoded.(string)
	if !ok {
		return nil, fmt.Errorf("%w: %q expects string, got %T", ErrTypeMismatch, knob.Key, decoded)
	}
	for _, allowed := range knob.EnumValues {
		if s == allowed {
			return nil, nil
		}
	}
	return nil, fmt.Errorf("%w: %q not in %v (key %q)", ErrEnumOutOfRange, s, knob.EnumValues, knob.Key)
}

func validateMultiselect(knob KnobDef, decoded interface{}) ([]string, error) {
	arr, ok := decoded.([]interface{})
	if !ok {
		return nil, fmt.Errorf("%w: %q expects array, got %T", ErrTypeMismatch, knob.Key, decoded)
	}
	for i, item := range arr {
		if _, ok := item.(string); !ok {
			return nil, fmt.Errorf("%w: %q[%d] expects string, got %T", ErrTypeMismatch, knob.Key, i, item)
		}
	}
	return nil, nil
}

func sliderWarnings(knob KnobDef, n float64) []string {
	var warnings []string
	if knob.SliderMax != nil && n > *knob.SliderMax {
		warnings = append(warnings, fmt.Sprintf("%s=%v exceeds typical slider max of %v", knob.Key, n, *knob.SliderMax))
	}
	if knob.SliderMin != nil && n < *knob.SliderMin {
		warnings = append(warnings, fmt.Sprintf("%s=%v below typical slider min of %v", knob.Key, n, *knob.SliderMin))
	}
	return warnings
}

func f(v float64) *float64 { return &v }
