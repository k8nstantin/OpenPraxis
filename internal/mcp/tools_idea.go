package mcp

import (
	"context"
	"fmt"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
	"github.com/k8nstantin/OpenPraxis/internal/entity"
)

func (s *Server) registerIdeaTools() {
	s.mcp.AddTool(
		mcplib.NewTool("idea_add",
			mcplib.WithDescription("Save a product idea, feature request, or improvement. Ideas are shared across sessions."),
			mcplib.WithString("title", mcplib.Required(), mcplib.Description("Idea title")),
			mcplib.WithString("description", mcplib.Description("Detailed description (stored as initial description_revision)")),
			mcplib.WithString("priority", mcplib.Description("low, medium, high, critical. Default: medium")),
			mcplib.WithString("project_id", mcplib.Description("Project ID to assign idea to (optional)")),
			mcplib.WithString("tags", mcplib.Description("Comma-separated tags")),
		),
		s.handleIdeaAdd,
	)

	s.mcp.AddTool(
		mcplib.NewTool("idea_list",
			mcplib.WithDescription("List all ideas. Filter by status."),
			mcplib.WithString("status", mcplib.Description("Filter: draft, active, closed, archived")),
		),
		s.handleIdeaList,
	)

	s.mcp.AddTool(
		mcplib.NewTool("idea_update",
			mcplib.WithDescription("Update an idea's status, priority, or description."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("Idea full UUID")),
			mcplib.WithString("title", mcplib.Description("New title")),
			mcplib.WithString("description", mcplib.Description("New description")),
			mcplib.WithString("status", mcplib.Description("draft, active, closed, archived")),
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
	if priority == "" {
		priority = "medium"
	}
	tags := splitCSV(argStr(a, "tags"))

	if s.node.Entities == nil {
		return errResult("entity store not initialised"), nil
	}

	// Encode priority into tags so it's preserved in the entity model.
	tags = appendPriorityTag(tags, priority)

	e, err := s.node.Entities.Create(entity.TypeIdea, title, entity.StatusDraft, tags,
		s.sessionSource(ctx), "idea_add")
	if err != nil {
		return errResult("save idea: %v", err), nil
	}

	// Record description as a description_revision comment if provided.
	if desc != "" {
		if _, err := s.node.RecordDescriptionChange(ctx, comments.TargetIdea, e.EntityUID, desc, ""); err != nil {
			// Non-fatal — the idea row was saved; description_revision can be
			// added manually if the comment insert fails.
			_ = err
		}
	}

	return textResult(fmt.Sprintf("Idea saved [%s]: %s (priority: %s)", e.EntityUID, e.Title, priority)), nil
}

func (s *Server) handleIdeaList(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	status := argStr(a, "status")

	if s.node.Entities == nil {
		return errResult("entity store not initialised"), nil
	}

	ideas, err := s.node.Entities.List(entity.TypeIdea, status, 50)
	if err != nil {
		return errResult("list ideas: %v", err), nil
	}

	if len(ideas) == 0 {
		return textResult("No ideas found."), nil
	}

	var output string
	for i, e := range ideas {
		priority := extractPriorityTag(e.Tags)
		tagsStr := ""
		otherTags := filterPriorityTags(e.Tags)
		if len(otherTags) > 0 {
			tagsStr = " [" + strings.Join(otherTags, ", ") + "]"
		}
		output += fmt.Sprintf("%d. [%s] %s — %s (%s%s)\n",
			i+1, e.EntityUID, e.Title, e.Status, priority, tagsStr)
	}
	return textResult(output), nil
}

func (s *Server) handleIdeaUpdate(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	id := argStr(a, "id")

	if s.node.Entities == nil {
		return errResult("entity store not initialised"), nil
	}

	existing, err := s.node.Entities.Get(id)
	if err != nil || existing == nil {
		return errResult("idea not found"), nil
	}

	title := argStr(a, "title")
	if title == "" {
		title = existing.Title
	}
	status := argStr(a, "status")
	if status == "" {
		status = existing.Status
	}
	priority := argStr(a, "priority")
	tagsStr := argStr(a, "tags")
	tags := existing.Tags
	if tagsStr != "" {
		tags = splitCSV(tagsStr)
	}
	if priority != "" {
		tags = appendPriorityTag(filterPriorityTags(tags), priority)
	}

	desc := argStr(a, "description")
	// DV consistency — append-only description_revision before the
	// denormalised UPDATE, same pattern as product / manifest / task.
	if desc != "" {
		if _, err := s.node.RecordDescriptionChange(ctx, comments.TargetIdea, existing.EntityUID, desc, ""); err != nil {
			return errResult("record revision: %v", err), nil
		}
	}

	if err := s.node.Entities.Update(existing.EntityUID, title, status, tags, s.sessionSource(ctx), "idea_update"); err != nil {
		return errResult("update idea: %v", err), nil
	}

	resolvedPriority := extractPriorityTag(tags)
	return textResult(fmt.Sprintf("Idea updated [%s]: %s (%s, %s)", existing.EntityUID, title, status, resolvedPriority)), nil
}

// appendPriorityTag adds a "priority:<level>" tag to the slice, replacing any
// existing priority tag so the set stays canonical.
func appendPriorityTag(tags []string, priority string) []string {
	if priority == "" {
		return tags
	}
	out := filterPriorityTags(tags)
	return append(out, "priority:"+priority)
}

// filterPriorityTags returns tags without any "priority:*" entries.
func filterPriorityTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		if !strings.HasPrefix(t, "priority:") {
			out = append(out, t)
		}
	}
	return out
}

// extractPriorityTag reads the priority level from tags, defaulting to
// "medium" when not present.
func extractPriorityTag(tags []string) string {
	for _, t := range tags {
		if strings.HasPrefix(t, "priority:") {
			return strings.TrimPrefix(t, "priority:")
		}
	}
	return "medium"
}
