// MCP tools for the Praxis Relationships store. Six tools mirror the
// Go API surface (Create / Remove / ListOutgoing / ListIncoming /
// History / Walk) so any agent can manage the unified edge graph from
// a Claude Code / Cursor / Codex / etc. session.
//
// Naming convention: rel_* prefix matches the short-form pattern used
// by memory_*, task_*, manifest_*, product_*. Discoverable via
// tools/list.
//
// All UUIDs MUST be full 36-char (visceral rule 14). Short markers are
// accepted for convenience on src_id / dst_id (the underlying store
// stores whatever is passed; resolution to full UUIDs is the caller's
// responsibility for now — PR/M2 may add a marker→UUID resolver here).
package mcp

import (
	"context"
	"encoding/json"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/k8nstantin/OpenPraxis/internal/relationships"
)

// jsonResult marshals v as pretty JSON and wraps it in a textResult.
// Per the agent-as-primary-reader framing (decision comment 019dc450-483a
// on the Praxis Git product), every read tool returns structured JSON
// so downstream agents skip the prose-parser. Humans inspecting via
// `mcp tools/call` get pretty-printed JSON which is still legible.
func jsonResult(v any) *mcplib.CallToolResult {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errResult("json marshal: %v", err)
	}
	return textResult(string(b))
}

