package comments

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Sentinel errors for the attachment surface. Callers (HTTP/MCP) match
// with errors.Is to map to HTTP status codes.
var (
	ErrAttachmentTooLarge   = errors.New("comments: attachment exceeds max size")
	ErrAttachmentMimeDenied = errors.New("comments: attachment mime type not allowed")
	ErrAttachmentNotFound   = errors.New("comments: attachment not found")
	ErrEmptyFilename        = errors.New("comments: filename cannot be empty")
)

// Default mime allowlist used when no per-scope override is set. Matches
// the M1 manifest spec — broad enough to cover screenshots, PDFs, JSON
// dumps, and zipped artifacts; narrow enough to refuse executable
// payloads outright.
var defaultAllowedMimes = []string{
	"image/",
	"text/",
	"application/pdf",
	"application/json",
	"application/xml",
	"application/zip",
}

// AttachmentRow is the wire + storage shape for a single comment
// attachment. Bytes never touch this struct or the DB — `StoragePath`
// points at the on-disk file under the data dir.
type AttachmentRow struct {
	ID          string    `json:"id"`
	CommentID   string    `json:"comment_id"`
	Filename    string    `json:"filename"`
	MimeType    string    `json:"mime_type"`
	SizeBytes   int64     `json:"size_bytes"`
	StoragePath string    `json:"-"` // never serialised to clients
	UploadedBy  string    `json:"uploaded_by"`
	CreatedAt   time.Time `json:"created_at"`
}

// AttachmentStore wraps the comment_attachments table + the on-disk
// storage tree under <dataDir>/attachments/<comment_id>/.
type AttachmentStore struct {
	db      *sql.DB
	rootDir string
}

// NewAttachmentStore wraps an existing DB handle. rootDir is the
// directory that holds per-comment subdirectories — typically
// <data_dir>/attachments. The directory is created lazily on first
// upload.
func NewAttachmentStore(db *sql.DB, rootDir string) *AttachmentStore {
	return &AttachmentStore{db: db, rootDir: rootDir}
}

// InitAttachmentSchema creates the comment_attachments table and its
// index. Idempotent. Soft-FK to comments.id is enforced in app code
// (Store.Delete cascades) — the project's existing tables don't use
// declarative FKs and turning them on retroactively risks rewriting
// the whole schema.
func InitAttachmentSchema(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS comment_attachments (
		id            TEXT NOT NULL PRIMARY KEY,
		comment_id    TEXT NOT NULL,
		filename      TEXT NOT NULL,
		mime_type     TEXT NOT NULL,
		size_bytes    INTEGER NOT NULL,
		storage_path  TEXT NOT NULL,
		uploaded_by   TEXT NOT NULL DEFAULT '',
		created_at    INTEGER NOT NULL,
		deleted_at    INTEGER NOT NULL DEFAULT 0
	)`)
	if err != nil {
		return fmt.Errorf("create comment_attachments: %w", err)
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_comment_attachments_comment_id
		ON comment_attachments(comment_id, created_at)`)
	if err != nil {
		return fmt.Errorf("create comment_attachments index: %w", err)
	}
	return nil
}

// safeFilenameRE permits alphanumeric, dot, underscore, hyphen. Anything
// else is replaced with `_` in SanitizeFilename.
var safeFilenameRE = regexp.MustCompile(`[^a-zA-Z0-9_.\-]`)

// SanitizeFilename strips the path component and replaces any character
// outside `a-zA-Z0-9_.-` with `_`. Returns empty when the result is
// empty or starts with a dot only — caller should reject.
func SanitizeFilename(raw string) string {
	// Drop any directory components a client may have sent.
	raw = filepath.Base(raw)
	cleaned := safeFilenameRE.ReplaceAllString(raw, "_")
	cleaned = strings.Trim(cleaned, ".")
	return cleaned
}

