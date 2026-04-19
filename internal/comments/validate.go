package comments

import (
	"errors"
	"strings"
)

// Exported sentinels for validation failures. M2's MCP + HTTP layers match
// these with errors.Is to translate into protocol-specific error codes /
// HTTP status codes. ErrEmptyAuthor and ErrEmptyBody live in store.go and
// are reused here — do not redeclare.
var (
	ErrUnknownTargetType  = errors.New("comments: unknown target_type")
	ErrEmptyTargetID      = errors.New("comments: target_id cannot be empty")
	ErrUnknownCommentType = errors.New("comments: unknown type")
)

// ValidateAdd checks every field of an Add-style request without touching
// the DB. Returns the first failure as a sentinel error; nil means OK.
// Body is trimmed before the empty check so whitespace-only input is
// rejected with ErrEmptyBody.
func ValidateAdd(target TargetType, targetID, author string, cType CommentType, body string) error {
	if !IsValidTargetType(string(target)) {
		return ErrUnknownTargetType
	}
	if targetID == "" {
		return ErrEmptyTargetID
	}
	if author == "" {
		return ErrEmptyAuthor
	}
	if !IsValidCommentType(string(cType)) {
		return ErrUnknownCommentType
	}
	if strings.TrimSpace(body) == "" {
		return ErrEmptyBody
	}
	return nil
}

// ValidateEdit checks only the new body. The id format is a store concern —
// Store.Edit already returns sql.ErrNoRows when the id does not exist.
func ValidateEdit(body string) error {
	if strings.TrimSpace(body) == "" {
		return ErrEmptyBody
	}
	return nil
}
