package manifest

import (
	"database/sql"
	"fmt"
	"time"
)

// Delusion represents an agent deviating from a manifest — going off-spec.
type Delusion struct {
	ID             int       `json:"id"`
	SessionID      string    `json:"session_id"`
	SourceNode     string    `json:"source_node"`
	ActionID       string    `json:"action_id"`
	TaskID         string    `json:"task_id"`
	ManifestID     string    `json:"manifest_id"`
	ManifestMarker string    `json:"manifest_marker"`
	ManifestTitle  string    `json:"manifest_title"`
	ToolName       string    `json:"tool_name"`
	ToolInput      string    `json:"tool_input"`
	Score          float64   `json:"score"`  // inverse — lower similarity to manifest = higher delusion
	Reason         string    `json:"reason"` // why it was flagged
	Status         string    `json:"status"` // flagged, confirmed, dismissed
	CreatedAt      time.Time `json:"created_at"`
}

// InitDelusions creates the delusions table.
func (s *Store) InitDelusions() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS delusions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		manifest_id TEXT NOT NULL,
		manifest_marker TEXT NOT NULL,
		manifest_title TEXT NOT NULL,
		tool_name TEXT NOT NULL,
		tool_input TEXT NOT NULL DEFAULT '',
		score REAL NOT NULL DEFAULT 0,
		reason TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'flagged',
		created_at TEXT NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("create delusions table: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_delusions_status ON delusions(status)`)
	if err != nil {
		return fmt.Errorf("create delusions status index: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_delusions_created ON delusions(created_at DESC)`)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_delusions_session ON delusions(session_id)`)
	if err != nil {
		return fmt.Errorf("create delusions session index: %w", err)
	}

	s.db.Exec(`ALTER TABLE delusions ADD COLUMN source_node TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE delusions ADD COLUMN action_id TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE delusions ADD COLUMN task_id TEXT NOT NULL DEFAULT ''`)
	return nil
}

// RecordDelusion stores a manifest deviation.
func (s *Store) RecordDelusion(sessionID, sourceNode, actionID, taskID, manifestID, manifestMarker, manifestTitle, toolName, toolInput string, score float64, reason string) error {
	if len(toolInput) > 2000 {
		toolInput = toolInput[:2000] + "..."
	}
	_, err := s.db.Exec(`INSERT INTO delusions (session_id, source_node, action_id, task_id, manifest_id, manifest_marker, manifest_title, tool_name, tool_input, score, reason, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'flagged', ?)`,
		sessionID, sourceNode, actionID, taskID, manifestID, manifestMarker, manifestTitle, toolName, toolInput, score, reason,
		time.Now().UTC().Format(time.RFC3339))
	return err
}

// ListDelusions returns delusion events.
func (s *Store) ListDelusions(status string, limit int) ([]Delusion, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `SELECT id, session_id, source_node, action_id, task_id, manifest_id, manifest_marker, manifest_title, tool_name, tool_input, score, reason, status, created_at FROM delusions`
	var args []any

	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}

	query += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Delusion
	for rows.Next() {
		var d Delusion
		var createdStr string
		if err := rows.Scan(&d.ID, &d.SessionID, &d.SourceNode, &d.ActionID, &d.TaskID, &d.ManifestID, &d.ManifestMarker, &d.ManifestTitle, &d.ToolName, &d.ToolInput, &d.Score, &d.Reason, &d.Status, &createdStr); err != nil {
			return nil, err
		}
		d.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		results = append(results, d)
	}
	return results, rows.Err()
}

// ListDelusionsByTask returns delusions for a specific task.
func (s *Store) ListDelusionsByTask(taskID string, limit int) ([]Delusion, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id, session_id, source_node, action_id, task_id, manifest_id, manifest_marker, manifest_title, tool_name, tool_input, score, reason, status, created_at
		FROM delusions WHERE task_id = ? OR task_id LIKE ? ORDER BY created_at DESC LIMIT ?`, taskID, taskID+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Delusion
	for rows.Next() {
		var d Delusion
		var createdStr string
		if err := rows.Scan(&d.ID, &d.SessionID, &d.SourceNode, &d.ActionID, &d.TaskID, &d.ManifestID, &d.ManifestMarker, &d.ManifestTitle, &d.ToolName, &d.ToolInput, &d.Score, &d.Reason, &d.Status, &createdStr); err != nil {
			return nil, err
		}
		d.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		results = append(results, d)
	}
	return results, rows.Err()
}

// UpdateDelusionStatus changes a delusion's status.
func (s *Store) UpdateDelusionStatus(id int, status string) error {
	_, err := s.db.Exec(`UPDATE delusions SET status = ? WHERE id = ?`, status, id)
	return err
}

// PendingDelusionCount returns flagged delusion count.
func (s *Store) PendingDelusionCount() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM delusions WHERE status = 'flagged'`).Scan(&count)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return count, err
}
