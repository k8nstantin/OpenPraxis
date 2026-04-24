package mcp

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/templates"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// registerTemplateTools registers the RC/M2 MCP surface for the
// prompt_templates table — SCD-2 CRUD, history, and point-in-time read.
func (s *Server) registerTemplateTools() {
	s.mcp.AddTool(
		mcplib.NewTool("template_create",
			mcplib.WithDescription("Create a new prompt-template override at a scope (product|manifest|task|agent). Returns the new template_uid."),
			mcplib.WithString("scope", mcplib.Required(), mcplib.Description("Scope tier: product|manifest|task|agent")),
			mcplib.WithString("scope_id", mcplib.Description("Scope entity id (empty for system — but system rows come from seed only)")),
			mcplib.WithString("section", mcplib.Required(), mcplib.Description("Prompt section key (preamble, visceral_rules, manifest_spec, task, instructions, git_workflow, closing_protocol)")),
			mcplib.WithString("title", mcplib.Description("Human-readable title; defaults to the section key")),
			mcplib.WithString("body", mcplib.Required(), mcplib.Description("Template body (text/template syntax)")),
			mcplib.WithString("reason", mcplib.Description("Audit reason recorded on the row")),
		),
		s.handleTemplateCreate,
	)

	s.mcp.AddTool(
		mcplib.NewTool("template_set",
			mcplib.WithDescription("Update the body of an existing template. Closes the prior active row and appends a new current row in one SCD-2 transaction."),
			mcplib.WithString("template_uid", mcplib.Required(), mcplib.Description("Logical template id")),
			mcplib.WithString("body", mcplib.Required(), mcplib.Description("New template body")),
			mcplib.WithString("reason", mcplib.Description("Audit reason recorded on the new row")),
		),
		s.handleTemplateSet,
	)

	s.mcp.AddTool(
		mcplib.NewTool("template_get",
			mcplib.WithDescription("Return the currently-active row for a template_uid."),
			mcplib.WithString("template_uid", mcplib.Required(), mcplib.Description("Logical template id")),
		),
		s.handleTemplateGet,
	)

	s.mcp.AddTool(
		mcplib.NewTool("template_history",
			mcplib.WithDescription("Return every row for a template_uid newest-first (active + closed + tombstoned)."),
			mcplib.WithString("template_uid", mcplib.Required(), mcplib.Description("Logical template id")),
		),
		s.handleTemplateHistory,
	)

	s.mcp.AddTool(
		mcplib.NewTool("template_at",
			mcplib.WithDescription("Return the row that was active for the given template_uid at the supplied RFC3339 timestamp."),
			mcplib.WithString("template_uid", mcplib.Required(), mcplib.Description("Logical template id")),
			mcplib.WithString("when", mcplib.Required(), mcplib.Description("RFC3339 timestamp")),
		),
		s.handleTemplateAt,
	)

	s.mcp.AddTool(
		mcplib.NewTool("template_list",
			mcplib.WithDescription("List active rows, optionally filtered by scope, scope_id, and/or section."),
			mcplib.WithString("scope", mcplib.Description("Filter by scope")),
			mcplib.WithString("scope_id", mcplib.Description("Filter by scope_id")),
			mcplib.WithString("section", mcplib.Description("Filter by section")),
		),
		s.handleTemplateList,
	)

	s.mcp.AddTool(
		mcplib.NewTool("template_tombstone",
			mcplib.WithDescription("Soft-delete every row for the given template_uid. Resolver falls through to the next-broader scope. Not revivable."),
			mcplib.WithString("template_uid", mcplib.Required(), mcplib.Description("Logical template id")),
			mcplib.WithString("reason", mcplib.Description("Audit reason recorded on the tombstone")),
		),
		s.handleTemplateTombstone,
	)
}

