package activity

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Event types — every event written to activity_log.
const (
	EventRunStarted       = "run_started"
	EventRunCompleted     = "run_completed"
	EventRunFailed        = "run_failed"
	EventRunCancelled     = "run_cancelled"
	EventMemoryStored     = "memory_stored"
	EventConversationSaved = "conversation_saved"
	EventSessionStarted   = "session_started"
	EventSessionEnded     = "session_ended"
	EventScheduleFired    = "schedule_fired"
)

// Row is one append-only entry in activity_log.
// Once inserted a row is never modified — tamper detection via Checksum.
type Row struct {
	ID           string  `json:"id"`            // UUID v7
	EntityUID    string  `json:"entity_uid"`    // product/manifest/task/memory/conversation UUID
	EntityType   string  `json:"entity_type"`   // product/manifest/task/memory/conversation
	Event        string  `json:"event"`         // EventRunStarted etc.
	Actor        string  `json:"actor"`         // agent/operator/system/mcp
	Summary      string  `json:"summary"`       // human-readable one-liner
	RunUID       string  `json:"run_uid"`       // → execution_log.run_uid
	SessionID    string  `json:"session_id"`    // → actions.session_id (correlation key)
	ParentID     string  `json:"parent_id"`     // parent activity row (product→manifest→task chain)
	Trigger      string  `json:"trigger"`       // schedule/manual/api/hook
	NodeID       string  `json:"node_id"`       // peer node that emitted this
	CostUSD      float64 `json:"cost_usd"`
	DurationMS   int64   `json:"duration_ms"`
	Turns        int     `json:"turns"`
	Actions      int     `json:"actions"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	Branch       string  `json:"branch"`
	PRNumber     *int    `json:"pr_number"`
	Error        string  `json:"error"`
	Metadata     string  `json:"metadata"`  // JSON blob
	Checksum     string  `json:"checksum"`  // SHA256 of immutable fields
	CreatedAt    string  `json:"created_at"` // RFC3339Nano, set on insert, never changed
}

// Store is the append-only activity log.
// No Update or Delete methods exist — rows are immutable after insert.
type Store struct {
	db *sql.DB
}

// NewStore creates a Store against the given DB handle.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// InitSchema creates activity_log and its indexes. Idempotent.
func InitSchema(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS activity_log (
		id            TEXT PRIMARY KEY,
		entity_uid    TEXT NOT NULL,
		entity_type   TEXT NOT NULL DEFAULT '',
		event         TEXT NOT NULL,
		actor         TEXT NOT NULL DEFAULT '',
		summary       TEXT NOT NULL DEFAULT '',
		run_uid       TEXT NOT NULL DEFAULT '',
		session_id    TEXT NOT NULL DEFAULT '',
		parent_id     TEXT NOT NULL DEFAULT '',
		trigger       TEXT NOT NULL DEFAULT '',
		node_id       TEXT NOT NULL DEFAULT '',
		cost_usd      REAL    NOT NULL DEFAULT 0,
		duration_ms   INTEGER NOT NULL DEFAULT 0,
		turns         INTEGER NOT NULL DEFAULT 0,
		actions       INTEGER NOT NULL DEFAULT 0,
		input_tokens  INTEGER NOT NULL DEFAULT 0,
		output_tokens INTEGER NOT NULL DEFAULT 0,
		branch        TEXT    NOT NULL DEFAULT '',
		pr_number     INTEGER,
		error         TEXT    NOT NULL DEFAULT '',
		metadata      TEXT    NOT NULL DEFAULT '{}',
		checksum      TEXT    NOT NULL DEFAULT '',
		created_at    TEXT    NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("activity_log: create table: %w", err)
	}
	for _, idx := range []string{
		`CREATE INDEX IF NOT EXISTS idx_activity_entity    ON activity_log(entity_uid, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_activity_event     ON activity_log(event, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_activity_run       ON activity_log(run_uid) WHERE run_uid != ''`,
		`CREATE INDEX IF NOT EXISTS idx_activity_session   ON activity_log(session_id) WHERE session_id != ''`,
		`CREATE INDEX IF NOT EXISTS idx_activity_parent    ON activity_log(parent_id) WHERE parent_id != ''`,
		`CREATE INDEX IF NOT EXISTS idx_activity_created   ON activity_log(created_at DESC)`,
	} {
		if _, err := db.Exec(idx); err != nil {
			return fmt.Errorf("activity_log: index: %w", err)
		}
	}
	return nil
}

// Insert appends one row. Sets ID and CreatedAt if empty. Computes Checksum.
// This is the only write path — no Update or Delete exists.
func (s *Store) Insert(ctx context.Context, r Row) (Row, error) {
	if r.ID == "" {
		id, err := uuid.NewV7()
		if err != nil {
			return r, fmt.Errorf("activity_log: generate id: %w", err)
		}
		r.ID = id.String()
	}
	if r.CreatedAt == "" {
		r.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if r.Metadata == "" {
		r.Metadata = "{}"
	}
	r.Checksum = checksum(r)

	_, err := s.db.ExecContext(ctx, `INSERT INTO activity_log (
		id, entity_uid, entity_type, event, actor, summary,
		run_uid, session_id, parent_id, trigger, node_id,
		cost_usd, duration_ms, turns, actions, input_tokens, output_tokens,
		branch, pr_number, error, metadata, checksum, created_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.EntityUID, r.EntityType, r.Event, r.Actor, r.Summary,
		r.RunUID, r.SessionID, r.ParentID, r.Trigger, r.NodeID,
		r.CostUSD, r.DurationMS, r.Turns, r.Actions, r.InputTokens, r.OutputTokens,
		r.Branch, r.PRNumber, r.Error, r.Metadata, r.Checksum, r.CreatedAt,
	)
	if err != nil {
		return r, fmt.Errorf("activity_log: insert: %w", err)
	}
	return r, nil
}

// ListByEntity returns rows for an entity newest-first.
func (s *Store) ListByEntity(ctx context.Context, entityUID string, limit int) ([]Row, error) {
	return s.query(ctx, `SELECT `+rowCols+` FROM activity_log
		WHERE entity_uid = ? ORDER BY created_at DESC LIMIT ?`, entityUID, limit)
}

// ListByRun returns all activity rows for a run_uid.
func (s *Store) ListByRun(ctx context.Context, runUID string) ([]Row, error) {
	return s.query(ctx, `SELECT `+rowCols+` FROM activity_log
		WHERE run_uid = ? ORDER BY created_at ASC`, runUID)
}

// ListBySession returns all activity rows for a session_id.
// Cross-reference: session_id also exists on actions.session_id.
func (s *Store) ListBySession(ctx context.Context, sessionID string) ([]Row, error) {
	return s.query(ctx, `SELECT `+rowCols+` FROM activity_log
		WHERE session_id = ? ORDER BY created_at ASC`, sessionID)
}

// ListSince returns rows created at or after asOf — time-travel query.
// asOf is RFC3339Nano. Pass "" to get all rows.
func (s *Store) ListSince(ctx context.Context, asOf string, limit int) ([]Row, error) {
	if asOf == "" {
		return s.query(ctx, `SELECT `+rowCols+` FROM activity_log
			ORDER BY created_at DESC LIMIT ?`, limit)
	}
	return s.query(ctx, `SELECT `+rowCols+` FROM activity_log
		WHERE created_at >= ? ORDER BY created_at DESC LIMIT ?`, asOf, limit)
}

// ListAsOf returns rows that existed at a point in time (created_at <= asOf).
// Combined with ListSince this gives full time-travel over the log.
func (s *Store) ListAsOf(ctx context.Context, asOf string, limit int) ([]Row, error) {
	return s.query(ctx, `SELECT `+rowCols+` FROM activity_log
		WHERE created_at <= ? ORDER BY created_at DESC LIMIT ?`, asOf, limit)
}

// VerifyChecksum returns true if the row's checksum matches its content.
func VerifyChecksum(r Row) bool {
	return r.Checksum == checksum(r)
}

// checksum computes SHA256 over the immutable identity fields of a row.
func checksum(r Row) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s|%s|%s|%s", r.ID, r.EntityUID, r.Event, r.CreatedAt, r.Summary, r.RunUID)
	return fmt.Sprintf("%x", h.Sum(nil))
}

const rowCols = `id, entity_uid, entity_type, event, actor, summary,
	run_uid, session_id, parent_id, trigger, node_id,
	cost_usd, duration_ms, turns, actions, input_tokens, output_tokens,
	branch, pr_number, error, metadata, checksum, created_at`

func (s *Store) query(ctx context.Context, q string, args ...any) ([]Row, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("activity_log: query: %w", err)
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		if err := rows.Scan(
			&r.ID, &r.EntityUID, &r.EntityType, &r.Event, &r.Actor, &r.Summary,
			&r.RunUID, &r.SessionID, &r.ParentID, &r.Trigger, &r.NodeID,
			&r.CostUSD, &r.DurationMS, &r.Turns, &r.Actions, &r.InputTokens, &r.OutputTokens,
			&r.Branch, &r.PRNumber, &r.Error, &r.Metadata, &r.Checksum, &r.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
