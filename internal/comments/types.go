package comments

type CommentType string

const (
	// TypePrompt is the active prompt/instructions for an entity.
	// Append-only; the latest revision is the current prompt.
	TypePrompt CommentType = "prompt"

	// TypeComment is everything else — notes, findings, decisions,
	// review results, execution summaries, watcher output.
	TypeComment CommentType = "comment"

	// Legacy aliases kept so old data and any stragglers compile.
	// All resolve to TypeComment except description_revision → TypePrompt.
	TypeExecutionReview     = TypeComment
	TypeUserNote            = TypeComment
	TypeWatcherFinding      = TypeComment
	TypeAgentNote           = TypeComment
	TypeDecision            = TypeComment
	TypeLink                = TypeComment
	TypeReviewRejection     = TypeComment
	TypeReviewApproval      = TypeComment
	TypeDescriptionRevision = TypePrompt
)

// NormalizeType maps legacy type names to the canonical two-type taxonomy.
// Agents trained on old types (execution_review, description_revision, etc.)
// are remapped transparently so no call ever fails on a stale type name.
func NormalizeType(raw string) CommentType {
	switch raw {
	case "", "user_note", "agent_note", "watcher_finding",
		"execution_review", "decision", "link",
		"review_rejection", "review_approval":
		return TypeComment
	case "description_revision":
		return TypePrompt
	default:
		return CommentType(raw)
	}
}

func AllCommentTypes() []CommentType {
	return []CommentType{TypePrompt, TypeComment}
}

func IsValidCommentType(s string) bool {
	switch CommentType(s) {
	case TypePrompt, TypeComment:
		return true
	}
	return false
}

func IsValidTargetType(s string) bool {
	return TargetType(s) == TargetEntity
}

type CommentTypeInfo struct {
	Type                   CommentType `json:"type"`
	Label                  string      `json:"label"`
	Description            string      `json:"description"`
	IncludeInPromptContext bool        `json:"include_in_prompt_context"`
}

// PromptContextTypes returns comment types injected into the agent prompt.
// Comments (notes, findings, decisions) are included; prompts are not
// re-injected since they drive the execution directly.
func PromptContextTypes() []CommentType {
	out := make([]CommentType, 0, len(Registry()))
	for _, info := range Registry() {
		if info.IncludeInPromptContext {
			out = append(out, info.Type)
		}
	}
	return out
}

func Registry() []CommentTypeInfo {
	return []CommentTypeInfo{
		{
			Type:                   TypePrompt,
			Label:                  "Prompt",
			Description:            "Active prompt / instructions for the entity. Latest revision is current.",
			IncludeInPromptContext: false,
		},
		{
			Type:                   TypeComment,
			Label:                  "Comment",
			Description:            "Notes, findings, decisions, review results, execution summaries — anything that informs the next prompt.",
			IncludeInPromptContext: true,
		},
	}
}
