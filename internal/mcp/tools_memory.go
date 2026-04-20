package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/memory"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) handleStore(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	content := argStr(a, "content")
	if content == "" {
		return errResult("content is required"), nil
	}

	path := argStr(a, "path")
	scope := argStr(a, "scope")
	project := argStr(a, "project")
	domain := argStr(a, "domain")
	memType := argStr(a, "type")

	mem, err := s.node.StoreMemory(ctx, content, path, memType, scope, project, domain, s.sessionSource(ctx), nil)
	if err != nil {
		return errResult("store failed: %v", err), nil
	}

	marker := mem.ID[:12]
	return textResult(fmt.Sprintf("Stored [%s]: %s\nPath: %s\nFull ID: %s", marker, mem.L0, mem.Path, mem.ID)), nil
}

func (s *Server) handleSearch(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	query := argStr(a, "query")
	if query == "" {
		return errResult("query is required"), nil
	}

	scope := argStr(a, "scope")
	project := argStr(a, "project")
	domain := argStr(a, "domain")
	tier := argStr(a, "tier")
	if tier == "" {
		tier = "l1"
	}

	limit := int(argFloat(a, "limit"))
	if limit <= 0 {
		limit = 5
	}

	results, err := s.node.SearchMemories(ctx, query, limit, scope, project, domain)
	if err != nil {
		return errResult("search failed: %v", err), nil
	}

	if len(results) == 0 {
		return textResult("No memories found."), nil
	}

	var output string
	for i, r := range results {
		marker := r.Memory.ID[:12]
		content := tierContent(r.Memory, tier)
		output += fmt.Sprintf("%d. [%s] (%.3f) %s\n   %s\n\n", i+1, marker, r.Score, r.Memory.Path, content)
	}

	return textResult(output), nil
}

func (s *Server) handleRecall(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	path := argStr(a, "path")
	id := argStr(a, "id")
	tier := argStr(a, "tier")
	if tier == "" {
		tier = "l2"
	}

	if path == "" && id == "" {
		return errResult("path or id is required"), nil
	}

	// Path prefix = directory listing
	if path != "" && memory.IsPathPrefix(path) {
		mems, err := s.node.Index.ListByPrefix(path, 50)
		if err != nil {
			return errResult("list failed: %v", err), nil
		}
		if len(mems) == 0 {
			return textResult(fmt.Sprintf("No memories under %s", path)), nil
		}
		var output string
		for _, m := range mems {
			output += fmt.Sprintf("- %s: %s\n", m.Path, m.L0)
		}
		return textResult(output), nil
	}

	if id == "" {
		// Pure path lookup (exact).
		mem, err := s.node.Index.GetByPath(path)
		if err != nil {
			return errResult("recall failed: %v", err), nil
		}
		if mem == nil {
			return textResult("Memory not found."), nil
		}
		return formatRecallHit(s, *mem, tier), nil
	}

	mem, ambiguous, err := s.resolveByID(ctx, id)
	if err != nil {
		return errResult("recall failed: %v", err), nil
	}
	if mem != nil {
		return formatRecallHit(s, *mem, tier), nil
	}
	if len(ambiguous) > 0 {
		return textResult(formatCandidates(id, ambiguous, false)), nil
	}

	// Rung 4: path fallback (in case caller put a path into the id slot).
	if looksPathy(id) {
		if pm, _ := s.node.Index.GetByPath(id); pm != nil {
			return formatRecallHit(s, *pm, tier), nil
		}
		if memory.IsPathPrefix(id) || strings.Contains(id, "/") {
			if mems, _ := s.node.Index.ListByPrefix(id, 10); len(mems) > 0 {
				return textResult(formatCandidates(id, mems, false)), nil
			}
		}
	}

	// Rung 5: semantic search fallback. Embedding may be unavailable — treat
	// errors as "no match" rather than bubbling them up to the caller.
	if results, serr := s.node.SearchMemories(ctx, id, 5, "", "", ""); serr == nil && len(results) > 0 {
		mems := make([]*memory.Memory, 0, len(results))
		for i := range results {
			m := results[i].Memory
			mems = append(mems, &m)
		}
		return textResult(formatCandidates(id, mems, true)), nil
	}

	return textResult("Memory not found."), nil
}

