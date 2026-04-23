package templates

import (
	"context"
	"database/sql"
	"fmt"
)

// Store is a read-only accessor for prompt_templates. The RC/M1 substrate
// ships no write API; RC/M2 layers in transactional SCD-2 mutations over
// this same table.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// DB returns the underlying connection. Seed() needs direct access to
// issue the transactional bootstrap insert.
func (s *Store) DB() *sql.DB { return s.db }

// activeFilter is the WHERE clause that pins a read to the currently-active
// row: open (status not closed/archived), not deleted, and the open end of
// its SCD-2 interval.
const activeFilter = `valid_to = '' AND deleted_at = '' AND status NOT IN ('closed','archive')`

const selectCols = `id, template_uid, title, scope, scope_id, section, body, status, tags,
	source_node, valid_from, valid_to, changed_by, reason, created_at, deleted_at`

func scanRow(sc interface{ Scan(...interface{}) error }) (*Template, error) {
	t := &Template{}
	err := sc.Scan(&t.ID, &t.TemplateUID, &t.Title, &t.Scope, &t.ScopeID, &t.Section,
		&t.Body, &t.Status, &t.Tags, &t.SourceNode, &t.ValidFrom, &t.ValidTo,
		&t.ChangedBy, &t.Reason, &t.CreatedAt, &t.DeletedAt)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// Get returns the active row for (scope, scopeID, section) or sql.ErrNoRows.
func (s *Store) Get(ctx context.Context, scope, scopeID, section string) (*Template, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+selectCols+` FROM prompt_templates
		 WHERE scope = ? AND scope_id = ? AND section = ? AND `+activeFilter+`
		 ORDER BY id DESC LIMIT 1`,
		scope, scopeID, section)
	t, err := scanRow(row)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// GetByUID returns the currently-active row for the given template_uid.
func (s *Store) GetByUID(ctx context.Context, uid string) (*Template, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+selectCols+` FROM prompt_templates
		 WHERE template_uid = ? AND `+activeFilter+`
		 ORDER BY id DESC LIMIT 1`,
		uid)
	return scanRow(row)
}

// List returns all active rows, optionally filtered by scope and/or section.
// Empty filter value disables that filter.
func (s *Store) List(ctx context.Context, scope, section string) ([]*Template, error) {
	q := `SELECT ` + selectCols + ` FROM prompt_templates WHERE ` + activeFilter
	args := []interface{}{}
	if scope != "" {
		q += ` AND scope = ?`
		args = append(args, scope)
	}
	if section != "" {
		q += ` AND section = ?`
		args = append(args, section)
	}
	q += ` ORDER BY scope, scope_id, section, id DESC`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list prompt_templates: %w", err)
	}
	defer rows.Close()

	var out []*Template
	for rows.Next() {
		t, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
