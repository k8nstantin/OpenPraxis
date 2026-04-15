package web

import (
	"fmt"
	"net/http"
	"time"

	"openloom/internal/node"

	"github.com/gorilla/mux"
)

func apiActions(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.URL.Query().Get("session")
		if sessionID != "" {
			actions, err := n.Actions.ListBySession(sessionID, 100)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			writeJSON(w, actions)
			return
		}
		actions, err := n.Actions.ListRecent(100)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, actions)
	}
}

func apiActionsByPeer(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actions, err := n.Actions.ListRecent(500)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		type actionItem struct {
			ID        string `json:"id"`
			SessionID string `json:"session_id"`
			ToolName  string `json:"tool_name"`
			ToolInput string `json:"tool_input"`
			CreatedAt string `json:"created_at"`
		}
		type sessionGroup struct {
			Session string       `json:"session"`
			Count   int          `json:"count"`
			Actions []actionItem `json:"actions"`
		}
		type peerGroup struct {
			PeerID   string         `json:"peer_id"`
			Count    int            `json:"count"`
			Sessions []sessionGroup `json:"sessions"`
		}

		type peerData struct {
			sessionOrder []string
			sessions     map[string][]actionItem
		}
		peers := make(map[string]*peerData)
		peerOrder := []string{}

		for _, a := range actions {
			peerID := a.SourceNode
			if peerID == "" {
				peerID = n.PeerID() // fallback for old records
			}
			sid := a.SessionID
			if sid == "" {
				sid = "unknown"
			}
			input := a.ToolInput
			if len(input) > 200 {
				input = input[:200] + "..."
			}

			pd, ok := peers[peerID]
			if !ok {
				pd = &peerData{sessions: make(map[string][]actionItem)}
				peers[peerID] = pd
				peerOrder = append(peerOrder, peerID)
			}
			if _, ok := pd.sessions[sid]; !ok {
				pd.sessionOrder = append(pd.sessionOrder, sid)
			}
			pd.sessions[sid] = append(pd.sessions[sid], actionItem{
				ID:        fmt.Sprintf("%v", a.ID),
				SessionID: a.SessionID,
				ToolName:  a.ToolName,
				ToolInput: input,
				CreatedAt: a.CreatedAt.UTC().Format(time.RFC3339),
			})
		}

		var result []peerGroup
		for _, pid := range peerOrder {
			pd := peers[pid]
			var sgs []sessionGroup
			totalCount := 0
			for _, sid := range pd.sessionOrder {
				alist := pd.sessions[sid]
				sgs = append(sgs, sessionGroup{
					Session: sid,
					Count:   len(alist),
					Actions: alist,
				})
				totalCount += len(alist)
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

func apiAction(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		action, err := n.Actions.GetByID(id)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if action == nil {
			http.Error(w, "not found", 404)
			return
		}
		writeJSON(w, action)
	}
}

