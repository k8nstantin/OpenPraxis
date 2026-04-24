package templates

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is returned by write-path operations when the target
// template_uid has no active row (or does not exist at all).
var ErrNotFound = errors.New("template not found")

// ErrDuplicateOverride is returned by Create when an active row already
// exists for the same (scope, scope_id, section) — preserves the
// invariant that the resolver can pick exactly one row per tier.
var ErrDuplicateOverride = errors.New("active template already exists for scope/section")

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

// ListWithScopeID extends List with an optional scope_id filter. Used by the
// HTTP/MCP list endpoints so an operator can narrow to one specific
// (scope, scope_id, section) triple.
func (s *Store) ListWithScopeID(ctx context.Context, scope, scopeID, section string) ([]*Template, error) {
	q := `SELECT ` + selectCols + ` FROM prompt_templates WHERE ` + activeFilter
	args := []interface{}{}
	if scope != "" {
		q += ` AND scope = ?`
		args = append(args, scope)
	}
	if scopeID != "" {
		q += ` AND scope_id = ?`
		args = append(args, scopeID)
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

// History returns every row (active and closed) for the given template_uid,
// newest-first by valid_from. Tombstoned rows are included so the caller
// can see the full audit trail.
func (s *Store) History(ctx context.Context, uid string) ([]*Template, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+selectCols+` FROM prompt_templates
		 WHERE template_uid = ?
		 ORDER BY valid_from DESC, id DESC`,
		uid)
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
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

// AtTime returns the row that was active at the given point-in-time:
// valid_from <= t AND (valid_to > t OR valid_to = ''). Tombstoned rows
// are excluded. Returns sql.ErrNoRows when nothing was active.
func (s *Store) AtTime(ctx context.Context, uid string, t time.Time) (*Template, error) {
	ts := t.UTC().Format(time.RFC3339Nano)
	row := s.db.QueryRowContext(ctx,
		`SELECT `+selectCols+` FROM prompt_templates
		 WHERE template_uid = ? AND deleted_at = ''
		   AND valid_from <= ? AND (valid_to > ? OR valid_to = '')
		 ORDER BY valid_from DESC LIMIT 1`,
		uid, ts, ts)
	return scanRow(row)
}

// Create inserts a brand-new override row at the given scope tier and
// returns its freshly-minted template_uid. Rejects an insert that would
// duplicate an existing active (scope, scopeID, section) triple.
func (s *Store) Create(ctx context.Context, scope, scopeID, section, title, body, changedBy, reason string) (string, error) {
	if scope == "" || section == "" {
		return "", fmt.Errorf("templates.Create: scope and section are required")
	}
	if scope == ScopeSystem {
		return "", fmt.Errorf("templates.Create: cannot create system-scope rows; seed only")
	}
	if title == "" {
		title = section
	}

	existing, err := s.Get(ctx, scope, scopeID, section)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("templates.Create existing check: %w", err)
	}
	if existing != nil {
		return "", ErrDuplicateOverride
	}

	u, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("templates.Create uuid: %w", err)
	}
	uid := u.String()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, `INSERT INTO prompt_templates
		(template_uid, title, scope, scope_id, section, body, status, tags,
		 source_node, valid_from, valid_to, changed_by, reason, created_at, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, 'open', '[]', '', ?, '', ?, ?, ?, '')`,
		uid, title, scope, scopeID, section, body, now, changedBy, reason, now)
	if err != nil {
		return "", fmt.Errorf("templates.Create insert: %w", err)
	}
	return uid, nil
}

// UpdateBody atomically closes the prior active row and inserts a new
// current row carrying the same identity columns but a new body. The
// transaction runs under BEGIN IMMEDIATE so concurrent writers against
// the same uid serialise instead of racing on the partial unique index.
//
// Identity columns (title, scope, scope_id, section, tags) are carried
// forward untouched — edits are intentionally limited to body.
func (s *Store) UpdateBody(ctx context.Context, uid, newBody, changedBy, reason string) error {
	if uid == "" {
		return fmt.Errorf("templates.UpdateBody: empty uid")
	}
	retries := 0
	for {
		err := s.updateBodyOnce(ctx, uid, newBody, changedBy, reason)
		if err == nil {
			return nil
		}
		if isLocked(err) && retries < 5 {
			retries++
			time.Sleep(time.Duration(10*retries) * time.Millisecond)
			continue
		}
		return err
	}
}

