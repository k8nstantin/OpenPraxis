package product

import "strings"

// Search finds products matching a query. Supports id-exact, id-prefix
// (UUID prefix substring), and keyword substring in title/description/
// tags. Mirrors manifest.Store.Search shape per 019daafb-b5e M1.
func (s *Store) Search(query string, limit int) ([]*Product, error) {
	if limit <= 0 {
		limit = 50
	}
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	pattern := "%" + q + "%"
	rows, err := s.db.Query(`SELECT id, title, description, status, source_node, tags, created_at, updated_at
		FROM products WHERE deleted_at = '' AND (id LIKE ? OR title LIKE ? OR description LIKE ? OR tags LIKE ?)
		ORDER BY CASE status WHEN 'draft' THEN 0 WHEN 'open' THEN 1 WHEN 'closed' THEN 2 WHEN 'archive' THEN 3 ELSE 4 END, updated_at DESC LIMIT ?`,
		pattern, pattern, pattern, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*Product
	for rows.Next() {
		p, err := scanProductRows(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, p)
	}
	s.EnrichWithCosts(results)
	return results, rows.Err()
}
