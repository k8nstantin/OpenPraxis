package web

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gorilla/mux"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/settings"
)

// Default upload cap when settings aren't wired or the knob lookup fails.
// Mirrors the catalog default in internal/settings/catalog.go.
const defaultAttachmentMaxMB = 10

// resolveAttachmentMaxMB asks the settings resolver for
// `comment_attachment_max_mb` at system scope. Production handlers walk
// the comment's containing entity (product/manifest/task) for per-scope
// overrides; M1 keeps it simple and resolves at system scope only —
// the comment row itself doesn't carry product/manifest/task ids without
// extra joins, and the system-scope value is what operators actually
// twiddle today.
func resolveAttachmentMaxMB(r *http.Request, n *node.Node) int {
	if n == nil || n.SettingsResolver == nil {
		return defaultAttachmentMaxMB
	}
	res, err := n.SettingsResolver.Resolve(r.Context(), settings.Scope{}, "comment_attachment_max_mb")
	if err != nil {
		return defaultAttachmentMaxMB
	}
	switch v := res.Value.(type) {
	case int:
		if v > 0 {
			return v
		}
	case int64:
		if v > 0 {
			return int(v)
		}
	case float64:
		if v > 0 {
			return int(v)
		}
	}
	return defaultAttachmentMaxMB
}

// resolveAllowedMimes reads the `comment_attachment_allowed_mimes`
// knob at system scope. Empty string → package default allowlist.
func resolveAllowedMimes(r *http.Request, n *node.Node) []string {
	if n == nil || n.SettingsResolver == nil {
		return nil
	}
	res, err := n.SettingsResolver.Resolve(r.Context(), settings.Scope{}, "comment_attachment_allowed_mimes")
	if err != nil {
		return nil
	}
	raw, ok := res.Value.(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// attachmentView is the wire shape returned by upload + list endpoints.
// StoragePath is intentionally absent — clients fetch via /api/attachments/{id}.
type attachmentView struct {
	ID         string `json:"id"`
	CommentID  string `json:"comment_id"`
	Filename   string `json:"filename"`
	MimeType   string `json:"mime_type"`
	SizeBytes  int64  `json:"size_bytes"`
	UploadedBy string `json:"uploaded_by"`
	CreatedAt  int64  `json:"created_at"`
	URL        string `json:"url"`
}

func toAttachmentView(a comments.AttachmentRow) attachmentView {
	return attachmentView{
		ID:         a.ID,
		CommentID:  a.CommentID,
		Filename:   a.Filename,
		MimeType:   a.MimeType,
		SizeBytes:  a.SizeBytes,
		UploadedBy: a.UploadedBy,
		CreatedAt:  a.CreatedAt.Unix(),
		URL:        "/api/attachments/" + a.ID,
	}
}

func writeAttachError(w http.ResponseWriter, code, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg, "code": code})
}

// uploadAttachment handles POST /api/comments/{commentId}/attachments.
// Accepts multipart/form-data with one or more `file` parts. Each file
// is validated (size + mime allowlist), persisted under
// <data_dir>/attachments/<commentId>/, and returned in the response array.
//
// Partial success is allowed: if 3 files arrive and one is over-cap, the
// other two are still saved and the response 207-style array carries
// per-file errors. M1 keeps it simple — first failing file aborts the
// batch with the corresponding HTTP status. The frontend uploads one
// file at a time anyway.
func uploadAttachment(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		commentID := mux.Vars(r)["commentId"]
		if commentID == "" {
			writeAttachError(w, "empty_id", "commentId required", http.StatusBadRequest)
			return
		}
		// Validate the comment exists.
		if n.Comments == nil {
			writeAttachError(w, "internal", "comments store unavailable", http.StatusInternalServerError)
			return
		}
		if _, err := n.Comments.Get(r.Context(), commentID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeAttachError(w, "not_found", "comment not found", http.StatusNotFound)
				return
			}
			writeAttachError(w, "internal", err.Error(), http.StatusInternalServerError)
			return
		}

		maxMB := resolveAttachmentMaxMB(r, n)
		maxBytes := int64(maxMB) * 1024 * 1024
		// Cap the request body at 1 MiB beyond the per-file cap so a
		// stray client can't tie up the server with a multi-GB upload.
		// MaxBytesReader applies to the whole multipart envelope.
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(1<<20))
		if err := r.ParseMultipartForm(maxBytes); err != nil {
			if strings.Contains(err.Error(), "request body too large") {
				writeAttachError(w, "too_large",
					fmt.Sprintf("attachment exceeds %d MB", maxMB),
					http.StatusRequestEntityTooLarge)
				return
			}
			writeAttachError(w, "invalid_multipart", err.Error(), http.StatusBadRequest)
			return
		}

		uploadedBy := r.URL.Query().Get("author")
		if uploadedBy == "" {
			uploadedBy = r.FormValue("author")
		}

		files := r.MultipartForm.File["file"]
		if len(files) == 0 {
			files = r.MultipartForm.File["files"]
		}
		if len(files) == 0 {
			writeAttachError(w, "no_file", "no file part in multipart form", http.StatusBadRequest)
			return
		}

		allowed := resolveAllowedMimes(r, n)
		out := make([]attachmentView, 0, len(files))
		for _, fh := range files {
			if fh.Size > maxBytes {
				writeAttachError(w, "too_large",
					fmt.Sprintf("file %q (%d bytes) exceeds %d MB cap",
						fh.Filename, fh.Size, maxMB),
					http.StatusRequestEntityTooLarge)
				return
			}
			mime := fh.Header.Get("Content-Type")
			if !comments.MimeAllowed(mime, allowed) {
				writeAttachError(w, "mime_denied",
					fmt.Sprintf("mime %q not in allowlist", mime),
					http.StatusUnsupportedMediaType)
				return
			}
			f, err := fh.Open()
			if err != nil {
				writeAttachError(w, "internal", err.Error(), http.StatusInternalServerError)
				return
			}
			data, err := io.ReadAll(f)
			f.Close()
			if err != nil {
				writeAttachError(w, "internal", err.Error(), http.StatusInternalServerError)
				return
			}
			a, err := n.Attachments.Insert(r.Context(), commentID, uploadedBy, fh.Filename, mime, data)
			if err != nil {
				writeAttachError(w, "internal", err.Error(), http.StatusInternalServerError)
				return
			}
			out = append(out, toAttachmentView(a))
		}
		writeJSON(w, map[string]any{"attachments": out})
	}
}

