package entity

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	TypeSkill    = "skill"
	TypeProduct  = "product"
	TypeManifest = "manifest"
	TypeTask     = "task"
	TypeIdea     = "idea"
	TypeRAG      = "RAG"

	StatusDraft    = "draft"
	StatusActive   = "active"
	StatusClosed   = "closed"
	StatusArchived = "archived"
)

// Entity is a single SCD-2 version row for any first-class OpenPraxis object.
// The "current" row for an entity_uid is the one where valid_to = ''.
type Entity struct {
	RowID        int64    `json:"row_id"`
	EntityUID    string   `json:"entity_uid"`
	Type         string   `json:"type"`
	Title        string   `json:"title"`
	Status       string   `json:"status"`
	Tags         []string `json:"tags"`
	ValidFrom    string   `json:"valid_from"`
	ValidTo      string   `json:"valid_to"`
	ChangedBy    string   `json:"changed_by"`
	ChangeReason string   `json:"change_reason"`
	CreatedAt    string   `json:"created_at"`
}

// Store manages entity persistence using SCD-2 semantics.
type Store struct {
	db *sql.DB
}

// NewStore creates the entities and nodes_entities tables and returns a Store.
func NewStore(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.initEntitiesSchema(); err != nil {
		return nil, err
	}
	if err := s.initNodesSchema(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) initEntitiesSchema() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS entities (
		row_id        INTEGER PRIMARY KEY AUTOINCREMENT,
		entity_uid    TEXT NOT NULL,
		type          TEXT NOT NULL,
		title         TEXT NOT NULL DEFAULT '',
		status        TEXT NOT NULL DEFAULT 'draft',
		tags          TEXT NOT NULL DEFAULT '[]',
		valid_from    TEXT NOT NULL,
		valid_to      TEXT NOT NULL DEFAULT '',
		changed_by    TEXT NOT NULL DEFAULT '',
		change_reason TEXT NOT NULL DEFAULT '',
		created_at    TEXT NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("entity: init: create entities table: %w", err)
	}

	_, err = s.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_entities_uid_current
		ON entities (entity_uid) WHERE valid_to = ''`)
	if err != nil {
		return fmt.Errorf("entity: init: create uid current index: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_entities_type_status
		ON entities (type, status) WHERE valid_to = ''`)
	if err != nil {
		return fmt.Errorf("entity: init: create type+status index: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_entities_uid_history
		ON entities (entity_uid, valid_from DESC)`)
	if err != nil {
		return fmt.Errorf("entity: init: create uid history index: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_entities_status_current
		ON entities (status) WHERE valid_to = ''`)
	if err != nil {
		return fmt.Errorf("entity: init: create status current index: %w", err)
	}

	return nil
}

// Create inserts the first SCD-2 row for a new entity and returns it.
func (s *Store) Create(entityType, title, status string, tags []string, changedBy, changeReason string) (*Entity, error) {
	if tags == nil {
		tags = []string{}
	}
	if status == "" {
		status = StatusDraft
	}

	uid := uuid.Must(uuid.NewV7()).String()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tagsJSON, _ := json.Marshal(tags)

	_, err := s.db.Exec(`INSERT INTO entities
		(entity_uid, type, title, status, tags, valid_from, valid_to, changed_by, change_reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?, '', ?, ?, ?)`,
		uid, entityType, title, status, string(tagsJSON), now, changedBy, changeReason, now)
	if err != nil {
		return nil, fmt.Errorf("entity: create: %w", err)
	}

	return &Entity{
		EntityUID:    uid,
		Type:         entityType,
		Title:        title,
		Status:       status,
		Tags:         tags,
		ValidFrom:    now,
		ValidTo:      "",
		ChangedBy:    changedBy,
		ChangeReason: changeReason,
		CreatedAt:    now,
	}, nil
}

// Get returns the current (valid_to = '') row for entityUID.
// Returns nil, nil when not found.
func (s *Store) Get(entityUID string) (*Entity, error) {
	row := s.db.QueryRow(`SELECT row_id, entity_uid, type, title, status, tags,
		valid_from, valid_to, changed_by, change_reason, created_at
		FROM entities WHERE entity_uid = ? AND valid_to = ''`, entityUID)
	return scanEntity(row)
}

