package marker

import (
	"database/sql"
	"fmt"
	"time"
)

// Store manages marker persistence in SQLite.
type Store struct {
	db *sql.DB
}

// NewStore creates a marker store using an existing SQLite connection.
func NewStore(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.init(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) init() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS markers (
		id TEXT PRIMARY KEY,
		target_id TEXT NOT NULL,
		target_type TEXT NOT NULL,
		target_path TEXT NOT NULL DEFAULT '',
		from_node TEXT NOT NULL,
		to_node TEXT NOT NULL DEFAULT 'all',
		message TEXT NOT NULL DEFAULT '',
		priority TEXT NOT NULL DEFAULT 'normal',
		status TEXT NOT NULL DEFAULT 'pending',
		created_at TEXT NOT NULL,
		seen_at TEXT,
		done_at TEXT
	)`)
	if err != nil {
		return fmt.Errorf("create markers table: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_markers_to ON markers(to_node, status)`)
	if err != nil {
		return fmt.Errorf("create markers index: %w", err)
	}

	return nil
}

// Save inserts a new marker.
func (s *Store) Save(m *Marker) error {
	_, err := s.db.Exec(`INSERT INTO markers (id, target_id, target_type, target_path, from_node, to_node, message, priority, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.TargetID, m.TargetType, m.TargetPath, m.FromNode, m.ToNode,
		m.Message, m.Priority, m.Status, m.CreatedAt.Format(time.RFC3339))
	return err
}

// ListForNode returns markers addressed to this node (or "all"), newest first.
func (s *Store) ListForNode(nodeID string, status string, limit int) ([]*Marker, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `SELECT id, target_id, target_type, target_path, from_node, to_node, message, priority, status, created_at, seen_at, done_at
		FROM markers WHERE (to_node = ? OR to_node = 'all')`
	args := []any{nodeID}

	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}

	query += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*Marker
	for rows.Next() {
		m, err := scanMarker(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, m)
	}
	return results, rows.Err()
}

// PendingCount returns number of pending markers for this node.
func (s *Store) PendingCount(nodeID string) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM markers WHERE (to_node = ? OR to_node = 'all') AND status = 'pending'`, nodeID).Scan(&count)
	return count, err
}

// MarkSeen updates marker status to "seen".
func (s *Store) MarkSeen(id string) error {
	_, err := s.db.Exec(`UPDATE markers SET status = 'seen', seen_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// MarkDone updates marker status to "done".
func (s *Store) MarkDone(id string) error {
	_, err := s.db.Exec(`UPDATE markers SET status = 'done', done_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// Delete removes a marker.
func (s *Store) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM markers WHERE id = ?`, id)
	return err
}

func scanMarker(rows *sql.Rows) (*Marker, error) {
	var m Marker
	var createdStr string
	var seenStr, doneStr sql.NullString

	err := rows.Scan(&m.ID, &m.TargetID, &m.TargetType, &m.TargetPath,
		&m.FromNode, &m.ToNode, &m.Message, &m.Priority, &m.Status,
		&createdStr, &seenStr, &doneStr)
	if err != nil {
		return nil, err
	}

	m.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	if seenStr.Valid {
		t, _ := time.Parse(time.RFC3339, seenStr.String)
		m.SeenAt = &t
	}
	if doneStr.Valid {
		t, _ := time.Parse(time.RFC3339, doneStr.String)
		m.DoneAt = &t
	}

	return &m, nil
}
