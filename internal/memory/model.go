package memory

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Memory represents a single memory entry in the shared store.
type Memory struct {
	ID          string   `json:"id"`
	Path        string   `json:"path"`
	L0          string   `json:"l0"`
	L1          string   `json:"l1"`
	L2          string   `json:"l2"`
	Type        string   `json:"type"`
	Tags        []string `json:"tags"`
	SourceAgent string   `json:"source_agent"`
	SourceNode  string   `json:"source_node"`
	Scope       string   `json:"scope"`
	Project     string   `json:"project"`
	Domain      string   `json:"domain"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
	AccessedAt  string   `json:"accessed_at"`
	AccessCount int      `json:"access_count"`
}

// Valid memory types.
var ValidTypes = []string{"insight", "comment", "pattern", "bug", "context", "reference", "visceral"}

// Valid scopes.
var ValidScopes = []string{"personal", "project", "team", "global"}

// NewMemory creates a Memory with generated ID and timestamps.
func NewMemory(content, path, memType, scope, project, domain, sourceAgent, sourceNode string, tags []string) (*Memory, error) {
	if content == "" {
		return nil, fmt.Errorf("content cannot be empty")
	}
	if memType == "" {
		memType = "insight"
	}
	if !isValid(memType, ValidTypes) {
		return nil, fmt.Errorf("invalid type %q, must be one of: %s", memType, strings.Join(ValidTypes, ", "))
	}
	if scope == "" {
		scope = "project"
	}
	if !isValid(scope, ValidScopes) {
		return nil, fmt.Errorf("invalid scope %q, must be one of: %s", scope, strings.Join(ValidScopes, ", "))
	}
	if domain == "" {
		domain = "general"
	}

	now := time.Now().UTC().Format(time.RFC3339)
	l0, l1 := GenerateTiers(content)

	slug := generateSlug(content)
	if path == "" {
		path = fmt.Sprintf("/%s/%s/%s/%s", scope, project, domain, slug)
	}

	return &Memory{
		ID:          uuid.Must(uuid.NewV7()).String(),
		Path:        path,
		L0:          l0,
		L1:          l1,
		L2:          content,
		Type:        memType,
		Tags:        tags,
		SourceAgent: sourceAgent,
		SourceNode:  sourceNode,
		Scope:       scope,
		Project:     project,
		Domain:      domain,
		CreatedAt:   now,
		UpdatedAt:   now,
		AccessedAt:  now,
		AccessCount: 0,
	}, nil
}

// ParsePath extracts scope, project, domain, and slug from a path like /project/gryphon/schema/my-memory.
func ParsePath(path string) (scope, project, domain, slug string) {
	path = strings.TrimPrefix(path, "/")
	parts := strings.SplitN(path, "/", 4)
	if len(parts) >= 1 {
		scope = parts[0]
	}
	if len(parts) >= 2 {
		project = parts[1]
	}
	if len(parts) >= 3 {
		domain = parts[2]
	}
	if len(parts) >= 4 {
		slug = parts[3]
	}
	return
}

// IsPathPrefix returns true if the path ends with "/" indicating a directory listing.
func IsPathPrefix(path string) bool {
	return strings.HasSuffix(path, "/")
}

func generateSlug(content string) string {
	// Take first ~50 chars, lowercase, replace non-alphanumeric with hyphens
	s := content
	if len(s) > 60 {
		s = s[:60]
	}
	s = strings.ToLower(s)

	var b strings.Builder
	lastDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}
	result := strings.Trim(b.String(), "-")
	if len(result) > 50 {
		result = result[:50]
	}
	return result
}

func isValid(val string, valid []string) bool {
	for _, v := range valid {
		if v == val {
			return true
		}
	}
	return false
}
