package settings

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// TaskLookup resolves a task's manifest id. Kept as a minimal interface so
// internal/settings can stay import-cycle-free vs internal/task — concrete
// adapters live in internal/task and are wired by the runner (M4).
type TaskLookup interface {
	GetTaskForSettings(ctx context.Context, taskID string) (TaskRec, error)
}

// ManifestLookup resolves a manifest's product id. Minimal interface vs
// internal/manifest for the same import-cycle reason as TaskLookup.
type ManifestLookup interface {
	GetManifestForSettings(ctx context.Context, manifestID string) (ManifestRec, error)
}

// TaskRec is the minimum task info the resolver needs. ManifestID may be ""
// for tasks that exist outside any manifest — in that case the walk skips
// straight from task scope to system scope.
type TaskRec struct {
	ID         string
	ManifestID string
}

// ManifestRec is the minimum manifest info the resolver needs. ProductID may
// be "" for orphan manifests; the walk then skips the product tier.
type ManifestRec struct {
	ID        string
	ProductID string
}

// Resolved is the output of Resolve — typed value plus provenance so callers
// (UI, audit logs, "where did this value come from?" tooltips) can show why a
// knob has the value it does.
type Resolved struct {
	Key      string      `json:"key"`
	Value    interface{} `json:"value"`     // typed per KnobDef.Type
	Source   ScopeType   `json:"source"`    // task | manifest | product | system
	SourceID string      `json:"source_id"` // entity id, or "" for system
}

// ErrResolveLookupFailed wraps errors returned by the TaskLookup or
// ManifestLookup adapters when scope normalization needs them. Callers can
// distinguish "the task does not exist" (lookup error) from "no entry was
// stored at this scope" (sql.ErrNoRows — handled internally as fallthrough).
var ErrResolveLookupFailed = errors.New("settings: resolve lookup failed")

// ErrResolveDecodeFailed wraps decode errors. Should never fire in practice
// because the catalog validates on Set, but we surface it as a sentinel so
// production callers can alert rather than panic on a corrupted row.
var ErrResolveDecodeFailed = errors.New("settings: resolve decode failed")

// Resolver walks task → manifest → product → system for any knob.
//
// The Resolver is read-only: it never writes to settings or invokes the
// lookups for any side effect. Construct a new Resolver per request if your
// store/lookup wiring is request-scoped; otherwise share one — the type holds
// no per-call state.
type Resolver struct {
	store     *Store
	tasks     TaskLookup
	manifests ManifestLookup
}

// NewResolver constructs a Resolver. Caller owns lifecycle of store + lookups.
// Either lookup may be nil if the caller guarantees the corresponding scope
// id will already be set on every Scope passed in (e.g. background jobs that
// know all three IDs upfront). If a lookup is required and nil, NormalizeScope
// returns an error.
func NewResolver(store *Store, tasks TaskLookup, manifests ManifestLookup) *Resolver {
	return &Resolver{store: store, tasks: tasks, manifests: manifests}
}

// Store returns the underlying settings store. Exposed so consumers like the
// task runner can persist runtime state (e.g. retry counters keyed by task
// scope) through the same settings table without a second injection point.
func (r *Resolver) Store() *Store { return r.store }

// Resolve returns the effective value for one knob at the given scope.
// Resolution order (first hit wins): task → manifest → product → system.
//
// Missing entries at any scope (sql.ErrNoRows) are not errors; they are the
// normal "try the next scope up" path. The only hard errors are:
//   - unknown key (ErrUnknownKey)
//   - decode failure on a stored value (ErrResolveDecodeFailed)
//   - lookup failure during scope normalization (ErrResolveLookupFailed)
//   - underlying DB errors (wrapped)
//
// Resolve normalizes the scope as it walks, so a Scope with only TaskID set
// is the canonical caller input.
func (r *Resolver) Resolve(ctx context.Context, scope Scope, key string) (Resolved, error) {
	knob, ok := KnobByKey(key)
	if !ok {
		return Resolved{}, fmt.Errorf("%w: %q", ErrUnknownKey, key)
	}

	if scope.TaskID != "" {
		hit, err := r.tryScope(ctx, ScopeTask, scope.TaskID, knob)
		if err != nil {
			return Resolved{}, err
		}
		if hit != nil {
			return *hit, nil
		}
	}

	if scope.ManifestID == "" && scope.TaskID != "" && r.tasks != nil {
		rec, err := r.tasks.GetTaskForSettings(ctx, scope.TaskID)
		if err != nil {
			return Resolved{}, fmt.Errorf("%w: task %q: %v", ErrResolveLookupFailed, scope.TaskID, err)
		}
		scope.ManifestID = rec.ManifestID
	}

	if scope.ManifestID != "" {
		hit, err := r.tryScope(ctx, ScopeManifest, scope.ManifestID, knob)
		if err != nil {
			return Resolved{}, err
		}
		if hit != nil {
			return *hit, nil
		}
	}

	if scope.ProductID == "" && scope.ManifestID != "" && r.manifests != nil {
		rec, err := r.manifests.GetManifestForSettings(ctx, scope.ManifestID)
		if err != nil {
			return Resolved{}, fmt.Errorf("%w: manifest %q: %v", ErrResolveLookupFailed, scope.ManifestID, err)
		}
		scope.ProductID = rec.ProductID
	}

	if scope.ProductID != "" {
		hit, err := r.tryScope(ctx, ScopeProduct, scope.ProductID, knob)
		if err != nil {
			return Resolved{}, err
		}
		if hit != nil {
			return *hit, nil
		}
	}

	def, _ := SystemDefault(key) // ok already verified via KnobByKey above
	return Resolved{Key: key, Value: def, Source: ScopeSystem, SourceID: ""}, nil
}

