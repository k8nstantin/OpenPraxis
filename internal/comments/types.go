package comments

type CommentType string

const (
	TypeExecutionReview CommentType = "execution_review"
	TypeUserNote        CommentType = "user_note"
	TypeWatcherFinding  CommentType = "watcher_finding"
	TypeAgentNote       CommentType = "agent_note"
	TypeDecision        CommentType = "decision"
	TypeLink            CommentType = "link"
	// TypeReviewRejection marks a completed task as needing rework.
	// Only attaches to target_type=task. Drives the "completed →
	// scheduled" re-run transition in task.Store.RejectCompletedTask
	// and the NeedsRework computation in TaskReviewStatus.
	TypeReviewRejection CommentType = "review_rejection"
	// TypeReviewApproval marks a completed task as explicitly signed
	// off. Does NOT change the task status — approval is an input to
	// manifest closure warnings, not a task-level transition.
	TypeReviewApproval CommentType = "review_approval"
	// TypeDescriptionRevision records an append-only edit of an entity's
	// description / content / instructions. The latest revision comment
	// is treated as the current text; earlier rows remain as history.
	TypeDescriptionRevision CommentType = "description_revision"
)

// AllCommentTypes returns the canonical ordering of comment types used by
// UI filter dropdowns in M3. Order matches the taxonomy in the M1 manifest;
// review types appended at the end so the pre-existing order stays stable
// and operators don't see dropdown items shifting.
func AllCommentTypes() []CommentType {
	return []CommentType{
		TypeExecutionReview,
		TypeUserNote,
		TypeWatcherFinding,
		TypeAgentNote,
		TypeDecision,
		TypeLink,
		TypeReviewRejection,
		TypeReviewApproval,
		TypeDescriptionRevision,
	}
}

func IsValidCommentType(s string) bool {
	switch CommentType(s) {
	case TypeExecutionReview, TypeUserNote, TypeWatcherFinding,
		TypeAgentNote, TypeDecision, TypeLink,
		TypeReviewRejection, TypeReviewApproval,
		TypeDescriptionRevision:
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
//
// IncludeInPromptContext controls whether the runner injects comments of
// this type into the agent's prompt as part of the prior-runs / other-
// comments context block. Defining this at the type level (not per task)
// keeps the taxonomy of "what belongs in a prompt" declarative: adding a
// new comment type is a one-line flag change here, not a runner.go edit.
// Default: true for types that carry feedback/history an agent needs to
// avoid repeating work; false for cross-references (link) and text that
// already lives on the entity (description_revision).
type CommentTypeInfo struct {
	Type                   CommentType `json:"type"`
	Label                  string      `json:"label"`
	Description            string      `json:"description"`
	IncludeInPromptContext bool        `json:"include_in_prompt_context"`
}

// PromptContextTypes returns the comment types whose Registry entry has
// IncludeInPromptContext=true. Runner.buildPrompt uses this to filter the
// set of comments it injects into the agent prompt, so new types added to
// Registry() participate automatically with a single flag.
func PromptContextTypes() []CommentType {
	out := make([]CommentType, 0, len(Registry()))
	for _, info := range Registry() {
		if info.IncludeInPromptContext {
			out = append(out, info.Type)
		}
	}
	return out
}

// Registry returns metadata for every comment type in the canonical order
// from AllCommentTypes(). It is the single source of truth consumed by the
// MCP tool descriptions, the HTTP enum payload, and the M3 UI dropdown.
func Registry() []CommentTypeInfo {
	return []CommentTypeInfo{
		{
			Type:                   TypeExecutionReview,
			Label:                  "Execution Review",
			Description:            "Post-run retrospective; what shipped, what failed, what the agent learned",
			IncludeInPromptContext: true,
		},
		{
			Type:                   TypeUserNote,
			Label:                  "Note",
			Description:            "Free-form human note on the entity",
			IncludeInPromptContext: true,
		},
		{
			Type:                   TypeWatcherFinding,
			Label:                  "Watcher Finding",
			Description:            "Auto-posted gate result — failures, warnings, or passes",
			IncludeInPromptContext: true,
		},
		{
			Type:                   TypeAgentNote,
			Label:                  "Agent Note",
			Description:            "Agent-authored observation; distinct from an execution review",
			IncludeInPromptContext: true,
		},
		{
			Type:                   TypeDecision,
			Label:                  "Decision",
			Description:            "Architectural or scope decision recorded against the entity",
			IncludeInPromptContext: true,
		},
		{
			Type:                   TypeLink,
			Label:                  "Link",
			Description:            "Cross-reference to another comment, PR, issue, or external doc",
			IncludeInPromptContext: false, // cross-refs: noise in prompt context
		},
		{
			Type:                   TypeReviewRejection,
			Label:                  "Review Rejection",
			Description:            "Reviewer rejected a completed task; kicks it back to scheduled for another pass",
			IncludeInPromptContext: true,
		},
		{
			Type:                   TypeReviewApproval,
			Label:                  "Review Approval",
			Description:            "Reviewer signed off a completed task; clears the needs-rework flag for manifest closure",
			IncludeInPromptContext: true,
		},
		{
			Type:                   TypeDescriptionRevision,
			Label:                  "Description Revision",
			Description:            "Append-only edit of description / content / instructions. Latest revision is the current text.",
			IncludeInPromptContext: false, // body lives on the entity column; don't re-inject
		},
	}
}
