package action

import (
	"database/sql"
	"fmt"
	"time"
)

// Match types for amnesia detection.
const (
	MatchTypeSimilarity      = "similarity"
	MatchTypeForbiddenPattern = "forbidden_pattern"
)

// Amnesia represents a visceral rule violation — the agent "forgot" a rule.
type Amnesia struct {
	ID             int       `json:"id"`
	SessionID      string    `json:"session_id"`
	SourceNode     string    `json:"source_node"`
	ActionID       string    `json:"action_id"`
	TaskID         string    `json:"task_id"`
	RuleID         string    `json:"rule_id"`
	RuleMarker     string    `json:"rule_marker"`
	RuleText       string    `json:"rule_text"`
	ToolName       string    `json:"tool_name"`
	ToolInput      string    `json:"tool_input"`
	Score          float64   `json:"score"`
	MatchType      string    `json:"match_type"`
	MatchedPattern string    `json:"matched_pattern"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}

// InitAmnesia creates the amnesia table on an existing DB.
func (s *Store) InitAmnesia() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS amnesia (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		action_id TEXT NOT NULL DEFAULT '',
		rule_id TEXT NOT NULL,
		rule_marker TEXT NOT NULL,
		rule_text TEXT NOT NULL,
		tool_name TEXT NOT NULL,
		tool_input TEXT NOT NULL DEFAULT '',
		score REAL NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'flagged',
		created_at TEXT NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("create amnesia table: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_amnesia_status ON amnesia(status)`)
	if err != nil {
		return fmt.Errorf("create amnesia status index: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_amnesia_created ON amnesia(created_at DESC)`)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_amnesia_session ON amnesia(session_id)`)
	if err != nil {
		return fmt.Errorf("create amnesia session index: %w", err)
	}

	s.db.Exec(`ALTER TABLE amnesia ADD COLUMN source_node TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE amnesia ADD COLUMN task_id TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE amnesia ADD COLUMN match_type TEXT NOT NULL DEFAULT 'similarity'`)
	s.db.Exec(`ALTER TABLE amnesia ADD COLUMN matched_pattern TEXT NOT NULL DEFAULT ''`)
	return nil
}

// RecordAmnesia stores a visceral rule violation.
func (s *Store) RecordAmnesia(sessionID, sourceNode, actionID, taskID, ruleID, ruleMarker, ruleText, toolName, toolInput string, score float64, matchType, matchedPattern string) error {
	if len(toolInput) > 1000 {
		toolInput = toolInput[:1000] + "..."
	}
	if matchType == "" {
		matchType = MatchTypeSimilarity
	}
	_, err := s.db.Exec(`INSERT INTO amnesia (session_id, source_node, action_id, task_id, rule_id, rule_marker, rule_text, tool_name, tool_input, score, match_type, matched_pattern, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'flagged', ?)`,
		sessionID, sourceNode, actionID, taskID, ruleID, ruleMarker, ruleText, toolName, toolInput, score, matchType, matchedPattern,
		time.Now().UTC().Format(time.RFC3339))
	return err
}

// ListAmnesia returns amnesia events, most recent first.
func (s *Store) ListAmnesia(status string, limit int) ([]Amnesia, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `SELECT id, session_id, source_node, action_id, task_id, rule_id, rule_marker, rule_text, tool_name, tool_input, score, match_type, matched_pattern, status, created_at FROM amnesia`
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

	return scanAmnesia(rows)
}

// ListAmnesiaByTask returns amnesia events for a specific task.
func (s *Store) ListAmnesiaByTask(taskID string, limit int) ([]Amnesia, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id, session_id, source_node, action_id, task_id, rule_id, rule_marker, rule_text, tool_name, tool_input, score, match_type, matched_pattern, status, created_at
		FROM amnesia WHERE task_id = ? OR task_id LIKE ? ORDER BY created_at DESC LIMIT ?`, taskID, taskID+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanAmnesia(rows)
}

// scanAmnesia extracts Amnesia records from rows.
func scanAmnesia(rows *sql.Rows) ([]Amnesia, error) {
	var results []Amnesia
	for rows.Next() {
		var a Amnesia
		var createdStr string
		if err := rows.Scan(&a.ID, &a.SessionID, &a.SourceNode, &a.ActionID, &a.TaskID, &a.RuleID, &a.RuleMarker, &a.RuleText, &a.ToolName, &a.ToolInput, &a.Score, &a.MatchType, &a.MatchedPattern, &a.Status, &createdStr); err != nil {
			return nil, err
		}
		a.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		if a.MatchType == "" {
			a.MatchType = MatchTypeSimilarity
		}
		results = append(results, a)
	}
	return results, rows.Err()
}

// UpdateStatus changes an amnesia event's status.
func (s *Store) UpdateStatus(id int, status string) error {
	_, err := s.db.Exec(`UPDATE amnesia SET status = ? WHERE id = ?`, status, id)
	return err
}

// CountByStatus returns counts per status.
func (s *Store) CountByStatus() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT status, COUNT(*) FROM amnesia GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		result[status] = count
	}
	return result, rows.Err()
}

// PendingCount returns number of flagged amnesia events.
func (s *Store) PendingCount() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM amnesia WHERE status = 'flagged'`).Scan(&count)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return count, err
}

// VisceralConfirmation tracks when a session confirms reading visceral rules.
type VisceralConfirmation struct {
	ID         int       `json:"id"`
	SessionID  string    `json:"session_id"`
	RulesCount int       `json:"rules_count"`
	CreatedAt  time.Time `json:"created_at"`
}

// InitConfirmations creates the confirmations table.
func (s *Store) InitConfirmations() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS visceral_confirmations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		rules_count INTEGER NOT NULL,
		created_at TEXT NOT NULL
	)`)
	return err
}

// RecordConfirmation stores a visceral rules confirmation.
func (s *Store) RecordConfirmation(sessionID string, rulesCount int) error {
	_, err := s.db.Exec(`INSERT INTO visceral_confirmations (session_id, rules_count, created_at) VALUES (?, ?, ?)`,
		sessionID, rulesCount, time.Now().UTC().Format(time.RFC3339))
	return err
}

// ListConfirmations returns recent confirmations.
func (s *Store) ListConfirmations(limit int) ([]VisceralConfirmation, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`SELECT id, session_id, rules_count, created_at FROM visceral_confirmations ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []VisceralConfirmation
	for rows.Next() {
		var c VisceralConfirmation
		var createdStr string
		if err := rows.Scan(&c.ID, &c.SessionID, &c.RulesCount, &createdStr); err != nil {
			return nil, err
		}
		c.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		results = append(results, c)
	}
	return results, rows.Err()
}
