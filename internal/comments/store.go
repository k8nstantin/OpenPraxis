package comments

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Store provides CRUD access to the comments table created by InitSchema.
//
// The Store does not apply the schema itself — callers must invoke InitSchema
// once on the shared DB handle. The Store also assumes the DB was opened with
// WAL mode + busy_timeout=5000 per visceral rule #10.
type Store struct {
	db          *sql.DB
	attachments *AttachmentStore
}

// NewStore wraps an existing DB handle. The caller retains ownership of the DB.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// SetAttachments wires an AttachmentStore so Store.Delete cascades to
// comment_attachments rows + on-disk files. Optional — nil is a no-op
// at delete time, preserving the M1 store's behavior for callers that
// don't care about attachments yet.
func (s *Store) SetAttachments(a *AttachmentStore) {
	s.attachments = a
}

// Exported sentinel errors so M2's API boundary (MCP + HTTP) can match with
// errors.Is. Comment type validation is deliberately deferred to M1-T3; Add
// does NOT validate cType here.
var (
	ErrEmptyAuthor = errors.New("comments: author cannot be empty")
	ErrEmptyBody   = errors.New("comments: body cannot be empty")
)

const (
	defaultListLimit = 100
	maxListLimit     = 1000
)

const selectCols = "id, target_type, target_id, author, type, body, created_at, updated_at, parent_id"

// Add inserts a new comment. ID is a UUID v7 generated here. created_at is
// unix seconds at insert time. Returns the fully populated Comment.
func (s *Store) Add(ctx context.Context, target TargetType, targetID, author string,
	cType CommentType, body string) (Comment, error) {

	if author == "" {
		return Comment{}, ErrEmptyAuthor
	}
	if body == "" {
		return Comment{}, ErrEmptyBody
	}

	id, err := uuid.NewV7()
	if err != nil {
		return Comment{}, fmt.Errorf("comments: uuid v7: %w", err)
	}

	now := time.Now().Unix()
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO comments (id, target_type, target_id, author, type, body, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id.String(), string(target), targetID, author, string(cType), body, now,
	)
	if err != nil {
		return Comment{}, fmt.Errorf("comments: add: %w", err)
	}

	return Comment{
		ID:         id.String(),
		TargetType: target,
		TargetID:   targetID,
		Author:     author,
		Type:       cType,
		Body:       body,
		CreatedAt:  time.Unix(now, 0).UTC(),
	}, nil
}

// Get returns the comment by ID. Returns sql.ErrNoRows if missing.
func (s *Store) Get(ctx context.Context, id string) (Comment, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+selectCols+` FROM comments WHERE id = ?`, id)

	c, err := scanComment(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Comment{}, err
		}
		return Comment{}, fmt.Errorf("comments: get: %w", err)
	}
	return c, nil
}

// List returns comments for (target, targetID) newest first. If typeFilter is
// nil, all types are returned. limit <= 0 defaults to 100; limit > 1000 is
// capped at 1000.
func (s *Store) List(ctx context.Context, target TargetType, targetID string,
	limit int, typeFilter *CommentType) ([]Comment, error) {

	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}

	var (
		rows *sql.Rows
		err  error
	)
	if typeFilter == nil {
		rows, err = s.db.QueryContext(ctx,
			`SELECT `+selectCols+` FROM comments
			 WHERE target_type = ? AND target_id = ?
			 ORDER BY created_at DESC, id DESC
			 LIMIT ?`,
			string(target), targetID, limit,
		)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT `+selectCols+` FROM comments
			 WHERE target_type = ? AND target_id = ? AND type = ?
			 ORDER BY created_at DESC, id DESC
			 LIMIT ?`,
			string(target), targetID, string(*typeFilter), limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("comments: list: %w", err)
	}
	defer rows.Close()

	var out []Comment
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, fmt.Errorf("comments: list scan: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("comments: list rows: %w", err)
	}
	return out, nil
}

// Edit replaces body and sets updated_at to now. Returns sql.ErrNoRows if the
// id does not exist. body cannot be empty.
func (s *Store) Edit(ctx context.Context, id, body string) error {
	if body == "" {
		return ErrEmptyBody
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE comments SET body = ?, updated_at = ? WHERE id = ?`,
		body, time.Now().Unix(), id,
	)
	if err != nil {
		return fmt.Errorf("comments: edit: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("comments: edit rows-affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Delete is a hard delete and is idempotent. Cascades to comment
// attachments (soft-delete row + remove on-disk file) when an
// AttachmentStore has been wired via SetAttachments. Cascade errors
// are non-fatal — the comment row deletion is the source of truth and
// orphan attachment rows are recoverable by a separate sweeper.
func (s *Store) Delete(ctx context.Context, id string) error {
	if s.attachments != nil {
		_ = s.attachments.DeleteByComment(ctx, id)
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM comments WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("comments: delete: %w", err)
	}
	return nil
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanComment(s scanner) (Comment, error) {
	var (
		c          Comment
		targetStr  string
		typeStr    string
		createdAt  int64
		updatedAt  sql.NullInt64
		parentID   sql.NullString
	)
	if err := s.Scan(&c.ID, &targetStr, &c.TargetID, &c.Author, &typeStr,
		&c.Body, &createdAt, &updatedAt, &parentID); err != nil {
		return Comment{}, err
	}
	c.TargetType = TargetType(targetStr)
	c.Type = CommentType(typeStr)
	c.CreatedAt = time.Unix(createdAt, 0).UTC()
	if updatedAt.Valid {
		t := time.Unix(updatedAt.Int64, 0).UTC()
		c.UpdatedAt = &t
	}
	if parentID.Valid {
		p := parentID.String
		c.ParentID = &p
	}
	return c, nil
}
