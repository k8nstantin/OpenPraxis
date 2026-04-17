package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/k8nstantin/OpenPraxis/internal/settings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// VisceralRuleLoader returns the text of every active visceral rule. The MCP
// and HTTP layers call this before writing settings that carry a
// visceral-backed cap (e.g. daily_budget_usd is capped by rule #8). The
// indirection keeps the core settings handlers testable without an
// index/memory store.
type VisceralRuleLoader func(ctx context.Context) ([]string, error)

// visceralBudgetRe extracts a numeric ceiling from a rule text like
// "daily budget = $100" or "cap is 100 USD". First capture group is the value.
var visceralBudgetRe = regexp.MustCompile(`\$?([0-9]+(?:\.[0-9]+)?)`)

// VisceralCapFor reports the numeric ceiling enforced by active visceral rules
// for a given knob key, if any. v1 hardcodes one mapping: daily_budget_usd →
// the first rule whose text contains "daily budget". Extend here as new caps
// are added. Returns (cap, true) when a cap applies; (0, false) otherwise.
func VisceralCapFor(key string, rules []string) (float64, bool) {
	switch key {
	case "daily_budget_usd":
		for _, rule := range rules {
			if !strings.Contains(strings.ToLower(rule), "daily budget") {
				continue
			}
			m := visceralBudgetRe.FindStringSubmatch(rule)
			if len(m) < 2 {
				continue
			}
			v, err := strconv.ParseFloat(m[1], 64)
			if err != nil {
				continue
			}
			return v, true
		}
	}
	return 0, false
}

func (s *Server) registerSettingsTools() {
	s.mcp.AddTool(
		mcplib.NewTool("settings_catalog",
			mcplib.WithDescription("Return the catalog of v1 knob definitions (type, range, default, enum values). UIs use this to render the settings grid."),
		),
		s.handleSettingsCatalog,
	)

	s.mcp.AddTool(
		mcplib.NewTool("settings_get",
			mcplib.WithDescription("Return explicit settings at a single scope (product, manifest, or task). No inheritance walk — see settings_resolve for that."),
			mcplib.WithString("scope_type", mcplib.Required(), mcplib.Description("Scope tier: 'product', 'manifest', or 'task'")),
			mcplib.WithString("scope_id", mcplib.Required(), mcplib.Description("Scope entity id")),
		),
		s.handleSettingsGet,
	)

	s.mcp.AddTool(
		mcplib.NewTool("settings_set",
			mcplib.WithDescription("Write an explicit setting at a single scope. Value must be a JSON-encoded string (e.g. '10' for int, '\"auto\"' for enum). Returns soft warnings but does not block the save; visceral-rule caps (e.g. daily_budget_usd ≤ $100) and catalog type mismatches hard-block."),
			mcplib.WithString("scope_type", mcplib.Required(), mcplib.Description("Scope tier: 'product', 'manifest', or 'task'")),
			mcplib.WithString("scope_id", mcplib.Required(), mcplib.Description("Scope entity id")),
			mcplib.WithString("key", mcplib.Required(), mcplib.Description("Knob key (must be in settings_catalog)")),
			mcplib.WithString("value", mcplib.Required(), mcplib.Description("JSON-encoded value matching the knob's type")),
		),
		s.handleSettingsSet,
	)

	s.mcp.AddTool(
		mcplib.NewTool("settings_resolve",
			mcplib.WithDescription("Return the effective value for every knob at task scope, walking task → manifest → product → system. Each entry includes provenance (source tier + source id)."),
			mcplib.WithString("task_id", mcplib.Required(), mcplib.Description("Task id to resolve for")),
		),
		s.handleSettingsResolve,
	)
}

// -------- handlers -----------------------------------------------------------

func (s *Server) handleSettingsCatalog(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	return jsonOrError(DoSettingsCatalog())
}

func (s *Server) handleSettingsGet(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	out, err := DoSettingsGet(ctx, s.node.SettingsStore, argStr(a, "scope_type"), argStr(a, "scope_id"))
	if err != nil {
		return errResult("settings_get: %v", err), nil
	}
	return jsonOrError(out)
}

func (s *Server) handleSettingsSet(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	out, err := DoSettingsSet(ctx, s.node.SettingsStore, s.LoadActiveVisceralRules,
		argStr(a, "scope_type"), argStr(a, "scope_id"),
		argStr(a, "key"), argStr(a, "value"),
		mcpSetAuthor(ctx))
	if err != nil {
		return errResult("settings_set: %v", err), nil
	}
	return jsonOrError(out)
}

