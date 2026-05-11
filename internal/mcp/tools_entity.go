package mcp

import (
	"context"
	"fmt"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
)

func (s *Server) registerEntityTools() {
	s.mcp.AddTool(
		mcplib.NewTool("entity_create",
			mcplib.WithDescription("Create an entity — a generic SCD-2 versioned object. Types: any type from entity_types table (built-in: skill, product, manifest, task, idea, RAG; extensible via POST /api/entity-types)."),
			mcplib.WithString("type", mcplib.Required(), mcplib.Description("Entity type — any name from the entity_types table (built-in: skill, product, manifest, task, idea, RAG)")),
			mcplib.WithString("title", mcplib.Required(), mcplib.Description("Entity title")),
			mcplib.WithString("description", mcplib.Description("Initial prompt/instructions body (stored as first revision)")),
			mcplib.WithString("status", mcplib.Description("draft, active, closed, archived. Default: draft")),
			mcplib.WithString("tags", mcplib.Description("Comma-separated tags")),
		),
		s.handleEntityCreate,
	)

	s.mcp.AddTool(
		mcplib.NewTool("entity_list",
			mcplib.WithDescription("List entities, optionally filtered by type and/or status. Types are DB-driven — use entity_types table for the full list of available types."),
			mcplib.WithString("type", mcplib.Description("Filter by type — any name from the entity_types table (built-in: skill, product, manifest, task, idea, RAG)")),
			mcplib.WithString("status", mcplib.Description("Filter by status: draft, active, closed, archived")),
			mcplib.WithNumber("limit", mcplib.Description("Max results. Default: 50")),
		),
		s.handleEntityList,
	)

	s.mcp.AddTool(
		mcplib.NewTool("entity_get",
			mcplib.WithDescription("Get an entity by ID. Returns full detail including SCD-2 metadata."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("Entity UID")),
		),
		s.handleEntityGet,
	)

	s.mcp.AddTool(
		mcplib.NewTool("entity_update",
			mcplib.WithDescription("Update an entity — merges provided fields with current values."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("Entity UID")),
			mcplib.WithString("title", mcplib.Description("New title")),
			mcplib.WithString("description", mcplib.Description("New prompt/instructions body (append-only — creates a new revision)")),
			mcplib.WithString("status", mcplib.Description("draft, active, closed, archived")),
			mcplib.WithString("tags", mcplib.Description("Comma-separated tags")),
		),
		s.handleEntityUpdate,
	)

	s.mcp.AddTool(
		mcplib.NewTool("entity_history",
			mcplib.WithDescription("Return all SCD-2 versions for an entity, newest first."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("Entity UID")),
			mcplib.WithNumber("limit", mcplib.Description("Max versions to return. Default: 20")),
		),
		s.handleEntityHistory,
	)

	s.mcp.AddTool(
		mcplib.NewTool("entity_search",
			mcplib.WithDescription("Search entities by title substring (case-insensitive). Types are DB-driven — use entity_types table for the full list of available types."),
			mcplib.WithString("query", mcplib.Required(), mcplib.Description("Search query")),
			mcplib.WithString("type", mcplib.Description("Filter by type — any name from the entity_types table (built-in: skill, product, manifest, task, idea, RAG)")),
			mcplib.WithNumber("limit", mcplib.Description("Max results. Default: 20")),
		),
		s.handleEntitySearch,
	)
}

func (s *Server) handleEntityCreate(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	entityType := argStr(a, "type")
	title := argStr(a, "title")
	desc := argStr(a, "description")
	status := argStr(a, "status")
	tags := splitCSV(argStr(a, "tags"))

	e, err := s.node.Entities.Create(entityType, title, status, tags, s.sessionSource(ctx), "")
	if err != nil {
		return errResult("create entity: %v", err), nil
	}

	if desc != "" {
		if _, err := s.node.RecordDescriptionChange(ctx, comments.TargetEntity, e.EntityUID, desc, ""); err != nil {
			return errResult("record description: %v", err), nil
		}
	}

	return textResult(fmt.Sprintf("Entity created [%s]: %s (%s/%s)", e.EntityUID, e.Title, e.Type, e.Status)), nil
}

