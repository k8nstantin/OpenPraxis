// Package settings implements the hierarchical knob system.
// Values are resolved at run time through task → manifest → product → system.
package settings

import "time"

type ScopeType string

const (
	ScopeSystem   ScopeType = "system"
	ScopeProduct  ScopeType = "product"
	ScopeManifest ScopeType = "manifest"
	ScopeTask     ScopeType = "task"
)

type Entry struct {
	ScopeType ScopeType `json:"scope_type"`
	ScopeID   string    `json:"scope_id"`
	Key       string    `json:"key"`
	Value     string    `json:"value"` // JSON-encoded
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy string    `json:"updated_by,omitempty"`
}

type Scope struct {
	ProductID  string
	ManifestID string
	TaskID     string
}
