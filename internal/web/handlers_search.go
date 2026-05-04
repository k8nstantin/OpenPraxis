package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/action"
	"github.com/k8nstantin/OpenPraxis/internal/conversation"
	"github.com/k8nstantin/OpenPraxis/internal/memory"
	"github.com/k8nstantin/OpenPraxis/internal/node"
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
	q, limit, _ := parseSearchParamsPaged(r)
	return q, limit
}

// parseSearchParamsPaged adds offset support for endpoints that drive
// infinite scroll (conversations, actions). Other endpoints stay on the
// non-paged helper for backwards compatibility with their bare-array shape.
func parseSearchParamsPaged(r *http.Request) (query string, limit, offset int) {
	query = r.URL.Query().Get("q")
	if query == "" {
		query = r.URL.Query().Get("query")
	}
	limit = 50
	if ls := r.URL.Query().Get("limit"); ls != "" {
		if n, err := strconv.Atoi(ls); err == nil && n > 0 {
			limit = n
		}
	}
	if os := r.URL.Query().Get("offset"); os != "" {
		if n, err := strconv.Atoi(os); err == nil && n >= 0 {
			offset = n
		}
	}
	return query, limit, offset
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

// conversationSearchItem mirrors the slim shape returned by
// /api/conversations (apiConversations) — id, title, summary, agent, project,
// tags, turn_count, created_at, updated_at. Turns are intentionally omitted;
// detail view /api/conversations/{id} exposes them.
//
// SnippetHTML carries a pre-rendered fragment of matched text with the
// query wrapped in <mark>. Safe for innerHTML: caller-supplied text is
// escaped, only the <mark> tags are literal. MatchType is "keyword" or
// "semantic".
type conversationSearchItem struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Summary     string   `json:"summary"`
	Agent       string   `json:"agent"`
	Project     string   `json:"project"`
	Tags        []string `json:"tags"`
	TurnCount   int      `json:"turn_count"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
	SnippetHTML string   `json:"snippet_html,omitempty"`
	MatchType   string   `json:"match_type,omitempty"`
}

// searchEnvelope is the paginated response shape used by the
// infinite-scroll endpoints (conversations, actions). Keyword hits go in
// Items and are pageable via Offset/Limit; Semantic is populated only on
// page 0 and holds semantic-overlay hits that did not already appear in the
// keyword set, for a "related by meaning" tail in the UI.
type searchEnvelope[T any] struct {
	Items    []T  `json:"items"`
	Total    int  `json:"total"`
	Offset   int  `json:"offset"`
	Limit    int  `json:"limit"`
	Semantic []T  `json:"semantic,omitempty"`
	HasMore  bool `json:"has_more"`
}

// turnsText joins turn content for snippet extraction. Kept simple — the
// snippet helper scans linearly so concatenation cost is O(total chars) and
// we read only what we need once.
func turnsText(turns []conversation.Turn) string {
	if len(turns) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, t := range turns {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(t.Content)
	}
	return sb.String()
}

func conversationItem(c *conversation.Conversation, snippet, matchType string) conversationSearchItem {
	turnCount := c.TurnCount
	if turnCount == 0 && len(c.Turns) > 0 {
		turnCount = len(c.Turns)
	}
	return conversationSearchItem{
		ID:          c.ID,
		Title:       c.Title,
		Summary:     c.Summary,
		Agent:       c.Agent,
		Project:     c.Project,
		Tags:        c.Tags,
		TurnCount:   turnCount,
		CreatedAt:   c.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   c.UpdatedAt.UTC().Format(time.RFC3339),
		SnippetHTML: snippet,
		MatchType:   matchType,
	}
}

func apiConversationsSearchGET(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q, limit, offset := parseSearchParamsPaged(r)
		emptyEnv := searchEnvelope[conversationSearchItem]{
			Items: []conversationSearchItem{}, Total: 0, Offset: offset, Limit: limit,
		}
		if q == "" {
			writeJSON(w, emptyEnv)
			return
		}
		agent := r.URL.Query().Get("agent")
		project := r.URL.Query().Get("project")

		// Keyword hits — pageable.
		kwRows, kwTotal, err := n.Conversations.SearchKeyword(q, limit, offset)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		items := make([]conversationSearchItem, 0, len(kwRows))
		seen := make(map[string]struct{}, len(kwRows))
		for _, c := range kwRows {
			if agent != "" && c.Agent != agent {
				continue
			}
			if project != "" && c.Project != project {
				continue
			}
			snippet := highlightSnippet(q, c.Summary, c.Title, turnsText(c.Turns))
			items = append(items, conversationItem(c, snippet, "keyword"))
			seen[c.ID] = struct{}{}
		}

		env := searchEnvelope[conversationSearchItem]{
			Items:   items,
			Total:   kwTotal,
			Offset:  offset,
			Limit:   limit,
			HasMore: offset+len(items) < kwTotal,
		}

		// Semantic overlay — only on page 0. Capped at 10 extra results.
		if offset == 0 {
			semResults, semErr := n.SearchConversations(r.Context(), q, 10, agent, project)
			if semErr == nil {
				for _, sr := range semResults {
					if _, dup := seen[sr.Conversation.ID]; dup {
						continue
					}
					c := sr.Conversation
					snippet := highlightSnippet(q, c.Summary, c.Title, turnsText(c.Turns))
					env.Semantic = append(env.Semantic, conversationItem(&c, snippet, "semantic"))
				}
			}
		}

		writeJSON(w, env)
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

// actionSearchItem is the shape delivered by the paged actions search.
// Embeds the full action record and adds SnippetHTML for the UI — that way
// detail-view expectations (tool_name, tool_input, tool_response, etc.)
// stay identical to the existing /api/actions payloads.
type actionSearchItem struct {
	action.Action
	SnippetHTML string `json:"snippet_html,omitempty"`
}

func apiActionsSearch(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q, limit, offset := parseSearchParamsPaged(r)
		env := searchEnvelope[actionSearchItem]{
			Items: []actionSearchItem{}, Total: 0, Offset: offset, Limit: limit,
		}
		// Empty q means "give me every action paged DESC". The
		// Actions Log surface uses a single endpoint for both browse and
		// search; q is purely optional. No empty-query short-circuit.
		var results []action.Action
		var total int
		var err error
		if q == "" {
			results, total, err = n.Actions.ListRecentPaged(limit, offset)
		} else {
			results, total, err = n.Actions.SearchPaged(q, limit, offset)
		}
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		items := make([]actionSearchItem, 0, len(results))
		for _, a := range results {
			var snippet string
			if q != "" {
				snippet = highlightSnippet(q, a.ToolInput, a.ToolResponse, a.ToolName)
			}
			items = append(items, actionSearchItem{Action: a, SnippetHTML: snippet})
		}
		env.Items = items
		env.Total = total
		env.HasMore = offset+len(items) < total
		writeJSON(w, env)
	}
}
