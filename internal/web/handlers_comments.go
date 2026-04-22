package web

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
	"github.com/k8nstantin/OpenPraxis/internal/node"
)

// markdownRenderer is the shared goldmark instance used to render comment
// bodies to HTML. GFM extensions are enabled (tables, task lists, strike,
// autolinks). Raw HTML in source is ALWAYS escaped — html.WithUnsafe() is
// deliberately omitted. See TestPOST_Comment_XSSEscape.
var markdownRenderer = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(
		html.WithXHTML(),
	),
)

// renderMarkdown converts a raw comment body to safe HTML. Returns the raw
// body wrapped in a <p> on goldmark error so the UI always has something to
// display.
func renderMarkdown(src string) string {
	var buf bytes.Buffer
	if err := markdownRenderer.Convert([]byte(src), &buf); err != nil {
		return "<p>" + escapeHTML(src) + "</p>"
	}
	return buf.String()
}

// escapeHTML is a tiny helper for the renderMarkdown fallback path.
func escapeHTML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return r.Replace(s)
}

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
	BodyHTML      string  `json:"body_html"`
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
		BodyHTML:     renderMarkdown(c.Body),
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

// TargetResolver canonicalizes a target_id from the URL path into the full
// UUID by looking up the entity. Accepts short markers (8-12 char prefixes)
// as well as full UUIDs. Returns an error if the target does not exist.
//
// Tests can pass a passthrough resolver (returns raw as-is) when entity
// stores are not wired; production uses nodeTargetResolver.
type TargetResolver func(target comments.TargetType, raw string) (string, error)

// passthroughResolver is the no-op TargetResolver for tests that don't wire
// real entity stores. Accepts whatever id the caller passes.
func passthroughResolver(_ comments.TargetType, raw string) (string, error) {
	return raw, nil
}

// nodeTargetResolver builds a TargetResolver backed by the node's entity
// stores. Each store's Get accepts marker or full UUID via `id = ? OR id LIKE ?`.
func nodeTargetResolver(n *node.Node) TargetResolver {
	return func(target comments.TargetType, raw string) (string, error) {
		switch target {
		case comments.TargetTask:
			if n.Tasks == nil {
				return raw, nil
			}
			t, err := n.Tasks.Get(raw)
			if err != nil {
				return "", fmt.Errorf("resolve target task %q: %w", raw, err)
			}
			if t == nil {
				return "", fmt.Errorf("target task not found: %s", raw)
			}
			return t.ID, nil
		case comments.TargetManifest:
			if n.Manifests == nil {
				return raw, nil
			}
			m, err := n.Manifests.Get(raw)
			if err != nil {
				return "", fmt.Errorf("resolve target manifest %q: %w", raw, err)
			}
			if m == nil {
				return "", fmt.Errorf("target manifest not found: %s", raw)
			}
			return m.ID, nil
		case comments.TargetProduct:
			if n.Products == nil {
				return raw, nil
			}
			p, err := n.Products.Get(raw)
			if err != nil {
				return "", fmt.Errorf("resolve target product %q: %w", raw, err)
			}
			if p == nil {
				return "", fmt.Errorf("target product not found: %s", raw)
			}
			return p.ID, nil
		case comments.TargetIdea:
			if n.Ideas == nil {
				return raw, nil
			}
			i, err := n.Ideas.Get(raw)
			if err != nil {
				return "", fmt.Errorf("resolve target idea %q: %w", raw, err)
			}
			if i == nil {
				return "", fmt.Errorf("target idea not found: %s", raw)
			}
			return i.ID, nil
		}
		return raw, nil
	}
}

// listComments is the shared handler body for
// GET /api/{products|manifests|tasks}/{id}/comments.
//
// Resolves the URL {id} to full UUID so short markers find the same
// target_id rows that MCP comment_add (also resolved) wrote.
func listComments(store *comments.Store, target comments.TargetType, resolve TargetResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rawID := mux.Vars(r)["id"]
		if rawID == "" {
			writeCommentError(w, "empty_target_id",
				comments.ErrEmptyTargetID.Error(), http.StatusBadRequest)
			return
		}
		id, err := resolve(target, rawID)
		if err != nil {
			writeCommentError(w, "target_not_found", err.Error(), http.StatusNotFound)
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
//
// Resolves the URL {id} to full UUID before insert so comments posted via
// HTTP never orphan on a short-marker target_id.
func addComment(store *comments.Store, target comments.TargetType, resolve TargetResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rawID := mux.Vars(r)["id"]
		if rawID == "" {
			writeCommentError(w, "empty_target_id",
				comments.ErrEmptyTargetID.Error(), http.StatusBadRequest)
			return
		}
		id, err := resolve(target, rawID)
		if err != nil {
			writeCommentError(w, "target_not_found", err.Error(), http.StatusNotFound)
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

// listCommentTypes serves GET /api/comments/types — the canonical Registry()
// order from internal/comments. M3 UI consumes this to populate the filter
// dropdown + add form type select so new types appear automatically.
func listCommentTypes() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, comments.Registry())
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
	{"ideas", comments.TargetIdea},
}

// registerCommentsRoutes attaches the 8 comment endpoints to /api using the
// given TargetResolver. Tests pass passthroughResolver; production wires
// nodeTargetResolver via registerCommentsRoutesFromNode.
func registerCommentsRoutes(api *mux.Router, store *comments.Store, resolve TargetResolver) {
	if resolve == nil {
		resolve = passthroughResolver
	}
	for _, s := range commentScopeRoutes {
		path := "/" + s.segment + "/{id}/comments"
		api.HandleFunc(path, listComments(store, s.target, resolve)).Methods("GET")
		api.HandleFunc(path, addComment(store, s.target, resolve)).Methods("POST")
	}
	api.HandleFunc("/comments/types", listCommentTypes()).Methods("GET")
	api.HandleFunc("/comments/{id}", editComment(store)).Methods("PATCH")
	api.HandleFunc("/comments/{id}", deleteComment(store)).Methods("DELETE")
}

// registerCommentsRoutesFromNode is the production entry point. Keeps the
// wire-up in handler.go a single line. Uses nodeTargetResolver so every
// HTTP comment call canonicalizes target_id to full UUID before insert/read.
func registerCommentsRoutesFromNode(api *mux.Router, n *node.Node) {
	registerCommentsRoutes(api, n.Comments, nodeTargetResolver(n))
}
