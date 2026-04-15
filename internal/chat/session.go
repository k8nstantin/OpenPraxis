package chat

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

// Session represents a chat session with its message history.
type Session struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	Model         string    `json:"model"`
	ThinkingLevel string    `json:"thinking_level"`
	Messages      []Message `json:"messages"`
	SystemPrompt  string    `json:"system_prompt"`
	TokensIn      int       `json:"tokens_in"`
	TokensOut     int       `json:"tokens_out"`
	CostUSD       float64   `json:"cost_usd"`
	SourceNode    string    `json:"source_node"`
	Deleted       bool      `json:"deleted"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// SessionStore manages chat sessions in SQLite.
type SessionStore struct {
	db *sql.DB
}

// NewSessionStore creates the store and ensures the table exists.
// Uses WAL mode + busy_timeout per visceral rule.
func NewSessionStore(db *sql.DB) (*SessionStore, error) {
	// WAL mode + busy_timeout (visceral rule #10)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		return nil, fmt.Errorf("set busy_timeout: %w", err)
	}

	ddl := `CREATE TABLE IF NOT EXISTS chat_sessions (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL DEFAULT 'New Chat',
		model TEXT NOT NULL DEFAULT 'anthropic/claude-sonnet-4-6',
		thinking_level TEXT NOT NULL DEFAULT 'off',
		messages TEXT NOT NULL DEFAULT '[]',
		system_prompt TEXT NOT NULL DEFAULT '',
		tokens_in INTEGER NOT NULL DEFAULT 0,
		tokens_out INTEGER NOT NULL DEFAULT 0,
		cost_usd REAL NOT NULL DEFAULT 0,
		source_node TEXT NOT NULL DEFAULT '',
		deleted INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`
	if _, err := db.Exec(ddl); err != nil {
		return nil, fmt.Errorf("create chat_sessions table: %w", err)
	}

	return &SessionStore{db: db}, nil
}

// Create creates a new chat session.
func (s *SessionStore) Create(model, sourceNode string) (*Session, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	sess := &Session{
		ID:            id.String(),
		Title:         "New Chat",
		Model:         model,
		ThinkingLevel: "off",
		Messages:      []Message{},
		SourceNode:    sourceNode,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	msgsJSON, _ := json.Marshal(sess.Messages)
	_, err = s.db.Exec(`INSERT INTO chat_sessions (id, title, model, thinking_level, messages, system_prompt, tokens_in, tokens_out, cost_usd, source_node, deleted, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, 0, 0, ?, 0, ?, ?)`,
		sess.ID, sess.Title, sess.Model, sess.ThinkingLevel, string(msgsJSON),
		sess.SystemPrompt, sess.SourceNode,
		now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("insert session: %w", err)
	}
	return sess, nil
}

// Get returns a session by ID.
func (s *SessionStore) Get(id string) (*Session, error) {
	row := s.db.QueryRow(`SELECT id, title, model, thinking_level, messages, system_prompt, tokens_in, tokens_out, cost_usd, source_node, deleted, created_at, updated_at
		FROM chat_sessions WHERE id = ?`, id)
	return scanSession(row)
}

// List returns all non-deleted sessions, most recent first.
func (s *SessionStore) List(limit int) ([]*Session, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id, title, model, thinking_level, messages, system_prompt, tokens_in, tokens_out, cost_usd, source_node, deleted, created_at, updated_at
		FROM chat_sessions WHERE deleted = 0 ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		sess, err := scanSessionRow(rows)
		if err != nil {
			continue
		}
		sessions = append(sessions, sess)
	}
	return sessions, nil
}

// ListDeleted returns soft-deleted sessions for recall.
func (s *SessionStore) ListDeleted(limit int) ([]*Session, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id, title, model, thinking_level, messages, system_prompt, tokens_in, tokens_out, cost_usd, source_node, deleted, created_at, updated_at
		FROM chat_sessions WHERE deleted = 1 ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		sess, err := scanSessionRow(rows)
		if err != nil {
			continue
		}
		sessions = append(sessions, sess)
	}
	return sessions, nil
}

// Update saves session state back to the database.
func (s *SessionStore) Update(sess *Session) error {
	msgsJSON, err := json.Marshal(sess.Messages)
	if err != nil {
		return err
	}
	sess.UpdatedAt = time.Now().UTC()
	_, err = s.db.Exec(`UPDATE chat_sessions SET title=?, model=?, thinking_level=?, messages=?, system_prompt=?, tokens_in=?, tokens_out=?, cost_usd=?, updated_at=? WHERE id=?`,
		sess.Title, sess.Model, sess.ThinkingLevel, string(msgsJSON), sess.SystemPrompt,
		sess.TokensIn, sess.TokensOut, sess.CostUSD,
		sess.UpdatedAt.Format(time.RFC3339), sess.ID)
	return err
}

// UpdateTitle sets the session title.
func (s *SessionStore) UpdateTitle(id, title string) error {
	_, err := s.db.Exec(`UPDATE chat_sessions SET title=?, updated_at=? WHERE id=?`,
		title, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// UpdateModel switches the model for a session.
func (s *SessionStore) UpdateModel(id, model string) error {
	_, err := s.db.Exec(`UPDATE chat_sessions SET model=?, updated_at=? WHERE id=?`,
		model, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// UpdateThinking sets the thinking level for a session.
func (s *SessionStore) UpdateThinking(id, level string) error {
	_, err := s.db.Exec(`UPDATE chat_sessions SET thinking_level=?, updated_at=? WHERE id=?`,
		level, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// Delete soft-deletes a session (moves to recall).
func (s *SessionStore) Delete(id string) error {
	_, err := s.db.Exec(`UPDATE chat_sessions SET deleted=1, updated_at=? WHERE id=?`,
		time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// Restore un-deletes a session from recall.
func (s *SessionStore) Restore(id string) error {
	_, err := s.db.Exec(`UPDATE chat_sessions SET deleted=0, updated_at=? WHERE id=?`,
		time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// Reset clears messages and resets token counts for a session.
func (s *SessionStore) Reset(id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE chat_sessions SET messages='[]', tokens_in=0, tokens_out=0, cost_usd=0, updated_at=? WHERE id=?`,
		now, id)
	return err
}

