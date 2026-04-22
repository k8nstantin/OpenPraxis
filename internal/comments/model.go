// Package comments implements free-form comments attached to products,
// manifests, and tasks. Comments are typed (see types.go) so the UI can
// render execution reviews, user notes, agent notes, decisions, etc.
// distinctly.
package comments

import "time"

type TargetType string

const (
	TargetProduct  TargetType = "product"
	TargetManifest TargetType = "manifest"
	TargetTask     TargetType = "task"
	TargetIdea     TargetType = "idea"
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
