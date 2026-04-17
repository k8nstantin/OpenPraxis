package settings

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Store provides CRUD access to the settings table initialized by InitSchema.
//
// The Store does not apply the schema itself — callers must invoke InitSchema
// once on the shared DB handle (node.go wires this on startup). The Store also
// assumes the DB was opened with WAL mode + busy_timeout=5000 per visceral
// rule #10; tests that construct their own DB must match that DSN.
type Store struct {
	db *sql.DB
}

// NewStore wraps an existing DB handle. The caller retains ownership of the DB.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// ErrInvalidScopeType is returned when a caller passes a scope_type that is
// not one of the four enum values. We surface this at the Go layer rather
// than letting SQLite's CHECK constraint fire, both for clearer error
// messages and to fail fast before the round-trip.
var ErrInvalidScopeType = errors.New("settings: invalid scope type")

func validateScope(scopeType ScopeType) error {
	switch scopeType {
	case ScopeSystem, ScopeProduct, ScopeManifest, ScopeTask:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrInvalidScopeType, string(scopeType))
	}
}

const selectCols = "scope_type, scope_id, key, value, updated_at, updated_by"

// Get returns the explicit entry at the given scope, without any inheritance
// walk. Returns sql.ErrNoRows if no entry exists at exactly this scope.
func (s *Store) Get(ctx context.Context, scopeType ScopeType, scopeID, key string) (Entry, error) {
	if err := validateScope(scopeType); err != nil {
		return Entry{}, err
	}

	row := s.db.QueryRowContext(ctx,
		`SELECT `+selectCols+` FROM settings
		 WHERE scope_type = ? AND scope_id = ? AND key = ?`,
		string(scopeType), scopeID, key,
	)

	var (
		e        Entry
		scopeStr string
		updated  int64
	)
	if err := row.Scan(&scopeStr, &e.ScopeID, &e.Key, &e.Value, &updated, &e.UpdatedBy); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Entry{}, err
		}
		return Entry{}, fmt.Errorf("settings: get: %w", err)
	}
	e.ScopeType = ScopeType(scopeStr)
	e.UpdatedAt = time.Unix(updated, 0).UTC()
	return e, nil
}

// Set writes or replaces the value for (scope_type, scope_id, key). The value
// is expected to be JSON-encoded by the caller (the knob catalog layer in
// M1-T3); Store treats it as opaque text. updatedBy is optional.
func (s *Store) Set(ctx context.Context, scopeType ScopeType, scopeID, key, value, updatedBy string) error {
	if err := validateScope(scopeType); err != nil {
		return err
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO settings
		 (scope_type, scope_id, key, value, updated_at, updated_by)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		string(scopeType), scopeID, key, value, time.Now().Unix(), updatedBy,
	)
	if err != nil {
		return fmt.Errorf("settings: set: %w", err)
	}
	return nil
}

// ListScope returns every entry at the given scope, ordered by key.
func (s *Store) ListScope(ctx context.Context, scopeType ScopeType, scopeID string) ([]Entry, error) {
	if err := validateScope(scopeType); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT `+selectCols+` FROM settings
		 WHERE scope_type = ? AND scope_id = ?
		 ORDER BY key`,
		string(scopeType), scopeID,
	)
	if err != nil {
		return nil, fmt.Errorf("settings: list scope: %w", err)
	}
	defer rows.Close()

	return scanEntries(rows)
}

// Delete removes the entry at (scope_type, scope_id, key). No error if the
// row did not exist — callers can treat Delete as idempotent.
func (s *Store) Delete(ctx context.Context, scopeType ScopeType, scopeID, key string) error {
	if err := validateScope(scopeType); err != nil {
		return err
	}

	_, err := s.db.ExecContext(ctx,
		`DELETE FROM settings
		 WHERE scope_type = ? AND scope_id = ? AND key = ?`,
		string(scopeType), scopeID, key,
	)
	if err != nil {
		return fmt.Errorf("settings: delete: %w", err)
	}
	return nil
}

// ListByKey returns every entry across every scope with this key, ordered
// by scope_type then scope_id. The resolver (M1-T4) uses this to walk the
// inheritance chain without four separate queries.
func (s *Store) ListByKey(ctx context.Context, key string) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+selectCols+` FROM settings
		 WHERE key = ?
		 ORDER BY scope_type, scope_id`,
		key,
	)
	if err != nil {
		return nil, fmt.Errorf("settings: list by key: %w", err)
	}
	defer rows.Close()

	return scanEntries(rows)
}

func scanEntries(rows *sql.Rows) ([]Entry, error) {
	var out []Entry
	for rows.Next() {
		var (
			e        Entry
			scopeStr string
			updated  int64
		)
		if err := rows.Scan(&scopeStr, &e.ScopeID, &e.Key, &e.Value, &updated, &e.UpdatedBy); err != nil {
			return nil, fmt.Errorf("settings: scan: %w", err)
		}
		e.ScopeType = ScopeType(scopeStr)
		e.UpdatedAt = time.Unix(updated, 0).UTC()
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("settings: rows: %w", err)
	}
	return out, nil
}
