package entity

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// EntityType is one SCD-2 row in the entity_types table.
type EntityType struct {
	TypeUID     string `json:"type_uid"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	Color       string `json:"color"`
	Icon        string `json:"icon"`
	ValidFrom   string `json:"valid_from"`
	ValidTo     string `json:"valid_to"`
	CreatedAt   string `json:"created_at"`
}

// TypesStore manages the entity_types table.
//
// createMu serializes Create's close-then-insert transaction. Same
// rationale as relationships.Store.createMu: SQLite WAL snapshot
// isolation lets two concurrent Creates both see "no current row",
// close-then-insert concurrently, and produce two rows with valid_to='',
// breaking the SCD-2 invariant. The mutex makes the close + insert
// atomic across goroutines.
type TypesStore struct {
	db       *sql.DB
	createMu sync.Mutex
}

// NewTypesStore creates a TypesStore and runs the idempotent schema migration.
func NewTypesStore(db *sql.DB) (*TypesStore, error) {
	s := &TypesStore{db: db}
	if err := s.InitSchema(db); err != nil {
		return nil, err
	}
	return s, nil
}

// InitSchema creates the entity_types table and its indexes if they do not exist.
// For existing DBs the color and icon columns are added via ALTER TABLE if missing.
func (s *TypesStore) InitSchema(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS entity_types (
		row_id        INTEGER PRIMARY KEY AUTOINCREMENT,
		type_uid      TEXT NOT NULL,
		name          TEXT NOT NULL,
		display_name  TEXT NOT NULL,
		description   TEXT NOT NULL DEFAULT '',
		color         TEXT NOT NULL DEFAULT '#6366f1',
		icon          TEXT NOT NULL DEFAULT 'Database',
		valid_from    TEXT NOT NULL,
		valid_to      TEXT NOT NULL DEFAULT '',
		changed_by    TEXT NOT NULL DEFAULT '',
		change_reason TEXT NOT NULL DEFAULT '',
		created_at    TEXT NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("entity_types: init: create table: %w", err)
	}

	// Idempotent ALTER TABLE for existing DBs that predate color/icon columns.
	for _, col := range []struct{ name, def string }{
		{"color", "TEXT NOT NULL DEFAULT '#6366f1'"},
		{"icon", "TEXT NOT NULL DEFAULT 'Database'"},
	} {
		_, _ = db.Exec(`ALTER TABLE entity_types ADD COLUMN ` + col.name + ` ` + col.def)
	}

	_, err = db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_entity_types_current
		ON entity_types(name) WHERE valid_to = ''`)
	if err != nil {
		return fmt.Errorf("entity_types: init: create current index: %w", err)
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_entity_types_uid
		ON entity_types(type_uid, valid_to)`)
	if err != nil {
		return fmt.Errorf("entity_types: init: create uid index: %w", err)
	}

	return nil
}

// builtinTypes is the seed set of entity types.
var builtinTypes = []struct {
	name        string
	displayName string
	description string
	color       string
	icon        string
}{
	{"skill", "Skill", "Governance rule or coding practice", "#f59e0b", "Wand2"},
	{"product", "Product", "Top-level product with manifests and tasks", "#3b82f6", "Boxes"},
	{"manifest", "Manifest", "Spec or plan that owns a set of tasks", "#a78bfa", "FileText"},
	{"task", "Task", "Atomic execution unit run by an agent", "#10b981", "CheckSquare"},
	{"idea", "Idea", "Unstructured concept or feature request", "#f97316", "Lightbulb"},
	{"RAG", "RAG", "Retrieval-augmented generation knowledge source", "#06b6d4", "Database"},
}

// Seed inserts the built-in entity types if they do not already exist.
// Idempotent — calling it multiple times is safe.
func (s *TypesStore) Seed(ctx context.Context) error {
	for _, t := range builtinTypes {
		exists, err := s.Exists(ctx, t.name)
		if err != nil {
			return fmt.Errorf("entity_types: seed: check %q: %w", t.name, err)
		}
		if exists {
			// Only backfill color/icon if they still hold the DB-DEFAULT values —
			// meaning the columns were just added via ALTER TABLE. If an operator
			// has customized them, leave them alone (respect SCD-2 spirit).
			_, _ = s.db.ExecContext(ctx,
				`UPDATE entity_types SET color=?, icon=?
				 WHERE name=? AND valid_to='' AND color='#6366f1' AND icon='Database'`,
				t.color, t.icon, t.name)
			continue
		}
		if _, err := s.Create(ctx, t.name, t.displayName, t.description, t.color, t.icon, "system"); err != nil {
			return fmt.Errorf("entity_types: seed: insert %q: %w", t.name, err)
		}
	}
	return nil
}

// List returns all current entity types (valid_to = '').
func (s *TypesStore) List(ctx context.Context) ([]EntityType, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT type_uid, name, display_name, description,
		color, icon, valid_from, valid_to, created_at
		FROM entity_types WHERE valid_to = ''
		ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("entity_types: list: %w", err)
	}
	defer rows.Close()

	var out []EntityType
	for rows.Next() {
		var et EntityType
		if err := rows.Scan(&et.TypeUID, &et.Name, &et.DisplayName, &et.Description,
			&et.Color, &et.Icon, &et.ValidFrom, &et.ValidTo, &et.CreatedAt); err != nil {
			return nil, fmt.Errorf("entity_types: list: scan: %w", err)
		}
		out = append(out, et)
	}
	return out, rows.Err()
}

// Create inserts a new current row for the given entity type name using
// SCD-2 semantics. If a current row already exists, it is closed and a
// new row is inserted in a single transaction.
func (s *TypesStore) Create(ctx context.Context, name, displayName, description, color, icon, changedBy string) (*EntityType, error) {
	if color == "" {
		color = "#6366f1"
	}
	if icon == "" {
		icon = "Database"
	}

	typeUID := uuid.Must(uuid.NewV7()).String()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Serialize close-then-insert across goroutines; see createMu docstring.
	s.createMu.Lock()
	defer s.createMu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("entity_types: create: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Close any existing current row for this name.
	_, err = tx.ExecContext(ctx, `UPDATE entity_types SET valid_to = ?
		WHERE name = ? AND valid_to = ''`, now, name)
	if err != nil {
		return nil, fmt.Errorf("entity_types: create: close prior: %w", err)
	}

	// Insert the new current row.
	_, err = tx.ExecContext(ctx, `INSERT INTO entity_types
		(type_uid, name, display_name, description, color, icon, valid_from, valid_to, changed_by, change_reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, '', ?, '', ?)`,
		typeUID, name, displayName, description, color, icon, now, changedBy, now)
	if err != nil {
		return nil, fmt.Errorf("entity_types: create: insert: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("entity_types: create: commit: %w", err)
	}

	return &EntityType{
		TypeUID:     typeUID,
		Name:        name,
		DisplayName: displayName,
		Description: description,
		Color:       color,
		Icon:        icon,
		ValidFrom:   now,
		ValidTo:     "",
		CreatedAt:   now,
	}, nil
}

// Exists returns true if a current row (valid_to = '') exists for name.
func (s *TypesStore) Exists(ctx context.Context, name string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM entity_types WHERE name = ? AND valid_to = ''`, name).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("entity_types: exists: %w", err)
	}
	return count > 0, nil
}

