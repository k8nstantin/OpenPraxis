package web

import (
	"encoding/json"
	"net/http"

	"github.com/k8nstantin/OpenPraxis/internal/node"

	"github.com/gorilla/mux"
)

func apiIdeasByPeer(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ideas, err := n.Ideas.List("", 200)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		type iItem struct {
			ID       string `json:"id"`
			Marker   string `json:"marker"`
			Title    string `json:"title"`
			Status   string `json:"status"`
			Priority string `json:"priority"`
		}
		type peerGroup struct {
			PeerID string  `json:"peer_id"`
			Count  int     `json:"count"`
			Ideas  []iItem `json:"ideas"`
		}
		peers := make(map[string][]iItem)
		peerOrder := []string{}
		for _, i := range ideas {
			pid := i.SourceNode
			if pid == "" {
				pid = n.PeerID()
			}
			if _, ok := peers[pid]; !ok {
				peerOrder = append(peerOrder, pid)
			}
			peers[pid] = append(peers[pid], iItem{ID: i.ID, Marker: i.Marker, Title: i.Title, Status: i.Status, Priority: i.Priority})
		}
		var result []peerGroup
		for _, pid := range peerOrder {
			items := peers[pid]
			result = append(result, peerGroup{PeerID: pid, Count: len(items), Ideas: items})
		}
		writeJSON(w, result)
	}
}

func apiIdeaList(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("status")
		ideas, err := n.Ideas.List(status, 50)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, ideas)
	}
}

func apiIdeaCreate(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Title       string   `json:"title"`
			Description string   `json:"description"`
			Priority    string   `json:"priority"`
			Tags        []string `json:"tags"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Title == "" {
			http.Error(w, "title is required", 400)
			return
		}
		i, err := n.Ideas.Create(req.Title, req.Description, "new", req.Priority, "dashboard", n.PeerID(), "", req.Tags)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, i)
	}
}

func apiIdeaGet(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		i, err := n.Ideas.Get(id)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if i == nil {
			http.Error(w, "not found", 404)
			return
		}
		writeJSON(w, i)
	}
}

func apiIdeaUpdate(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		existing, err := n.Ideas.Get(id)
		if err != nil || existing == nil {
			http.Error(w, "not found", 404)
			return
		}
		var req struct {
			Title       *string  `json:"title"`
			Description *string  `json:"description"`
			Status      *string  `json:"status"`
			Priority    *string  `json:"priority"`
			ProjectID   *string  `json:"project_id"`
			Tags        []string `json:"tags"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		title := existing.Title
		if req.Title != nil { title = *req.Title }
		desc := existing.Description
		if req.Description != nil { desc = *req.Description }
		status := existing.Status
		if req.Status != nil { status = *req.Status }
		priority := existing.Priority
		if req.Priority != nil { priority = *req.Priority }
		projectID := existing.ProjectID
		if req.ProjectID != nil {
			projectID, err = n.ResolveProductID(*req.ProjectID)
			if err != nil {
				writeError(w, err.Error(), 400)
				return
			}
		}
		tags := existing.Tags
		if req.Tags != nil { tags = req.Tags }
		if err := n.Ideas.Update(existing.ID, title, desc, status, priority, projectID, tags); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		updated, _ := n.Ideas.Get(existing.ID)
		writeJSON(w, updated)
	}
}

func apiIdeaDelete(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		if err := n.Ideas.Delete(id); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": "deleted"})
	}
}
