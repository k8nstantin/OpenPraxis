package comments

type CommentType string

const (
	TypeExecutionReview CommentType = "execution_review"
	TypeUserNote        CommentType = "user_note"
	TypeWatcherFinding  CommentType = "watcher_finding"
	TypeAgentNote       CommentType = "agent_note"
	TypeDecision        CommentType = "decision"
	TypeLink            CommentType = "link"
)

// AllCommentTypes returns the canonical ordering of comment types used by
// UI filter dropdowns in M3. Order matches the taxonomy in the M1 manifest.
func AllCommentTypes() []CommentType {
	return []CommentType{
		TypeExecutionReview,
		TypeUserNote,
		TypeWatcherFinding,
		TypeAgentNote,
		TypeDecision,
		TypeLink,
	}
}

func IsValidCommentType(s string) bool {
	switch CommentType(s) {
	case TypeExecutionReview, TypeUserNote, TypeWatcherFinding,
		TypeAgentNote, TypeDecision, TypeLink:
		return true
	}
	return false
}

func IsValidTargetType(s string) bool {
	switch TargetType(s) {
	case TargetProduct, TargetManifest, TargetTask:
		return true
	}
	return false
}

// CommentTypeInfo carries UI/doc metadata per type. M3 renders the filter
// dropdown from Registry() so new types added here automatically appear.
type CommentTypeInfo struct {
	Type        CommentType `json:"type"`
	Label       string      `json:"label"`
	Description string      `json:"description"`
}

// Registry returns metadata for every comment type in the canonical order
// from AllCommentTypes(). It is the single source of truth consumed by the
// MCP tool descriptions, the HTTP enum payload, and the M3 UI dropdown.
func Registry() []CommentTypeInfo {
	return []CommentTypeInfo{
		{
			Type:        TypeExecutionReview,
			Label:       "Execution Review",
			Description: "Post-run retrospective; what shipped, what failed, what the agent learned",
		},
		{
			Type:        TypeUserNote,
			Label:       "Note",
			Description: "Free-form human note on the entity",
		},
		{
			Type:        TypeWatcherFinding,
			Label:       "Watcher Finding",
			Description: "Auto-posted gate result — failures, warnings, or passes",
		},
		{
			Type:        TypeAgentNote,
			Label:       "Agent Note",
			Description: "Agent-authored observation; distinct from an execution review",
		},
		{
			Type:        TypeDecision,
			Label:       "Decision",
			Description: "Architectural or scope decision recorded against the entity",
		},
		{
			Type:        TypeLink,
			Label:       "Link",
			Description: "Cross-reference to another comment, PR, issue, or external doc",
		},
	}
}