// Rename closes the current row for oldName and inserts a new row with
// the SAME type_uid so that entity references remain stable across renames.
// Create() is for genuinely new types (always generates a fresh UUID);
// Rename() is for "same logical type, different name" mutations.
//
// Returns ErrNotFound (as a wrapped error) if no current row for oldName exists.
func (s *TypesStore) Rename(ctx context.Context, oldName, newName, displayName, description, color, icon, changedBy string) (*EntityType, error) {
	if color == "" {
		color = "#6366f1"
	}
	if icon == "" {
		icon = "Database"
	}
	if displayName == "" {
		displayName = newName
	}

	s.createMu.Lock()
	defer s.createMu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Fetch the existing type_uid so the new row inherits it.
	var existingUID string
	err := s.db.QueryRowContext(ctx,
		`SELECT type_uid FROM entity_types WHERE name = ? AND valid_to = '' LIMIT 1`, oldName).Scan(&existingUID)
	if err != nil {
		return nil, fmt.Errorf("entity_types: rename: fetch uid for %q: %w", oldName, err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("entity_types: rename: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Close the current row for oldName.
	_, err = tx.ExecContext(ctx,
		`UPDATE entity_types SET valid_to = ? WHERE name = ? AND valid_to = ''`, now, oldName)
	if err != nil {
		return nil, fmt.Errorf("entity_types: rename: close prior: %w", err)
	}

	// Insert new row with the same type_uid.
	_, err = tx.ExecContext(ctx,
		`INSERT INTO entity_types
		(type_uid, name, display_name, description, color, icon, valid_from, valid_to, changed_by, change_reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, '', ?, 'rename', ?)`,
		existingUID, newName, displayName, description, color, icon, now, changedBy, now)
	if err != nil {
		return nil, fmt.Errorf("entity_types: rename: insert: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("entity_types: rename: commit: %w", err)
	}

	return &EntityType{
		TypeUID:     existingUID,
		Name:        newName,
		DisplayName: displayName,
		Description: description,
		Color:       color,
		Icon:        icon,
		ValidFrom:   now,
		ValidTo:     "",
		CreatedAt:   now,
	}, nil
}