// listAttachments handles GET /api/comments/{commentId}/attachments.
func listAttachments(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		commentID := mux.Vars(r)["commentId"]
		if commentID == "" {
			writeAttachError(w, "empty_id", "commentId required", http.StatusBadRequest)
			return
		}
		if n.Attachments == nil {
			writeJSON(w, map[string]any{"attachments": []attachmentView{}})
			return
		}
		rows, err := n.Attachments.ListByComment(r.Context(), commentID)
		if err != nil {
			writeAttachError(w, "internal", err.Error(), http.StatusInternalServerError)
			return
		}
		out := make([]attachmentView, 0, len(rows))
		for _, a := range rows {
			out = append(out, toAttachmentView(a))
		}
		writeJSON(w, map[string]any{"attachments": out})
	}
}

// serveAttachment handles GET /api/attachments/{id}. Streams the file
// with the recorded mime type + an inline-or-attachment Content-
// Disposition header. Inline for browser-friendly mimes (image/*,
// application/pdf, text/*); attachment for everything else so the file
// downloads instead of executing.
func serveAttachment(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		if id == "" || n.Attachments == nil {
			writeAttachError(w, "empty_id", "attachment id required", http.StatusBadRequest)
			return
		}
		a, err := n.Attachments.Get(r.Context(), id)
		if err != nil {
			if errors.Is(err, comments.ErrAttachmentNotFound) {
				writeAttachError(w, "not_found", "attachment not found", http.StatusNotFound)
				return
			}
			writeAttachError(w, "internal", err.Error(), http.StatusInternalServerError)
			return
		}
		f, err := os.Open(a.StoragePath)
		if err != nil {
			writeAttachError(w, "not_found", "attachment file missing", http.StatusNotFound)
			return
		}
		defer f.Close()

		mime := a.MimeType
		if mime == "" {
			mime = "application/octet-stream"
		}
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Content-Length", strconv.FormatInt(a.SizeBytes, 10))
		disposition := "attachment"
		if strings.HasPrefix(mime, "image/") || strings.HasPrefix(mime, "text/") || mime == "application/pdf" {
			disposition = "inline"
		}
		w.Header().Set("Content-Disposition",
			fmt.Sprintf(`%s; filename="%s"`, disposition, a.Filename))
		_, _ = io.Copy(w, f)
	}
}

