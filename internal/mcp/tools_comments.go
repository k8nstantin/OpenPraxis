package mcp

import (
	"context"
	"fmt"

	"github.com/k8nstantin/OpenPraxis/internal/comments"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// handleCommentAdd posts a comment on a product, manifest, or task.
// Matches the HTTP addComment handler in internal/web/handlers_comments.go:246
// so MCP and HTTP paths stay behaviorally identical.
//
// target_id is accepted as either the full UUID or a short marker (first
// 8-12 chars). The handler resolves to the full UUID before insert so the
// dashboard — which queries by full UUID — always finds the comment. Agents
// that interpolate the short marker from the runner prompt stop orphaning
// rows. Non-existent target_ids return a clean error rather than silently
// writing a dangling row.
func (s *Server) handleCommentAdd(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	targetType := argStr(a, "target_type")
	targetID := argStr(a, "target_id")
	author := argStr(a, "author")
	cType := argStr(a, "type")
	body := argStr(a, "body")

	if s.node.Comments == nil {
		return errResult("comments store not initialised"), nil
	}

	target := comments.TargetType(targetType)
	ct := comments.CommentType(cType)
	if err := comments.ValidateAdd(target, targetID, author, ct, body); err != nil {
		return errResult("validate: %v", err), nil
	}

	resolvedID, err := s.resolveCommentTarget(target, targetID)
	if err != nil {
		return errResult("%v", err), nil
	}

	c, err := s.node.Comments.Add(ctx, target, resolvedID, author, ct, body)
	if err != nil {
		return errResult("add comment: %v", err), nil
	}

	return textResult(fmt.Sprintf("Comment added [%s] on %s %s by %s (type=%s)",
		c.ID, c.TargetType, c.TargetID, c.Author, c.Type)), nil
}

// resolveCommentTarget canonicalizes a short-marker or full-UUID target_id
// into the full 36-char UUID by looking it up in the respective entity store.
// Returns an error if the target does not exist. Safe to pass a full UUID —
// each entity's Get method accepts either form via id = ? OR id LIKE ?.
//
// When a store is not wired (e.g. tests with a minimal Node), the raw id is
// returned unchanged so tool registration still works.
func (s *Server) resolveCommentTarget(target comments.TargetType, raw string) (string, error) {
	switch target {
	case comments.TargetTask:
		if s.node.Tasks == nil {
			return raw, nil
		}
		t, err := s.node.Tasks.Get(raw)
		if err != nil {
			return "", fmt.Errorf("resolve target task %q: %w", raw, err)
		}
		if t == nil {
			return "", fmt.Errorf("target task not found: %s", raw)
		}
		return t.ID, nil
	case comments.TargetManifest:
		if s.node.Manifests == nil {
			return raw, nil
		}
		m, err := s.node.Manifests.Get(raw)
		if err != nil {
			return "", fmt.Errorf("resolve target manifest %q: %w", raw, err)
		}
		if m == nil {
			return "", fmt.Errorf("target manifest not found: %s", raw)
		}
		return m.ID, nil
	case comments.TargetProduct:
		if s.node.Products == nil {
			return raw, nil
		}
		p, err := s.node.Products.Get(raw)
		if err != nil {
			return "", fmt.Errorf("resolve target product %q: %w", raw, err)
		}
		if p == nil {
			return "", fmt.Errorf("target product not found: %s", raw)
		}
		return p.ID, nil
	}
	return raw, nil
}
