package web

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
	"github.com/k8nstantin/OpenPraxis/internal/node"
)

// M2-T5 — HTTP surface for comments. Thin wrapper over comments.Store;
// mirrors the MCP surface in parallel task M2-T4. Error codes match the
// sentinel table documented in the M2 manifest so UI + agents can share
// one taxonomy.

// commentMaxBodyBytes caps request bodies at 1 MiB. Longer bodies produce
// 413 with code=body_too_large before JSON decode.
const commentMaxBodyBytes = 1 << 20

// commentView is the wire shape — the comments.Comment with added ISO
// timestamp fields. Defined here (not in internal/comments/model.go) so
// the core model stays free of HTTP concerns.
type commentView struct {
	ID            string  `json:"id"`
	TargetType    string  `json:"target_type"`
	TargetID      string  `json:"target_id"`
	Author        string  `json:"author"`
	Type          string  `json:"type"`
	Body          string  `json:"body"`
	CreatedAt     int64   `json:"created_at"`
	CreatedAtISO  string  `json:"created_at_iso"`
	UpdatedAt     *int64  `json:"updated_at,omitempty"`
	UpdatedAtISO  string  `json:"updated_at_iso,omitempty"`
	ParentID      *string `json:"parent_id,omitempty"`
}

func toCommentView(c comments.Comment) commentView {
	v := commentView{
		ID:           c.ID,
		TargetType:   string(c.TargetType),
		TargetID:     c.TargetID,
		Author:       c.Author,
		Type:         string(c.Type),
		Body:         c.Body,
		CreatedAt:    c.CreatedAt.Unix(),
		CreatedAtISO: c.CreatedAt.UTC().Format(time.RFC3339),
		ParentID:     c.ParentID,
	}
	if c.UpdatedAt != nil {
		u := c.UpdatedAt.Unix()
		v.UpdatedAt = &u
		v.UpdatedAtISO = c.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return v
}

// writeCommentError emits {error, code} at the given status. The code field
// is what test and UI clients match on; error carries a human message.
func writeCommentError(w http.ResponseWriter, code, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg, "code": code})
}

// commentErrorStatus maps a comments sentinel to (code, status).
func commentErrorStatus(err error) (string, int) {
	switch {
	case errors.Is(err, comments.ErrUnknownTargetType):
		return "unknown_target_type", http.StatusNotFound
	case errors.Is(err, comments.ErrEmptyTargetID):
		return "empty_target_id", http.StatusBadRequest
	case errors.Is(err, comments.ErrUnknownCommentType):
		return "unknown_type", http.StatusBadRequest
	case errors.Is(err, comments.ErrEmptyAuthor):
		return "empty_author", http.StatusBadRequest
	case errors.Is(err, comments.ErrEmptyBody):
		return "empty_body", http.StatusBadRequest
	}
	return "internal", http.StatusInternalServerError
}

// validCommentTypesList is the stable, sorted set surfaced in error messages
// so clients get a deterministic list of acceptable types.
func validCommentTypesList() string {
	all := comments.AllCommentTypes()
	parts := make([]string, 0, len(all))
	for _, t := range all {
		parts = append(parts, string(t))
	}
	return strings.Join(parts, ", ")
}

// readBodyCapped enforces the 1 MiB cap and surfaces body_too_large before
// invoking JSON decode. It returns false iff a response has already been
// written.
func readBodyCapped(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, commentMaxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		if errors.As(err, new(*http.MaxBytesError)) ||
			strings.Contains(err.Error(), "http: request body too large") {
			writeCommentError(w, "body_too_large",
				fmt.Sprintf("request body exceeds %d bytes", commentMaxBodyBytes),
				http.StatusRequestEntityTooLarge)
			return false
		}
		writeCommentError(w, "invalid_body", "invalid request body: "+err.Error(),
			http.StatusBadRequest)
		return false
	}
	return true
}

// parseLimit honors the ?limit query param. Returns (limit, true) on success,
// or (0, false) after writing a 400 response for non-numeric input. limit<=0
// is passed through to the store (which applies its own default/cap).
func parseLimit(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return 0, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		writeCommentError(w, "invalid_limit",
			"limit must be an integer", http.StatusBadRequest)
		return 0, false
	}
	return n, true
}

// parseTypeFilter honors the ?type query param. Returns (nil, true) when no
// type is provided. Returns (nil, false) after writing a 400 for an unknown
// type string.
func parseTypeFilter(w http.ResponseWriter, r *http.Request) (*comments.CommentType, bool) {
	raw := r.URL.Query().Get("type")
	if raw == "" {
		return nil, true
	}
	if !comments.IsValidCommentType(raw) {
		writeCommentError(w, "unknown_type",
			fmt.Sprintf("unknown type %q (valid: %s)", raw, validCommentTypesList()),
			http.StatusBadRequest)
		return nil, false
	}
	ct := comments.CommentType(raw)
	return &ct, true
}

