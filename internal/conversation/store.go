package conversation

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
)

// Store manages conversation persistence in SQLite + vector search.
type Store struct {
	db        *sql.DB
	dimension int
}

// NewStore creates a conversation store using an existing SQLite connection.
func NewStore(db *sql.DB, dimension int) (*Store, error) {
	s := &Store{db: db, dimension: dimension}
	if err := s.init(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) init() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS conversations (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		summary TEXT NOT NULL,
		agent TEXT NOT NULL DEFAULT '',
		project TEXT NOT NULL DEFAULT '',
		tags TEXT NOT NULL DEFAULT '[]',
		turns TEXT NOT NULL DEFAULT '[]',
		turn_count INTEGER NOT NULL DEFAULT 0,
		source_node TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		accessed_at TEXT NOT NULL,
		access_count INTEGER NOT NULL DEFAULT 0
	)`)
	if err != nil {
		return fmt.Errorf("create conversations table: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_conv_updated ON conversations(updated_at DESC)`)
	if err != nil {
		return fmt.Errorf("create created_at index: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_conv_agent ON conversations(agent)`)
	if err != nil {
		return fmt.Errorf("create agent index: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_conv_project ON conversations(project)`)
	if err != nil {
		return fmt.Errorf("create project index: %w", err)
	}

	// Vector table for conversation embeddings
	vecSQL := fmt.Sprintf(`CREATE VIRTUAL TABLE IF NOT EXISTS vec_conversations USING vec0(
		id TEXT PRIMARY KEY,
		embedding float[%d]
	)`, s.dimension)
	_, err = s.db.Exec(vecSQL)
	if err != nil {
		return fmt.Errorf("create vec_conversations: %w", err)
	}

	return nil
}

// Save stores a conversation and its embedding.
func (s *Store) Save(conv *Conversation, embedding []float32) error {
	tagsJSON, _ := json.Marshal(conv.Tags)
	turnsJSON, _ := json.Marshal(conv.Turns)
	vecBlob, err := sqlite_vec.SerializeFloat32(embedding)
	if err != nil {
		return fmt.Errorf("serialize vector: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`INSERT INTO conversations (id, title, summary, agent, project, tags, turns, turn_count, source_node, created_at, updated_at, accessed_at, access_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title=excluded.title, summary=excluded.summary, tags=excluded.tags,
			turns=excluded.turns, turn_count=excluded.turn_count, updated_at=excluded.updated_at,
			accessed_at=excluded.accessed_at`,
		conv.ID, conv.Title, conv.Summary, conv.Agent, conv.Project,
		string(tagsJSON), string(turnsJSON), conv.TurnCount, conv.SourceNode,
		conv.CreatedAt.UTC().Format(time.RFC3339), conv.UpdatedAt.UTC().Format(time.RFC3339),
		conv.AccessedAt.UTC().Format(time.RFC3339), conv.AccessCount)
	if err != nil {
		return fmt.Errorf("insert conversation: %w", err)
	}

	_, _ = tx.Exec(`DELETE FROM vec_conversations WHERE id = ?`, conv.ID)
	_, err = tx.Exec(`INSERT INTO vec_conversations (id, embedding) VALUES (?, ?)`, conv.ID, vecBlob)
	if err != nil {
		return fmt.Errorf("insert conversation vector: %w", err)
	}

	return tx.Commit()
}

// GetByID retrieves a conversation by ID.
func (s *Store) GetByID(id string) (*Conversation, error) {
	row := s.db.QueryRow(`SELECT id, title, summary, agent, project, tags, turns, turn_count, source_node, created_at, updated_at, accessed_at, access_count FROM conversations WHERE id = ?`, id)
	return scanConversation(row)
}

// List returns conversations sorted by date, with optional filters.
func (s *Store) List(agent, project string, limit, offset int) ([]*Conversation, error) {
	if limit <= 0 {
		limit = 20
	}

	query := `SELECT id, title, summary, agent, project, tags, turns, turn_count, source_node, created_at, updated_at, accessed_at, access_count FROM conversations WHERE 1=1`
	var args []any

	if agent != "" {
		query += ` AND agent = ?`
		args = append(args, agent)
	}
	if project != "" {
		query += ` AND project = ?`
		args = append(args, project)
	}

	query += ` ORDER BY updated_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*Conversation
	for rows.Next() {
		conv, err := scanConversationRows(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, conv)
	}
	return results, rows.Err()
}

// ListByDateRange returns conversations within a date range.
func (s *Store) ListByDateRange(from, to time.Time, limit int) ([]*Conversation, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id, title, summary, agent, project, tags, turns, turn_count, source_node, created_at, updated_at, accessed_at, access_count
		FROM conversations WHERE created_at >= ? AND created_at <= ? ORDER BY created_at DESC LIMIT ?`,
		from.Format(time.RFC3339), to.Format(time.RFC3339), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*Conversation
	for rows.Next() {
		conv, err := scanConversationRows(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, conv)
	}
	return results, rows.Err()
}

// Search performs semantic search over conversations.
func (s *Store) Search(queryVec []float32, limit int, agent, project string) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 5
	}

	vecBlob, err := sqlite_vec.SerializeFloat32(queryVec)
	if err != nil {
		return nil, err
	}

	fetchLimit := limit * 4
	if fetchLimit < 20 {
		fetchLimit = 20
	}

	rows, err := s.db.Query(`SELECT id, distance FROM vec_conversations WHERE embedding MATCH ? ORDER BY distance LIMIT ?`, vecBlob, fetchLimit)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		id       string
		distance float64
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.distance); err != nil {
			return nil, err
		}
		candidates = append(candidates, c)
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	// Batch-fetch all conversations in a single query instead of N+1 GetByID calls
	ids := make([]string, len(candidates))
	for i, c := range candidates {
		ids[i] = c.id
	}
	convMap, err := s.getByIDs(ids)
	if err != nil {
		return nil, fmt.Errorf("batch fetch conversations: %w", err)
	}

	var results []SearchResult
	for _, c := range candidates {
		conv, ok := convMap[c.id]
		if !ok {
			continue
		}
		if agent != "" && conv.Agent != agent {
			continue
		}
		if project != "" && conv.Project != project {
			continue
		}

		score := 1.0 / (1.0 + c.distance)
		results = append(results, SearchResult{
			Conversation: *conv,
			Distance:     c.distance,
			Score:        math.Round(score*1000) / 1000,
		})
		if len(results) >= limit {
			break
		}
	}

	return results, nil
}

// getByIDs batch-fetches conversations by a list of IDs in a single query.
func (s *Store) getByIDs(ids []string) (map[string]*Conversation, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	// Build WHERE id IN (?, ?, ...) with positional args
	placeholders := make([]byte, 0, len(ids)*2)
	args := make([]any, len(ids))
	for i, id := range ids {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
		args[i] = id
	}

	query := `SELECT id, title, summary, agent, project, tags, turns, turn_count, source_node, created_at, updated_at, accessed_at, access_count FROM conversations WHERE id IN (` + string(placeholders) + `)`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]*Conversation, len(ids))
	for rows.Next() {
		conv, err := scanConversationRows(rows)
		if err != nil {
			return nil, err
		}
		result[conv.ID] = conv
	}
	return result, rows.Err()
}

// Count returns total conversations.
func (s *Store) Count() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM conversations`).Scan(&count)
	return count, err
}

// TouchAccess updates accessed_at and increments access_count.
func (s *Store) TouchAccess(id string) error {
	_, err := s.db.Exec(`UPDATE conversations SET accessed_at = datetime('now'), access_count = access_count + 1 WHERE id = ?`, id)
	return err
}

// Delete removes a conversation.
func (s *Store) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM conversations WHERE id = ?`, id)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM vec_conversations WHERE id = ?`, id)
	return err
}

func scanConversation(row *sql.Row) (*Conversation, error) {
	var c Conversation
	var tagsStr, turnsStr, createdStr, updatedStr, accessedStr string

	err := row.Scan(&c.ID, &c.Title, &c.Summary, &c.Agent, &c.Project,
		&tagsStr, &turnsStr, &c.TurnCount, &c.SourceNode,
		&createdStr, &updatedStr, &accessedStr, &c.AccessCount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(tagsStr), &c.Tags); err != nil {
		slog.Warn("unmarshal conversation field failed", "field", "tags", "error", err)
	}
	if err := json.Unmarshal([]byte(turnsStr), &c.Turns); err != nil {
		slog.Warn("unmarshal conversation field failed", "field", "turns", "error", err)
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	c.AccessedAt, _ = time.Parse(time.RFC3339, accessedStr)

	return &c, nil
}

func scanConversationRows(rows *sql.Rows) (*Conversation, error) {
	var c Conversation
	var tagsStr, turnsStr, createdStr, updatedStr, accessedStr string

	err := rows.Scan(&c.ID, &c.Title, &c.Summary, &c.Agent, &c.Project,
		&tagsStr, &turnsStr, &c.TurnCount, &c.SourceNode,
		&createdStr, &updatedStr, &accessedStr, &c.AccessCount)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(tagsStr), &c.Tags); err != nil {
		slog.Warn("unmarshal conversation field failed", "field", "tags", "error", err)
	}
	if err := json.Unmarshal([]byte(turnsStr), &c.Turns); err != nil {
		slog.Warn("unmarshal conversation field failed", "field", "turns", "error", err)
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	c.AccessedAt, _ = time.Parse(time.RFC3339, accessedStr)

	return &c, nil
}
