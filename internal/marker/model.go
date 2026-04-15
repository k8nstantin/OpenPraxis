package marker

import (
	"time"

	"github.com/google/uuid"
)

// Marker is a flag/tag placed on a memory or conversation to draw a peer's attention.
type Marker struct {
	ID         string    `json:"id"`
	TargetID   string    `json:"target_id"`   // Memory or Conversation ID
	TargetType string    `json:"target_type"`  // "memory" or "conversation"
	TargetPath string    `json:"target_path"`  // Path (for memories) or title (for conversations)
	FromNode   string    `json:"from_node"`    // Who flagged it
	ToNode     string    `json:"to_node"`      // Target peer ("all" for broadcast)
	Message    string    `json:"message"`      // Why this is flagged
	Priority   string    `json:"priority"`     // "normal", "high", "urgent"
	Status     string    `json:"status"`       // "pending", "seen", "done"
	CreatedAt  time.Time `json:"created_at"`
	SeenAt     *time.Time `json:"seen_at,omitempty"`
	DoneAt     *time.Time `json:"done_at,omitempty"`
}

// NewMarker creates a marker.
func NewMarker(targetID, targetType, targetPath, fromNode, toNode, message, priority string) *Marker {
	if priority == "" {
		priority = "normal"
	}
	if toNode == "" {
		toNode = "all"
	}

	return &Marker{
		ID:         uuid.Must(uuid.NewV7()).String(),
		TargetID:   targetID,
		TargetType: targetType,
		TargetPath: targetPath,
		FromNode:   fromNode,
		ToNode:     toNode,
		Message:    message,
		Priority:   priority,
		Status:     "pending",
		CreatedAt:  time.Now().UTC(),
	}
}
