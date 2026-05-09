// Package comments implements free-form comments attached to entities.
// All comments use TargetEntity — one target type, two comment types (prompt/comment).
package comments

import "time"

type TargetType string

const (
	TargetEntity TargetType = "entity"

	// Legacy aliases — kept so call sites compile during migration; all resolve to TargetEntity.
	TargetProduct  = TargetEntity
	TargetManifest = TargetEntity
	TargetTask     = TargetEntity
	TargetIdea     = TargetEntity
)

type Comment struct {
	ID         string      `json:"id"`
	TargetType TargetType  `json:"target_type"`
	TargetID   string      `json:"target_id"`
	Author     string      `json:"author"`
	Type       CommentType `json:"type"`
	Body       string      `json:"body"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  *time.Time  `json:"updated_at,omitempty"`
	ParentID   *string     `json:"parent_id,omitempty"`
}
