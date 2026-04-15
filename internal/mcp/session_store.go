package mcp

import (
	"database/sql"
	"fmt"
	"time"
)

// SessionStore persists MCP session data to SQLite.
// The in-memory AgentTracker remains the cache for active sessions;
// this store is the durable source of truth that survives restarts.
type SessionStore struct {
	db *sql.DB
}

// NewSessionStore creates a session store using an existing SQLite connection.
func NewSessionStore(db *sql.DB) (*SessionStore, error) {
	s := &SessionStore{db: db}
	if err := s.init(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *SessionStore) init() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		mcp_session_id TEXT NOT NULL,
		agent TEXT NOT NULL DEFAULT '',
		node TEXT NOT NULL DEFAULT '',
		conversation_id TEXT NOT NULL DEFAULT '',
		tool_calls INTEGER NOT NULL DEFAULT 0,
		connected_at TEXT NOT NULL,
		disconnected_at TEXT NOT NULL DEFAULT '',
		last_seen_at TEXT NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("create sessions table: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_sessions_agent ON sessions(agent)`)
	if err != nil {
		return fmt.Errorf("create sessions agent index: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_sessions_connected ON sessions(connected_at)`)
	if err != nil {
		return fmt.Errorf("create sessions connected index: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_sessions_node ON sessions(node)`)
	if err != nil {
		return fmt.Errorf("create sessions node index: %w", err)
	}

	// Migration: add cost tracking columns
	s.db.Exec(`ALTER TABLE sessions ADD COLUMN input_tokens INTEGER NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE sessions ADD COLUMN output_tokens INTEGER NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE sessions ADD COLUMN cache_read_tokens INTEGER NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE sessions ADD COLUMN cache_create_tokens INTEGER NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE sessions ADD COLUMN cost_usd REAL NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE sessions ADD COLUMN model TEXT NOT NULL DEFAULT ''`)

	// Session turns — persists tool call content in real-time (not just count)
	_, err = s.db.Exec(`CREATE TABLE IF NOT EXISTS session_turns (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		role TEXT NOT NULL,
		tool_name TEXT NOT NULL DEFAULT '',
		content TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("create session_turns table: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_session_turns_session ON session_turns(session_id)`)
	if err != nil {
		return fmt.Errorf("create session_turns index: %w", err)
	}

	return nil
}

// Insert persists a new session row.
func (s *SessionStore) Insert(info *SessionInfo) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`INSERT INTO sessions (id, mcp_session_id, agent, node, conversation_id, tool_calls, connected_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		info.UUID, info.MCPSession, info.Agent, info.Node,
		info.ConversationID, info.ToolCalls,
		info.ConnectedAt.UTC().Format(time.RFC3339), now)
	return err
}

// UpdateToolCalls updates the tool call count and last_seen_at timestamp.
func (s *SessionStore) UpdateToolCalls(uuid string, toolCalls int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE sessions SET tool_calls = ?, last_seen_at = ? WHERE id = ?`,
		toolCalls, now, uuid)
	return err
}

// UpdateCost updates the session's token counts and computed cost.
func (s *SessionStore) UpdateCost(uuid string, inputTokens, outputTokens, cacheReadTokens, cacheCreateTokens int, costUSD float64, model string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE sessions SET input_tokens = ?, output_tokens = ?, cache_read_tokens = ?, cache_create_tokens = ?, cost_usd = ?, model = ?, last_seen_at = ? WHERE id = ?`,
		inputTokens, outputTokens, cacheReadTokens, cacheCreateTokens, costUSD, model, now, uuid)
	return err
}

// RecordTurn persists a single conversation turn (tool call or result) immediately.
func (s *SessionStore) RecordTurn(sessionID, role, toolName, content string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if len(content) > 5000 {
		content = content[:5000] + "..."
	}
	_, err := s.db.Exec(`INSERT INTO session_turns (session_id, role, tool_name, content, created_at)
		VALUES (?, ?, ?, ?, ?)`, sessionID, role, toolName, content, now)
	return err
}

// ListTurns returns all turns for a session, ordered chronologically.
func (s *SessionStore) ListTurns(sessionID string, limit int) ([]SessionTurn, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.db.Query(`SELECT id, session_id, role, tool_name, content, created_at
		FROM session_turns WHERE session_id = ? ORDER BY id ASC LIMIT ?`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []SessionTurn
	for rows.Next() {
		var t SessionTurn
		var createdStr string
		if err := rows.Scan(&t.ID, &t.SessionID, &t.Role, &t.ToolName, &t.Content, &createdStr); err != nil {
			return nil, err
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		result = append(result, t)
	}
	return result, rows.Err()
}

// TurnCount returns the number of persisted turns for a session.
func (s *SessionStore) TurnCount(sessionID string) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM session_turns WHERE session_id = ?`, sessionID).Scan(&count)
	return count, err
}

// SessionTurn is a persisted conversation turn.
type SessionTurn struct {
	ID        int       `json:"id"`
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"`
	ToolName  string    `json:"tool_name"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// MarkDisconnected sets the disconnected_at timestamp.
func (s *SessionStore) MarkDisconnected(uuid string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE sessions SET disconnected_at = ?, last_seen_at = ? WHERE id = ?`,
		now, now, uuid)
	return err
}

// Count returns total session count (optionally only connected sessions).
func (s *SessionStore) Count(connectedOnly bool) (int, error) {
	query := `SELECT COUNT(*) FROM sessions`
	if connectedOnly {
		query += ` WHERE disconnected_at = ''`
	}
	var count int
	err := s.db.QueryRow(query).Scan(&count)
	return count, err
}

// List returns sessions ordered by connected_at desc, with optional filters.
func (s *SessionStore) List(connectedOnly bool, limit int) ([]*SessionInfo, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT id, mcp_session_id, agent, node, conversation_id, tool_calls, connected_at, disconnected_at, last_seen_at
		FROM sessions`
	if connectedOnly {
		query += ` WHERE disconnected_at = ''`
	}
	query += ` ORDER BY connected_at DESC LIMIT ?`

	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	return scanSessions(rows)
}

// ListByAgent returns sessions for a specific agent.
func (s *SessionStore) ListByAgent(agent string, limit int) ([]*SessionInfo, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id, mcp_session_id, agent, node, conversation_id, tool_calls, connected_at, disconnected_at, last_seen_at
		FROM sessions WHERE agent = ? ORDER BY connected_at DESC LIMIT ?`, agent, limit)
	if err != nil {
		return nil, fmt.Errorf("list sessions by agent: %w", err)
	}
	defer rows.Close()

	return scanSessions(rows)
}

