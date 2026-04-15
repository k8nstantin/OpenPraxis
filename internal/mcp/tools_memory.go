package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"openloom/internal/memory"

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

	var mem *memory.Memory
	var err error
	if id != "" {
		mem, err = s.node.Index.GetByID(id)
		// If exact ID not found, try prefix match (short marker)
		if mem == nil && len(id) <= 8 {
			mem, err = s.node.Index.GetByIDPrefix(id)
		}
	} else {
		mem, err = s.node.Index.GetByPath(path)
	}
	if err != nil {
		return errResult("recall failed: %v", err), nil
	}
	if mem == nil {
		return textResult("Memory not found."), nil
	}

	if err := s.node.Index.TouchAccess(mem.ID); err != nil {
		slog.Warn("touch memory access failed", "error", err)
	}

	marker := mem.ID[:12]
	content := tierContent(*mem, tier)
	output := fmt.Sprintf("[%s] %s\nPath: %s\nType: %s\nSource: %s @ %s\nCreated: %s\nFull ID: %s\n\n%s",
		marker, mem.L0, mem.Path, mem.Type, mem.SourceAgent, mem.SourceNode, mem.CreatedAt, mem.ID, content)

	return textResult(output), nil
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
