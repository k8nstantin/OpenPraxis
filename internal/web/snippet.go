package web

import (
	"html"
	"strings"
)

// highlightSnippet finds the first case-insensitive occurrence of q in any of
// the provided text fields, returns an HTML snippet with ~80 chars of context
// on each side, and wraps the match with <mark>...</mark>. Non-match text is
// HTML-escaped so it is safe to render with innerHTML.
//
// Fallback when q matches nothing in any field: returns the first 160 chars of
// the first non-empty field, HTML-escaped, without any <mark>. Returns empty
// string only when every field is empty.
//
// Note: text args are scanned in order, so pass the field you'd prefer to
// snippet from first (e.g. summary before raw turns blob).
func highlightSnippet(q string, texts ...string) string {
	q = strings.TrimSpace(q)
	for _, text := range texts {
		if text == "" {
			continue
		}
		idx := caseInsensitiveIndex(text, q)
		if q == "" || idx < 0 {
			continue
		}
		start := idx - 80
		if start < 0 {
			start = 0
		}
		end := idx + len(q) + 80
		if end > len(text) {
			end = len(text)
		}
		prefix := ""
		if start > 0 {
			prefix = "…"
		}
		suffix := ""
		if end < len(text) {
			suffix = "…"
		}
		before := html.EscapeString(text[start:idx])
		match := html.EscapeString(text[idx : idx+len(q)])
		after := html.EscapeString(text[idx+len(q) : end])
		return prefix + before + "<mark>" + match + "</mark>" + after + suffix
	}
	// No literal match — fall back to the first non-empty field truncated.
	for _, text := range texts {
		if text == "" {
			continue
		}
		trimmed := text
		if len(trimmed) > 160 {
			trimmed = trimmed[:160] + "…"
		}
		return html.EscapeString(trimmed)
	}
	return ""
}

func caseInsensitiveIndex(haystack, needle string) int {
	if needle == "" {
		return -1
	}
	return strings.Index(strings.ToLower(haystack), strings.ToLower(needle))
}