// registerRelationshipsTools wires the six rel_* MCP tools onto the
// server. Called from server.go's registerAll() (similar to the other
// register*Tools methods). Must run AFTER s.node is set so the handlers
// can dispatch into n.Relationships.
func (s *Server) registerRelationshipsTools() {
	s.mcp.AddTool(
		mcplib.NewTool("rel_create",
			mcplib.WithDescription(
				"Create or replace the current edge between two entities (product / manifest / task). "+
					"If a current edge already exists for (src_id, dst_id, kind), it is closed (valid_to set) "+
					"and a new current row is inserted in one transaction. Rows are NEVER deleted — every "+
					"mutation is preserved in the SCD-2 audit trail.",
			),
			mcplib.WithString("src_kind", mcplib.Required(), mcplib.Description("Source entity kind: product | manifest | task")),
			mcplib.WithString("src_id", mcplib.Required(), mcplib.Description("Source full UUID")),
			mcplib.WithString("dst_kind", mcplib.Required(), mcplib.Description("Destination entity kind: product | manifest | task")),
			mcplib.WithString("dst_id", mcplib.Required(), mcplib.Description("Destination full UUID")),
			mcplib.WithString("kind", mcplib.Required(), mcplib.Description("Edge kind: owns | depends_on | reviews | links_to")),
			mcplib.WithString("metadata", mcplib.Description("Optional JSON metadata blob")),
			mcplib.WithString("reason", mcplib.Description("Free-text why — populates the audit trail")),
		),
		s.handleRelCreate,
	)

	s.mcp.AddTool(
		mcplib.NewTool("rel_remove",
			mcplib.WithDescription(
				"Close the current edge for (src_id, dst_id, kind) by setting valid_to to now. "+
					"Does NOT insert a replacement; the edge stops existing as of now. Idempotent — "+
					"no current row is a no-op success. The closing row's created_by and reason "+
					"are overwritten with the close attribution so the audit trail captures who/why.",
			),
			mcplib.WithString("src_id", mcplib.Required(), mcplib.Description("Source full UUID")),
			mcplib.WithString("dst_id", mcplib.Required(), mcplib.Description("Destination full UUID")),
			mcplib.WithString("kind", mcplib.Required(), mcplib.Description("Edge kind to close")),
			mcplib.WithString("reason", mcplib.Description("Why this edge is being closed — for audit")),
		),
		s.handleRelRemove,
	)

	s.mcp.AddTool(
		mcplib.NewTool("rel_list_outgoing",
			mcplib.WithDescription(
				"List edges leaving src_id. By default returns CURRENT (valid_to='') edges only. "+
					"Pass as_of=ISO8601 for time-travel ('what edges left this node at past time T?'). "+
					"Returns JSON array of edges. Hits idx_rel_src_current on the hot path.",
			),
			mcplib.WithString("src_id", mcplib.Required(), mcplib.Description("Source full UUID")),
			mcplib.WithString("kind", mcplib.Description("Edge kind filter (omit for all kinds)")),
			mcplib.WithString("as_of", mcplib.Description("ISO8601 timestamp for time-travel; omit for current state")),
		),
		s.handleRelListOutgoing,
	)

	s.mcp.AddTool(
		mcplib.NewTool("rel_list_incoming",
			mcplib.WithDescription(
				"List edges arriving at dst_id. Reverse-direction lookup. Pass as_of for time-travel. "+
					"Returns JSON array.",
			),
			mcplib.WithString("dst_id", mcplib.Required(), mcplib.Description("Destination full UUID")),
			mcplib.WithString("kind", mcplib.Description("Edge kind filter (omit for all kinds)")),
			mcplib.WithString("as_of", mcplib.Description("ISO8601 timestamp for time-travel; omit for current state")),
		),
		s.handleRelListIncoming,
	)

	s.mcp.AddTool(
		mcplib.NewTool("rel_history",
			mcplib.WithDescription(
				"Return every version of one specific edge tuple chronologically (oldest first). "+
					"Includes both closed historical rows AND the current row (if any) at the tail. "+
					"Use to answer 'when was this edge added/changed/removed and by whom?'",
			),
			mcplib.WithString("src_id", mcplib.Required(), mcplib.Description("Source full UUID")),
			mcplib.WithString("dst_id", mcplib.Required(), mcplib.Description("Destination full UUID")),
			mcplib.WithString("kind", mcplib.Required(), mcplib.Description("Edge kind")),
		),
		s.handleRelHistory,
	)

	s.mcp.AddTool(
		mcplib.NewTool("rel_walk",
			mcplib.WithDescription(
				"Recursive DAG walk from a root entity. By default follows CURRENT outgoing edges; "+
					"pass as_of for time-travel walks. Returns JSON array of {id, kind, via_kind, via_src, depth}. "+
					"Diamond-shape paths dedupe to one node per id (shortest path wins). "+
					"max_depth=0 returns root only; default 100; >100 clamped.",
			),
			mcplib.WithString("root_id", mcplib.Required(), mcplib.Description("Root full UUID")),
			mcplib.WithString("root_kind", mcplib.Required(), mcplib.Description("Root entity kind")),
			mcplib.WithString("edge_kinds", mcplib.Description("Comma-separated edge kinds to follow (omit for all)")),
			mcplib.WithNumber("max_depth", mcplib.Description("Hop ceiling. 0 = root only. Default 100; >100 clamped.")),
			mcplib.WithString("as_of", mcplib.Description("ISO8601 timestamp for time-travel; omit for current state")),
		),
		s.handleRelWalk,
	)

	s.mcp.AddTool(
		mcplib.NewTool("rel_get",
			mcplib.WithDescription(
				"Fetch the CURRENT edge for (src_id, dst_id, kind) if one exists. "+
					"Returns JSON {found, edge}. Cheaper than rel_list_outgoing + filter when "+
					"you only need to check one specific edge. Returns {found:false} cleanly "+
					"if no current edge exists — not an error.",
			),
			mcplib.WithString("src_id", mcplib.Required(), mcplib.Description("Source full UUID")),
			mcplib.WithString("dst_id", mcplib.Required(), mcplib.Description("Destination full UUID")),
			mcplib.WithString("kind", mcplib.Required(), mcplib.Description("Edge kind")),
		),
		s.handleRelGet,
	)

	s.mcp.AddTool(
		mcplib.NewTool("rel_health",
			mcplib.WithDescription(
				"Return current edge count + total rows (with history) for the relationships table. "+
					"Returns JSON {current_edges, total_rows, table_exists}. Cheap probe for dashboard "+
					"overview / ops health checks.",
			),
		),
		s.handleRelHealth,
	)

	s.mcp.AddTool(
		mcplib.NewTool("rel_backfill",
			mcplib.WithDescription(
				"INTERNAL — for migration paths only (PR/M2). Inserts a single row with caller-controlled "+
					"valid_from AND valid_to (including fully-closed historical rows from legacy dep tables). "+
					"Bypasses Create's close-then-insert dance. Do NOT call from application code; the SCD-2 "+
					"invariant (one current row per edge tuple) is the caller's responsibility.",
			),
			mcplib.WithString("src_kind", mcplib.Required(), mcplib.Description("Source entity kind")),
			mcplib.WithString("src_id", mcplib.Required(), mcplib.Description("Source full UUID")),
			mcplib.WithString("dst_kind", mcplib.Required(), mcplib.Description("Destination entity kind")),
			mcplib.WithString("dst_id", mcplib.Required(), mcplib.Description("Destination full UUID")),
			mcplib.WithString("kind", mcplib.Required(), mcplib.Description("Edge kind")),
			mcplib.WithString("valid_from", mcplib.Required(), mcplib.Description("Historical ISO8601 — when the edge became active")),
			mcplib.WithString("valid_to", mcplib.Description("ISO8601 if closed; omit for active row")),
			mcplib.WithString("metadata", mcplib.Description("Optional JSON metadata blob")),
			mcplib.WithString("reason", mcplib.Description("Migration reason e.g. 'PR/M2 from task_dependency'")),
		),
		s.handleRelBackfill,
	)
}