func (s *Server) handleTemplateCreate(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	if s.node.Templates == nil {
		return errResult("templates store not initialized"), nil
	}
	a := args(req)
	uid, err := s.node.Templates.Create(ctx,
		argStr(a, "scope"), argStr(a, "scope_id"), argStr(a, "section"),
		argStr(a, "title"), argStr(a, "body"),
		mcpTemplateAuthor(ctx), argStr(a, "reason"))
	if errors.Is(err, templates.ErrDuplicateOverride) {
		return errResult("duplicate override: %v", err), nil
	}
	if err != nil {
		return errResult("template_create: %v", err), nil
	}
	t, err := s.node.Templates.GetByUID(ctx, uid)
	if err != nil {
		return errResult("template_create read-back: %v", err), nil
	}
	return jsonOrError(t)
}

func (s *Server) handleTemplateSet(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	if s.node.Templates == nil {
		return errResult("templates store not initialized"), nil
	}
	a := args(req)
	uid := argStr(a, "template_uid")
	if uid == "" {
		return errResult("template_uid is required"), nil
	}
	if err := s.node.Templates.UpdateBody(ctx, uid, argStr(a, "body"), mcpTemplateAuthor(ctx), argStr(a, "reason")); err != nil {
		if errors.Is(err, templates.ErrNotFound) {
			return errResult("template_uid %s not found or inactive", uid), nil
		}
		return errResult("template_set: %v", err), nil
	}
	t, err := s.node.Templates.GetByUID(ctx, uid)
	if err != nil {
		return errResult("template_set read-back: %v", err), nil
	}
	return jsonOrError(t)
}

func (s *Server) handleTemplateGet(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	if s.node.Templates == nil {
		return errResult("templates store not initialized"), nil
	}
	uid := argStr(args(req), "template_uid")
	t, err := s.node.Templates.GetByUID(ctx, uid)
	if errors.Is(err, sql.ErrNoRows) {
		return errResult("template_uid %s not found", uid), nil
	}
	if err != nil {
		return errResult("template_get: %v", err), nil
	}
	return jsonOrError(t)
}

func (s *Server) handleTemplateHistory(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	if s.node.Templates == nil {
		return errResult("templates store not initialized"), nil
	}
	uid := argStr(args(req), "template_uid")
	rows, err := s.node.Templates.History(ctx, uid)
	if err != nil {
		return errResult("template_history: %v", err), nil
	}
	if rows == nil {
		rows = []*templates.Template{}
	}
	return jsonOrError(rows)
}

func (s *Server) handleTemplateAt(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	if s.node.Templates == nil {
		return errResult("templates store not initialized"), nil
	}
	a := args(req)
	uid := argStr(a, "template_uid")
	when := argStr(a, "when")
	t, err := time.Parse(time.RFC3339, when)
	if err != nil {
		return errResult("invalid when: %v", err), nil
	}
	row, err := s.node.Templates.AtTime(ctx, uid, t)
	if errors.Is(err, sql.ErrNoRows) {
		return errResult("no active row for %s at %s", uid, when), nil
	}
	if err != nil {
		return errResult("template_at: %v", err), nil
	}
	return jsonOrError(row)
}

func (s *Server) handleTemplateList(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	if s.node.Templates == nil {
		return errResult("templates store not initialized"), nil
	}
	a := args(req)
	rows, err := s.node.Templates.ListWithScopeID(ctx,
		argStr(a, "scope"), argStr(a, "scope_id"), argStr(a, "section"))
	if err != nil {
		return errResult("template_list: %v", err), nil
	}
	if rows == nil {
		rows = []*templates.Template{}
	}
	return jsonOrError(rows)
}

func (s *Server) handleTemplateTombstone(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	if s.node.Templates == nil {
		return errResult("templates store not initialized"), nil
	}
	a := args(req)
	uid := argStr(a, "template_uid")
	if err := s.node.Templates.Tombstone(ctx, uid, mcpTemplateAuthor(ctx), argStr(a, "reason")); err != nil {
		if errors.Is(err, templates.ErrNotFound) {
			return errResult("template_uid %s not found", uid), nil
		}
		return errResult("template_tombstone: %v", err), nil
	}
	return textResult("tombstoned " + uid), nil
}

// mcpTemplateAuthor mirrors mcpSetAuthor but with a "tpl:" prefix so
// template writes are distinguishable from settings writes in audit.
func mcpTemplateAuthor(ctx context.Context) string {
	s := mcpSetAuthor(ctx)
	if s == "" {
		return "mcp:unknown"
	}
	return s
}