// GetAt returns the entity row whose SCD-2 window contains asOf.
// asOf must be an RFC3339Nano string. Returns nil, nil when not found.
func (s *Store) GetAt(_ context.Context, entityUID, asOf string) (*Entity, error) {
	row := s.db.QueryRow(`SELECT row_id, entity_uid, type, title, status, tags,
		valid_from, valid_to, changed_by, change_reason, created_at
		FROM entities
		WHERE entity_uid = ?
		  AND valid_from <= ?
		  AND (valid_to > ? OR valid_to = '')
		ORDER BY valid_from DESC
		LIMIT 1`, entityUID, asOf, asOf)
	return scanEntity(row)
}

// List returns current rows filtered by entityType and/or status.
// Pass empty strings to skip a filter. limit=0 returns all rows.
func (s *Store) List(entityType, status string, limit int) ([]*Entity, error) {
	query := `SELECT row_id, entity_uid, type, title, status, tags,
		valid_from, valid_to, changed_by, change_reason, created_at
		FROM entities WHERE valid_to = ''`
	var args []any

	if entityType != "" {
		query += ` AND type = ?`
		args = append(args, entityType)
	}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("entity: list: %w", err)
	}
	defer rows.Close()
	return scanEntities(rows)
}

// ListAsOf returns rows representing the state of entities of entityType at asOf.
// Pass empty entityType to include all types. limit=0 returns all rows.
func (s *Store) ListAsOf(_ context.Context, entityType, asOf string, limit int) ([]*Entity, error) {
	query := `SELECT row_id, entity_uid, type, title, status, tags,
		valid_from, valid_to, changed_by, change_reason, created_at
		FROM entities
		WHERE valid_from <= ?
		  AND (valid_to > ? OR valid_to = '')`
	args := []any{asOf, asOf}

	if entityType != "" {
		query += ` AND type = ?`
		args = append(args, entityType)
	}
	query += ` ORDER BY created_at DESC`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("entity: list_as_of: %w", err)
	}
	defer rows.Close()
	return scanEntities(rows)
}

