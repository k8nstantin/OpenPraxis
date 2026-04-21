package action

import "strings"

// Search finds actions matching a query. Supports id match (LIKE on the
// integer id cast to text in SQLite) plus keyword substring in
// tool_name, tool_input, and tool_response. Mirrors manifest.Store.Search
// shape per 019daafb-b5e M1 — single OR-set LIKE, trim + empty-check
// guard to avoid `%%` matching every row.
func (s *Store) Search(query string, limit int) ([]Action, error) {
	items, _, err := s.SearchPaged(query, limit, 0)
	return items, err
}

// SearchPaged is Search plus offset + total. Returns the paged slice and the
// total match count so the caller can drive infinite-scroll pagination
// without a second COUNT query round-trip. Empty query returns (nil, 0, nil).
func (s *Store) SearchPaged(query string, limit, offset int) ([]Action, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, 0, nil
	}
	pattern := "%" + q + "%"

	var total int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM actions
		  WHERE CAST(id AS TEXT) LIKE ? OR tool_name LIKE ? OR tool_input LIKE ? OR tool_response LIKE ?`,
		pattern, pattern, pattern, pattern,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Query(
		`SELECT id, session_id, source_node, task_id, tool_name, tool_input, tool_response, cwd, created_at
		   FROM actions
		  WHERE CAST(id AS TEXT) LIKE ? OR tool_name LIKE ? OR tool_input LIKE ? OR tool_response LIKE ?
		  ORDER BY created_at DESC
		  LIMIT ? OFFSET ?`,
		pattern, pattern, pattern, pattern, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items, err := scanActions(rows)
	return items, total, err
}
