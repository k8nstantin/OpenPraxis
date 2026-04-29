package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"github.com/k8nstantin/OpenPraxis/internal/mcp"
	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/settings"
)

// Hierarchical execution-controls settings handlers (M2-T6 of issue #34).
// Thin wrappers around mcp.DoSettingsX core functions so MCP and HTTP share
// one validation + visceral-cap path. All writes record an "http:" author
// prefix to distinguish HTTP-originated rows from "mcp:"-prefixed MCP writes.

// settingsKnownScopes lists the writable scope tiers in URL order so the
// router-registration loop is unambiguous.
var settingsKnownScopes = []string{"product", "manifest", "task"}

// httpSettingsAuthor returns the author string recorded on settings writes from
// the HTTP layer. Mirrors mcpSetAuthor's prefix convention so audits can tell
// at a glance which surface produced a row. Identity falls back to RemoteAddr
// when no auth middleware is in place yet.
func httpSettingsAuthor(r *http.Request) string {
	if r == nil {
		return "http:unknown"
	}
	if id := r.Header.Get("X-User-Id"); id != "" {
		return "http:" + id
	}
	addr := r.RemoteAddr
	if addr == "" {
		return "http:unknown"
	}
	if i := strings.LastIndex(addr, ":"); i > 0 {
		addr = addr[:i]
	}
	return "http:" + addr
}

// httpVisceralRuleLoader wraps Node.Index.ListByType("visceral", …) as a
// VisceralRuleLoader. Both this loader and Server.LoadActiveVisceralRules read
// the same memory.Index rows, so MCP and HTTP always see the same ceiling.
func httpVisceralRuleLoader(n *node.Node) mcp.VisceralRuleLoader {
	return func(_ context.Context) ([]string, error) {
		mems, err := n.Index.ListByType("visceral", 100)
		if err != nil {
			return nil, err
		}
		texts := make([]string, 0, len(mems))
		for _, m := range mems {
			texts = append(texts, m.L2)
		}
		return texts, nil
	}
}

// settingsHTTPStatus maps a settings/MCP error to the HTTP status the
// handlers should return. Catalog/scope/cap violations are user errors (400);
// resolver lookup failures are missing-entity errors (404); anything else is
// treated as a server-side fault (500).
func settingsHTTPStatus(err error) int {
	switch {
	case errors.Is(err, settings.ErrUnknownKey),
		errors.Is(err, settings.ErrTypeMismatch),
		errors.Is(err, settings.ErrEnumOutOfRange),
		errors.Is(err, settings.ErrInvalidScopeType):
		return http.StatusBadRequest
	case errors.Is(err, settings.ErrResolveLookupFailed):
		return http.StatusNotFound
	}
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "scope_type"),
		strings.Contains(msg, "are required"),
		strings.HasPrefix(msg, "Visceral rule"),
		strings.Contains(msg, "is read-only"):
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

// entryView is the GET response row shape — settings.Entry plus an ISO
// timestamp twin so the dashboard does not have to format unix seconds.
type entryView struct {
	ScopeType    settings.ScopeType `json:"scope_type"`
	ScopeID      string             `json:"scope_id"`
	Key          string             `json:"key"`
	Value        string             `json:"value"`
	UpdatedAt    int64              `json:"updated_at"`
	UpdatedAtISO string             `json:"updated_at_iso"`
	UpdatedBy    string             `json:"updated_by,omitempty"`
}

func toEntryView(e settings.Entry) entryView {
	return entryView{
		ScopeType:    e.ScopeType,
		ScopeID:      e.ScopeID,
		Key:          e.Key,
		Value:        e.Value,
		UpdatedAt:    e.UpdatedAt.Unix(),
		UpdatedAtISO: e.UpdatedAt.UTC().Format(time.RFC3339),
		UpdatedBy:    e.UpdatedBy,
	}
}

// scopeGetResponse is the GET /api/{scope}/:id/settings shape.
type scopeGetResponse struct {
	ScopeType string      `json:"scope_type"`
	ScopeID   string      `json:"scope_id"`
	Entries   []entryView `json:"entries"`
}

// putKeyResult records the per-key outcome of a partial-success PUT. ok=false
// rows carry an Error explaining the rejection; ok=true rows may carry
// non-blocking Warnings (slider out of typical range, etc.).
type putKeyResult struct {
	Key      string   `json:"key"`
	OK       bool     `json:"ok"`
	Warnings []string `json:"warnings,omitempty"`
	Error    string   `json:"error,omitempty"`
}

// putResponse is the PUT /api/{scope}/:id/settings shape. We chose 200 with
// per-key results over 207 Multi-Status: simpler client handling, no proxy
// quirks, and the body already carries unambiguous per-key state.
type putResponse struct {
	Results []putKeyResult `json:"results"`
}

// resolvedEntryView matches mcp.ResolveOut's resolved-map value shape. Mirrored
// so the HTTP and MCP shapes stay byte-compatible — UI and agents can reuse
// one client.
type resolvedResponse struct {
	TaskID   string                       `json:"task_id"`
	Resolved map[string]settings.Resolved `json:"resolved"`
}

// -------- handlers -----------------------------------------------------------

func apiSettingsCatalog() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, mcp.DoSettingsCatalog())
	}
}