// Update closes the current row for entityUID (sets valid_to=now) and inserts
// a new row carrying the updated fields, all inside a transaction.
func (s *Store) Update(entityUID, title, status string, tags []string, changedBy, changeReason string) error {
	if tags == nil {
		tags = []string{}
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	tagsJSON, _ := json.Marshal(tags)

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("entity: update: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Read createdAt and type from the current row so the new row preserves them.
	var createdAt, entityType string
	err = tx.QueryRow(`SELECT created_at, type FROM entities
		WHERE entity_uid = ? AND valid_to = ''`, entityUID).Scan(&createdAt, &entityType)
	if err == sql.ErrNoRows {
		return fmt.Errorf("entity: update: %s not found", entityUID)
	}
	if err != nil {
		return fmt.Errorf("entity: update: read current: %w", err)
	}

	// Close the current row.
	_, err = tx.Exec(`UPDATE entities SET valid_to = ?
		WHERE entity_uid = ? AND valid_to = ''`, now, entityUID)
	if err != nil {
		return fmt.Errorf("entity: update: close current: %w", err)
	}

	// Insert the replacement row.
	_, err = tx.Exec(`INSERT INTO entities
		(entity_uid, type, title, status, tags, valid_from, valid_to, changed_by, change_reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?, '', ?, ?, ?)`,
		entityUID, entityType, title, status, string(tagsJSON), now, changedBy, changeReason, createdAt)
	if err != nil {
		return fmt.Errorf("entity: update: insert new: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("entity: update: commit: %w", err)
	}
	return nil
}

// History returns all SCD-2 rows for entityUID, newest-first.
func (s *Store) History(entityUID string) ([]*Entity, error) {
	rows, err := s.db.Query(`SELECT row_id, entity_uid, type, title, status, tags,
		valid_from, valid_to, changed_by, change_reason, created_at
		FROM entities WHERE entity_uid = ?
		ORDER BY valid_from DESC`, entityUID)
	if err != nil {
		return nil, fmt.Errorf("entity: history: %w", err)
	}
	defer rows.Close()
	return scanEntities(rows)
}

// ListByIDs returns current (valid_to = '') rows for the given entity UIDs in a
// single query. Missing or non-current entities are silently omitted.
func (s *Store) ListByIDs(ids []string) ([]*Entity, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	query := `SELECT row_id, entity_uid, type, title, status, tags,
		valid_from, valid_to, changed_by, change_reason, created_at
		FROM entities WHERE entity_uid IN (` + strings.Join(placeholders, ",") + `) AND valid_to = ''`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("entity: list_by_ids: %w", err)
	}
	defer rows.Close()
	return scanEntities(rows)
}

// ListByTypes returns all current (valid_to = '') entities whose type is in
// the provided list. Executes a single query using IN (...) instead of one
// query per type, which is more efficient for the tree handler fan-out.
// Pass status="" to skip status filtering. limit=0 returns all rows.
func (s *Store) ListByTypes(types []string, status string, limit int) ([]*Entity, error) {
	if len(types) == 0 {
		return []*Entity{}, nil
	}
	placeholders := make([]string, len(types))
	args := make([]any, len(types))
	for i, t := range types {
		placeholders[i] = "?"
		args[i] = t
	}
	query := `SELECT row_id, entity_uid, type, title, status, tags,
		valid_from, valid_to, changed_by, change_reason, created_at
		FROM entities WHERE valid_to = '' AND type IN (` + strings.Join(placeholders, ",") + `)`
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("entity: list_by_types: %w", err)
	}
	defer rows.Close()
	return scanEntities(rows)
}

// Search returns current rows where title contains query (case-insensitive
// substring), optionally filtered by entityType. limit=0 returns all matches.
func (s *Store) Search(query, entityType string, limit int) ([]*Entity, error) {
	q := strings.TrimSpace(query)
	pattern := "%" + q + "%"

	sql := `SELECT row_id, entity_uid, type, title, status, tags,
		valid_from, valid_to, changed_by, change_reason, created_at
		FROM entities WHERE valid_to = '' AND title LIKE ?`
	args := []any{pattern}

	if entityType != "" {
		sql += ` AND type = ?`
		args = append(args, entityType)
	}
	sql += ` ORDER BY created_at DESC`
	if limit > 0 {
		sql += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := s.db.Query(sql, args...)
	if err != nil {
		return nil, fmt.Errorf("entity: search: %w", err)
	}
	defer rows.Close()
	return scanEntities(rows)
}

func scanEntity(row *sql.Row) (*Entity, error) {
	var e Entity
	var tagsStr string
	err := row.Scan(&e.RowID, &e.EntityUID, &e.Type, &e.Title, &e.Status,
		&tagsStr, &e.ValidFrom, &e.ValidTo, &e.ChangedBy, &e.ChangeReason, &e.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("entity: scan: %w", err)
	}
	if err := json.Unmarshal([]byte(tagsStr), &e.Tags); err != nil {
		slog.Warn("entity: unmarshal tags failed", "entity_uid", e.EntityUID, "error", err)
		e.Tags = []string{}
	}
	return &e, nil
}

func scanEntities(rows *sql.Rows) ([]*Entity, error) {
	var results []*Entity
	for rows.Next() {
		var e Entity
		var tagsStr string
		err := rows.Scan(&e.RowID, &e.EntityUID, &e.Type, &e.Title, &e.Status,
			&tagsStr, &e.ValidFrom, &e.ValidTo, &e.ChangedBy, &e.ChangeReason, &e.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("entity: scan row: %w", err)
		}
		if err := json.Unmarshal([]byte(tagsStr), &e.Tags); err != nil {
			slog.Warn("entity: unmarshal tags failed", "entity_uid", e.EntityUID, "error", err)
			e.Tags = []string{}
		}
		results = append(results, &e)
	}
	return results, rows.Err()
}
