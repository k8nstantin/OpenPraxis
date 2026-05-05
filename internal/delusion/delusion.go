// Package delusion manages the delusions table — records of agents deviating
// from active manifests (going off-spec). Extracted from internal/manifest so
// neither internal/node nor internal/web need to import the manifest package.
package delusion

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

// Store wraps the delusions table.
type Store struct {
	db *sql.DB
}

// New returns a Store backed by db. InitSchema must have been called first
// (node.New does this via the manifest store's InitDelusions; this store
// reads from/writes to the same table).
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// InitSchema creates the delusions table and its indexes. Idempotent.
func InitSchema(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS delusions (
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
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_delusions_status ON delusions(status)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_delusions_created ON delusions(created_at DESC)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_delusions_session ON delusions(session_id)`)
	db.Exec(`ALTER TABLE delusions ADD COLUMN source_node TEXT NOT NULL DEFAULT ''`)
	db.Exec(`ALTER TABLE delusions ADD COLUMN action_id TEXT NOT NULL DEFAULT ''`)
	db.Exec(`ALTER TABLE delusions ADD COLUMN task_id TEXT NOT NULL DEFAULT ''`)
	return nil
}

// Record stores a manifest deviation.
func (s *Store) Record(sessionID, sourceNode, actionID, taskID, manifestID, manifestMarker, manifestTitle, toolName, toolInput string, score float64, reason string) error {
	if len(toolInput) > 2000 {
		toolInput = toolInput[:2000] + "..."
	}
	_, err := s.db.Exec(`INSERT INTO delusions (session_id, source_node, action_id, task_id, manifest_id, manifest_marker, manifest_title, tool_name, tool_input, score, reason, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'flagged', ?)`,
		sessionID, sourceNode, actionID, taskID, manifestID, manifestMarker, manifestTitle, toolName, toolInput, score, reason,
		time.Now().UTC().Format(time.RFC3339))
	return err
}

// List returns delusion events, newest first.
func (s *Store) List(status string, limit int) ([]Delusion, error) {
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

// UpdateStatus changes a delusion's status.
func (s *Store) UpdateStatus(id int, status string) error {
	_, err := s.db.Exec(`UPDATE delusions SET status = ? WHERE id = ?`, status, id)
	return err
}

// PendingCount returns flagged delusion count.
func (s *Store) PendingCount() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM delusions WHERE status = 'flagged'`).Scan(&count)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return count, err
}