// ─── Handlers ───────────────────────────────────────────────────────

// All handlers below return JSON via jsonResult() so downstream agents
// skip prose-parsing. Per the agent-as-primary-reader framing
// (decision comment 019dc450-483a on Praxis Git), the WRITER pays the
// structuring cost ONCE so the N reader-agents don't pay parse cost N
// times. textResult is reserved for write-ack messages where there's
// no meaningful structured payload.

func (s *Server) handleRelCreate(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	e := relationships.Edge{
		SrcKind:   argStr(a, "src_kind"),
		SrcID:     argStr(a, "src_id"),
		DstKind:   argStr(a, "dst_kind"),
		DstID:     argStr(a, "dst_id"),
		Kind:      argStr(a, "kind"),
		Metadata:  argStr(a, "metadata"),
		CreatedBy: s.sessionSource(ctx),
		Reason:    argStr(a, "reason"),
	}
	if err := s.node.Relationships.Create(ctx, e); err != nil {
		return errResult("rel_create: %v", err), nil
	}
	return jsonResult(map[string]any{
		"ok":       true,
		"src_kind": e.SrcKind, "src_id": e.SrcID,
		"dst_kind": e.DstKind, "dst_id": e.DstID,
		"kind": e.Kind,
	}), nil
}

func (s *Server) handleRelRemove(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	srcID := argStr(a, "src_id")
	dstID := argStr(a, "dst_id")
	kind := argStr(a, "kind")
	reason := argStr(a, "reason")
	if err := s.node.Relationships.Remove(ctx, srcID, dstID, kind, s.sessionSource(ctx), reason); err != nil {
		return errResult("rel_remove: %v", err), nil
	}
	return jsonResult(map[string]any{
		"ok":     true,
		"src_id": srcID, "dst_id": dstID, "kind": kind,
	}), nil
}

func (s *Server) handleRelListOutgoing(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	srcID := argStr(a, "src_id")
	kind := argStr(a, "kind")
	asOf := argStr(a, "as_of")

	var (
		edges []relationships.Edge
		err   error
	)
	if asOf == "" {
		edges, err = s.node.Relationships.ListOutgoing(ctx, srcID, kind)
	} else {
		edges, err = s.node.Relationships.ListOutgoingAt(ctx, srcID, kind, asOf)
	}
	if err != nil {
		return errResult("rel_list_outgoing: %v", err), nil
	}
	return jsonResult(map[string]any{
		"src_id": srcID,
		"kind":   kind,
		"as_of":  asOf,
		"count":  len(edges),
		"edges":  edges,
	}), nil
}