// resolveByID walks rungs 1-3 (exact id, id prefix, id substring). Returns a
// single match, or a candidate list when ambiguous. Both nil means no hit.
func (s *Server) resolveByID(ctx context.Context, id string) (*memory.Memory, []*memory.Memory, error) {
	// Rung 1: exact ID.
	if mem, err := s.node.Index.GetByID(id); err != nil {
		return nil, nil, err
	} else if mem != nil {
		return mem, nil, nil
	}

	// Rung 2: ID prefix (any length). Ambiguous => candidate list.
	mems, err := s.node.Index.GetByIDPrefixAll(id, 10)
	if err != nil {
		return nil, nil, err
	}
	if len(mems) == 1 {
		return mems[0], nil, nil
	}
	if len(mems) > 1 {
		return nil, mems, nil
	}

	// Rung 3: ID substring. Useful when the caller pasted a fragment spanning a
	// dash (e.g. "019daac8-cdb" where char 9 is "-" and rung-2 LIKE fails).
	sub, err := s.node.Index.FindByIDSubstring(id, 10)
	if err != nil {
		return nil, nil, err
	}
	if len(sub) == 1 {
		return sub[0], nil, nil
	}
	if len(sub) > 1 {
		return nil, sub, nil
	}
	return nil, nil, nil
}

func formatRecallHit(s *Server, mem memory.Memory, tier string) *mcplib.CallToolResult {
	if err := s.node.Index.TouchAccess(mem.ID); err != nil {
		slog.Warn("touch memory access failed", "error", err)
	}
	marker := mem.ID[:12]
	content := tierContent(mem, tier)
	return textResult(fmt.Sprintf("[%s] %s\nPath: %s\nType: %s\nSource: %s @ %s\nCreated: %s\nFull ID: %s\n\n%s",
		marker, mem.L0, mem.Path, mem.Type, mem.SourceAgent, mem.SourceNode, mem.CreatedAt, mem.ID, content))
}

func formatCandidates(query string, mems []*memory.Memory, viaSearch bool) string {
	var b strings.Builder
	if viaSearch {
		fmt.Fprintf(&b, "No id/path match for %q. Semantic search candidates:\n", query)
	} else {
		fmt.Fprintf(&b, "No exact match for %q. Closest candidates:\n", query)
	}
	for i, m := range mems {
		marker := m.ID
		if len(marker) > 12 {
			marker = marker[:12]
		}
		fmt.Fprintf(&b, "%d. [%s] %s — %s\n", i+1, marker, m.Path, m.L0)
	}
	b.WriteString("Run again with a longer prefix or full id.")
	return b.String()
}

func looksPathy(s string) bool {
	return strings.HasPrefix(s, "/") || strings.Contains(s, "/")
}

func (s *Server) handleList(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	tree, err := s.node.Index.Tree()
	if err != nil {
		return errResult("list failed: %v", err), nil
	}

	if len(tree) == 0 {
		return textResult("No memories stored yet."), nil
	}

	data, _ := json.MarshalIndent(tree, "", "  ")
	return textResult(string(data)), nil
}

func (s *Server) handleForget(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	if !argBool(a, "confirm") {
		return errResult("confirm must be true to delete"), nil
	}

	path := argStr(a, "path")
	id := argStr(a, "id")

	if path == "" && id == "" {
		return errResult("path or id is required"), nil
	}

	if id != "" {
		if err := s.node.DeleteMemory(id); err != nil {
			return errResult("delete failed: %v", err), nil
		}
		return textResult(fmt.Sprintf("Deleted memory %s", id)), nil
	}

	if memory.IsPathPrefix(path) {
		count, err := s.node.DeleteByPrefix(path)
		if err != nil {
			return errResult("delete failed: %v", err), nil
		}
		return textResult(fmt.Sprintf("Deleted %d memories under %s", count, path)), nil
	}

	// Exact path
	mem, err := s.node.Index.GetByPath(path)
	if err != nil {
		return errResult("lookup failed: %v", err), nil
	}
	if mem == nil {
		return textResult("Memory not found."), nil
	}
	if err := s.node.DeleteMemory(mem.ID); err != nil {
		return errResult("delete failed: %v", err), nil
	}
	return textResult(fmt.Sprintf("Deleted: %s", path)), nil
}

func (s *Server) handleStatus(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	count, _ := s.node.Index.Count()
	uptime := time.Since(s.node.StartedAt).Round(time.Second)

	status := map[string]any{
		"node":      s.node.PeerID(),
		"memories":  count,
		"agents":    s.tracker.Count(),
		"peers":     0, // TODO: from peer registry
		"uptime":    uptime.String(),
		"embedding": s.node.Config.Embedding.Model,
		"data_dir":  s.node.Config.Storage.DataDir,
	}

	data, _ := json.MarshalIndent(status, "", "  ")
	return textResult(string(data)), nil
}

func (s *Server) handlePeers(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	// TODO: wire to peer registry
	return textResult("No peers connected yet. Peer discovery will be enabled when the sync server starts."), nil
}

func tierContent(mem memory.Memory, tier string) string {
	switch tier {
	case "l0":
		return mem.L0
	case "l1":
		return mem.L1
	case "l2":
		return mem.L2
	default:
		return mem.L1
	}
}
