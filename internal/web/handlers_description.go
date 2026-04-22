package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
	"github.com/k8nstantin/OpenPraxis/internal/node"
)

// DV/M3 — HTTP sugar endpoints over description_revision comments.
// Three endpoints per entity type (product / manifest / task); 9 total:
//
//   GET  /api/{scope}/{id}/description/history
//   GET  /api/{scope}/{id}/description/revisions/{comment_id}
//   POST /api/{scope}/{id}/description/restore/{comment_id}
//
// scope ∈ {products, manifests, tasks}. Short-marker and full-UUID
// target IDs both work — the underlying Node helpers canonicalise.

// revisionView is the JSON shape for a single description_revision entry,
// with both unix + ISO timestamps so clients don't have to format.
type revisionView struct {
	ID           string `json:"id"`
	Version      int    `json:"version"`
	Author       string `json:"author"`
	Body         string `json:"body"`
	CreatedAt    int64  `json:"created_at"`
	CreatedAtISO string `json:"created_at_iso"`
}

func toRevisionView(r node.RevisionEntry) revisionView {
	return revisionView{
		ID:           r.ID,
		Version:      r.Version,
		Author:       r.Author,
		Body:         r.Body,
		CreatedAt:    r.CreatedAt,
		CreatedAtISO: time.Unix(r.CreatedAt, 0).UTC().Format(time.RFC3339),
	}
}

// restoreRequest is the POST body for the restore endpoint. Author is
// optional — when empty the helper falls back to the peer UUID.
type restoreRequest struct {
	Author string `json:"author"`
}

// restoreResponse returns the newly-created revision + a hint of whether
// the restore actually wrote a new row. restored=false means the target
// body already matched the historical body, so we no-op'd.
type restoreResponse struct {
	Restored  bool          `json:"restored"`
	NewID     string        `json:"new_revision_id,omitempty"`
	From      string        `json:"restored_from"`
	Revisions []revisionView `json:"history,omitempty"`
}

// apiDescriptionHistory — GET /api/{scope}/{id}/description/history
func apiDescriptionHistory(n *node.Node, target comments.TargetType) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		if id == "" {
			writeError(w, "id required", http.StatusBadRequest)
			return
		}
		limit := 100
		if raw := r.URL.Query().Get("limit"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil {
				writeError(w, "limit must be an integer", http.StatusBadRequest)
				return
			}
			limit = parsed
		}
		rows, err := n.DescriptionHistory(r.Context(), target, id, limit)
		if err != nil {
			writeError(w, err.Error(), descriptionErrStatus(err))
			return
		}
		views := make([]revisionView, 0, len(rows))
		for _, r := range rows {
			views = append(views, toRevisionView(r))
		}
		writeJSON(w, map[string]any{"items": views})
	}
}

// apiDescriptionGetRevision — GET /api/{scope}/{id}/description/revisions/{comment_id}
func apiDescriptionGetRevision(n *node.Node, target comments.TargetType) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["id"]
		commentID := vars["comment_id"]
		if id == "" || commentID == "" {
			writeError(w, "id and comment_id required", http.StatusBadRequest)
			return
		}
		rev, err := n.GetDescriptionRevision(r.Context(), target, id, commentID)
		if err != nil {
			writeError(w, err.Error(), descriptionErrStatus(err))
			return
		}
		if rev == nil {
			writeError(w, "revision not found", http.StatusNotFound)
			return
		}
		writeJSON(w, toRevisionView(*rev))
	}
}

// apiDescriptionRestore — POST /api/{scope}/{id}/description/restore/{comment_id}
func apiDescriptionRestore(n *node.Node, target comments.TargetType) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["id"]
		commentID := vars["comment_id"]
		if id == "" || commentID == "" {
			writeError(w, "id and comment_id required", http.StatusBadRequest)
			return
		}
		var req restoreRequest
		if r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, "invalid request body", http.StatusBadRequest)
				return
			}
		}
		newID, err := n.RestoreDescription(r.Context(), target, id, commentID, req.Author)
		if err != nil {
			writeError(w, err.Error(), descriptionErrStatus(err))
			return
		}
		writeJSON(w, restoreResponse{
			Restored: newID != "",
			NewID:    newID,
			From:     commentID,
		})
	}
}

// descriptionErrStatus maps helper errors to HTTP statuses. Helpers in
// internal/node currently return unwrapped fmt.Errorf strings, so string
// matching is the pragmatic v1 path; a sentinel-error refactor on those
// helpers would let this become errors.Is checks instead.
func descriptionErrStatus(err error) int {
	if err == nil {
		return 200
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "not found"),
		strings.Contains(msg, "does not belong"),
		strings.Contains(msg, "is not a description_revision"):
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

// descriptionScopeRoutes pairs URL-plural segment with TargetType, mirroring
// commentScopeRoutes so the loop stays declarative.
var descriptionScopeRoutes = []struct {
	segment string
	target  comments.TargetType
}{
	{"products", comments.TargetProduct},
	{"manifests", comments.TargetManifest},
	{"tasks", comments.TargetTask},
	{"ideas", comments.TargetIdea},
}

// registerDescriptionRoutes attaches the 9 DV/M3 endpoints to /api.
func registerDescriptionRoutes(api *mux.Router, n *node.Node) {
	for _, s := range descriptionScopeRoutes {
		base := "/" + s.segment + "/{id}/description"
		api.HandleFunc(base+"/history", apiDescriptionHistory(n, s.target)).Methods("GET")
		api.HandleFunc(base+"/revisions/{comment_id}", apiDescriptionGetRevision(n, s.target)).Methods("GET")
		api.HandleFunc(base+"/restore/{comment_id}", apiDescriptionRestore(n, s.target)).Methods("POST")
	}
}
