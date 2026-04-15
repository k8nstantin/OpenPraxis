package web

import (
	"encoding/json"
	"net/http"

	"openloom/internal/node"
	"openloom/internal/setup"

	"github.com/gorilla/mux"
)

func apiProfileGet(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{
			"uuid":         n.Config.Node.UUID,
			"display_name": n.Config.Node.DisplayName,
			"email":        n.Config.Node.Email,
			"avatar":       n.Config.Node.Avatar,
		})
	}
}

func apiProfileUpdate(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			DisplayName string `json:"display_name"`
			Email       string `json:"email"`
			Avatar      string `json:"avatar"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", 400)
			return
		}
		n.Config.Node.DisplayName = req.DisplayName
		n.Config.Node.Email = req.Email
		n.Config.Node.Avatar = req.Avatar
		if err := n.Config.Save(); err != nil {
			http.Error(w, "save config: "+err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func apiSettingsAgents() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agents := setup.DetectAgents()
		writeJSON(w, agents)
	}
}

func apiAgentConnect() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		agents := setup.DetectAgents()
		for _, a := range agents {
			if a.ID == id {
				if err := setup.ConnectAgent(a); err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				writeJSON(w, map[string]string{"status": "connected", "agent": a.Name})
				return
			}
		}
		http.Error(w, "agent not found", 404)
	}
}

func apiAgentDisconnect() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		agents := setup.DetectAgents()
		for _, a := range agents {
			if a.ID == id {
				if err := setup.DisconnectAgent(a); err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				writeJSON(w, map[string]string{"status": "disconnected", "agent": a.Name})
				return
			}
		}
		http.Error(w, "agent not found", 404)
	}
}
