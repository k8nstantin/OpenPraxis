package action

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Polarity constants for rule classification.
const (
	PolarityProhibition = "prohibition"
	PolarityInstruction = "instruction"
	PolarityPermission  = "permission"
)

// MatchTypeMissingConfirm is the match type for missing visceral confirmation.
const MatchTypeMissingConfirm = "missing_confirm"

// RulePattern stores extracted polarity and patterns for a visceral rule.
type RulePattern struct {
	ID                int       `json:"id"`
	RuleID            string    `json:"rule_id"`
	Polarity          string    `json:"polarity"`
	RequiredPatterns  []string  `json:"required_patterns"`
	ForbiddenPatterns []string  `json:"forbidden_patterns"`
	AutoExtracted     bool      `json:"auto_extracted"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// InitRulePatterns creates the rule_patterns table.
func (s *Store) InitRulePatterns() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS rule_patterns (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		rule_id TEXT NOT NULL UNIQUE,
		polarity TEXT NOT NULL DEFAULT 'prohibition',
		required_patterns TEXT NOT NULL DEFAULT '[]',
		forbidden_patterns TEXT NOT NULL DEFAULT '[]',
		auto_extracted INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("create rule_patterns table: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_rule_patterns_rule ON rule_patterns(rule_id)`)
	return err
}

// SetRulePattern upserts polarity and patterns for a rule.
func (s *Store) SetRulePattern(ruleID, polarity string, required, forbidden []string, autoExtracted bool) error {
	reqJSON, _ := json.Marshal(required)
	forbJSON, _ := json.Marshal(forbidden)
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.Exec(`INSERT INTO rule_patterns (rule_id, polarity, required_patterns, forbidden_patterns, auto_extracted, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(rule_id) DO UPDATE SET
			polarity = excluded.polarity,
			required_patterns = excluded.required_patterns,
			forbidden_patterns = excluded.forbidden_patterns,
			auto_extracted = excluded.auto_extracted,
			updated_at = excluded.updated_at`,
		ruleID, polarity, string(reqJSON), string(forbJSON), autoExtracted, now, now)
	return err
}

// GetRulePattern returns patterns for a specific rule.
func (s *Store) GetRulePattern(ruleID string) (*RulePattern, error) {
	var rp RulePattern
	var reqStr, forbStr string
	var createdStr, updatedStr string
	var autoInt int

	err := s.db.QueryRow(`SELECT id, rule_id, polarity, required_patterns, forbidden_patterns, auto_extracted, created_at, updated_at
		FROM rule_patterns WHERE rule_id = ?`, ruleID).Scan(
		&rp.ID, &rp.RuleID, &rp.Polarity, &reqStr, &forbStr, &autoInt, &createdStr, &updatedStr)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(reqStr), &rp.RequiredPatterns); err != nil {
		slog.Warn("unmarshal rule patterns failed", "field", "required_patterns", "error", err)
	}
	if err := json.Unmarshal([]byte(forbStr), &rp.ForbiddenPatterns); err != nil {
		slog.Warn("unmarshal rule patterns failed", "field", "forbidden_patterns", "error", err)
	}
	rp.AutoExtracted = autoInt == 1
	rp.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	rp.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)

	if rp.RequiredPatterns == nil {
		rp.RequiredPatterns = []string{}
	}
	if rp.ForbiddenPatterns == nil {
		rp.ForbiddenPatterns = []string{}
	}
	return &rp, nil
}