// MimeAllowed returns true when mime matches any prefix or exact entry
// in allowed. Prefixes are recognised by a trailing slash (e.g.
// "image/" matches "image/png"). An empty allowed list defaults to the
// package's defaultAllowedMimes.
func MimeAllowed(mime string, allowed []string) bool {
	mime = strings.ToLower(strings.TrimSpace(mime))
	if mime == "" {
		return false
	}
	if len(allowed) == 0 {
		allowed = defaultAllowedMimes
	}
	for _, a := range allowed {
		a = strings.ToLower(strings.TrimSpace(a))
		if a == "" {
			continue
		}
		if strings.HasSuffix(a, "/") {
			if strings.HasPrefix(mime, a) {
				return true
			}
			continue
		}
		if mime == a {
			return true
		}
	}
	return false
}

// DefaultAllowedMimes returns a copy of the package default so callers
// can present it in error messages without mutating the source slice.
func DefaultAllowedMimes() []string {
	out := make([]string, len(defaultAllowedMimes))
	copy(out, defaultAllowedMimes)
	return out
}

// pendingDir is the on-disk directory for orphan attachments — uploaded
// during compose before a comment id exists. Claim() rebinds them to a
// real comment without moving the bytes; storage_path is stable.
const pendingDir = "_pending"

// Insert writes a new attachment row + persists the bytes on disk under
// rootDir/<comment_id>/<id>__<filename>. Caller is responsible for
// validating commentID exists, mime type is allowed, and size <= cap;
// Insert performs a sanity check on the filename only.
func (s *AttachmentStore) Insert(ctx context.Context, commentID, uploadedBy, filename, mimeType string, data []byte) (AttachmentRow, error) {
	if commentID == "" {
		return AttachmentRow{}, fmt.Errorf("comments: comment_id required")
	}
	return s.insertAt(ctx, commentID, commentID, uploadedBy, filename, mimeType, data)
}

// InsertOrphan writes an attachment row with an empty comment_id, used
// by the BlockNote composer to upload during compose before the comment
// is posted. Bytes land in rootDir/_pending/<id>__<filename>. Call Claim
// to bind the row to a comment after the post lands.
func (s *AttachmentStore) InsertOrphan(ctx context.Context, uploadedBy, filename, mimeType string, data []byte) (AttachmentRow, error) {
	return s.insertAt(ctx, "", pendingDir, uploadedBy, filename, mimeType, data)
}

// Claim binds an orphan attachment (comment_id == "") to a real
// commentID. Returns ErrAttachmentNotFound when no orphan row matched.
func (s *AttachmentStore) Claim(ctx context.Context, id, commentID string) (AttachmentRow, error) {
	if commentID == "" {
		return AttachmentRow{}, fmt.Errorf("comments: comment_id required")
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE comment_attachments SET comment_id = ?
		 WHERE id = ? AND comment_id = '' AND deleted_at = 0`,
		commentID, id)
	if err != nil {
		return AttachmentRow{}, fmt.Errorf("comments: claim attachment: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return AttachmentRow{}, ErrAttachmentNotFound
	}
	return s.Get(ctx, id)
}

func (s *AttachmentStore) insertAt(ctx context.Context, commentID, dirName, uploadedBy, filename, mimeType string, data []byte) (AttachmentRow, error) {
	clean := SanitizeFilename(filename)
	if clean == "" {
		return AttachmentRow{}, ErrEmptyFilename
	}

	id, err := uuid.NewV7()
	if err != nil {
		return AttachmentRow{}, fmt.Errorf("comments: uuid: %w", err)
	}
	idStr := id.String()

	dir := filepath.Join(s.rootDir, dirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return AttachmentRow{}, fmt.Errorf("comments: mkdir: %w", err)
	}
	storagePath := filepath.Join(dir, idStr+"__"+clean)
	if err := os.WriteFile(storagePath, data, 0o644); err != nil {
		return AttachmentRow{}, fmt.Errorf("comments: write file: %w", err)
	}

	now := time.Now().Unix()
	_, err = s.db.ExecContext(ctx, `INSERT INTO comment_attachments
		(id, comment_id, filename, mime_type, size_bytes, storage_path, uploaded_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		idStr, commentID, clean, mimeType, int64(len(data)), storagePath, uploadedBy, now,
	)
	if err != nil {
		_ = os.Remove(storagePath)
		return AttachmentRow{}, fmt.Errorf("comments: insert attachment: %w", err)
	}
	return AttachmentRow{
		ID:          idStr,
		CommentID:   commentID,
		Filename:    clean,
		MimeType:    mimeType,
		SizeBytes:   int64(len(data)),
		StoragePath: storagePath,
		UploadedBy:  uploadedBy,
		CreatedAt:   time.Unix(now, 0).UTC(),
	}, nil
}

const attachSelectCols = "id, comment_id, filename, mime_type, size_bytes, storage_path, uploaded_by, created_at"

// Get returns a single non-deleted attachment by id.
func (s *AttachmentStore) Get(ctx context.Context, id string) (AttachmentRow, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+attachSelectCols+` FROM comment_attachments WHERE id = ? AND deleted_at = 0`, id)
	return scanAttachment(row)
}

