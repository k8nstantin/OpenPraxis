package chat

import (
	"context"
	"fmt"
	"strings"
)

// ContextBuilder builds system prompts with OpenLoom context injected.
type ContextBuilder struct {
	bridge NodeBridge
}

func NewContextBuilder(bridge NodeBridge) *ContextBuilder {
	return &ContextBuilder{bridge: bridge}
}

// Build constructs the full system prompt with OpenLoom context.
// It auto-searches memories relevant to the user's message and includes
// visceral rules, active manifests, and running tasks.
func (cb *ContextBuilder) Build(ctx context.Context, userMessage string) string {
	var parts []string

	parts = append(parts, "You are an AI assistant integrated into OpenLoom, a shared memory layer for coding agents. You have access to tools that let you search memories, list manifests and tasks, and retrieve conversation history.")

	// Visceral rules
	rules := cb.buildVisceralRules()
	if rules != "" {
		parts = append(parts, rules)
	}

	// Active manifests summary
	manifests := cb.buildManifestSummary()
	if manifests != "" {
		parts = append(parts, manifests)
	}

	// Running tasks summary
	tasks := cb.buildTaskSummary()
	if tasks != "" {
		parts = append(parts, tasks)
	}

	// Auto-search relevant memories
	memories := cb.buildRelevantMemories(ctx, userMessage)
	if memories != "" {
		parts = append(parts, memories)
	}

	parts = append(parts, "Use the available tools to look up information when the user asks about memories, manifests, tasks, or conversations. Always provide specific markers and IDs in your responses.")

	return strings.Join(parts, "\n\n")
}

func (cb *ContextBuilder) buildVisceralRules() string {
	rules, err := cb.bridge.ListVisceralRules()
	if err != nil || len(rules) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Visceral Rules (%d mandatory)\n", len(rules)))
	for i, r := range rules {
		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, r.Marker, r.Text))
	}
	return sb.String()
}

func (cb *ContextBuilder) buildManifestSummary() string {
	manifests, err := cb.bridge.ListManifests("open", 10)
	if err != nil || len(manifests) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Active Manifests (%d)\n", len(manifests)))
	for _, m := range manifests {
		sb.WriteString(fmt.Sprintf("- [%s] %s", m.Marker, m.Title))
		if m.JiraRef != "" {
			sb.WriteString(fmt.Sprintf(" (Jira: %s)", m.JiraRef))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func (cb *ContextBuilder) buildTaskSummary() string {
	tasks, err := cb.bridge.ListTasks("running", 10)
	if err != nil || len(tasks) == 0 {
		tasks, err = cb.bridge.ListTasks("scheduled", 10)
		if err != nil || len(tasks) == 0 {
			return ""
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Tasks (%d)\n", len(tasks)))
	for _, t := range tasks {
		sb.WriteString(fmt.Sprintf("- [%s] %s (status: %s)\n", t.Marker, t.Title, t.Status))
	}
	return sb.String()
}

func (cb *ContextBuilder) buildRelevantMemories(ctx context.Context, userMessage string) string {
	if userMessage == "" {
		return ""
	}

	results, err := cb.bridge.SearchMemories(ctx, userMessage, 3)
	if err != nil || len(results) == 0 {
		return ""
	}

	var relevant []string
	for _, r := range results {
		if r.Score < 0.3 {
			continue
		}
		marker := r.ID[:12]
		relevant = append(relevant, fmt.Sprintf("- [%s] %s: %s", marker, r.Path, r.L1))
	}
	if len(relevant) == 0 {
		return ""
	}

	return "## Relevant Memories\n" + strings.Join(relevant, "\n")
}
