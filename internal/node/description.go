package node

import (
	"context"
	"fmt"
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
//
// After the legacy store purge, products / manifests / ideas / entities all
// resolve through the unified entity store. Tasks still have their own store.
func (n *Node) currentDescription(target comments.TargetType, idOrMarker string) (body, fullID string, err error) {
	switch target {
	case comments.TargetTask:
		if n.Tasks == nil {
			return "", "", nil
		}
		t, err := n.Tasks.Get(idOrMarker)
		if err != nil || t == nil {
			return "", "", err
		}
		return t.Description, t.ID, nil
	case comments.TargetProduct, comments.TargetManifest, comments.TargetIdea, comments.TargetEntity:
		if n.Entities == nil {
			return "", "", nil
		}
		e, err := n.Entities.Get(idOrMarker)
		if err != nil || e == nil {
			return "", "", err
		}
		return e.Title, e.EntityUID, nil
	}
	return "", "", nil
}

// RevisionEntry is a single description_revision presented to API / MCP
// callers. Version is 1-based with 1 being the oldest revision — i.e. the
// backfilled seed row or the first user edit after schema rollout.
type RevisionEntry struct {
	ID        string `json:"id"`
	Version   int    `json:"version"`
	Author    string `json:"author"`
	Body      string `json:"body"`
	CreatedAt int64  `json:"created_at"`
}

// DescriptionHistory returns the description_revision comments for the given
// entity, newest first, with a 1-based Version field derived from insertion
// order (oldest = 1). The targetID argument may be a short marker or the full
// UUID; the returned rows always carry full comment IDs.
func (n *Node) DescriptionHistory(
	ctx context.Context,
	target comments.TargetType,
	targetID string,
	limit int,
) ([]RevisionEntry, error) {
	if n == nil || n.Comments == nil {
		return nil, nil
	}
	_, fullID, err := n.currentDescription(target, targetID)
	if err != nil {
		return nil, err
	}
	if fullID == "" {
		return nil, fmt.Errorf("%s not found: %s", target, targetID)
	}
	ct := comments.TypeDescriptionRevision
	rows, err := n.Comments.List(ctx, target, fullID, limit, &ct)
	if err != nil {
		return nil, err
	}
	// List returns newest first. Version numbering is oldest-first, so
	// the last element gets version 1 and the first element gets len(rows).
	out := make([]RevisionEntry, 0, len(rows))
	total := len(rows)
	for i, c := range rows {
		out = append(out, RevisionEntry{
			ID:        c.ID,
			Version:   total - i,
			Author:    c.Author,
			Body:      c.Body,
			CreatedAt: c.CreatedAt.Unix(),
		})
	}
	return out, nil
}

// GetDescriptionRevision fetches a single description_revision by id and
// enforces that it actually belongs to the given (target, targetID) pair
// so operators can't read a revision from a sibling entity by guessing the
// comment id.
func (n *Node) GetDescriptionRevision(
	ctx context.Context,
	target comments.TargetType,
	targetID, commentID string,
) (*RevisionEntry, error) {
	if n == nil || n.Comments == nil {
		return nil, nil
	}
	_, fullID, err := n.currentDescription(target, targetID)
	if err != nil {
		return nil, err
	}
	if fullID == "" {
		return nil, fmt.Errorf("%s not found: %s", target, targetID)
	}
	c, err := n.Comments.Get(ctx, commentID)
	if err != nil {
		return nil, err
	}
	if c.Type != comments.TypeDescriptionRevision {
		return nil, fmt.Errorf("comment %s is not a description_revision", commentID)
	}
	if c.TargetType != target || c.TargetID != fullID {
		return nil, fmt.Errorf("revision %s does not belong to %s %s", commentID, target, fullID)
	}
	// Version is the count of revisions with created_at <= this one.
	ct := comments.TypeDescriptionRevision
	all, err := n.Comments.List(ctx, target, fullID, 0, &ct)
	if err != nil {
		return nil, err
	}
	version := 0
	total := len(all)
	for i, row := range all {
		if row.ID == c.ID {
			version = total - i
			break
		}
	}
	return &RevisionEntry{
		ID:        c.ID,
		Version:   version,
		Author:    c.Author,
		Body:      c.Body,
		CreatedAt: c.CreatedAt.Unix(),
	}, nil
}

// RestoreDescription re-applies a prior description_revision as the current
// body. The mechanism is deliberately additive: we record a *new*
// description_revision whose body equals the historical revision's body, then
// denormalise that body back onto the entity column via the appropriate
// store.Update. The original revision row is untouched so the full trail
// remains intact — an operator can see "restored from X" as the latest row.
//
// Returns the newly-created revision ID. When the historical body already
// matches the current body (i.e. nothing to do) returns ("", nil).
func (n *Node) RestoreDescription(
	ctx context.Context,
	target comments.TargetType,
	targetID, fromCommentID, author string,
) (string, error) {
	if n == nil || n.Comments == nil {
		return "", nil
	}
	rev, err := n.GetDescriptionRevision(ctx, target, targetID, fromCommentID)
	if err != nil {
		return "", err
	}
	_, fullID, err := n.currentDescription(target, targetID)
	if err != nil {
		return "", err
	}
	if fullID == "" {
		return "", fmt.Errorf("%s not found: %s", target, targetID)
	}

	newCommentID, err := n.RecordDescriptionChange(ctx, target, fullID, rev.Body, author)
	if err != nil {
		return "", err
	}
	if newCommentID == "" {
		// Historical body already matches current; no UPDATE needed.
		return "", nil
	}

	if err := n.writeEntityDescription(target, fullID, rev.Body); err != nil {
		return "", err
	}
	return newCommentID, nil
}

// writeEntityDescription updates only the denormalised description/content
// column for the given entity, preserving all other fields. Used by
// RestoreDescription so the HTTP/MCP restore path doesn't have to re-send
// the whole PATCH payload.
func (n *Node) writeEntityDescription(target comments.TargetType, fullID, body string) error {
	switch target {
	case comments.TargetTask:
		if n.Tasks == nil {
			return fmt.Errorf("tasks store not wired")
		}
		_, err := n.Tasks.Update(fullID, nil, &body)
		return err
	case comments.TargetProduct, comments.TargetManifest, comments.TargetIdea, comments.TargetEntity:
		if n.Entities == nil {
			return fmt.Errorf("entities store not wired")
		}
		e, err := n.Entities.Get(fullID)
		if err != nil || e == nil {
			return fmt.Errorf("entity not found: %s", fullID)
		}
		return n.Entities.Update(e.EntityUID, e.Title, e.Status, e.Tags, "system", "description restore")
	}
	return fmt.Errorf("unsupported target type: %s", target)
}
