package mcp

import (
	"context"
	"fmt"
	"log/slog"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) registerVisceralTools() {
	s.mcp.AddTool(
		mcplib.NewTool("visceral_rules",
			mcplib.WithDescription("MUST be called first on every session start. Returns mandatory operating rules that override all other behavior. These are non-negotiable constraints set by the user."),
		),
		s.handleVisceralRules,
	)

	s.mcp.AddTool(
		mcplib.NewTool("visceral_confirm",
			mcplib.WithDescription("MUST be called immediately after visceral_rules. Confirms you have read and will follow all rules. Include the count of rules acknowledged."),
			mcplib.WithNumber("rules_count", mcplib.Required(), mcplib.Description("Number of visceral rules acknowledged")),
		),
		s.handleVisceralConfirm,
	)

	s.mcp.AddTool(
		mcplib.NewTool("visceral_set",
			mcplib.WithDescription("Add or update a visceral rule. Visceral rules are mandatory operating constraints that every agent session must follow. Use this when the user says 'always do X' or 'never do Y'."),
			mcplib.WithString("rule", mcplib.Required(), mcplib.Description("The rule text. Be precise and unambiguous.")),
			mcplib.WithString("id", mcplib.Description("Rule ID to update. Omit to create new.")),
		),
		s.handleVisceralSet,
	)

	s.mcp.AddTool(
		mcplib.NewTool("visceral_remove",
			mcplib.WithDescription("Remove a visceral rule by ID."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("Rule ID (full UUID)")),
		),
		s.handleVisceralRemove,
	)
}

func (s *Server) handleVisceralRules(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	rules, err := s.node.Index.ListByType("visceral", 100)
	if err != nil {
		return errResult("load visceral rules: %v", err), nil
	}

	if len(rules) == 0 {
		return textResult("No visceral rules set. The user has not defined any mandatory operating constraints yet."), nil
	}

	var output string
	output += fmt.Sprintf("=== VISCERAL RULES (%d) ===\nThese are MANDATORY. Follow every rule without exception.\n\n", len(rules))
	for i, r := range rules {
		output += fmt.Sprintf("%d. [%s] %s\n", i+1, r.ID, r.L2)
	}
	output += "\n=== END VISCERAL RULES ==="

	return textResult(output), nil
}

func (s *Server) handleVisceralConfirm(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	count := int(argFloat(a, "rules_count"))

	source := s.sessionSource(ctx)
	if err := s.node.Actions.RecordConfirmation(source, count); err != nil {
		slog.Warn("record visceral confirmation failed", "error", err)
	}

	slog.Info("visceral confirmed", "source", source, "rules_count", count)

	return textResult(fmt.Sprintf("Confirmed: %d visceral rules acknowledged. Proceed with your task.", count)), nil
}

func (s *Server) handleVisceralSet(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	rule := argStr(a, "rule")
	if rule == "" {
		return errResult("rule is required"), nil
	}

	id := argStr(a, "id")
	if id != "" {
		// Update existing — delete old first
		existing, _ := s.node.Index.GetByID(id)
		if existing == nil {
			existing, _ = s.node.Index.GetByIDPrefix(id)
		}
		if existing != nil {
			if err := s.node.DeleteMemory(existing.ID); err != nil {
				slog.Warn("delete existing visceral rule failed", "error", err)
			}
		}
	}

	mem, err := s.node.StoreMemory(ctx, rule, "/visceral/rules/", "visceral", "global", "", "visceral", s.sessionSource(ctx), nil)
	if err != nil {
		return errResult("store visceral rule: %v", err), nil
	}

	return textResult(fmt.Sprintf("Visceral rule set [%s]: %s", mem.ID, rule)), nil
}

func (s *Server) handleVisceralRemove(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	id := argStr(a, "id")
	if id == "" {
		return errResult("id is required"), nil
	}

	mem, _ := s.node.Index.GetByID(id)
	if mem == nil {
		mem, _ = s.node.Index.GetByIDPrefix(id)
	}
	if mem == nil {
		return textResult("Rule not found."), nil
	}
	if mem.Type != "visceral" {
		return errResult("that memory is not a visceral rule"), nil
	}

	if err := s.node.DeleteMemory(mem.ID); err != nil {
		return errResult("delete: %v", err), nil
	}

	return textResult(fmt.Sprintf("Visceral rule [%s] removed.", mem.ID[:12])), nil
}