func apiScopeSettingsGet(n *node.Node, scopeType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		fullID, err := n.ResolveScopeID(scopeType, id)
		if err != nil {
			writeError(w, err.Error(), http.StatusNotFound)
			return
		}
		out, err := mcp.DoSettingsGet(r.Context(), n.SettingsStore, scopeType, fullID)
		if err != nil {
			writeError(w, err.Error(), settingsHTTPStatus(err))
			return
		}
		views := make([]entryView, 0, len(out.Entries))
		for _, e := range out.Entries {
			views = append(views, toEntryView(e))
		}
		writeJSON(w, scopeGetResponse{
			ScopeType: out.ScopeType,
			ScopeID:   out.ScopeID,
			Entries:   views,
		})
	}
}

func apiScopeSettingsPut(n *node.Node, scopeType string) http.HandlerFunc {
	loader := httpVisceralRuleLoader(n)
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		// Validate the scope_id UUID up front. Post marker rip-out
		// (eb49bef) ResolveScopeID rejects non-UUIDs with 404 instead
		// of attempting prefix-match.
		fullID, err := n.ResolveScopeID(scopeType, id)
		if err != nil {
			writeError(w, err.Error(), http.StatusNotFound)
			return
		}
		// Body is {key: jsonRawValue} — values are kept raw so we can pass the
		// JSON-encoded form straight into DoSettingsSet (which expects the
		// same wire shape the MCP tool accepts).
		var raw map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			writeError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if len(raw) == 0 {
			writeError(w, "request body must contain at least one key", http.StatusBadRequest)
			return
		}
		author := httpSettingsAuthor(r)
		results := make([]putKeyResult, 0, len(raw))
		for key, val := range raw {
			out, err := mcp.DoSettingsSet(r.Context(), n.SettingsStore, loader,
				scopeType, fullID, key, string(val), author)
			if err != nil {
				results = append(results, putKeyResult{
					Key:   key,
					OK:    false,
					Error: err.Error(),
				})
				continue
			}
			results = append(results, putKeyResult{
				Key:      key,
				OK:       out.OK,
				Warnings: out.Warnings,
			})
		}
		writeJSON(w, putResponse{Results: results})
	}
}

func apiTaskSettingsResolved(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		out, err := mcp.DoSettingsResolve(r.Context(), n.SettingsResolver, id)
		if err != nil {
			writeError(w, err.Error(), settingsHTTPStatus(err))
			return
		}
		writeJSON(w, resolvedResponse{TaskID: out.TaskID, Resolved: out.Resolved})
	}
}

// apiScopeSettingsDelete removes the explicit entry at (scopeType, id, key) so
// the resolver falls back to the next tier up. This is the "Reset to inherited"
// action in the dashboard — M2 built GET + PUT but no per-key DELETE; M3-T7
// added this minimal endpoint so the Reset button in OL.renderKnobSection has
// semantically clear wire shape instead of overloading PUT with a null value.
//
// Unknown keys are rejected (keeps typo safety on the URL). The store's Delete
// is idempotent, so deleting a key that has no explicit row returns 200.
func apiScopeSettingsDelete(n *node.Node, scopeType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["id"]
		key := vars["key"]
		if id == "" || key == "" {
			writeError(w, "scope id and key are required", http.StatusBadRequest)
			return
		}
		if _, ok := settings.KnobByKey(key); !ok {
			writeError(w, fmt.Sprintf("%s: %q", settings.ErrUnknownKey, key), http.StatusBadRequest)
			return
		}
		st := settings.ScopeType(scopeType)
		if err := mcp.ValidateWritableScope(st); err != nil {
			writeError(w, err.Error(), settingsHTTPStatus(err))
			return
		}
		fullID, err := n.ResolveScopeID(scopeType, id)
		if err != nil {
			writeError(w, err.Error(), http.StatusNotFound)
			return
		}
		if err := n.SettingsStore.Delete(r.Context(), st, fullID, key); err != nil {
			writeError(w, err.Error(), settingsHTTPStatus(err))
			return
		}
		writeJSON(w, map[string]any{
			"ok":         true,
			"scope_type": scopeType,
			"scope_id":   fullID,
			"key":        key,
		})
	}
}

// registerSettingsExecRoutes attaches all 9 hierarchical-settings endpoints to
// the /api subrouter. Centralized here so the wiring stays in lockstep with
// the catalog of scopes.
func registerSettingsExecRoutes(api *mux.Router, n *node.Node) {
	api.HandleFunc("/settings/catalog", apiSettingsCatalog()).Methods("GET")
	for _, scope := range settingsKnownScopes {
		path := fmt.Sprintf("/%ss/{id}/settings", scope)
		api.HandleFunc(path, apiScopeSettingsGet(n, scope)).Methods("GET")
		api.HandleFunc(path, apiScopeSettingsPut(n, scope)).Methods("PUT")
		// Per-key delete — Reset-to-inherited for the knob UI (M3-T7).
		api.HandleFunc(path+"/{key}", apiScopeSettingsDelete(n, scope)).Methods("DELETE")
	}
	api.HandleFunc("/tasks/{id}/settings/resolved", apiTaskSettingsResolved(n)).Methods("GET")
}