// uploadOrphanAttachment handles POST /api/attachments. Multipart upload
// with no comment_id required — used by the BlockNote composer's
// uploadFile hook so the operator sees the image inline while typing,
// before the comment row exists. The returned id is held by the editor
// and Claimed when the comment lands.
func uploadOrphanAttachment(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if n.Attachments == nil {
			writeAttachError(w, "internal", "attachment store unavailable", http.StatusInternalServerError)
			return
		}
		maxMB := resolveAttachmentMaxMB(r, n)
		maxBytes := int64(maxMB) * 1024 * 1024
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(1<<20))
		if err := r.ParseMultipartForm(maxBytes); err != nil {
			if strings.Contains(err.Error(), "request body too large") {
				writeAttachError(w, "too_large",
					fmt.Sprintf("attachment exceeds %d MB", maxMB),
					http.StatusRequestEntityTooLarge)
				return
			}
			writeAttachError(w, "invalid_multipart", err.Error(), http.StatusBadRequest)
			return
		}

		uploadedBy := r.URL.Query().Get("author")
		if uploadedBy == "" {
			uploadedBy = r.FormValue("author")
		}

		files := r.MultipartForm.File["file"]
		if len(files) == 0 {
			files = r.MultipartForm.File["files"]
		}
		if len(files) == 0 {
			writeAttachError(w, "no_file", "no file part in multipart form", http.StatusBadRequest)
			return
		}

		allowed := resolveAllowedMimes(r, n)
		out := make([]attachmentView, 0, len(files))
		for _, fh := range files {
			if fh.Size > maxBytes {
				writeAttachError(w, "too_large",
					fmt.Sprintf("file %q (%d bytes) exceeds %d MB cap",
						fh.Filename, fh.Size, maxMB),
					http.StatusRequestEntityTooLarge)
				return
			}
			mime := fh.Header.Get("Content-Type")
			if !comments.MimeAllowed(mime, allowed) {
				writeAttachError(w, "mime_denied",
					fmt.Sprintf("mime %q not in allowlist", mime),
					http.StatusUnsupportedMediaType)
				return
			}
			f, err := fh.Open()
			if err != nil {
				writeAttachError(w, "internal", err.Error(), http.StatusInternalServerError)
				return
			}
			data, err := io.ReadAll(f)
			f.Close()
			if err != nil {
				writeAttachError(w, "internal", err.Error(), http.StatusInternalServerError)
				return
			}
			a, err := n.Attachments.InsertOrphan(r.Context(), uploadedBy, fh.Filename, mime, data)
			if err != nil {
				writeAttachError(w, "internal", err.Error(), http.StatusInternalServerError)
				return
			}
			out = append(out, toAttachmentView(a))
		}
		writeJSON(w, map[string]any{"attachments": out})
	}
}

// claimAttachment handles POST /api/attachments/{id}/claim. Body:
// {"comment_id": "..."}. Binds an orphan attachment row to a real
// comment after the comment is posted. 404 when the row isn't an orphan
// (already bound) or doesn't exist.
func claimAttachment(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		if id == "" || n.Attachments == nil {
			writeAttachError(w, "empty_id", "attachment id required", http.StatusBadRequest)
			return
		}
		var body struct {
			CommentID string `json:"comment_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeAttachError(w, "invalid_json", err.Error(), http.StatusBadRequest)
			return
		}
		if body.CommentID == "" {
			writeAttachError(w, "empty_id", "comment_id required", http.StatusBadRequest)
			return
		}
		if n.Comments != nil {
			if _, err := n.Comments.Get(r.Context(), body.CommentID); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeAttachError(w, "not_found", "comment not found", http.StatusNotFound)
					return
				}
				writeAttachError(w, "internal", err.Error(), http.StatusInternalServerError)
				return
			}
		}
		a, err := n.Attachments.Claim(r.Context(), id, body.CommentID)
		if err != nil {
			if errors.Is(err, comments.ErrAttachmentNotFound) {
				writeAttachError(w, "not_found", "orphan attachment not found", http.StatusNotFound)
				return
			}
			writeAttachError(w, "internal", err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"attachment": toAttachmentView(a)})
	}
}

// deleteAttachment handles DELETE /api/attachments/{id}.
func deleteAttachment(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		if id == "" || n.Attachments == nil {
			writeAttachError(w, "empty_id", "attachment id required", http.StatusBadRequest)
			return
		}
		if err := n.Attachments.SoftDelete(r.Context(), id); err != nil {
			if errors.Is(err, comments.ErrAttachmentNotFound) {
				writeAttachError(w, "not_found", "attachment not found", http.StatusNotFound)
				return
			}
			writeAttachError(w, "internal", err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "id": id})
	}
}

func registerAttachmentRoutes(api *mux.Router, n *node.Node) {
	api.HandleFunc("/comments/{commentId}/attachments", uploadAttachment(n)).Methods("POST")
	api.HandleFunc("/comments/{commentId}/attachments", listAttachments(n)).Methods("GET")
	api.HandleFunc("/attachments", uploadOrphanAttachment(n)).Methods("POST")
	api.HandleFunc("/attachments/{id}/claim", claimAttachment(n)).Methods("POST")
	api.HandleFunc("/attachments/{id}", serveAttachment(n)).Methods("GET")
	api.HandleFunc("/attachments/{id}", deleteAttachment(n)).Methods("DELETE")
}