func (s *Store) updateBodyOnce(ctx context.Context, uid, newBody, changedBy, reason string) error {
	// Grab a dedicated connection so BEGIN IMMEDIATE + COMMIT run on the
	// same underlying SQLite handle. Using db.ExecContext directly would
	// spray the three statements across pooled conns.
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("templates.UpdateBody conn: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return fmt.Errorf("templates.UpdateBody begin: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), `ROLLBACK`)
		}
	}()

	var count int
	if err := conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM prompt_templates WHERE template_uid = ? AND valid_to = '' AND deleted_at = ''`,
		uid).Scan(&count); err != nil {
		return fmt.Errorf("templates.UpdateBody check: %w", err)
	}
	if count == 0 {
		return ErrNotFound
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)

	if _, err := conn.ExecContext(ctx,
		`UPDATE prompt_templates SET valid_to = ? WHERE template_uid = ? AND valid_to = ''`,
		now, uid); err != nil {
		return fmt.Errorf("templates.UpdateBody close: %w", err)
	}

	if _, err := conn.ExecContext(ctx, `
		INSERT INTO prompt_templates
		  (template_uid, title, scope, scope_id, section, body, status, tags, source_node,
		   valid_from, valid_to, changed_by, reason, created_at, deleted_at)
		SELECT template_uid, title, scope, scope_id, section, ?, status, tags, source_node,
		       ?, '', ?, ?, ?, ''
		FROM prompt_templates
		WHERE template_uid = ? AND valid_to = ?
		ORDER BY id DESC LIMIT 1`,
		newBody, now, changedBy, reason, now, uid, now,
	); err != nil {
		return fmt.Errorf("templates.UpdateBody insert: %w", err)
	}

	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		return fmt.Errorf("templates.UpdateBody commit: %w", err)
	}
	committed = true
	return nil
}

// Tombstone soft-deletes every row for the given template_uid by stamping
// deleted_at. The resolver already skips rows where deleted_at != '' so
// subsequent Resolve calls fall through to the next-broader scope.
// Reviving a tombstoned uid is not supported — operators must create a
// new template_uid at the same scope.
func (s *Store) Tombstone(ctx context.Context, uid, changedBy, reason string) error {
	if uid == "" {
		return fmt.Errorf("templates.Tombstone: empty uid")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(ctx,
		`UPDATE prompt_templates
		 SET deleted_at = ?, valid_to = CASE WHEN valid_to = '' THEN ? ELSE valid_to END,
		     changed_by = ?, reason = ?
		 WHERE template_uid = ? AND deleted_at = ''`,
		now, now, changedBy, reason, uid)
	if err != nil {
		return fmt.Errorf("templates.Tombstone: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// CloseStatus stamps status='closed' on the currently-active row for uid.
// The resolver's activeFilter excludes closed rows, so this makes the
// scope fall through to the next-broader tier without deleting history.
func (s *Store) CloseStatus(ctx context.Context, uid, changedBy, reason string) error {
	if uid == "" {
		return fmt.Errorf("templates.CloseStatus: empty uid")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(ctx,
		`UPDATE prompt_templates SET status = 'closed', changed_by = ?, reason = ?
		 WHERE template_uid = ? AND valid_to = '' AND deleted_at = ''`,
		changedBy, reason, uid)
	if err != nil {
		return fmt.Errorf("templates.CloseStatus: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	_ = now
	return nil
}

// isLocked reports whether err is SQLite's "database is locked" error,
// which the store retries a few times before surfacing.
func isLocked(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "database is locked") || strings.Contains(msg, "SQLITE_BUSY")
}
