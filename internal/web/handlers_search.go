package web

import (
	"net/http"
	"strconv"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/action"
	"github.com/k8nstantin/OpenPraxis/internal/idea"
	"github.com/k8nstantin/OpenPraxis/internal/memory"
	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/product"
	"github.com/k8nstantin/OpenPraxis/internal/task"
)

// GET-alias helpers for per-tab scoped search. See manifest 019daafb-b5e M2.
// The existing POST routes (memories/search, manifests/search, conversations/search)
// stay untouched for MCP/programmatic consumers — these additive GET
// handlers let the frontend use a uniform `GET /api/<type>/search?q=&limit=`
// call across every tab.
//
// Response-shape contract (manifest 019dac18-638): every GET /api/<type>/search
// returns a flat [] of entities matching the list endpoint's shape, and never
// `null` on empty — the frontend can render results without unwrapping.
// Scored/semantic data is still available via POST, or via `?scored=true` on
// the memories/conversations GET endpoints.

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

// scoreRef surfaces score/distance alongside the unwrapped entity list when
// a caller opts in via `?scored=true`.
type scoreRef struct {
	ID       string  `json:"id"`
	Score    float64 `json:"score"`
	Distance float64 `json:"distance"`
}

func apiMemoriesSearchGET(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q, limit := parseSearchParams(r)
		if q == "" {
			writeJSON(w, []memory.Memory{})
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
		items := make([]memory.Memory, 0, len(results))
		scores := make([]scoreRef, 0, len(results))
		for _, sr := range results {
			items = append(items, sr.Memory)
			scores = append(scores, scoreRef{ID: sr.Memory.ID, Score: sr.Score, Distance: sr.Distance})
		}
		if r.URL.Query().Get("scored") == "true" {
			writeJSON(w, map[string]any{"items": items, "scores": scores})
			return
		}
		writeJSON(w, items)
	}
}

func apiManifestsSearchGET(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q, limit := parseSearchParams(r)
		if q == "" {
			writeJSON(w, []enrichedManifest{})
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

// conversationSearchItem mirrors the slim shape returned by
// /api/conversations (apiConversations) — id, title, summary, agent, project,
// tags, turn_count, created_at, updated_at. Turns are intentionally omitted;
// detail view /api/conversations/{id} exposes them.
type conversationSearchItem struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Summary   string   `json:"summary"`
	Agent     string   `json:"agent"`
	Project   string   `json:"project"`
	Tags      []string `json:"tags"`
	TurnCount int      `json:"turn_count"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

func apiConversationsSearchGET(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q, limit := parseSearchParams(r)
		if q == "" {
			writeJSON(w, []conversationSearchItem{})
			return
		}
		agent := r.URL.Query().Get("agent")
		project := r.URL.Query().Get("project")
		results, err := n.SearchConversations(r.Context(), q, limit, agent, project)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		items := make([]conversationSearchItem, 0, len(results))
		scores := make([]scoreRef, 0, len(results))
		for _, sr := range results {
			c := sr.Conversation
			turnCount := c.TurnCount
			if turnCount == 0 && len(c.Turns) > 0 {
				turnCount = len(c.Turns)
			}
			items = append(items, conversationSearchItem{
				ID:        c.ID,
				Title:     c.Title,
				Summary:   c.Summary,
				Agent:     c.Agent,
				Project:   c.Project,
				Tags:      c.Tags,
				TurnCount: turnCount,
				CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339),
				UpdatedAt: c.UpdatedAt.UTC().Format(time.RFC3339),
			})
			scores = append(scores, scoreRef{ID: c.ID, Score: sr.Score, Distance: sr.Distance})
		}
		if r.URL.Query().Get("scored") == "true" {
			writeJSON(w, map[string]any{"items": items, "scores": scores})
			return
		}
		writeJSON(w, items)
	}
}

func apiTasksSearch(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q, limit := parseSearchParams(r)
		if q == "" {
			writeJSON(w, []*task.Task{})
			return
		}
		results, err := n.Tasks.Search(q, limit)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		if results == nil {
			results = []*task.Task{}
		}
		writeJSON(w, results)
	}
}

func apiProductsSearch(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q, limit := parseSearchParams(r)
		if q == "" {
			writeJSON(w, []*product.Product{})
			return
		}
		results, err := n.Products.Search(q, limit)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		if results == nil {
			results = []*product.Product{}
		}
		writeJSON(w, results)
	}
}

func apiIdeasSearch(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q, limit := parseSearchParams(r)
		if q == "" {
			writeJSON(w, []*idea.Idea{})
			return
		}
		results, err := n.Ideas.Search(q, limit)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		if results == nil {
			results = []*idea.Idea{}
		}
		writeJSON(w, results)
	}
}

func apiActionsSearch(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q, limit := parseSearchParams(r)
		if q == "" {
			writeJSON(w, []action.Action{})
			return
		}
		results, err := n.Actions.Search(q, limit)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		if results == nil {
			results = []action.Action{}
		}
		writeJSON(w, results)
	}
}