func (s *Server) handleSettingsResolve(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	out, err := DoSettingsResolve(ctx, s.node.SettingsResolver, argStr(a, "task_id"))
	if err != nil {
		return errResult("settings_resolve: %v", err), nil
	}
	return jsonOrError(out)
}

// -------- core (testable) ----------------------------------------------------

// CatalogOut is the wire shape of DoSettingsCatalog. Exported so HTTP handlers
// can name the type when serializing to JSON.
type CatalogOut struct {
	Knobs []settings.KnobDef `json:"knobs"`
}

// DoSettingsCatalog returns the full v1 knob catalog. Pure — safe to call from
// any caller (MCP, HTTP, test).
func DoSettingsCatalog() CatalogOut {
	return CatalogOut{Knobs: settings.Catalog()}
}

// GetOut is the wire shape of DoSettingsGet — explicit entries at a single
// scope, with the scope echoed back so callers can confirm the lookup target.
type GetOut struct {
	ScopeType string           `json:"scope_type"`
	ScopeID   string           `json:"scope_id"`
	Entries   []settings.Entry `json:"entries"`
}

// DoSettingsGet reads the explicit settings written at one scope. No
// inheritance walk — use DoSettingsResolve for that.
func DoSettingsGet(ctx context.Context, store *settings.Store, scopeType, scopeID string) (GetOut, error) {
	if store == nil {
		return GetOut{}, fmt.Errorf("settings store not configured")
	}
	if scopeType == "" || scopeID == "" {
		return GetOut{}, fmt.Errorf("scope_type and scope_id are required")
	}
	st := settings.ScopeType(scopeType)
	if err := ValidateWritableScope(st); err != nil {
		return GetOut{}, err
	}
	entries, err := store.ListScope(ctx, st, scopeID)
	if err != nil {
		return GetOut{}, err
	}
	if entries == nil {
		entries = []settings.Entry{}
	}
	return GetOut{ScopeType: scopeType, ScopeID: scopeID, Entries: entries}, nil
}

// SetOut is the wire shape of DoSettingsSet. OK is true when the write
// succeeded; Warnings are soft (e.g. slider out of range) but do not block.
// Entry is the readback after write so callers can verify persistence;
// Catalog is the matching knob definition for UI hinting.
type SetOut struct {
	OK       bool              `json:"ok"`
	Warnings []string          `json:"warnings,omitempty"`
	Entry    *settings.Entry   `json:"entry,omitempty"`
	Catalog  *settings.KnobDef `json:"catalog,omitempty"`
}

// DoSettingsSet writes an explicit value at one scope after running:
//  1. catalog validation (type/enum/range)
//  2. visceral-rule cap enforcement for capped knobs (e.g. daily_budget_usd)
//
// loadRules may be nil; in that case capped keys are blocked unless the cap
// lookup returns no rule. Author is the caller identity recorded on the row
// (e.g. "mcp:sess-X" or "http:user-Y").
func DoSettingsSet(
	ctx context.Context,
	store *settings.Store,
	loadRules VisceralRuleLoader,
	scopeType, scopeID, key, value, author string,
) (SetOut, error) {
	if store == nil {
		return SetOut{}, fmt.Errorf("settings store not configured")
	}
	if scopeType == "" || scopeID == "" || key == "" || value == "" {
		return SetOut{}, fmt.Errorf("scope_type, scope_id, key, value are required")
	}
	st := settings.ScopeType(scopeType)
	if err := ValidateWritableScope(st); err != nil {
		return SetOut{}, err
	}

	warnings, err := settings.ValidateValue(key, value)
	if err != nil {
		return SetOut{}, err
	}

	// Visceral-rule clamp — MCP/HTTP layer only. Catalog layer intentionally
	// skips this so pure catalog validation stays shape/type focused.
	if HasVisceralCap(key) {
		var rules []string
		if loadRules != nil {
			loaded, lerr := loadRules(ctx)
			if lerr != nil {
				return SetOut{}, fmt.Errorf("load visceral rules: %w", lerr)
			}
			rules = loaded
		}
		if ceiling, ok := VisceralCapFor(key, rules); ok {
			exceeds, err := ValueExceedsCap(value, ceiling)
			if err != nil {
				return SetOut{}, err
			}
			if exceeds {
				return SetOut{}, fmt.Errorf("Visceral rule #8 caps %s at $%v. Raise the rule first via visceral_set.", key, ceiling)
			}
		}
	}

	if err := store.Set(ctx, st, scopeID, key, value, author); err != nil {
		return SetOut{}, err
	}

	entry, err := store.Get(ctx, st, scopeID, key)
	if err != nil {
		return SetOut{}, fmt.Errorf("readback after set: %w", err)
	}
	knob, _ := settings.KnobByKey(key)
	return SetOut{OK: true, Warnings: warnings, Entry: &entry, Catalog: &knob}, nil
}

