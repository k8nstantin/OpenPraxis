package idea

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// Idea represents a product idea, feature request, or improvement note.
type Idea struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`   // draft, open, closed, archive
	Priority    string    `json:"priority"` // low, medium, high, critical
	Tags        []string  `json:"tags"`
	Author      string    `json:"author"`
	SourceNode  string    `json:"source_node"`
	ProjectID   string    `json:"project_id"` // optional project grouping
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Store manages idea persistence.
type Store struct {
	db *sql.DB
}

// NewStore creates an idea store.
func NewStore(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.init(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) init() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS ideas (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'draft',
		priority TEXT NOT NULL DEFAULT 'medium',
		tags TEXT NOT NULL DEFAULT '[]',
		author TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("create ideas table: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_ideas_status ON ideas(status)`)
	if err != nil {
		return fmt.Errorf("create ideas status index: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_ideas_updated ON ideas(updated_at DESC)`)
	if err != nil {
		return err
	}
	s.db.Exec(`ALTER TABLE ideas ADD COLUMN source_node TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE ideas ADD COLUMN deleted_at TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE ideas ADD COLUMN project_id TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_ideas_project ON ideas(project_id)`)
	return nil
}

// Create stores a new idea.
func (s *Store) Create(title, description, status, priority, author, sourceNode, projectID string, tags []string) (*Idea, error) {
	if status == "" {
		status = "draft"
	}
	if priority == "" {
		priority = "medium"
	}
	if tags == nil {
		tags = []string{}
	}

	id := uuid.Must(uuid.NewV7()).String()
	now := time.Now().UTC()
	tagsJSON, _ := json.Marshal(tags)

	_, err := s.db.Exec(`INSERT INTO ideas (id, title, description, status, priority, tags, author, source_node, project_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, title, description, status, priority, string(tagsJSON), author, sourceNode, projectID,
		now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}

	return &Idea{
		ID: id, Title: title, Description: description,
		Status: status, Priority: priority, Tags: tags,
		Author: author, SourceNode: sourceNode, ProjectID: projectID, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// Update modifies an existing idea.
func (s *Store) Update(id, title, description, status, priority, projectID string, tags []string) error {
	now := time.Now().UTC()
	tagsJSON, _ := json.Marshal(tags)
	_, err := s.db.Exec(`UPDATE ideas SET title=?, description=?, status=?, priority=?, project_id=?, tags=?, updated_at=? WHERE id=?`,
		title, description, status, priority, projectID, string(tagsJSON), now.Format(time.RFC3339), id)
	return err
}

// Get retrieves an idea by full UUID.
func (s *Store) Get(id string) (*Idea, error) {
	row := s.db.QueryRow(`SELECT id, title, description, status, priority, tags, author, source_node, project_id, created_at, updated_at
		FROM ideas WHERE id = ? AND deleted_at = ''`, id)
	return scanIdea(row)
}

// ListByProject returns ideas belonging to a project.
func (s *Store) ListByProject(projectID string, limit int) ([]*Idea, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id, title, description, status, priority, tags, author, source_node, project_id, created_at, updated_at
		FROM ideas WHERE project_id = ? AND deleted_at = '' ORDER BY CASE status WHEN 'draft' THEN 0 WHEN 'open' THEN 1 WHEN 'closed' THEN 2 WHEN 'archive' THEN 3 ELSE 4 END, updated_at DESC LIMIT ?`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []*Idea
	for rows.Next() {
		i, err := scanIdeaRows(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, i)
	}
	return results, rows.Err()
}

// List returns ideas sorted by updated_at.
func (s *Store) List(status string, limit int) ([]*Idea, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT id, title, description, status, priority, tags, author, source_node, project_id, created_at, updated_at FROM ideas WHERE deleted_at = ''`
	var args []any
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY CASE status WHEN 'draft' THEN 0 WHEN 'open' THEN 1 WHEN 'closed' THEN 2 WHEN 'archive' THEN 3 ELSE 4 END, updated_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*Idea
	for rows.Next() {
		i, err := scanIdeaRows(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, i)
	}
	return results, rows.Err()
}

// Delete soft-deletes an idea.
func (s *Store) Delete(id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE ideas SET deleted_at = ? WHERE id = ? AND deleted_at = ''`, now, id)
	return err
}

// ListDeleted returns soft-deleted ideas.
func (s *Store) ListDeleted(limit int) ([]*Idea, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id, title, description, status, priority, tags, author, source_node, project_id, created_at, updated_at
		FROM ideas WHERE deleted_at != '' ORDER BY deleted_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []*Idea
	for rows.Next() {
		i, err := scanIdeaRows(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, i)
	}
	return results, rows.Err()
}

// Restore un-deletes a soft-deleted idea.
func (s *Store) Restore(id string) error {
	_, err := s.db.Exec(`UPDATE ideas SET deleted_at = '' WHERE id = ? AND deleted_at != ''`, id)
	return err
}

func scanIdea(row *sql.Row) (*Idea, error) {
	var i Idea
	var tagsStr, createdStr, updatedStr string
	err := row.Scan(&i.ID, &i.Title, &i.Description, &i.Status, &i.Priority, &tagsStr, &i.Author, &i.SourceNode, &i.ProjectID, &createdStr, &updatedStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(tagsStr), &i.Tags); err != nil {
		slog.Warn("unmarshal idea field failed", "field", "tags", "error", err)
	}
	i.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	i.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	return &i, nil
}

func scanIdeaRows(rows *sql.Rows) (*Idea, error) {
	var i Idea
	var tagsStr, createdStr, updatedStr string
	err := rows.Scan(&i.ID, &i.Title, &i.Description, &i.Status, &i.Priority, &tagsStr, &i.Author, &i.SourceNode, &i.ProjectID, &createdStr, &updatedStr)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(tagsStr), &i.Tags); err != nil {
		slog.Warn("unmarshal idea field failed", "field", "tags", "error", err)
	}
	i.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	i.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	return &i, nil
}