// ListRulePatterns returns all stored rule patterns.
func (s *Store) ListRulePatterns() ([]RulePattern, error) {
	rows, err := s.db.Query(`SELECT id, rule_id, polarity, required_patterns, forbidden_patterns, auto_extracted, created_at, updated_at
		FROM rule_patterns ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []RulePattern
	for rows.Next() {
		var rp RulePattern
		var reqStr, forbStr string
		var createdStr, updatedStr string
		var autoInt int

		if err := rows.Scan(&rp.ID, &rp.RuleID, &rp.Polarity, &reqStr, &forbStr, &autoInt, &createdStr, &updatedStr); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(reqStr), &rp.RequiredPatterns); err != nil {
			slog.Warn("unmarshal rule patterns failed", "field", "required_patterns", "error", err)
		}
		if err := json.Unmarshal([]byte(forbStr), &rp.ForbiddenPatterns); err != nil {
			slog.Warn("unmarshal rule patterns failed", "field", "forbidden_patterns", "error", err)
		}
		rp.AutoExtracted = autoInt == 1
		rp.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		rp.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)

		if rp.RequiredPatterns == nil {
			rp.RequiredPatterns = []string{}
		}
		if rp.ForbiddenPatterns == nil {
			rp.ForbiddenPatterns = []string{}
		}
		results = append(results, rp)
	}
	return results, rows.Err()
}

// DeleteRulePattern removes patterns for a rule.
func (s *Store) DeleteRulePattern(ruleID string) error {
	_, err := s.db.Exec(`DELETE FROM rule_patterns WHERE rule_id = ?`, ruleID)
	return err
}

// ClassifyPolarity determines if a rule is prohibition, instruction, or permission.
func ClassifyPolarity(ruleText string) string {
	lower := strings.TrimSpace(strings.ToLower(ruleText))

	// Permission rules — pure allowances, nothing is forbidden
	if strings.HasPrefix(lower, "allow ") || strings.HasPrefix(lower, "permit ") {
		return PolarityPermission
	}

	// Prohibition rules — something is forbidden
	if strings.HasPrefix(lower, "never ") ||
		strings.HasPrefix(lower, "don't ") ||
		strings.HasPrefix(lower, "do not ") ||
		strings.HasPrefix(lower, "dont ") ||
		strings.HasPrefix(lower, "no ") ||
		strings.HasPrefix(lower, "avoid ") ||
		strings.HasPrefix(lower, "stop ") ||
		strings.HasPrefix(lower, "refuse ") {
		return PolarityProhibition
	}

	// Instruction rules — something is required
	if strings.HasPrefix(lower, "use ") ||
		strings.HasPrefix(lower, "always ") ||
		strings.HasPrefix(lower, "we always ") ||
		strings.HasPrefix(lower, "every ") ||
		strings.HasPrefix(lower, "must ") ||
		strings.HasPrefix(lower, "only ") ||
		strings.HasPrefix(lower, "prefer ") {
		return PolarityInstruction
	}

	// Default to prohibition (safer — enables embedding-based detection)
	return PolarityProhibition
}

// ExtractPatterns parses a rule's text to identify required and forbidden patterns.
// Returns (required, forbidden) slices.
func ExtractPatterns(ruleText string) ([]string, []string) {
	lower := strings.TrimSpace(strings.ToLower(ruleText))
	var required, forbidden []string

	// Split into clauses by common separators
	// e.g. "use only python for text processing no sed" → analyze each part
	words := strings.Fields(lower)

	// --- Extract forbidden patterns ---

	// Pattern: "no X" / "not X" / "never X" anywhere in the text
	for i, w := range words {
		if (w == "no" || w == "not" || w == "never" || w == "without") && i+1 < len(words) {
			candidate := words[i+1]
			candidate = cleanPattern(candidate)
			if isToolOrTechnique(candidate) {
				forbidden = append(forbidden, candidate)
			}
		}
	}

	// Pattern: "instead of X"
	for i, w := range words {
		if w == "instead" && i+1 < len(words) && words[i+1] == "of" && i+2 < len(words) {
			candidate := cleanPattern(words[i+2])
			if isToolOrTechnique(candidate) {
				forbidden = append(forbidden, candidate)
			}
		}
	}

	// --- Extract required patterns ---

	// Pattern: "use X" / "use only X"
	for i, w := range words {
		if w == "use" && i+1 < len(words) {
			next := words[i+1]
			if next == "only" && i+2 < len(words) {
				candidate := cleanPattern(words[i+2])
				if isToolOrTechnique(candidate) {
					required = append(required, candidate)
				}
			} else {
				candidate := cleanPattern(next)
				if isToolOrTechnique(candidate) {
					required = append(required, candidate)
				}
			}
		}
	}

	// Pattern: "only X" at start
	if len(words) > 1 && words[0] == "only" {
		candidate := cleanPattern(words[1])
		if isToolOrTechnique(candidate) {
			required = append(required, candidate)
		}
	}

	// Infer forbidden from "only X" context — if we know the category, add common alternatives
	if len(required) > 0 {
		forbidden = append(forbidden, inferForbiddenAlternatives(required, lower)...)
	}

	// Deduplicate
	required = dedupe(required)
	forbidden = dedupe(forbidden)

	// Don't let something appear in both lists
	forbidden = subtract(forbidden, required)

	return required, forbidden
}

// CheckForbiddenPatterns checks if an action description contains any forbidden pattern.
// Returns the first matched pattern and whether a match was found.
func CheckForbiddenPatterns(actionDesc string, forbidden []string) (string, bool) {
	lower := strings.ToLower(actionDesc)
	for _, pat := range forbidden {
		// Check for the pattern as a word boundary match
		// e.g. "sed" should match "sed -i" but not "used" or "based"
		if containsWord(lower, pat) {
			return pat, true
		}
	}
	return "", false
}

// containsWord checks if text contains pattern as a whole word (not substring of another word).
func containsWord(text, word string) bool {
	idx := 0
	for {
		pos := strings.Index(text[idx:], word)
		if pos == -1 {
			return false
		}
		absPos := idx + pos
		endPos := absPos + len(word)

		// Check left boundary: start of string or non-alphanumeric
		leftOK := absPos == 0 || !isAlphaNum(text[absPos-1])
		// Check right boundary: end of string or non-alphanumeric
		rightOK := endPos >= len(text) || !isAlphaNum(text[endPos])

		if leftOK && rightOK {
			return true
		}
		idx = absPos + 1
		if idx >= len(text) {
			return false
		}
	}
}

func isAlphaNum(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

// cleanPattern removes punctuation from pattern candidates.
func cleanPattern(s string) string {
	s = strings.TrimRight(s, ".,;:!?")
	s = strings.TrimLeft(s, "\"'(")
	s = strings.TrimRight(s, "\"')")
	return s
}

// isToolOrTechnique returns true if the word looks like a tool/technique name
// (not a common English word that would cause false matches).
var commonWords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "are": true, "was": true,
	"be": true, "to": true, "of": true, "and": true, "or": true, "for": true,
	"in": true, "on": true, "at": true, "by": true, "with": true, "from": true,
	"it": true, "that": true, "this": true, "what": true, "which": true,
	"when": true, "where": true, "how": true, "do": true, "does": true,
	"did": true, "will": true, "would": true, "should": true, "could": true,
	"can": true, "may": true, "might": true, "shall": true, "have": true,
	"has": true, "had": true, "been": true, "being": true, "if": true,
	"then": true, "else": true, "so": true, "but": true, "not": true,
	"all": true, "any": true, "some": true, "each": true, "every": true,
	"other": true, "such": true, "than": true, "too": true, "very": true,
	"just": true, "also": true, "only": true, "more": true, "most": true,
	"new": true, "first": true, "last": true, "own": true, "same": true,
	"them": true, "they": true, "their": true, "we": true, "our": true,
	"you": true, "your": true, "my": true, "i": true, "me": true,
	"need": true, "processing": true, "fix": true, "problem": true,
	"create": true, "modification": true, "code": true, "scripts": true,
	"fly": true, "source": true, "original": true, "text": true,
	"reusable": true, "tools": true, "tool": true,
}

func isToolOrTechnique(word string) bool {
	if len(word) < 2 {
		return false
	}
	return !commonWords[word]
}

// inferForbiddenAlternatives maps known tool categories to their common alternatives.
// If the rule requires "python" for "text processing", infer that sed/awk/perl are forbidden.
var toolAlternatives = map[string][]string{
	"python":  {"sed", "awk", "perl"},
	"python3": {"sed", "awk", "perl"},
	"go":      {},
	"curl":    {"wget", "httpie"},
	"wget":    {"curl"},
}

func inferForbiddenAlternatives(required []string, ruleText string) []string {
	var inferred []string
	for _, req := range required {
		if alts, ok := toolAlternatives[req]; ok {
			inferred = append(inferred, alts...)
		}
	}
	return inferred
}

func dedupe(s []string) []string {
	seen := make(map[string]bool, len(s))
	var out []string
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func subtract(a, b []string) []string {
	bSet := make(map[string]bool, len(b))
	for _, v := range b {
		bSet[v] = true
	}
	var out []string
	for _, v := range a {
		if !bSet[v] {
			out = append(out, v)
		}
	}
	return out
}
