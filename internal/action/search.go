package action

import "strings"

// Search finds actions matching a query. Supports id match (LIKE on the
// integer id cast to text in SQLite) plus keyword substring in
// tool_name, tool_input, and tool_response. Mirrors manifest.Store.Search
// shape per 019daafb-b5e M1 — single OR-set LIKE, trim + empty-check
// guard to avoid `%%` matching every row.
func (s *Store) Search(query string, limit int) ([]Action, error) {
	if limit <= 0 {
		limit = 50
	}
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	pattern := "%" + q + "%"
	rows, err := s.db.Query(`SELECT id, session_id, source_node, task_id, tool_name, tool_input, tool_response, cwd, created_at
		FROM actions WHERE CAST(id AS TEXT) LIKE ? OR tool_name LIKE ? OR tool_input LIKE ? OR tool_response LIKE ?
		ORDER BY created_at DESC LIMIT ?`, pattern, pattern, pattern, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanActions(rows)
}
