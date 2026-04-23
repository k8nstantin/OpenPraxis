package templates

import (
	"context"
	"database/sql"
	"errors"
)

// AgentLookup answers what agent name is effectively assigned to the given
// task — the resolver uses it to walk into the agent-scope tier between
// product and system. Nil means "skip agent tier".
type AgentLookup interface {
	AgentForTask(ctx context.Context, taskID string) (string, error)
}

// ScopeLookup fills in manifest and product scope ids for a task. Nil
// means the caller will pass those explicitly (e.g. tests).
type ScopeLookup interface {
	ManifestAndProductForTask(ctx context.Context, taskID string) (manifestID, productID string, err error)
}

// Resolver walks task → manifest → product → agent → system, returning
// the first currently-active body it finds for `section`. Returns "" if
// no active row exists at any scope (caller falls back to the package
// defaults).
type Resolver struct {
	store  *Store
	scope  ScopeLookup
	agents AgentLookup
}

func NewResolver(store *Store, scope ScopeLookup, agents AgentLookup) *Resolver {
	return &Resolver{store: store, scope: scope, agents: agents}
}

// Resolve returns the active template body for `section` at `taskID`.
// Missing-row at a tier falls through to the next tier. Only DB errors
// other than sql.ErrNoRows surface.
func (r *Resolver) Resolve(ctx context.Context, section, taskID string) (string, error) {
	if taskID != "" {
		if body, ok, err := r.lookup(ctx, ScopeTask, taskID, section); err != nil {
			return "", err
		} else if ok {
			return body, nil
		}
	}

	var manifestID, productID string
	if taskID != "" && r.scope != nil {
		var err error
		manifestID, productID, err = r.scope.ManifestAndProductForTask(ctx, taskID)
		if err != nil {
			return "", err
		}
	}

	if manifestID != "" {
		if body, ok, err := r.lookup(ctx, ScopeManifest, manifestID, section); err != nil {
			return "", err
		} else if ok {
			return body, nil
		}
	}

	if productID != "" {
		if body, ok, err := r.lookup(ctx, ScopeProduct, productID, section); err != nil {
			return "", err
		} else if ok {
			return body, nil
		}
	}

	if taskID != "" && r.agents != nil {
		if agentName, err := r.agents.AgentForTask(ctx, taskID); err != nil {
			return "", err
		} else if agentName != "" {
			if body, ok, err := r.lookup(ctx, ScopeAgent, agentName, section); err != nil {
				return "", err
			} else if ok {
				return body, nil
			}
		}
	}

	if body, ok, err := r.lookup(ctx, ScopeSystem, "", section); err != nil {
		return "", err
	} else if ok {
		return body, nil
	}

	return "", nil
}

// lookup returns (body, found, error). sql.ErrNoRows is folded into found=false.
func (r *Resolver) lookup(ctx context.Context, scope, scopeID, section string) (string, bool, error) {
	t, err := r.store.Get(ctx, scope, scopeID, section)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return t.Body, true, nil
}
