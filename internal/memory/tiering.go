package memory

import (
	"strings"
	"unicode"
)

// GenerateTiers produces L0 (one-liner) and L1 (summary paragraph) from full content (L2).
// Uses heuristic extraction: first sentence for L0, first paragraph for L1.
func GenerateTiers(content string) (l0, l1 string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", ""
	}

	l0 = extractFirstSentence(content)
	l1 = extractFirstParagraph(content)

	// L1 should be longer than L0 but capped at ~2000 chars
	if len(l1) <= len(l0) {
		l1 = l0
	}
	if len(l1) > 2000 {
		l1 = l1[:2000]
		// Trim to last complete sentence
		if idx := strings.LastIndexAny(l1, ".!?"); idx > 0 {
			l1 = l1[:idx+1]
		}
	}

	return l0, l1
}

// extractFirstSentence returns the first sentence (ending with . ! or ?).
func extractFirstSentence(s string) string {
	// Find first sentence-ending punctuation followed by space or end
	for i, r := range s {
		if r == '.' || r == '!' || r == '?' {
			// Check it's not a decimal or abbreviation
			if i+1 >= len(s) {
				return s[:i+1]
			}
			next := rune(s[i+1])
			if unicode.IsSpace(next) || next == '"' || next == '\'' || next == '\n' {
				return s[:i+1]
			}
		}
		// Stop at newline if no sentence ending found
		if r == '\n' && i > 20 {
			return strings.TrimSpace(s[:i])
		}
	}

	// No sentence ending found — return first 200 chars
	if len(s) > 200 {
		if idx := strings.LastIndex(s[:200], " "); idx > 0 {
			return s[:idx]
		}
		return s[:200]
	}
	return s
}

// extractFirstParagraph returns text up to the first blank line, or first ~500 chars.
func extractFirstParagraph(s string) string {
	// Look for double newline (paragraph break)
	if idx := strings.Index(s, "\n\n"); idx > 0 {
		return strings.TrimSpace(s[:idx])
	}

	// No paragraph break — return up to 500 chars at a sentence boundary
	if len(s) > 500 {
		chunk := s[:500]
		if idx := strings.LastIndexAny(chunk, ".!?"); idx > 0 {
			return chunk[:idx+1]
		}
		if idx := strings.LastIndex(chunk, " "); idx > 0 {
			return chunk[:idx]
		}
		return chunk
	}

	return s
}
