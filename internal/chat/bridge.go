package chat

import "context"

// NodeBridge abstracts the node layer so chat doesn't import node directly.
// This breaks the import cycle: node imports chat (for SessionStore),
// and chat uses this interface (satisfied by node.Node via adapter).
type NodeBridge interface {
	SearchMemories(ctx context.Context, query string, limit int) ([]MemoryResult, error)
	RecallMemory(id string) (*MemoryResult, error)
	ListManifests(status string, limit int) ([]ManifestSummary, error)
	GetManifest(id string) (*ManifestDetail, error)
	ListTasks(status string, limit int) ([]TaskSummary, error)
	GetTask(id string) (*TaskDetail, error)
	SearchConversations(ctx context.Context, query string, limit int) ([]ConversationResult, error)
	ListVisceralRules() ([]VisceralRule, error)
}

// MemoryResult is a search result from the memory index.
type MemoryResult struct {
	ID    string  `json:"id"`
	Path  string  `json:"path"`
	L0    string  `json:"l0"`
	L1    string  `json:"l1"`
	L2    string  `json:"l2"`
	Type  string  `json:"type"`
	Score float64 `json:"score"`
}

// ManifestSummary is a brief manifest listing.
type ManifestSummary struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	JiraRef string `json:"jira_ref"`
}

// ManifestDetail is a full manifest.
type ManifestDetail struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	Version int    `json:"version"`
	Author  string `json:"author"`
	JiraRef string `json:"jira_ref"`
	Content string `json:"content"`
}

// TaskSummary is a brief task listing.
type TaskSummary struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Schedule string `json:"schedule"`
}

// TaskDetail is a full task.
type TaskDetail struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Schedule    string `json:"schedule"`
	Agent       string `json:"agent"`
	ManifestID  string `json:"manifest_id"`
	Description string `json:"description"`
}

// ConversationResult is a conversation search result.
type ConversationResult struct {
	Title     string  `json:"title"`
	Agent     string  `json:"agent"`
	TurnCount int     `json:"turn_count"`
	Score     float64 `json:"score"`
}

// VisceralRule is a single visceral rule.
type VisceralRule struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}
