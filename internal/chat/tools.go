package chat

import (
	"context"
	"encoding/json"
	"fmt"
)

// ChatTools provides tool definitions and execution for chat function calling.
type ChatTools struct {
	bridge NodeBridge
}

func NewChatTools(bridge NodeBridge) *ChatTools {
	return &ChatTools{bridge: bridge}
}

// Definitions returns the tool definitions to send to the LLM.
func (ct *ChatTools) Definitions() []ToolDef {
	return []ToolDef{
		{
			Name:        "memory_search",
			Description: "Semantic search across OpenPraxis memories. Returns ranked results with relevance scores.",
			InputSchema: `{"type":"object","properties":{"query":{"type":"string","description":"Natural language search query"},"limit":{"type":"integer","description":"Max results (default 5)"}},"required":["query"]}`,
		},
		{
			Name:        "memory_recall",
			Description: "Get a specific memory by its full UUID.",
			InputSchema: `{"type":"object","properties":{"id":{"type":"string","description":"Memory full UUID"}},"required":["id"]}`,
		},
		{
			Name:        "manifest_list",
			Description: "List manifests with optional status filter.",
			InputSchema: `{"type":"object","properties":{"status":{"type":"string","description":"Filter: draft, open, closed, archive"}}}`,
		},
		{
			Name:        "manifest_get",
			Description: "Get a manifest by full UUID.",
			InputSchema: `{"type":"object","properties":{"id":{"type":"string","description":"Manifest full UUID"}},"required":["id"]}`,
		},
		{
			Name:        "task_list",
			Description: "List tasks with optional status filter.",
			InputSchema: `{"type":"object","properties":{"status":{"type":"string","description":"Filter: running, scheduled, waiting, completed, failed, cancelled"}}}`,
		},
		{
			Name:        "task_get",
			Description: "Get a task by full UUID.",
			InputSchema: `{"type":"object","properties":{"id":{"type":"string","description":"Task full UUID"}},"required":["id"]}`,
		},
		{
			Name:        "conversation_search",
			Description: "Semantic search over saved agent conversations.",
			InputSchema: `{"type":"object","properties":{"query":{"type":"string","description":"Search query"},"limit":{"type":"integer","description":"Max results (default 5)"}},"required":["query"]}`,
		},
		{
			Name:        "visceral_rules",
			Description: "Get the current visceral rules (mandatory operating constraints).",
			InputSchema: `{"type":"object","properties":{}}`,
		},
	}
}

