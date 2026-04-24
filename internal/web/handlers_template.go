package web

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/templates"
)

// registerTemplateRoutes wires the RC/M1 read-only prompt template
// endpoints plus the RC/M2 write path (create/update/history/point-in-
// time/tombstone).
func registerTemplateRoutes(api *mux.Router, n *node.Node) {
	api.HandleFunc("/templates", apiTemplatesList(n)).Methods("GET")
	api.HandleFunc("/templates", apiTemplateCreate(n)).Methods("POST")
	api.HandleFunc("/templates/{uid}", apiTemplateGet(n)).Methods("GET")
	api.HandleFunc("/templates/{uid}", apiTemplateUpdate(n)).Methods("PUT")
	api.HandleFunc("/templates/{uid}", apiTemplateDelete(n)).Methods("DELETE")
	api.HandleFunc("/templates/{uid}/history", apiTemplateHistory(n)).Methods("GET")
	api.HandleFunc("/templates/{uid}/at", apiTemplateAt(n)).Methods("GET")
}

func apiTemplatesList(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if n.Templates == nil {
			writeError(w, "templates store not initialized", http.StatusServiceUnavailable)
			return
		}
		scope := r.URL.Query().Get("scope")
		scopeID := r.URL.Query().Get("scope_id")
		section := r.URL.Query().Get("section")
		rows, err := n.Templates.ListWithScopeID(r.Context(), scope, scopeID, section)
		if err != nil {
			writeError(w, "list templates: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if rows == nil {
			rows = []*templates.Template{}
		}
		writeJSON(w, rows)
	}
}

func apiTemplateGet(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if n.Templates == nil {
			writeError(w, "templates store not initialized", http.StatusServiceUnavailable)
			return
		}
		uid := mux.Vars(r)["uid"]
		t, err := n.Templates.GetByUID(r.Context(), uid)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, "template not found", http.StatusNotFound)
			return
		}
		if err != nil {
			writeError(w, "get template: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, t)
	}
}

type templateCreateReq struct {
	Scope     string `json:"scope"`
	ScopeID   string `json:"scope_id"`
	Section   string `json:"section"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	ChangedBy string `json:"changed_by"`
	Reason    string `json:"reason"`
}

func apiTemplateCreate(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if n.Templates == nil {
			writeError(w, "templates store not initialized", http.StatusServiceUnavailable)
			return
		}
		var req templateCreateReq
		if !decodeBody(w, r, &req) {
			return
		}
		if req.Scope == "" || req.Section == "" {
			writeError(w, "scope and section are required", http.StatusBadRequest)
			return
		}
		if req.ChangedBy == "" {
			req.ChangedBy = "http:unknown"
		} else {
			req.ChangedBy = "http:" + req.ChangedBy
		}
		uid, err := n.Templates.Create(r.Context(), req.Scope, req.ScopeID, req.Section,
			req.Title, req.Body, req.ChangedBy, req.Reason)
		if errors.Is(err, templates.ErrDuplicateOverride) {
			writeError(w, err.Error(), http.StatusConflict)
			return
		}
		if err != nil {
			writeError(w, "create template: "+err.Error(), http.StatusInternalServerError)
			return
		}
		t, err := n.Templates.GetByUID(r.Context(), uid)
		if err != nil {
			writeError(w, "get created template: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, t)
	}
}

type templateUpdateReq struct {
	Body      string `json:"body"`
	ChangedBy string `json:"changed_by"`
	Reason    string `json:"reason"`
	Status    string `json:"status"`
}

func apiTemplateUpdate(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if n.Templates == nil {
			writeError(w, "templates store not initialized", http.StatusServiceUnavailable)
			return
		}
		uid := mux.Vars(r)["uid"]
		var req templateUpdateReq
		if !decodeBody(w, r, &req) {
			return
		}
		author := req.ChangedBy
		if author == "" {
			author = "http:unknown"
		} else {
			author = "http:" + author
		}
		if req.Status == "closed" {
			if err := n.Templates.CloseStatus(r.Context(), uid, author, req.Reason); err != nil {
				if errors.Is(err, templates.ErrNotFound) {
					writeError(w, "template not found", http.StatusNotFound)
					return
				}
				writeError(w, "close template: "+err.Error(), http.StatusInternalServerError)
				return
			}
		} else {
			if err := n.Templates.UpdateBody(r.Context(), uid, req.Body, author, req.Reason); err != nil {
				if errors.Is(err, templates.ErrNotFound) {
					writeError(w, "template not found", http.StatusNotFound)
					return
				}
				writeError(w, "update template: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}
		t, err := n.Templates.GetByUID(r.Context(), uid)
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, map[string]string{"uid": uid, "status": "closed"})
			return
		}
		if err != nil {
			writeError(w, "get updated template: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, t)
	}
}

func apiTemplateDelete(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if n.Templates == nil {
			writeError(w, "templates store not initialized", http.StatusServiceUnavailable)
			return
		}
		uid := mux.Vars(r)["uid"]
		author := "http:unknown"
		if cb := r.URL.Query().Get("changed_by"); cb != "" {
			author = "http:" + cb
		}
		reason := r.URL.Query().Get("reason")
		if err := n.Templates.Tombstone(r.Context(), uid, author, reason); err != nil {
			if errors.Is(err, templates.ErrNotFound) {
				writeError(w, "template not found", http.StatusNotFound)
				return
			}
			writeError(w, "tombstone template: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"uid": uid, "deleted": "true"})
	}
}

func apiTemplateHistory(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if n.Templates == nil {
			writeError(w, "templates store not initialized", http.StatusServiceUnavailable)
			return
		}
		uid := mux.Vars(r)["uid"]
		rows, err := n.Templates.History(r.Context(), uid)
		if err != nil {
			writeError(w, "history: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if rows == nil {
			rows = []*templates.Template{}
		}
		writeJSON(w, rows)
	}
}

func apiTemplateAt(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if n.Templates == nil {
			writeError(w, "templates store not initialized", http.StatusServiceUnavailable)
			return
		}
		uid := mux.Vars(r)["uid"]
		tParam := r.URL.Query().Get("t")
		if tParam == "" {
			writeError(w, "t query parameter required (RFC3339)", http.StatusBadRequest)
			return
		}
		at, err := time.Parse(time.RFC3339, tParam)
		if err != nil {
			writeError(w, "invalid t: "+err.Error(), http.StatusBadRequest)
			return
		}
		row, err := n.Templates.AtTime(r.Context(), uid, at)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, "no active template at that time", http.StatusNotFound)
			return
		}
		if err != nil {
			writeError(w, "at: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, row)
	}
}