// AddTokens increments token counts and cost for a session.
func (s *SessionStore) AddTokens(id string, tokensIn, tokensOut int, cost float64) error {
	_, err := s.db.Exec(`UPDATE chat_sessions SET tokens_in=tokens_in+?, tokens_out=tokens_out+?, cost_usd=cost_usd+?, updated_at=? WHERE id=?`,
		tokensIn, tokensOut, cost, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// AppendMessage adds a message to the session's history.
func (s *SessionStore) AppendMessage(id string, msg Message) error {
	sess, err := s.Get(id)
	if err != nil {
		return err
	}
	sess.Messages = append(sess.Messages, msg)
	return s.Update(sess)
}

// TotalCost returns the sum of cost_usd across all sessions.
func (s *SessionStore) TotalCost() (float64, error) {
	var total sql.NullFloat64
	err := s.db.QueryRow(`SELECT SUM(cost_usd) FROM chat_sessions`).Scan(&total)
	if err != nil {
		return 0, err
	}
	return total.Float64, nil
}

// TotalCostToday returns cost for today (UTC).
func (s *SessionStore) TotalCostToday() (float64, error) {
	today := time.Now().UTC().Format("2006-01-02")
	var total sql.NullFloat64
	err := s.db.QueryRow(`SELECT SUM(cost_usd) FROM chat_sessions WHERE updated_at >= ?`, today).Scan(&total)
	if err != nil {
		return 0, err
	}
	return total.Float64, nil
}

func scanSession(row *sql.Row) (*Session, error) {
	var sess Session
	var msgsJSON, createdAt, updatedAt string
	var deleted int
	err := row.Scan(&sess.ID, &sess.Title, &sess.Model, &sess.ThinkingLevel, &msgsJSON,
		&sess.SystemPrompt, &sess.TokensIn, &sess.TokensOut, &sess.CostUSD,
		&sess.SourceNode, &deleted, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	sess.Deleted = deleted == 1
	if err := json.Unmarshal([]byte(msgsJSON), &sess.Messages); err != nil {
		log.Printf("WARNING: unmarshal chat session messages: %v", err)
	}
	sess.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	sess.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &sess, nil
}

func scanSessionRow(rows *sql.Rows) (*Session, error) {
	var sess Session
	var msgsJSON, createdAt, updatedAt string
	var deleted int
	err := rows.Scan(&sess.ID, &sess.Title, &sess.Model, &sess.ThinkingLevel, &msgsJSON,
		&sess.SystemPrompt, &sess.TokensIn, &sess.TokensOut, &sess.CostUSD,
		&sess.SourceNode, &deleted, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	sess.Deleted = deleted == 1
	if err := json.Unmarshal([]byte(msgsJSON), &sess.Messages); err != nil {
		log.Printf("WARNING: unmarshal chat session messages: %v", err)
	}
	sess.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	sess.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &sess, nil
}
