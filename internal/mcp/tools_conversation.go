package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"openloom/internal/conversation"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) handleConversationSave(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	turnsJSON := argStr(a, "turns_json")
	if turnsJSON == "" {
		return errResult("turns_json is required"), nil
	}

	var turns []conversation.Turn
	if err := json.Unmarshal([]byte(turnsJSON), &turns); err != nil {
		return errResult("invalid turns_json: %v", err), nil
	}
	if len(turns) == 0 {
		return errResult("turns cannot be empty"), nil
	}

	title := argStr(a, "title")
	agent := argStr(a, "agent")
	project := argStr(a, "project")

	conv, err := s.node.SaveConversation(ctx, title, agent, project, turns, nil)
	if err != nil {
		return errResult("save failed: %v", err), nil
	}

	return textResult(fmt.Sprintf("Saved conversation: %s\nID: %s\nTitle: %s\nTurns: %d",
		conv.ID, conv.ID, conv.Title, conv.TurnCount)), nil
}

func (s *Server) handleConversationSearch(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	query := argStr(a, "query")
	if query == "" {
		return errResult("query is required"), nil
	}

	agent := argStr(a, "agent")
	project := argStr(a, "project")
	limit := int(argFloat(a, "limit"))
	if limit <= 0 {
		limit = 5
	}

	results, err := s.node.SearchConversations(ctx, query, limit, agent, project)
	if err != nil {
		return errResult("search failed: %v", err), nil
	}

	if len(results) == 0 {
		return textResult("No conversations found."), nil
	}

	var output string
	for i, r := range results {
		c := r.Conversation
		output += fmt.Sprintf("%d. [%.3f] %s\n   Agent: %s | Turns: %d | %s\n   %s\n\n",
			i+1, r.Score, c.Title, c.Agent, c.TurnCount,
			c.CreatedAt.Format("2006-01-02 15:04"), truncate(c.Summary, 200))
	}

	return textResult(output), nil
}

func (s *Server) handleConversationList(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	agent := argStr(a, "agent")
	project := argStr(a, "project")
	limit := int(argFloat(a, "limit"))
	if limit <= 0 {
		limit = 20
	}

	convos, err := s.node.Conversations.List(agent, project, limit, 0)
	if err != nil {
		return errResult("list failed: %v", err), nil
	}

	if len(convos) == 0 {
		return textResult("No conversations saved yet."), nil
	}

	var output string
	for _, c := range convos {
		output += fmt.Sprintf("- [%s] %s (%s, %d turns, %s)\n",
			c.ID[:12], c.Title, c.Agent, c.TurnCount, c.CreatedAt.Format("2006-01-02 15:04"))
	}

	return textResult(output), nil
}

func (s *Server) handleConversationGet(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	id := argStr(a, "id")
	if id == "" {
		return errResult("id is required"), nil
	}

	conv, err := s.node.Conversations.GetByID(id)
	if err != nil {
		return errResult("get failed: %v", err), nil
	}
	if conv == nil {
		return textResult("Conversation not found."), nil
	}

	if err := s.node.Conversations.TouchAccess(id); err != nil {
		slog.Warn("touch conversation access failed", "error", err)
	}

	var output string
	output += fmt.Sprintf("Title: %s\nID: %s\nAgent: %s\nProject: %s\nDate: %s\nTurns: %d\n\n",
		conv.Title, conv.ID, conv.Agent, conv.Project,
		conv.CreatedAt.Format("2006-01-02 15:04:05"), conv.TurnCount)

	for _, t := range conv.Turns {
		role := "User"
		if t.Role == "assistant" {
			role = "Assistant"
		}
		output += fmt.Sprintf("--- %s ---\n%s\n\n", role, t.Content)
	}

	return textResult(output), nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
