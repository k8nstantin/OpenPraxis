package mcp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/comments"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// CommentErrorCode is the stable machine-readable string returned on the tool
// error envelope's "code" field. The HTTP layer (M2-T5) reuses the same codes
// so MCP + HTTP clients can share error-handling logic.
const (
	CodeUnknownTargetType = "unknown_target_type"
	CodeEmptyTargetID     = "empty_target_id"
	CodeUnknownType       = "unknown_type"
	CodeEmptyAuthor       = "empty_author"
	CodeEmptyBody         = "empty_body"
	CodeNotFound          = "not_found"
)

// commentDTO is the wire shape returned by comment_add / comment_edit. Wraps
// comments.Comment with RFC3339 timestamps so downstream UIs don't have to
// re-derive them from unix seconds.
type commentDTO struct {
	comments.Comment
	CreatedAtISO string `json:"created_at_iso"`
	UpdatedAtISO string `json:"updated_at_iso,omitempty"`
}

func toDTO(c comments.Comment) commentDTO {
	d := commentDTO{Comment: c, CreatedAtISO: c.CreatedAt.UTC().Format(time.RFC3339)}
	if c.UpdatedAt != nil {
		d.UpdatedAtISO = c.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return d
}

func toDTOList(in []comments.Comment) []commentDTO {
	out := make([]commentDTO, 0, len(in))
	for _, c := range in {
		out = append(out, toDTO(c))
	}
	return out
}

// commentTypesInline renders the 6 (or more) types from the Registry as a
// bullet-list for tool descriptions. Generated at registration time so adding
// a new type in internal/comments flows through automatically.
func commentTypesInline() string {
	reg := comments.Registry()
	parts := make([]string, 0, len(reg))
	for _, info := range reg {
		parts = append(parts, fmt.Sprintf("%q (%s)", string(info.Type), info.Label))
	}
	return strings.Join(parts, ", ")
}

func (s *Server) registerCommentsTools() {
	typesList := commentTypesInline()

	s.mcp.AddTool(
		mcplib.NewTool("comment_add",
			mcplib.WithDescription("Attach a comment to a product, manifest, or task. Valid types: "+typesList+". Returns the stored comment with RFC3339 timestamps."),
			mcplib.WithString("target_type", mcplib.Required(), mcplib.Description("One of 'product', 'manifest', or 'task'")),
			mcplib.WithString("target_id", mcplib.Required(), mcplib.Description("Entity id the comment attaches to")),
			mcplib.WithString("author", mcplib.Required(), mcplib.Description("Author identity (agent, user, or bot)")),
			mcplib.WithString("type", mcplib.Required(), mcplib.Description("Comment type; must be one of: "+typesList)),
			mcplib.WithString("body", mcplib.Required(), mcplib.Description("Comment body (non-empty after trim)")),
		),
		s.handleCommentAdd,
	)

	s.mcp.AddTool(
		mcplib.NewTool("comment_list",
			mcplib.WithDescription("List comments for a (target_type, target_id) pair, newest first. Optional type_filter narrows to one of: "+typesList+". Server caps the page at 1000."),
			mcplib.WithString("target_type", mcplib.Required(), mcplib.Description("One of 'product', 'manifest', or 'task'")),
			mcplib.WithString("target_id", mcplib.Required(), mcplib.Description("Entity id the comments attach to")),
			mcplib.WithNumber("limit", mcplib.Description("Max rows to return. Defaults to 100, capped at 1000.")),
			mcplib.WithString("type_filter", mcplib.Description("Optional single comment type to filter to; empty string means all types")),
		),
		s.handleCommentList,
	)

	s.mcp.AddTool(
		mcplib.NewTool("comment_edit",
			mcplib.WithDescription("Replace a comment's body and bump updated_at. Only body is mutable — target, author, and type are immutable."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("Comment id to edit")),
			mcplib.WithString("body", mcplib.Required(), mcplib.Description("New body (non-empty after trim)")),
		),
		s.handleCommentEdit,
	)

	s.mcp.AddTool(
		mcplib.NewTool("comment_delete",
			mcplib.WithDescription("Hard-delete a comment. Idempotent — deleting a missing id still returns ok."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("Comment id to delete")),
		),
		s.handleCommentDelete,
	)
}

// -------- handlers -----------------------------------------------------------

func (s *Server) handleCommentAdd(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	target := comments.TargetType(argStr(a, "target_type"))
	targetID := argStr(a, "target_id")
	author := argStr(a, "author")
	cType := comments.CommentType(argStr(a, "type"))
	body := argStr(a, "body")

	if verr := comments.ValidateAdd(target, targetID, author, cType, body); verr != nil {
		return validationErrResult(verr), nil
	}
	if s.node.Comments == nil {
		return errResult("comment_add: comments store not configured"), nil
	}
	c, err := s.node.Comments.Add(ctx, target, targetID, author, cType, body)
	if err != nil {
		return errResult("comment_add: %v", err), nil
	}
	_ = mcpSetAuthor(ctx) // reserved for future updated_by column; MCP author = c.Author
	return jsonOrError(toDTO(c))
}

func (s *Server) handleCommentList(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	target := comments.TargetType(argStr(a, "target_type"))
	targetID := argStr(a, "target_id")

	if !comments.IsValidTargetType(string(target)) {
		return codedErrResult(CodeUnknownTargetType, comments.ErrUnknownTargetType.Error()), nil
	}
	if targetID == "" {
		return codedErrResult(CodeEmptyTargetID, comments.ErrEmptyTargetID.Error()), nil
	}

	limit := int(argFloat(a, "limit"))
	var typeFilter *comments.CommentType
	if tf := argStr(a, "type_filter"); tf != "" {
		if !comments.IsValidCommentType(tf) {
			return codedErrResult(CodeUnknownType, comments.ErrUnknownCommentType.Error()), nil
		}
		ct := comments.CommentType(tf)
		typeFilter = &ct
	}

	if s.node.Comments == nil {
		return errResult("comment_list: comments store not configured"), nil
	}
	rows, err := s.node.Comments.List(ctx, target, targetID, limit, typeFilter)
	if err != nil {
		return errResult("comment_list: %v", err), nil
	}
	return jsonOrError(map[string]any{"comments": toDTOList(rows)})
}

func (s *Server) handleCommentEdit(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	id := argStr(a, "id")
	body := argStr(a, "body")

	if id == "" {
		return codedErrResult(CodeNotFound, "comments: id is required"), nil
	}
	if verr := comments.ValidateEdit(body); verr != nil {
		return validationErrResult(verr), nil
	}
	if s.node.Comments == nil {
		return errResult("comment_edit: comments store not configured"), nil
	}
	if err := s.node.Comments.Edit(ctx, id, body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return codedErrResult(CodeNotFound, "comments: id not found"), nil
		}
		return errResult("comment_edit: %v", err), nil
	}
	c, err := s.node.Comments.Get(ctx, id)
	if err != nil {
		return errResult("comment_edit: readback: %v", err), nil
	}
	return jsonOrError(toDTO(c))
}

