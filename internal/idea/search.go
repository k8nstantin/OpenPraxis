package idea

import "strings"

// Search finds ideas matching a query. Supports id-exact, id-prefix
// (UUID prefix substring), and keyword substring in title/description/
// tags. Mirrors manifest.Store.Search shape per 019daafb-b5e M1.
func (s *Store) Search(query string, limit int) ([]*Idea, error) {
	if limit <= 0 {
		limit = 50
	}
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	pattern := "%" + q + "%"
	rows, err := s.db.Query(`SELECT id, title, description, status, priority, tags, author, source_node, project_id, created_at, updated_at
		FROM ideas WHERE deleted_at = '' AND (id LIKE ? OR title LIKE ? OR description LIKE ? OR tags LIKE ?)
		ORDER BY CASE status WHEN 'draft' THEN 0 WHEN 'open' THEN 1 WHEN 'closed' THEN 2 WHEN 'archive' THEN 3 ELSE 4 END, updated_at DESC LIMIT ?`,
		pattern, pattern, pattern, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*Idea
	for rows.Next() {
		i, err := scanIdeaRows(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, i)
	}
	return results, rows.Err()
}