// ListByNode returns sessions for a specific node.
func (s *SessionStore) ListByNode(nodeName string, limit int) ([]*SessionInfo, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id, mcp_session_id, agent, node, conversation_id, tool_calls, connected_at, disconnected_at, last_seen_at
		FROM sessions WHERE node = ? ORDER BY connected_at DESC LIMIT ?`, nodeName, limit)
	if err != nil {
		return nil, fmt.Errorf("list sessions by node: %w", err)
	}
	defer rows.Close()

	return scanSessions(rows)
}

// Get retrieves a single session by UUID.
func (s *SessionStore) Get(uuid string) (*SessionInfo, error) {
	row := s.db.QueryRow(`SELECT id, mcp_session_id, agent, node, conversation_id, tool_calls, connected_at, disconnected_at, last_seen_at
		FROM sessions WHERE id = ?`, uuid)

	var info SessionInfo
	var connectedAt, disconnectedAt, lastSeenAt string
	err := row.Scan(&info.UUID, &info.MCPSession, &info.Agent, &info.Node,
		&info.ConversationID, &info.ToolCalls,
		&connectedAt, &disconnectedAt, &lastSeenAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	info.ConnectedAt, _ = time.Parse(time.RFC3339, connectedAt)
	if disconnectedAt != "" {
		t, _ := time.Parse(time.RFC3339, disconnectedAt)
		info.DisconnectedAt = &t
	}
	info.LastSeenAt, _ = time.Parse(time.RFC3339, lastSeenAt)

	return &info, nil
}

func scanSessions(rows *sql.Rows) ([]*SessionInfo, error) {
	var result []*SessionInfo
	for rows.Next() {
		var info SessionInfo
		var connectedAt, disconnectedAt, lastSeenAt string
		if err := rows.Scan(&info.UUID, &info.MCPSession, &info.Agent, &info.Node,
			&info.ConversationID, &info.ToolCalls,
			&connectedAt, &disconnectedAt, &lastSeenAt); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		info.ConnectedAt, _ = time.Parse(time.RFC3339, connectedAt)
		if disconnectedAt != "" {
			t, _ := time.Parse(time.RFC3339, disconnectedAt)
			info.DisconnectedAt = &t
		}
		info.LastSeenAt, _ = time.Parse(time.RFC3339, lastSeenAt)
		result = append(result, &info)
	}
	return result, rows.Err()
}
