package web

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"openpraxis/internal/node"

	"github.com/gorilla/mux"
)

func apiConversations(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agent := r.URL.Query().Get("agent")
		project := r.URL.Query().Get("project")
		convos, err := n.Conversations.List(agent, project, 50, 0) // already ORDER BY updated_at DESC
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		// Return without full turns for list view (lighter payload)
		type listItem struct {
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
		var items []listItem
		for _, c := range convos {
			items = append(items, listItem{
				ID: c.ID, Title: c.Title, Summary: c.Summary,
				Agent: c.Agent, Project: c.Project, Tags: c.Tags,
				TurnCount: c.TurnCount,
				CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339),
				UpdatedAt: c.UpdatedAt.UTC().Format(time.RFC3339),
			})
		}
		writeJSON(w, items)
	}
}

func apiConversationsByPeer(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		convos, err := n.Conversations.List("", "", 200, 0)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}

		type convItem struct {
			ID        string `json:"id"`
			Title     string `json:"title"`
			Agent     string `json:"agent"`
			TurnCount int    `json:"turn_count"`
			UpdatedAt string `json:"updated_at"`
		}
		type sessionGroup struct {
			Session       string     `json:"session"`
			Count         int        `json:"count"`
			Conversations []convItem `json:"conversations"`
		}
		type peerGroup struct {
			PeerID   string         `json:"peer_id"`
			Count    int            `json:"count"`
			Sessions []sessionGroup `json:"sessions"`
		}

		type peerData struct {
			sessionOrder []string
			sessions     map[string][]convItem
		}
		peers := make(map[string]*peerData)
		peerOrder := []string{}

		for _, c := range convos {
			peerID := c.SourceNode
			if peerID == "" {
				peerID = "unknown"
			}
			agent := c.Agent
			if agent == "" {
				agent = "unknown"
			}

			pd, ok := peers[peerID]
			if !ok {
				pd = &peerData{sessions: make(map[string][]convItem)}
				peers[peerID] = pd
				peerOrder = append(peerOrder, peerID)
			}
			if _, ok := pd.sessions[agent]; !ok {
				pd.sessionOrder = append(pd.sessionOrder, agent)
			}
			pd.sessions[agent] = append(pd.sessions[agent], convItem{
				ID:        c.ID,
				Title:     c.Title,
				Agent:     c.Agent,
				TurnCount: c.TurnCount,
				UpdatedAt: c.UpdatedAt.UTC().Format(time.RFC3339),
			})
		}

		var result []peerGroup
		for _, pid := range peerOrder {
			pd := peers[pid]
			var sgs []sessionGroup
			totalCount := 0
			for _, agent := range pd.sessionOrder {
				clist := pd.sessions[agent]
				sgs = append(sgs, sessionGroup{
					Session:       agent,
					Count:         len(clist),
					Conversations: clist,
				})
				totalCount += len(clist)
			}
			result = append(result, peerGroup{
				PeerID:   pid,
				Count:    totalCount,
				Sessions: sgs,
			})
		}
		writeJSON(w, result)
	}
}

func apiConversationSearch(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query   string `json:"query"`
			Agent   string `json:"agent"`
			Project string `json:"project"`
			Limit   int    `json:"limit"`
		}
		if !decodeBody(w, r, &req) {
			return
		}
		if req.Limit <= 0 {
			req.Limit = 10
		}
		results, err := n.SearchConversations(r.Context(), req.Query, req.Limit, req.Agent, req.Project)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, results)
	}
}

func apiConversationActions(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		convID := mux.Vars(r)["id"]
		// Derive session ID from conversation ID
		// hook-captured: "hook-{session_id}", manual: use as-is
		sessionID := convID
		if strings.HasPrefix(convID, "hook-") {
			sessionID = convID[5:]
		}
		actions, err := n.Actions.ListBySession(sessionID, 200)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		type actionSummary struct {
			ID        string `json:"id"`
			ToolName  string `json:"tool_name"`
			ToolInput string `json:"tool_input"`
			CreatedAt string `json:"created_at"`
		}
		var items []actionSummary
		for _, a := range actions {
			input := a.ToolInput
			if len(input) > 200 {
				input = input[:200] + "..."
			}
			items = append(items, actionSummary{
				ID:        a.ID,
				ToolName:  a.ToolName,
				ToolInput: input,
				CreatedAt: a.CreatedAt.UTC().Format(time.RFC3339),
			})
		}
		writeJSON(w, items)
	}
}

func apiConversation(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		conv, err := n.Conversations.GetByID(id)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		if conv == nil {
			writeError(w, "not found", 404)
			return
		}
		if err := n.Conversations.TouchAccess(id); err != nil {
			slog.Warn("touch conversation access failed", "error", err)
		}
		writeJSON(w, conv)
	}
}

