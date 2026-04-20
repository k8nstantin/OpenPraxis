package task

import "strings"

// Search finds tasks matching a query. Supports id-exact, id-prefix
// (marker or UUID prefix), id-substring, and keyword substring in
// title/description. Mirrors manifest.Store.Search (see 019daafb-b5e
// M1 per-entity scoped search) — the shape is a single LIKE OR-set
// because the sibling ladder in internal/memory is only warranted where
// semantic fallback and path lookups live.
func (s *Store) Search(query string, limit int) ([]*Task, error) {
	if limit <= 0 {
		limit = 50
	}
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	pattern := "%" + q + "%"
	rows, err := s.db.Query(`SELECT `+taskColumns+`
		FROM tasks WHERE deleted_at = '' AND (id LIKE ? OR title LIKE ? OR description LIKE ?)
		ORDER BY updated_at DESC LIMIT ?`, pattern, pattern, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTasks(rows)
}
