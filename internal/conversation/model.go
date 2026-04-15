package conversation

import (
	"time"

	"github.com/google/uuid"
)

// Conversation represents a saved agent conversation.
type Conversation struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`        // Auto-generated or user-provided
	Summary     string    `json:"summary"`       // AI-generated or first few lines
	Agent       string    `json:"agent"`         // claude-code, cursor, copilot, etc.
	Project     string    `json:"project"`       // Project context
	Tags        []string  `json:"tags"`
	Turns       []Turn    `json:"turns"`
	TurnCount   int       `json:"turn_count"`
	SourceNode  string    `json:"source_node"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	AccessedAt  time.Time `json:"accessed_at"`
	AccessCount int       `json:"access_count"`
}

// Turn represents a single message in a conversation.
type Turn struct {
	Role    string `json:"role"`    // "user" or "assistant"
	Content string `json:"content"`
	Model   string `json:"model,omitempty"` // e.g. "claude-opus-4-6"
}

// NewConversation creates a Conversation with generated ID and timestamps.
func NewConversation(title, agent, project, sourceNode string, turns []Turn, tags []string) *Conversation {
	now := time.Now().UTC()

	if title == "" {
		title = generateTitle(turns)
	}
	summary := BuildSummary(turns)

	return &Conversation{
		ID:          uuid.Must(uuid.NewV7()).String(),
		Title:       title,
		Summary:     summary,
		Agent:       agent,
		Project:     project,
		Tags:        tags,
		Turns:       turns,
		TurnCount:   len(turns),
		SourceNode:  sourceNode,
		CreatedAt:   now,
		UpdatedAt:   now,
		AccessedAt:  now,
		AccessCount: 0,
	}
}

// generateTitle creates a title from the first user message.
func generateTitle(turns []Turn) string {
	for _, t := range turns {
		if t.Role == "user" && len(t.Content) > 0 {
			title := t.Content
			if len(title) > 80 {
				// Cut at word boundary
				title = title[:80]
				for i := len(title) - 1; i > 40; i-- {
					if title[i] == ' ' {
						title = title[:i]
						break
					}
				}
				title += "..."
			}
			return title
		}
	}
	return "Untitled conversation"
}

// BuildSummary creates a summary from the conversation turns.
func BuildSummary(turns []Turn) string {
	var summary string
	for _, t := range turns {
		if len(summary) > 2000 {
			break
		}
		prefix := "User: "
		if t.Role == "assistant" {
			prefix = "Assistant: "
		}
		line := prefix + t.Content
		if len(line) > 300 {
			line = line[:300] + "..."
		}
		summary += line + "\n"
	}
	if len(summary) > 2000 {
		summary = summary[:2000]
	}
	return summary
}

// SearchResult for conversation search.
type SearchResult struct {
	Conversation Conversation `json:"conversation"`
	Distance     float64      `json:"distance"`
	Score        float64      `json:"score"`
}
