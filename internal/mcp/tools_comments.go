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
// target_id must be the full 36-char UUID (post marker rip-out — prefixes
// return a "not found" error). The handler validates the target exists
// before insert so non-existent target_ids return a clean error rather
// than silently writing a dangling row.
func (s *Server) handleCommentAdd(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	targetType := argStr(a, "target_type")
	targetID := argStr(a, "target_id")
	author := argStr(a, "author")
	// Normalize legacy type names → prompt or comment.
	// Agents trained on old type taxonomy may send execution_review,
	// description_revision, user_note, etc. Map them transparently.
	ct := comments.NormalizeType(argStr(a, "type"))
	body := argStr(a, "body")

	if s.node.Comments == nil {
		return errResult("comments store not initialised"), nil
	}

	target := comments.TargetType(targetType)
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

// resolveCommentTarget validates a full-UUID target_id by looking it up
// in the respective entity store. Returns an error if the target does
// not exist. Post marker rip-out (eb49bef) each entity's Get method
// only accepts the full 36-char UUID — prefixes return nil.
//
// When a store is not wired (e.g. tests with a minimal Node), the raw id is
// returned unchanged so tool registration still works.
func (s *Server) resolveCommentTarget(target comments.TargetType, raw string) (string, error) {
	switch target {
	case comments.TargetTask:
		if s.node.Entities == nil {
			return raw, nil
		}
		e, err := s.node.Entities.Get(raw)
		if err != nil {
			return "", fmt.Errorf("resolve target task %q: %w", raw, err)
		}
		if e == nil {
			return "", fmt.Errorf("target task not found: %s", raw)
		}
		return e.EntityUID, nil
	case comments.TargetManifest, comments.TargetProduct:
		if s.node.Entities == nil {
			return raw, nil
		}
		e, err := s.node.Entities.Get(raw)
		if err != nil {
			return "", fmt.Errorf("resolve target %s %q: %w", target, raw, err)
		}
		if e == nil {
			return "", fmt.Errorf("target %s not found: %s", target, raw)
		}
		return e.EntityUID, nil
	}
	return raw, nil
}
