package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
)

// DV/M4 — MCP tools over description_revision history. Thin stdio
// equivalents of the HTTP endpoints in DV/M3; both surfaces share the
// same Node helpers (DescriptionHistory / GetDescriptionRevision /
// RestoreDescription) so behaviour stays in lockstep.

func (s *Server) registerDescriptionTools() {
	s.mcp.AddTool(
		mcplib.NewTool("description_history",
			mcplib.WithDescription("List the description/content/instructions revision history for a product, manifest, or task. Revisions are append-only; the latest revision is always the current body. Returns newest-first."),
			mcplib.WithString("target_type", mcplib.Required(), mcplib.Description("Entity type: product, manifest, or task")),
			mcplib.WithString("target_id", mcplib.Required(), mcplib.Description("Entity full UUID")),
			mcplib.WithNumber("limit", mcplib.Description("Max revisions to return. Default 100, cap 1000.")),
		),
		s.handleDescriptionHistory,
	)

	s.mcp.AddTool(
		mcplib.NewTool("description_get_revision",
			mcplib.WithDescription("Fetch a single description_revision comment by id. Enforces that the revision belongs to the given target — you cannot read a revision from a sibling entity by guessing its id."),
			mcplib.WithString("target_type", mcplib.Required(), mcplib.Description("Entity type: product, manifest, or task")),
			mcplib.WithString("target_id", mcplib.Required(), mcplib.Description("Entity full UUID")),
			mcplib.WithString("revision_id", mcplib.Required(), mcplib.Description("The description_revision comment UUID")),
		),
		s.handleDescriptionGetRevision,
	)

	s.mcp.AddTool(
		mcplib.NewTool("description_restore",
			mcplib.WithDescription("Restore a prior description revision. Revisions are append-only; restore creates a new description_revision whose body equals the historical revision's body and denormalises that body back onto the entity. The original revision row is untouched."),
			mcplib.WithString("target_type", mcplib.Required(), mcplib.Description("Entity type: product, manifest, or task")),
			mcplib.WithString("target_id", mcplib.Required(), mcplib.Description("Entity full UUID")),
			mcplib.WithString("revision_id", mcplib.Required(), mcplib.Description("The historical description_revision comment UUID to restore")),
			mcplib.WithString("author", mcplib.Description("Optional author name for the new revision. Defaults to this peer's UUID.")),
		),
		s.handleDescriptionRestore,
	)
}

// parseTargetType canonicalises the target_type string to a comments
// TargetType. Returns an error result when the value is unrecognised.
func parseTargetType(raw string) (comments.TargetType, error) {
	switch raw {
	case "product":
		return comments.TargetProduct, nil
	case "manifest":
		return comments.TargetManifest, nil
	case "task":
		return comments.TargetTask, nil
	case "idea":
		return comments.TargetIdea, nil
	}
	return "", fmt.Errorf("target_type must be one of: product, manifest, task, idea (got %q)", raw)
}

func (s *Server) handleDescriptionHistory(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := req.GetArguments()
	target, err := parseTargetType(argStr(a, "target_type"))
	if err != nil {
		return errResult("%v", err), nil
	}
	targetID := argStr(a, "target_id")
	if targetID == "" {
		return errResult("target_id is required"), nil
	}
	limit := 100
	if v, ok := a["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}
	rows, err := s.node.DescriptionHistory(ctx, target, targetID, limit)
	if err != nil {
		return errResult("%v", err), nil
	}
	out, err := json.MarshalIndent(map[string]any{
		"target_type": string(target),
		"target_id":   targetID,
		"items":       rows,
	}, "", "  ")
	if err != nil {
		return errResult("marshal: %v", err), nil
	}
	return textResult(string(out)), nil
}

func (s *Server) handleDescriptionGetRevision(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := req.GetArguments()
	target, err := parseTargetType(argStr(a, "target_type"))
	if err != nil {
		return errResult("%v", err), nil
	}
	targetID := argStr(a, "target_id")
	revisionID := argStr(a, "revision_id")
	if targetID == "" || revisionID == "" {
		return errResult("target_id and revision_id are required"), nil
	}
	rev, err := s.node.GetDescriptionRevision(ctx, target, targetID, revisionID)
	if err != nil {
		return errResult("%v", err), nil
	}
	if rev == nil {
		return errResult("revision not found"), nil
	}
	out, err := json.MarshalIndent(rev, "", "  ")
	if err != nil {
		return errResult("marshal: %v", err), nil
	}
	return textResult(string(out)), nil
}

func (s *Server) handleDescriptionRestore(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := req.GetArguments()
	target, err := parseTargetType(argStr(a, "target_type"))
	if err != nil {
		return errResult("%v", err), nil
	}
	targetID := argStr(a, "target_id")
	revisionID := argStr(a, "revision_id")
	if targetID == "" || revisionID == "" {
		return errResult("target_id and revision_id are required"), nil
	}
	author := argStr(a, "author")

	newID, err := s.node.RestoreDescription(ctx, target, targetID, revisionID, author)
	if err != nil {
		return errResult("%v", err), nil
	}
	if newID == "" {
		return textResult(fmt.Sprintf("Restore no-op: current body already matches revision %s", revisionID)), nil
	}
	return textResult(fmt.Sprintf("Restored %s %s from revision %s. New revision: %s",
		target, targetID, revisionID, newID)), nil
}