func (s *Server) handleCommentDelete(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	id := argStr(a, "id")
	if id == "" {
		return codedErrResult(CodeNotFound, "comments: id is required"), nil
	}
	if s.node.Comments == nil {
		return errResult("comment_delete: comments store not configured"), nil
	}
	if err := s.node.Comments.Delete(ctx, id); err != nil {
		return errResult("comment_delete: %v", err), nil
	}
	return jsonOrError(map[string]any{"ok": true})
}

// -------- error helpers ------------------------------------------------------

// validationErrResult translates comments validation sentinels to an MCP tool
// error result with a machine-readable code embedded in the text payload.
func validationErrResult(err error) *mcplib.CallToolResult {
	switch {
	case errors.Is(err, comments.ErrUnknownTargetType):
		return codedErrResult(CodeUnknownTargetType, err.Error())
	case errors.Is(err, comments.ErrEmptyTargetID):
		return codedErrResult(CodeEmptyTargetID, err.Error())
	case errors.Is(err, comments.ErrUnknownCommentType):
		return codedErrResult(CodeUnknownType, err.Error())
	case errors.Is(err, comments.ErrEmptyAuthor):
		return codedErrResult(CodeEmptyAuthor, err.Error())
	case errors.Is(err, comments.ErrEmptyBody):
		return codedErrResult(CodeEmptyBody, err.Error())
	default:
		return errResult("comments: %v", err)
	}
}

// codedErrResult returns an isError=true CallToolResult whose text payload is a
// single-line JSON object `{"code":"...","message":"..."}`. Tool error
// envelopes in mcp-go don't carry structured fields, so we encode the code
// into the message — callers parse the JSON to branch on the code.
func codedErrResult(code, message string) *mcplib.CallToolResult {
	// Avoid json import churn — message is already human-friendly; code is a
	// fixed identifier from this file's const block, so simple concatenation
	// is safe (no unescaped quotes possible).
	payload := fmt.Sprintf(`{"code":%q,"message":%q}`, code, message)
	return mcplib.NewToolResultError(payload)
}