// ResolveAll returns the effective value for every knob in the catalog at
// the given scope. The implementation issues at most three DB queries
// (one per non-system scope tier) regardless of catalog size — important
// because UI screens render the whole knob grid on every page load.
//
// Design choice C from the task spec: snapshot each tier with one
// store.ListScope call, build per-tier maps, then iterate the catalog and
// consult the maps in inheritance order. O(N) in catalog size, with a fixed
// query budget.
func (r *Resolver) ResolveAll(ctx context.Context, scope Scope) (map[string]Resolved, error) {
	normalized, err := r.NormalizeScope(ctx, scope)
	if err != nil {
		return nil, err
	}

	taskMap, err := r.snapshotScope(ctx, ScopeTask, normalized.TaskID)
	if err != nil {
		return nil, err
	}
	manifestMap, err := r.snapshotScope(ctx, ScopeManifest, normalized.ManifestID)
	if err != nil {
		return nil, err
	}
	productMap, err := r.snapshotScope(ctx, ScopeProduct, normalized.ProductID)
	if err != nil {
		return nil, err
	}

	out := make(map[string]Resolved, len(Catalog()))
	for _, knob := range Catalog() {
		resolved, err := r.pickFromMaps(knob, normalized, taskMap, manifestMap, productMap)
		if err != nil {
			return nil, err
		}
		out[knob.Key] = resolved
	}
	return out, nil
}

// NormalizeScope fills in ManifestID + ProductID from the TaskID, if either
// is missing. A Scope with only TaskID set is the canonical caller input;
// the walk transparently promotes to manifest + product via the lookups.
//
// NormalizeScope respects caller overrides: if ManifestID or ProductID is
// already set on the input scope, it is left as-is. This lets callers pin a
// scope to a specific manifest even when the task technically belongs to a
// different one (used by previews, "what-if" queries).
func (r *Resolver) NormalizeScope(ctx context.Context, scope Scope) (Scope, error) {
	if scope.TaskID != "" && scope.ManifestID == "" {
		if r.tasks == nil {
			return scope, fmt.Errorf("%w: task lookup is nil but TaskID=%q needs promotion", ErrResolveLookupFailed, scope.TaskID)
		}
		rec, err := r.tasks.GetTaskForSettings(ctx, scope.TaskID)
		if err != nil {
			return scope, fmt.Errorf("%w: task %q: %v", ErrResolveLookupFailed, scope.TaskID, err)
		}
		scope.ManifestID = rec.ManifestID
	}

	if scope.ManifestID != "" && scope.ProductID == "" {
		if r.manifests == nil {
			return scope, fmt.Errorf("%w: manifest lookup is nil but ManifestID=%q needs promotion", ErrResolveLookupFailed, scope.ManifestID)
		}
		rec, err := r.manifests.GetManifestForSettings(ctx, scope.ManifestID)
		if err != nil {
			return scope, fmt.Errorf("%w: manifest %q: %v", ErrResolveLookupFailed, scope.ManifestID, err)
		}
		scope.ProductID = rec.ProductID
	}

	return scope, nil
}

// tryScope reads the explicit entry for (scopeType, scopeID, knob.Key). On a
// hit it decodes and wraps in a Resolved; on sql.ErrNoRows it returns nil so
// the caller falls through to the next scope tier.
func (r *Resolver) tryScope(ctx context.Context, scopeType ScopeType, scopeID string, knob KnobDef) (*Resolved, error) {
	entry, err := r.store.Get(ctx, scopeType, scopeID, knob.Key)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("settings: resolve %s scope %q key %q: %w", scopeType, scopeID, knob.Key, err)
	}
	value, err := decodeValue(knob, entry.Value)
	if err != nil {
		return nil, err
	}
	return &Resolved{Key: knob.Key, Value: value, Source: scopeType, SourceID: scopeID}, nil
}

// snapshotScope reads every entry at one scope tier in a single query, keyed
// by knob name for O(1) lookup. Returns an empty (non-nil) map when scopeID
// is "" so the caller can index without nil-checks.
func (r *Resolver) snapshotScope(ctx context.Context, scopeType ScopeType, scopeID string) (map[string]Entry, error) {
	if scopeID == "" {
		return map[string]Entry{}, nil
	}
	entries, err := r.store.ListScope(ctx, scopeType, scopeID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]Entry, len(entries))
	for _, e := range entries {
		out[e.Key] = e
	}
	return out, nil
}

