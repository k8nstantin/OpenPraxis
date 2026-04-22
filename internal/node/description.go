package node

import (
	"context"
	"strings"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
)

// RecordDescriptionChange compares the proposed new body to the entity's
// current denormalised description / content / instructions and, if they
// differ, inserts a description_revision comment on the entity so the full
// edit history is preserved append-only. Returns the new comment ID, or
// empty string when the body is unchanged and no revision was recorded.
//
// Semantics (DV/M2):
//   - Entity.description remains the source of truth for fast reads. The
//     caller is expected to UPDATE it AFTER this call — or abort the whole
//     edit if this returns an error.
//   - Revisions are inserted on every real change. Whitespace-only edits
//     (trailing newline, reformatting) are deliberately not recorded.
//   - No-op safe: empty new body, missing stores, or matching current body
//     all return ("", nil) without side effects.
//   - Not strictly transactional with the entity UPDATE itself — in the
//     rare case that the entity UPDATE fails after this call succeeds, we
//     leave a stray revision that represents "someone tried to change it".
//     Acceptable for v1; a shared-tx helper is a bigger refactor deferred
//     to a later manifest.
//
// Call sites: HTTP apiProductUpdate / apiManifestUpdate / apiTaskUpdate and
// MCP handleProductUpdate / handleManifestUpdate.
func (n *Node) RecordDescriptionChange(
	ctx context.Context,
	target comments.TargetType,
	targetID string,
	newBody string,
	author string,
) (string, error) {
	if n == nil || n.Comments == nil {
		return "", nil
	}
	if strings.TrimSpace(newBody) == "" {
		return "", nil
	}

	currentBody, fullID, err := n.currentDescription(target, targetID)
	if err != nil {
		return "", err
	}
	if fullID == "" {
		// Entity does not exist yet or lookup store is nil — the caller
		// will surface that via its own UPDATE path; don't record a
		// revision against a phantom id.
		return "", nil
	}
	if strings.TrimSpace(currentBody) == strings.TrimSpace(newBody) {
		return "", nil
	}

	if author == "" {
		author = n.PeerID()
	}

	c, err := n.Comments.Add(ctx, target, fullID, author, comments.TypeDescriptionRevision, newBody)
	if err != nil {
		return "", err
	}
	return c.ID, nil
}

// currentDescription reads the entity's current denormalised body + returns
// the full UUID so callers (and the revision insert) operate on canonical
// ids instead of short markers. Returns ("", "", nil) for missing rows.
func (n *Node) currentDescription(target comments.TargetType, idOrMarker string) (body, fullID string, err error) {
	switch target {
	case comments.TargetProduct:
		if n.Products == nil {
			return "", "", nil
		}
		p, err := n.Products.Get(idOrMarker)
		if err != nil || p == nil {
			return "", "", err
		}
		return p.Description, p.ID, nil
	case comments.TargetManifest:
		if n.Manifests == nil {
			return "", "", nil
		}
		m, err := n.Manifests.Get(idOrMarker)
		if err != nil || m == nil {
			return "", "", err
		}
		return m.Content, m.ID, nil
	case comments.TargetTask:
		if n.Tasks == nil {
			return "", "", nil
		}
		t, err := n.Tasks.Get(idOrMarker)
		if err != nil || t == nil {
			return "", "", err
		}
		return t.Description, t.ID, nil
	}
	return "", "", nil
}