// ListByComment returns non-deleted attachments for a comment, oldest
// first so the UI can render them in upload order.
func (s *AttachmentStore) ListByComment(ctx context.Context, commentID string) ([]AttachmentRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+attachSelectCols+` FROM comment_attachments
		 WHERE comment_id = ? AND deleted_at = 0
		 ORDER BY created_at ASC, id ASC`, commentID)
	if err != nil {
		return nil, fmt.Errorf("comments: list attachments: %w", err)
	}
	defer rows.Close()
	var out []AttachmentRow
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return nil, fmt.Errorf("comments: scan attachment: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("comments: list attachments rows: %w", err)
	}
	return out, nil
}

// SoftDelete marks the row deleted and removes the on-disk file.
// Returns ErrAttachmentNotFound if no live row matched.
func (s *AttachmentStore) SoftDelete(ctx context.Context, id string) error {
	a, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	res, err := s.db.ExecContext(ctx,
		`UPDATE comment_attachments SET deleted_at = ? WHERE id = ? AND deleted_at = 0`,
		now, id)
	if err != nil {
		return fmt.Errorf("comments: soft delete: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrAttachmentNotFound
	}
	if a.StoragePath != "" {
		_ = os.Remove(a.StoragePath)
	}
	return nil
}

// DeleteByComment soft-deletes every attachment for a comment. Used by
// commentStore.Delete to cascade. Errors removing on-disk files are
// swallowed — the row deletion is the source of truth and a stray
// orphaned file is recoverable.
func (s *AttachmentStore) DeleteByComment(ctx context.Context, commentID string) error {
	rows, err := s.ListByComment(ctx, commentID)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	if _, err := s.db.ExecContext(ctx,
		`UPDATE comment_attachments SET deleted_at = ?
		 WHERE comment_id = ? AND deleted_at = 0`,
		now, commentID); err != nil {
		return fmt.Errorf("comments: cascade delete: %w", err)
	}
	for _, a := range rows {
		_ = os.Remove(a.StoragePath)
	}
	dir := filepath.Join(s.rootDir, commentID)
	_ = os.Remove(dir)
	return nil
}

func scanAttachment(s scanner) (AttachmentRow, error) {
	var (
		a       AttachmentRow
		created int64
	)
	if err := s.Scan(&a.ID, &a.CommentID, &a.Filename, &a.MimeType,
		&a.SizeBytes, &a.StoragePath, &a.UploadedBy, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AttachmentRow{}, ErrAttachmentNotFound
		}
		return AttachmentRow{}, err
	}
	a.CreatedAt = time.Unix(created, 0).UTC()
	return a, nil
}
