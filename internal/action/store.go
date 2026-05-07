package action

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Action represents a single tool call by an agent session.
type Action struct {
	ID           string    `json:"id"`
	SessionID    string    `json:"session_id"`
	SourceNode   string    `json:"source_node"`
	TaskID       string    `json:"task_id"`
	ToolName     string    `json:"tool_name"`
	ToolInput    string    `json:"tool_input"`
	ToolResponse string    `json:"tool_response"`
	CWD          string    `json:"cwd"`
	TurnNumber   int       `json:"turn_number"` // agent turn this action belongs to; 0 = pre-feature
	CreatedAt    time.Time `json:"created_at"`
}

// Store manages action persistence in SQLite.
type Store struct {
	db *sql.DB
}

// NewStore creates an action store using an existing SQLite connection.
func NewStore(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.init(); err != nil {
		return nil, err
	}
	if err := s.InitAmnesia(); err != nil {
		return nil, err
	}
	if err := s.InitConfirmations(); err != nil {
		return nil, err
	}
	if err := s.InitRulePatterns(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) init() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS actions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		tool_name TEXT NOT NULL,
		tool_input TEXT NOT NULL DEFAULT '',
		tool_response TEXT NOT NULL DEFAULT '',
		cwd TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("create actions table: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_actions_session ON actions(session_id)`)
	if err != nil {
		return fmt.Errorf("create session index: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_actions_created ON actions(created_at DESC)`)
	if err != nil {
		return fmt.Errorf("create created index: %w", err)
	}

	// Migrations
	s.db.Exec(`ALTER TABLE actions ADD COLUMN source_node TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE actions ADD COLUMN task_id TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE actions ADD COLUMN turn_number INTEGER NOT NULL DEFAULT 0`)

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_actions_task ON actions(task_id)`)
	if err != nil {
		return fmt.Errorf("create task index: %w", err)
	}

	return nil
}

// Record stores a new action.
func (s *Store) Record(sessionID, sourceNode, toolName string, toolInput, toolResponse any, cwd string) error {
	inputStr := marshalAny(toolInput)
	responseStr := marshalAny(toolResponse)

	// Truncate large responses
	if len(responseStr) > 5000 {
		responseStr = responseStr[:5000] + "..."
	}

	_, err := s.db.Exec(`INSERT INTO actions (session_id, source_node, tool_name, tool_input, tool_response, cwd, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sessionID, sourceNode, toolName, inputStr, responseStr, cwd,
		time.Now().UTC().Format(time.RFC3339))
	return err
}

// RecordForTask stores an action linked to a task and returns the inserted row ID.
// turnNumber is the current agent turn (1-based); 0 means unknown/pre-feature rows.
func (s *Store) RecordForTask(taskID, sourceNode, toolName, toolInput, toolResponse, cwd string, turnNumber int) (int64, error) {
	// Truncate large responses
	if len(toolResponse) > 5000 {
		toolResponse = toolResponse[:5000] + "..."
	}

	result, err := s.db.Exec(`INSERT INTO actions (session_id, source_node, task_id, tool_name, tool_input, tool_response, cwd, turn_number, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"task:"+taskID, sourceNode, taskID, toolName, toolInput, toolResponse, cwd, turnNumber,
		time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// UpdateResponseByID updates the tool_response of a specific action by row ID.
func (s *Store) UpdateResponseByID(actionID int64, response string) error {
	if len(response) > 5000 {
		response = response[:5000] + "..."
	}
	_, err := s.db.Exec(`UPDATE actions SET tool_response = ? WHERE id = ?`, response, actionID)
	return err
}

// ListBySession returns actions for a session, most recent first.
func (s *Store) ListBySession(sessionID string, limit int) ([]Action, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT id, session_id, source_node, task_id, tool_name, tool_input, tool_response, cwd, turn_number, created_at
		FROM actions WHERE session_id = ? OR session_id LIKE ? ORDER BY created_at DESC LIMIT ?`, sessionID, sessionID+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanActions(rows)
}

// ListRecent returns the most recent actions across all sessions.
func (s *Store) ListRecent(limit int) ([]Action, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id, session_id, source_node, task_id, tool_name, tool_input, tool_response, cwd, turn_number, created_at
		FROM actions ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanActions(rows)
}

// ListRecentPaged is ListRecent + offset + total. Drives the
// Actions Log "show every action ever, browse with pagination" feed
// when no search query is set. Mirrors SearchPaged so the UI can hit
// a single endpoint shape with optional q.
func (s *Store) ListRecentPaged(limit, offset int) ([]Action, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	total, err := s.Count()
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(`SELECT id, session_id, source_node, task_id, tool_name, tool_input, tool_response, cwd, turn_number, created_at
		FROM actions ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items, err := scanActions(rows)
	return items, total, err
}

// GetByID returns a single action by ID.
func (s *Store) GetByID(id string) (*Action, error) {
	row := s.db.QueryRow(`SELECT id, session_id, source_node, task_id, tool_name, tool_input, tool_response, cwd, turn_number, created_at FROM actions WHERE id = ?`, id)
	var a Action
	var createdStr string
	err := row.Scan(&a.ID, &a.SessionID, &a.SourceNode, &a.TaskID, &a.ToolName, &a.ToolInput, &a.ToolResponse, &a.CWD, &a.TurnNumber, &createdStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	return &a, nil
}

// ListByTask returns actions for a task.
func (s *Store) ListByTask(taskID string, limit int) ([]Action, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT id, session_id, source_node, task_id, tool_name, tool_input, tool_response, cwd, turn_number, created_at
		FROM actions WHERE task_id = ? OR task_id LIKE ? ORDER BY created_at DESC LIMIT ?`, taskID, taskID+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanActions(rows)
}

// Count returns total actions.
func (s *Store) Count() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM actions`).Scan(&count)
	return count, err
}

// SessionSummary returns action counts per session.
func (s *Store) SessionSummary() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT session_id, COUNT(*) FROM actions GROUP BY session_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int)
	for rows.Next() {
		var sid string
		var count int
		if err := rows.Scan(&sid, &count); err != nil {
			return nil, err
		}
		result[sid] = count
	}
	return result, rows.Err()
}

func scanActions(rows *sql.Rows) ([]Action, error) {
	var actions []Action
	for rows.Next() {
		var a Action
		var createdStr string
		if err := rows.Scan(&a.ID, &a.SessionID, &a.SourceNode, &a.TaskID, &a.ToolName, &a.ToolInput, &a.ToolResponse, &a.CWD, &a.TurnNumber, &createdStr); err != nil {
			return nil, err
		}
		a.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		actions = append(actions, a)
	}
	return actions, rows.Err()
}

func marshalAny(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case nil:
		return ""
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(b)
	}
}
