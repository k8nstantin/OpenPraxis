package mcp

import (
	"context"
	"fmt"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
)

func (s *Server) registerIdeaTools() {
	s.mcp.AddTool(
		mcplib.NewTool("idea_add",
			mcplib.WithDescription("Save a product idea, feature request, or improvement. Ideas are shared across sessions."),
			mcplib.WithString("title", mcplib.Required(), mcplib.Description("Idea title")),
			mcplib.WithString("description", mcplib.Description("Detailed description")),
			mcplib.WithString("priority", mcplib.Description("low, medium, high, critical. Default: medium")),
			mcplib.WithString("project_id", mcplib.Description("Project ID or marker to assign idea to (optional)")),
			mcplib.WithString("tags", mcplib.Description("Comma-separated tags")),
		),
		s.handleIdeaAdd,
	)

	s.mcp.AddTool(
		mcplib.NewTool("idea_list",
			mcplib.WithDescription("List all ideas. Filter by status."),
			mcplib.WithString("status", mcplib.Description("Filter: draft, open, closed, archive")),
		),
		s.handleIdeaList,
	)

	s.mcp.AddTool(
		mcplib.NewTool("idea_update",
			mcplib.WithDescription("Update an idea's status, priority, or description."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("Idea ID or 8-char marker")),
			mcplib.WithString("title", mcplib.Description("New title")),
			mcplib.WithString("description", mcplib.Description("New description")),
			mcplib.WithString("status", mcplib.Description("draft, open, closed, archive")),
			mcplib.WithString("priority", mcplib.Description("low, medium, high, critical")),
			mcplib.WithString("tags", mcplib.Description("Comma-separated tags")),
		),
		s.handleIdeaUpdate,
	)
}

func (s *Server) handleIdeaAdd(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	title := argStr(a, "title")
	desc := argStr(a, "description")
	priority := argStr(a, "priority")
	tags := splitCSV(argStr(a, "tags"))

	projectID, err := s.node.ResolveProductID(argStr(a, "project_id"))
	if err != nil {
		return errResult("%v", err), nil
	}
	idea, err := s.node.Ideas.Create(title, desc, "draft", priority, s.sessionSource(ctx), s.node.PeerID(), projectID, tags)
	if err != nil {
		return errResult("save idea: %v", err), nil
	}

	return textResult(fmt.Sprintf("Idea saved [%s]: %s (priority: %s)", idea.Marker, idea.Title, idea.Priority)), nil
}

func (s *Server) handleIdeaList(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	status := argStr(a, "status")

	ideas, err := s.node.Ideas.List(status, 50)
	if err != nil {
		return errResult("list ideas: %v", err), nil
	}

	if len(ideas) == 0 {
		return textResult("No ideas found."), nil
	}

	var output string
	for i, idea := range ideas {
		tags := ""
		if len(idea.Tags) > 0 {
			tags = " [" + strings.Join(idea.Tags, ", ") + "]"
		}
		output += fmt.Sprintf("%d. [%s] %s — %s (%s, %s%s)\n",
			i+1, idea.Marker, idea.Title, idea.Description, idea.Status, idea.Priority, tags)
	}
	return textResult(output), nil
}

func (s *Server) handleIdeaUpdate(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	id := argStr(a, "id")

	existing, err := s.node.Ideas.Get(id)
	if err != nil || existing == nil {
		return errResult("idea not found"), nil
	}

	title := argStr(a, "title")
	if title == "" { title = existing.Title }
	desc := argStr(a, "description")
	if desc == "" { desc = existing.Description }
	status := argStr(a, "status")
	if status == "" { status = existing.Status }
	priority := argStr(a, "priority")
	if priority == "" { priority = existing.Priority }
	tagsStr := argStr(a, "tags")
	tags := existing.Tags
	if tagsStr != "" { tags = splitCSV(tagsStr) }

	// DV consistency — append-only description_revision before the
	// denormalised UPDATE, same pattern as product / manifest / task.
	if argStr(a, "description") != "" {
		if _, err := s.node.RecordDescriptionChange(ctx, comments.TargetIdea, existing.ID, desc, ""); err != nil {
			return errResult("record revision: %v", err), nil
		}
	}

	if err := s.node.Ideas.Update(existing.ID, title, desc, status, priority, existing.ProjectID, tags); err != nil {
		return errResult("update idea: %v", err), nil
	}

	return textResult(fmt.Sprintf("Idea updated [%s]: %s (%s, %s)", existing.Marker, title, status, priority)), nil
}