// listCommentsResponse is the GET-list shape.
type listCommentsResponse struct {
	Comments []commentView `json:"comments"`
}

// addCommentRequest is the POST body shape.
type addCommentRequest struct {
	Author string `json:"author"`
	Type   string `json:"type"`
	Body   string `json:"body"`
}

// editCommentRequest is the PATCH body shape.
type editCommentRequest struct {
	Body string `json:"body"`
}

// listComments is the shared handler body for
// GET /api/{products|manifests|tasks}/{id}/comments.
func listComments(store *comments.Store, target comments.TargetType) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		if id == "" {
			writeCommentError(w, "empty_target_id",
				comments.ErrEmptyTargetID.Error(), http.StatusBadRequest)
			return
		}
		limit, ok := parseLimit(w, r)
		if !ok {
			return
		}
		filter, ok := parseTypeFilter(w, r)
		if !ok {
			return
		}
		out, err := store.List(r.Context(), target, id, limit, filter)
		if err != nil {
			writeCommentError(w, "internal", err.Error(), http.StatusInternalServerError)
			return
		}
		views := make([]commentView, 0, len(out))
		for _, c := range out {
			views = append(views, toCommentView(c))
		}
		writeJSON(w, listCommentsResponse{Comments: views})
	}
}

// addComment is the shared handler body for
// POST /api/{products|manifests|tasks}/{id}/comments.
func addComment(store *comments.Store, target comments.TargetType) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		if id == "" {
			writeCommentError(w, "empty_target_id",
				comments.ErrEmptyTargetID.Error(), http.StatusBadRequest)
			return
		}
		var req addCommentRequest
		if !readBodyCapped(w, r, &req) {
			return
		}
		cType := comments.CommentType(req.Type)
		if err := comments.ValidateAdd(target, id, req.Author, cType, req.Body); err != nil {
			code, status := commentErrorStatus(err)
			writeCommentError(w, code, err.Error(), status)
			return
		}
		c, err := store.Add(r.Context(), target, id, req.Author, cType, req.Body)
		if err != nil {
			code, status := commentErrorStatus(err)
			writeCommentError(w, code, err.Error(), status)
			return
		}
		writeJSON(w, toCommentView(c))
	}
}

// editComment handles PATCH /api/comments/:id.
func editComment(store *comments.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		if id == "" {
			writeCommentError(w, "empty_id", "comment id required",
				http.StatusBadRequest)
			return
		}
		var req editCommentRequest
		if !readBodyCapped(w, r, &req) {
			return
		}
		if err := comments.ValidateEdit(req.Body); err != nil {
			code, status := commentErrorStatus(err)
			writeCommentError(w, code, err.Error(), status)
			return
		}
		if err := store.Edit(r.Context(), id, req.Body); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeCommentError(w, "not_found",
					"comment not found", http.StatusNotFound)
				return
			}
			writeCommentError(w, "internal", err.Error(),
				http.StatusInternalServerError)
			return
		}
		c, err := store.Get(r.Context(), id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeCommentError(w, "not_found",
					"comment not found", http.StatusNotFound)
				return
			}
			writeCommentError(w, "internal", err.Error(),
				http.StatusInternalServerError)
			return
		}
		writeJSON(w, toCommentView(c))
	}
}

// deleteComment handles DELETE /api/comments/:id. Idempotent per store
// semantics — repeated deletes return 200.
func deleteComment(store *comments.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		if id == "" {
			writeCommentError(w, "empty_id", "comment id required",
				http.StatusBadRequest)
			return
		}
		if err := store.Delete(r.Context(), id); err != nil {
			writeCommentError(w, "internal", err.Error(),
				http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "id": id})
	}
}

// commentScopeRoutes pairs a URL-plural segment with its TargetType so the
// registration loop stays declarative.
var commentScopeRoutes = []struct {
	segment string
	target  comments.TargetType
}{
	{"products", comments.TargetProduct},
	{"manifests", comments.TargetManifest},
	{"tasks", comments.TargetTask},
}

// registerCommentsRoutes attaches the 8 comment endpoints to /api.
func registerCommentsRoutes(api *mux.Router, store *comments.Store) {
	for _, s := range commentScopeRoutes {
		path := "/" + s.segment + "/{id}/comments"
		api.HandleFunc(path, listComments(store, s.target)).Methods("GET")
		api.HandleFunc(path, addComment(store, s.target)).Methods("POST")
	}
	api.HandleFunc("/comments/{id}", editComment(store)).Methods("PATCH")
	api.HandleFunc("/comments/{id}", deleteComment(store)).Methods("DELETE")
}

// registerCommentsRoutesFromNode is the production entry point. Keeps the
// wire-up in handler.go a single line.
func registerCommentsRoutesFromNode(api *mux.Router, n *node.Node) {
	registerCommentsRoutes(api, n.Comments)
}