// ResolveOut is the wire shape of DoSettingsResolve — every knob's effective
// value with provenance (source tier + source id). Stable across MCP and HTTP
// so dashboards and agents share the same mental model.
type ResolveOut struct {
	TaskID   string                       `json:"task_id"`
	Resolved map[string]settings.Resolved `json:"resolved"`
}

// DoSettingsResolve walks task → manifest → product → system for every knob
// and returns the effective value for each.
func DoSettingsResolve(ctx context.Context, resolver *settings.Resolver, taskID string) (ResolveOut, error) {
	if resolver == nil {
		return ResolveOut{}, fmt.Errorf("settings resolver not configured")
	}
	if taskID == "" {
		return ResolveOut{}, fmt.Errorf("task_id is required")
	}
	resolved, err := resolver.ResolveAll(ctx, settings.Scope{TaskID: taskID})
	if err != nil {
		return ResolveOut{}, err
	}
	return ResolveOut{TaskID: taskID, Resolved: resolved}, nil
}

// -------- helpers ------------------------------------------------------------

// ValidateWritableScope rejects scope_type values that aren't one of the three
// user-writable tiers. System-scope values come from the catalog defaults and
// cannot be mutated via the MCP/HTTP surface.
func ValidateWritableScope(st settings.ScopeType) error {
	switch st {
	case settings.ScopeProduct, settings.ScopeManifest, settings.ScopeTask:
		return nil
	case settings.ScopeSystem:
		return fmt.Errorf("scope_type %q is read-only — system defaults come from the catalog", st)
	default:
		return fmt.Errorf("scope_type must be one of product|manifest|task, got %q", string(st))
	}
}

// HasVisceralCap reports whether a knob key has a visceral-rule ceiling
// registered in v1. Used to short-circuit the visceral-rule load for keys
// that never need it.
func HasVisceralCap(key string) bool {
	return key == "daily_budget_usd"
}

// ValueExceedsCap parses a JSON-encoded numeric value and reports whether it
// is greater than the ceiling. Type mismatch returns an error; non-numeric
// knobs should never reach this path (HasVisceralCap gates entry).
func ValueExceedsCap(jsonValue string, ceiling float64) (bool, error) {
	var n float64
	if err := json.Unmarshal([]byte(jsonValue), &n); err != nil {
		return false, fmt.Errorf("visceral cap check: %q is not numeric: %w", jsonValue, err)
	}
	return n > ceiling, nil
}

// LoadActiveVisceralRules pulls the text of every active visceral rule from
// the memory index. Production wiring for DoSettingsSet — both MCP and HTTP
// handlers route through this so the rule source is unambiguous.
func (s *Server) LoadActiveVisceralRules(_ context.Context) ([]string, error) {
	mems, err := s.node.Index.ListByType("visceral", 100)
	if err != nil {
		return nil, err
	}
	texts := make([]string, 0, len(mems))
	for _, m := range mems {
		texts = append(texts, m.L2)
	}
	return texts, nil
}

// mcpSetAuthor returns the author string recorded on settings writes. Uses the
// MCP session id when available so audits can trace who set what; falls back
// to "mcp:unknown" for calls that arrive without a session (e.g. bare stdio
// invocations before initialize). The "mcp:" prefix distinguishes MCP writes
// from HTTP writes ("http:" prefix, M2-T6).
func mcpSetAuthor(ctx context.Context) string {
	session := mcpserver.ClientSessionFromContext(ctx)
	if session == nil {
		return "mcp:unknown"
	}
	id := session.SessionID()
	if id == "" {
		return "mcp:unknown"
	}
	return "mcp:" + id
}

// jsonOrError marshals v as pretty JSON and returns it as a tool text result,
// or returns an MCP error result when marshal fails.
func jsonOrError(v interface{}) (*mcplib.CallToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errResult("marshal: %v", err), nil
	}
	return textResult(string(data)), nil
}
