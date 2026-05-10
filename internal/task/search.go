package task

// Search is a no-op stub. The tasks table has been retired; use
// entity.Store.Search for task search functionality.
func (s *Store) Search(query string, limit int) ([]*Task, error) {
	return nil, nil
}