func (s *Server) handleRelListIncoming(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	dstID := argStr(a, "dst_id")
	kind := argStr(a, "kind")
	asOf := argStr(a, "as_of")

	var (
		edges []relationships.Edge
		err   error
	)
	if asOf == "" {
		edges, err = s.node.Relationships.ListIncoming(ctx, dstID, kind)
	} else {
		edges, err = s.node.Relationships.ListIncomingAt(ctx, dstID, kind, asOf)
	}
	if err != nil {
		return errResult("rel_list_incoming: %v", err), nil
	}
	return jsonResult(map[string]any{
		"dst_id": dstID,
		"kind":   kind,
		"as_of":  asOf,
		"count":  len(edges),
		"edges":  edges,
	}), nil
}

func (s *Server) handleRelHistory(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	srcID := argStr(a, "src_id")
	dstID := argStr(a, "dst_id")
	kind := argStr(a, "kind")
	edges, err := s.node.Relationships.History(ctx, srcID, dstID, kind)
	if err != nil {
		return errResult("rel_history: %v", err), nil
	}
	return jsonResult(map[string]any{
		"src_id": srcID, "dst_id": dstID, "kind": kind,
		"versions": len(edges),
		"history":  edges,
	}), nil
}

func (s *Server) handleRelWalk(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	rootID := argStr(a, "root_id")
	rootKind := argStr(a, "root_kind")
	asOf := argStr(a, "as_of")

	// edge_kinds is a comma-separated list (or empty for "all kinds")
	var edgeKinds []string
	if raw := argStr(a, "edge_kinds"); raw != "" {
		edgeKinds = splitCSV(raw)
	}

	// Align with the store's clamp semantic (C2 from self-review):
	// 0 means "root only," negative or > MaxWalkDepth → MaxWalkDepth.
	// Previously the MCP layer forced 0 → 100, hiding the root-only
	// path the store supports. Now we pass through unchanged and let
	// the store clamp.
	maxDepth := int(argFloat(a, "max_depth"))

	var (
		rows []relationships.WalkRow
		err  error
	)
	if asOf == "" {
		rows, err = s.node.Relationships.Walk(ctx, rootID, rootKind, edgeKinds, maxDepth)
	} else {
		rows, err = s.node.Relationships.WalkAt(ctx, rootID, rootKind, edgeKinds, maxDepth, asOf)
	}
	if err != nil {
		return errResult("rel_walk: %v", err), nil
	}
	return jsonResult(map[string]any{
		"root_id":   rootID,
		"root_kind": rootKind,
		"as_of":     asOf,
		"count":     len(rows),
		"nodes":     rows,
	}), nil
}

func (s *Server) handleRelGet(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	srcID := argStr(a, "src_id")
	dstID := argStr(a, "dst_id")
	kind := argStr(a, "kind")
	edge, found, err := s.node.Relationships.Get(ctx, srcID, dstID, kind)
	if err != nil {
		return errResult("rel_get: %v", err), nil
	}
	if !found {
		return jsonResult(map[string]any{"found": false}), nil
	}
	return jsonResult(map[string]any{"found": true, "edge": edge}), nil
}

func (s *Server) handleRelHealth(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	h, err := s.node.Relationships.Health(ctx)
	if err != nil {
		return errResult("rel_health: %v", err), nil
	}
	return jsonResult(map[string]any{
		"current_edges": h.CurrentEdges,
		"total_rows":    h.TotalRows,
		"table_exists":  h.TableExists,
	}), nil
}

func (s *Server) handleRelBackfill(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	e := relationships.Edge{
		SrcKind:   argStr(a, "src_kind"),
		SrcID:     argStr(a, "src_id"),
		DstKind:   argStr(a, "dst_kind"),
		DstID:     argStr(a, "dst_id"),
		Kind:      argStr(a, "kind"),
		Metadata:  argStr(a, "metadata"),
		ValidFrom: argStr(a, "valid_from"),
		ValidTo:   argStr(a, "valid_to"),
		CreatedBy: s.sessionSource(ctx),
		Reason:    argStr(a, "reason"),
	}
	if err := s.node.Relationships.BackfillRow(ctx, e); err != nil {
		return errResult("rel_backfill: %v", err), nil
	}
	return jsonResult(map[string]any{
		"ok":         true,
		"src_id":     e.SrcID,
		"dst_id":     e.DstID,
		"kind":       e.Kind,
		"valid_from": e.ValidFrom,
		"valid_to":   e.ValidTo,
	}), nil
}
