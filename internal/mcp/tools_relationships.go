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
	"fmt"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/k8nstantin/OpenPraxis/internal/relationships"
)

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
				"List CURRENT edges leaving src_id. valid_to='' rows only. Optionally filter by kind. "+
					"Hits the partial index idx_rel_src_current — O(log n + matches) regardless of how "+
					"much history accumulates.",
			),
			mcplib.WithString("src_id", mcplib.Required(), mcplib.Description("Source full UUID")),
			mcplib.WithString("kind", mcplib.Description("Edge kind filter (omit for all kinds)")),
		),
		s.handleRelListOutgoing,
	)

	s.mcp.AddTool(
		mcplib.NewTool("rel_list_incoming",
			mcplib.WithDescription(
				"List CURRENT edges arriving at dst_id. Reverse-direction lookup: 'who depends on this manifest?', "+
					"'which products own this entity?', etc. Hits idx_rel_dst_current.",
			),
			mcplib.WithString("dst_id", mcplib.Required(), mcplib.Description("Destination full UUID")),
			mcplib.WithString("kind", mcplib.Description("Edge kind filter (omit for all kinds)")),
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
				"Recursive DAG walk from a root entity. Follows outgoing edges (CURRENT only — valid_to='') "+
					"up to max_depth hops, returning every reachable node with the edge kind that led to it. "+
					"This is the headline reader: ONE query replaces what used to be 6+ table joins for "+
					"a hierarchy walk. Diamond-shape paths dedupe to one node per id (shortest path wins).",
			),
			mcplib.WithString("root_id", mcplib.Required(), mcplib.Description("Root full UUID")),
			mcplib.WithString("root_kind", mcplib.Required(), mcplib.Description("Root entity kind")),
			mcplib.WithString("edge_kinds", mcplib.Description("Comma-separated edge kinds to follow (omit for all)")),
			mcplib.WithNumber("max_depth", mcplib.Description("Hop ceiling. Default 100; clamped to [0, 100].")),
		),
		s.handleRelWalk,
	)
}

// ─── Handlers ───────────────────────────────────────────────────────

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
	return textResult(fmt.Sprintf(
		"Edge created/replaced: %s(%s) → %s(%s) kind=%s",
		e.SrcKind, shortID(e.SrcID), e.DstKind, shortID(e.DstID), e.Kind,
	)), nil
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
	return textResult(fmt.Sprintf(
		"Edge closed (idempotent): %s → %s kind=%s",
		shortID(srcID), shortID(dstID), kind,
	)), nil
}

func (s *Server) handleRelListOutgoing(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	edges, err := s.node.Relationships.ListOutgoing(ctx, argStr(a, "src_id"), argStr(a, "kind"))
	if err != nil {
		return errResult("rel_list_outgoing: %v", err), nil
	}
	return textResult(formatEdgeList(edges, "outgoing")), nil
}

func (s *Server) handleRelListIncoming(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	edges, err := s.node.Relationships.ListIncoming(ctx, argStr(a, "dst_id"), argStr(a, "kind"))
	if err != nil {
		return errResult("rel_list_incoming: %v", err), nil
	}
	return textResult(formatEdgeList(edges, "incoming")), nil
}

func (s *Server) handleRelHistory(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	edges, err := s.node.Relationships.History(ctx, argStr(a, "src_id"), argStr(a, "dst_id"), argStr(a, "kind"))
	if err != nil {
		return errResult("rel_history: %v", err), nil
	}
	if len(edges) == 0 {
		return textResult("No history for this edge tuple."), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "History (%d versions, oldest first):\n\n", len(edges))
	for i, e := range edges {
		state := "CURRENT"
		if e.ValidTo != "" {
			state = "closed at " + e.ValidTo
		}
		fmt.Fprintf(&b, "%d. valid_from=%s [%s]\n   by=%s reason=%q\n\n",
			i+1, e.ValidFrom, state, e.CreatedBy, e.Reason)
	}
	return textResult(b.String()), nil
}

func (s *Server) handleRelWalk(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	rootID := argStr(a, "root_id")
	rootKind := argStr(a, "root_kind")

	// edge_kinds is a comma-separated list (or empty for "all kinds")
	var edgeKinds []string
	if raw := argStr(a, "edge_kinds"); raw != "" {
		edgeKinds = splitCSV(raw)
	}

	// Numeric args come in as float64 (JSON number); cast to int and
	// clamp non-positive to the store's default of 100.
	maxDepth := int(argFloat(a, "max_depth"))
	if maxDepth <= 0 {
		maxDepth = 100
	}

	rows, err := s.node.Relationships.Walk(ctx, rootID, rootKind, edgeKinds, maxDepth)
	if err != nil {
		return errResult("rel_walk: %v", err), nil
	}
	if len(rows) == 0 {
		return textResult("Walk returned no nodes (root may not exist)."), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Walk from %s (%s), %d node(s):\n\n", shortID(rootID), rootKind, len(rows))
	for _, r := range rows {
		via := "(root)"
		if r.ViaSrc != "" {
			via = fmt.Sprintf("via %s from %s", r.ViaKind, shortID(r.ViaSrc))
		}
		// Indent by depth so the walk reads as a tree at a glance.
		indent := strings.Repeat("  ", r.Depth)
		fmt.Fprintf(&b, "%s[d=%d] %s %s %s\n", indent, r.Depth, r.Kind, shortID(r.ID), via)
	}
	return textResult(b.String()), nil
}

// formatEdgeList renders a list of current edges as human-readable text.
// Used by rel_list_outgoing and rel_list_incoming. Direction is for the
// header label; the row format is symmetric.
func formatEdgeList(edges []relationships.Edge, direction string) string {
	if len(edges) == 0 {
		return fmt.Sprintf("No %s edges (current state).", direction)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d %s edge(s):\n\n", len(edges), direction)
	for _, e := range edges {
		fmt.Fprintf(&b, "  %s(%s) → %s(%s) kind=%s\n",
			e.SrcKind, shortID(e.SrcID), e.DstKind, shortID(e.DstID), e.Kind)
		if e.Reason != "" {
			fmt.Fprintf(&b, "    reason: %s\n", e.Reason)
		}
	}
	return b.String()
}

// shortID returns the first 12 chars of an ID for terse rendering. Full
// UUID stays in the row; the human-facing list just shows the marker.
func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}