// Execute runs a tool by name with the given JSON input, returns the result string.
func (ct *ChatTools) Execute(ctx context.Context, name, inputJSON string) (string, error) {
	var input map[string]any
	if inputJSON != "" {
		if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
			return "", fmt.Errorf("invalid tool input: %w", err)
		}
	}
	if input == nil {
		input = map[string]any{}
	}

	switch name {
	case "memory_search":
		return ct.memorySearch(ctx, input)
	case "memory_recall":
		return ct.memoryRecall(input)
	case "manifest_list":
		return ct.manifestList(input)
	case "manifest_get":
		return ct.manifestGet(input)
	case "task_list":
		return ct.taskList(input)
	case "task_get":
		return ct.taskGet(input)
	case "conversation_search":
		return ct.conversationSearch(ctx, input)
	case "visceral_rules":
		return ct.visceralRules()
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

func (ct *ChatTools) memorySearch(ctx context.Context, input map[string]any) (string, error) {
	query, _ := input["query"].(string)
	if query == "" {
		return "query is required", nil
	}
	limit := 5
	if v, ok := input["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}

	results, err := ct.bridge.SearchMemories(ctx, query, limit)
	if err != nil {
		return fmt.Sprintf("search failed: %v", err), nil
	}
	if len(results) == 0 {
		return "No memories found.", nil
	}

	var out string
	for i, r := range results {
		out += fmt.Sprintf("%d. [%s] (%.3f) %s\n   %s\n\n", i+1, r.ID, r.Score, r.Path, r.L1)
	}
	return out, nil
}

func (ct *ChatTools) memoryRecall(input map[string]any) (string, error) {
	id, _ := input["id"].(string)
	if id == "" {
		return "id is required", nil
	}

	mem, err := ct.bridge.RecallMemory(id)
	if err != nil {
		return fmt.Sprintf("recall failed: %v", err), nil
	}
	if mem == nil {
		return fmt.Sprintf("Memory %s not found", id), nil
	}

	return fmt.Sprintf("[%s] %s\nPath: %s\nType: %s\n\n%s", mem.ID, mem.L0, mem.Path, mem.Type, mem.L2), nil
}

func (ct *ChatTools) manifestList(input map[string]any) (string, error) {
	status, _ := input["status"].(string)
	manifests, err := ct.bridge.ListManifests(status, 20)
	if err != nil {
		return fmt.Sprintf("list failed: %v", err), nil
	}
	if len(manifests) == 0 {
		return "No manifests found.", nil
	}

	var out string
	for _, m := range manifests {
		out += fmt.Sprintf("[%s] %s (status: %s)\n", m.ID, m.Title, m.Status)
	}
	return out, nil
}

func (ct *ChatTools) manifestGet(input map[string]any) (string, error) {
	id, _ := input["id"].(string)
	if id == "" {
		return "id is required", nil
	}

	m, err := ct.bridge.GetManifest(id)
	if err != nil || m == nil {
		return fmt.Sprintf("Manifest %s not found", id), nil
	}

	return fmt.Sprintf("[%s] %s\nStatus: %s | Version: %d | Author: %s\nJira: %s\n\n%s",
		m.ID, m.Title, m.Status, m.Version, m.Author, m.JiraRef, m.Content), nil
}

func (ct *ChatTools) taskList(input map[string]any) (string, error) {
	status, _ := input["status"].(string)
	tasks, err := ct.bridge.ListTasks(status, 20)
	if err != nil {
		return fmt.Sprintf("list failed: %v", err), nil
	}
	if len(tasks) == 0 {
		return "No tasks found.", nil
	}

	var out string
	for _, t := range tasks {
		out += fmt.Sprintf("[%s] %s (status: %s, schedule: %s)\n", t.ID, t.Title, t.Status, t.Schedule)
	}
	return out, nil
}

func (ct *ChatTools) taskGet(input map[string]any) (string, error) {
	id, _ := input["id"].(string)
	if id == "" {
		return "id is required", nil
	}

	t, err := ct.bridge.GetTask(id)
	if err != nil || t == nil {
		return fmt.Sprintf("Task %s not found", id), nil
	}

	return fmt.Sprintf("[%s] %s\nStatus: %s | Schedule: %s\nAgent: %s\nManifest: %s\n\n%s",
		t.ID, t.Title, t.Status, t.Schedule, t.Agent, t.ManifestID, t.Description), nil
}

func (ct *ChatTools) conversationSearch(ctx context.Context, input map[string]any) (string, error) {
	query, _ := input["query"].(string)
	if query == "" {
		return "query is required", nil
	}
	limit := 5
	if v, ok := input["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}

	results, err := ct.bridge.SearchConversations(ctx, query, limit)
	if err != nil {
		return fmt.Sprintf("search failed: %v", err), nil
	}
	if len(results) == 0 {
		return "No conversations found.", nil
	}

	var out string
	for i, r := range results {
		out += fmt.Sprintf("%d. (%.3f) %s — %s [%d turns]\n", i+1, r.Score, r.Title, r.Agent, r.TurnCount)
	}
	return out, nil
}

func (ct *ChatTools) visceralRules() (string, error) {
	rules, err := ct.bridge.ListVisceralRules()
	if err != nil {
		return fmt.Sprintf("failed: %v", err), nil
	}
	if len(rules) == 0 {
		return "No visceral rules set.", nil
	}

	var out string
	out += fmt.Sprintf("=== VISCERAL RULES (%d) ===\n", len(rules))
	for i, r := range rules {
		out += fmt.Sprintf("%d. [%s] %s\n", i+1, r.ID, r.Text)
	}
	return out, nil
}
