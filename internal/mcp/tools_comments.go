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

	c, err := s.node.Comments.Add(ctx, target, targetID, author, ct, body)
	if err != nil {
		return errResult("add comment: %v", err), nil
	}

	return textResult(fmt.Sprintf("Comment added [%s] on %s %s by %s (type=%s)",
		c.ID, c.TargetType, c.TargetID, c.Author, c.Type)), nil
}