func (s *Server) handleEntityList(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	entityType := argStr(a, "type")
	status := argStr(a, "status")
	limit := int(argFloat(a, "limit"))
	if limit <= 0 {
		limit = 50
	}

	entities, err := s.node.Entities.List(entityType, status, limit)
	if err != nil {
		return errResult("list entities: %v", err), nil
	}

	if len(entities) == 0 {
		return textResult("No entities found."), nil
	}

	var lines []string
	for i, e := range entities {
		lines = append(lines, fmt.Sprintf("%d. [%s] %s — %s/%s", i+1, e.EntityUID, e.Title, e.Type, e.Status))
	}
	return textResult(strings.Join(lines, "\n")), nil
}

func (s *Server) handleEntityGet(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	id := argStr(a, "id")

	e, err := s.node.Entities.Get(id)
	if err != nil {
		return errResult("get entity: %v", err), nil
	}
	if e == nil {
		return textResult("Entity not found."), nil
	}

	tags := "none"
	if len(e.Tags) > 0 {
		tags = strings.Join(e.Tags, ", ")
	}

	return textResult(fmt.Sprintf("[%s] %s\nType: %s | Status: %s\nTags: %s\nValid from: %s\nChanged by: %s",
		e.EntityUID, e.Title, e.Type, e.Status, tags, e.ValidFrom, e.ChangedBy)), nil
}

func (s *Server) handleEntityUpdate(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	id := argStr(a, "id")

	existing, err := s.node.Entities.Get(id)
	if err != nil || existing == nil {
		return errResult("entity not found: %s", id), nil
	}

	title := argStr(a, "title")
	if title == "" {
		title = existing.Title
	}
	status := argStr(a, "status")
	if status == "" {
		status = existing.Status
	}
	tagsStr := argStr(a, "tags")
	tags := existing.Tags
	if tagsStr != "" {
		tags = splitCSV(tagsStr)
	}
	desc := argStr(a, "description")

	if desc != "" {
		if _, err := s.node.RecordDescriptionChange(ctx, comments.TargetEntity, existing.EntityUID, desc, ""); err != nil {
			return errResult("record description: %v", err), nil
		}
	}

	if err := s.node.Entities.Update(existing.EntityUID, title, status, tags, s.sessionSource(ctx), ""); err != nil {
		return errResult("update entity: %v", err), nil
	}

	return textResult(fmt.Sprintf("Entity updated [%s]: %s (%s)", existing.EntityUID, title, status)), nil
}

func (s *Server) handleEntityHistory(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	id := argStr(a, "id")
	limit := int(argFloat(a, "limit"))
	if limit <= 0 {
		limit = 20
	}

	versions, err := s.node.Entities.History(id)
	if err != nil {
		return errResult("entity history: %v", err), nil
	}
	if len(versions) == 0 {
		return textResult("No history found."), nil
	}

	if limit > len(versions) {
		limit = len(versions)
	}

	var lines []string
	for i, v := range versions[:limit] {
		version := len(versions) - i
		validTo := v.ValidTo
		if validTo == "" {
			validTo = "current"
		}
		lines = append(lines, fmt.Sprintf("v%d [%s] %s/%s | from: %s to: %s | by: %s",
			version, v.EntityUID, v.Title, v.Status, v.ValidFrom, validTo, v.ChangedBy))
	}
	return textResult(strings.Join(lines, "\n")), nil
}

func (s *Server) handleEntitySearch(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	query := argStr(a, "query")
	entityType := argStr(a, "type")
	limit := int(argFloat(a, "limit"))
	if limit <= 0 {
		limit = 20
	}

	entities, err := s.node.Entities.Search(query, entityType, limit)
	if err != nil {
		return errResult("search entities: %v", err), nil
	}

	if len(entities) == 0 {
		return textResult("No entities found."), nil
	}

	var lines []string
	for i, e := range entities {
		lines = append(lines, fmt.Sprintf("%d. [%s] %s — %s/%s", i+1, e.EntityUID, e.Title, e.Type, e.Status))
	}
	return textResult(strings.Join(lines, "\n")), nil
}
