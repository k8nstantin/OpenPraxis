package web

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/templates"
)

// registerTemplateRoutes wires the RC/M1 read-only prompt template
// endpoints. PUT/history/restore land in RC/M2.
func registerTemplateRoutes(api *mux.Router, n *node.Node) {
	api.HandleFunc("/templates", apiTemplatesList(n)).Methods("GET")
	api.HandleFunc("/templates/{uid}", apiTemplateGet(n)).Methods("GET")
}

func apiTemplatesList(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if n.Templates == nil {
			writeError(w, "templates store not initialized", http.StatusServiceUnavailable)
			return
		}
		scope := r.URL.Query().Get("scope")
		section := r.URL.Query().Get("section")
		rows, err := n.Templates.List(r.Context(), scope, section)
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
