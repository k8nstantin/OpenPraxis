package web

import (
	"net/http"
	"strconv"

	"github.com/k8nstantin/OpenPraxis/internal/node"
)

// GET-alias helpers for per-tab scoped search. See manifest 019daafb-b5e M2.
// The existing POST routes (memories/search, manifests/search, conversations/search)
// stay untouched for MCP/programmatic consumers — these additive GET
// handlers let the frontend use a uniform `GET /api/<type>/search?q=&limit=`
// call across every tab.

func parseSearchParams(r *http.Request) (string, int) {
	q := r.URL.Query().Get("q")
	if q == "" {
		q = r.URL.Query().Get("query")
	}
	limit := 50
	if ls := r.URL.Query().Get("limit"); ls != "" {
		if n, err := strconv.Atoi(ls); err == nil && n > 0 {
			limit = n
		}
	}
	return q, limit
}

func apiMemoriesSearchGET(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q, limit := parseSearchParams(r)
		if q == "" {
			writeJSON(w, []any{})
			return
		}
		scope := r.URL.Query().Get("scope")
		project := r.URL.Query().Get("project")
		domain := r.URL.Query().Get("domain")
		results, err := n.SearchMemories(r.Context(), q, limit, scope, project, domain)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, results)
	}
}

func apiManifestsSearchGET(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q, limit := parseSearchParams(r)
		if q == "" {
			writeJSON(w, []any{})
			return
		}
		results, err := n.Manifests.Search(q, limit)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, enrichManifests(n, results))
	}
}

func apiConversationsSearchGET(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q, limit := parseSearchParams(r)
		if q == "" {
			writeJSON(w, []any{})
			return
		}
		agent := r.URL.Query().Get("agent")
		project := r.URL.Query().Get("project")
		results, err := n.SearchConversations(r.Context(), q, limit, agent, project)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, results)
	}
}

func apiTasksSearch(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q, limit := parseSearchParams(r)
		if q == "" {
			writeJSON(w, []any{})
			return
		}
		results, err := n.Tasks.Search(q, limit)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, results)
	}
}

func apiProductsSearch(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q, limit := parseSearchParams(r)
		if q == "" {
			writeJSON(w, []any{})
			return
		}
		results, err := n.Products.Search(q, limit)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, results)
	}
}

func apiIdeasSearch(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q, limit := parseSearchParams(r)
		if q == "" {
			writeJSON(w, []any{})
			return
		}
		results, err := n.Ideas.Search(q, limit)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, results)
	}
}

func apiActionsSearch(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q, limit := parseSearchParams(r)
		if q == "" {
			writeJSON(w, []any{})
			return
		}
		results, err := n.Actions.Search(q, limit)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, results)
	}
}