// pickFromMaps walks the inheritance chain in memory using the per-tier maps
// produced by snapshotScope. Mirrors Resolve's order but without re-querying.
func (r *Resolver) pickFromMaps(
	knob KnobDef,
	scope Scope,
	taskMap, manifestMap, productMap map[string]Entry,
) (Resolved, error) {
	tiers := []struct {
		scopeType ScopeType
		scopeID   string
		entries   map[string]Entry
	}{
		{ScopeTask, scope.TaskID, taskMap},
		{ScopeManifest, scope.ManifestID, manifestMap},
		{ScopeProduct, scope.ProductID, productMap},
	}
	for _, t := range tiers {
		if t.scopeID == "" {
			continue
		}
		entry, ok := t.entries[knob.Key]
		if !ok {
			continue
		}
		value, err := decodeValue(knob, entry.Value)
		if err != nil {
			return Resolved{}, err
		}
		return Resolved{Key: knob.Key, Value: value, Source: t.scopeType, SourceID: t.scopeID}, nil
	}

	def, _ := SystemDefault(knob.Key)
	return Resolved{Key: knob.Key, Value: def, Source: ScopeSystem, SourceID: ""}, nil
}

// decodeValue dispatches on KnobType to produce a typed Go value from the
// stored JSON-encoded string. Catalog.ValidateValue runs on Set, so decode
// failures here indicate either DB corruption or a knob whose type changed
// after rows were already written — both warrant a hard error.
func decodeValue(knob KnobDef, jsonValue string) (interface{}, error) {
	switch knob.Type {
	case KnobInt:
		return decodeInt(knob, jsonValue)
	case KnobFloat:
		return decodeFloat(knob, jsonValue)
	case KnobString:
		return decodeString(knob, jsonValue)
	case KnobEnum:
		return decodeEnum(knob, jsonValue)
	case KnobMultiselect:
		return decodeMultiselect(knob, jsonValue)
	default:
		return nil, fmt.Errorf("%w: knob %q has unsupported type %q", ErrResolveDecodeFailed, knob.Key, knob.Type)
	}
}

// decodeInt reads a JSON number and casts to int64. Go's encoding/json
// surfaces all numbers as float64 by default; we round-trip through int64 so
// downstream consumers get a stable integral type rather than a float that
// might quietly lose precision past 2^53.
func decodeInt(knob KnobDef, jsonValue string) (interface{}, error) {
	var n float64
	if err := json.Unmarshal([]byte(jsonValue), &n); err != nil {
		return nil, fmt.Errorf("%w: knob %q expects int, raw=%q: %v", ErrResolveDecodeFailed, knob.Key, jsonValue, err)
	}
	return int64(n), nil
}

func decodeFloat(knob KnobDef, jsonValue string) (interface{}, error) {
	var n float64
	if err := json.Unmarshal([]byte(jsonValue), &n); err != nil {
		return nil, fmt.Errorf("%w: knob %q expects float, raw=%q: %v", ErrResolveDecodeFailed, knob.Key, jsonValue, err)
	}
	return n, nil
}

func decodeString(knob KnobDef, jsonValue string) (interface{}, error) {
	var s string
	if err := json.Unmarshal([]byte(jsonValue), &s); err != nil {
		return nil, fmt.Errorf("%w: knob %q expects string, raw=%q: %v", ErrResolveDecodeFailed, knob.Key, jsonValue, err)
	}
	return s, nil
}

// decodeEnum decodes the same shape as a string but is a separate function so
// future enum-specific normalization (case-folding, alias resolution) has a
// dedicated home.
func decodeEnum(knob KnobDef, jsonValue string) (interface{}, error) {
	var s string
	if err := json.Unmarshal([]byte(jsonValue), &s); err != nil {
		return nil, fmt.Errorf("%w: knob %q expects enum string, raw=%q: %v", ErrResolveDecodeFailed, knob.Key, jsonValue, err)
	}
	return s, nil
}

// decodeMultiselect decodes a JSON array of strings into []string. We accept
// the [] interface{} round-trip so empty arrays don't surprise callers.
func decodeMultiselect(knob KnobDef, jsonValue string) (interface{}, error) {
	var raw []interface{}
	if err := json.Unmarshal([]byte(jsonValue), &raw); err != nil {
		return nil, fmt.Errorf("%w: knob %q expects array, raw=%q: %v", ErrResolveDecodeFailed, knob.Key, jsonValue, err)
	}
	out := make([]string, 0, len(raw))
	for i, item := range raw {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%w: knob %q[%d] expects string, got %T", ErrResolveDecodeFailed, knob.Key, i, item)
		}
		out = append(out, s)
	}
	return out, nil
}
