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
