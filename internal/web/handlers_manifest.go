package web

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"openloom/internal/node"
	"openloom/internal/task"

	"github.com/gorilla/mux"
)

func apiManifestsByPeer(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		manifests, err := n.Manifests.List("", 200)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		type mItem struct {
			ID         string  `json:"id"`
			Marker     string  `json:"marker"`
			Title      string  `json:"title"`
			Status     string  `json:"status"`
			ProjectID  string  `json:"project_id"`
			TotalTasks int     `json:"total_tasks"`
			TotalTurns int     `json:"total_turns"`
			TotalCost  float64 `json:"total_cost"`
		}
		type peerGroup struct {
			PeerID    string  `json:"peer_id"`
			Count     int     `json:"count"`
			Manifests []mItem `json:"manifests"`
		}
		peers := make(map[string][]mItem)
		peerOrder := []string{}
		for _, m := range manifests {
			pid := m.SourceNode
			if pid == "" {
				pid = n.PeerID()
			}
			if _, ok := peers[pid]; !ok {
				peerOrder = append(peerOrder, pid)
			}
			peers[pid] = append(peers[pid], mItem{ID: m.ID, Marker: m.Marker, Title: m.Title, Status: m.Status, ProjectID: m.ProjectID, TotalTasks: m.TotalTasks, TotalTurns: m.TotalTurns, TotalCost: m.TotalCost})
		}
		var result []peerGroup
		for _, pid := range peerOrder {
			items := peers[pid]
			result = append(result, peerGroup{PeerID: pid, Count: len(items), Manifests: items})
		}
		writeJSON(w, result)
	}
}

func apiManifestList(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("status")
		manifests, err := n.Manifests.List(status, 50)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, manifests)
	}
}

func apiManifestCreate(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Title       string   `json:"title"`
			Description string   `json:"description"`
			Content     string   `json:"content"`
			Status      string   `json:"status"`
			ProjectID   string   `json:"project_id"`
			JiraRefs    []string `json:"jira_refs"`
			Tags        []string `json:"tags"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Title == "" {
			http.Error(w, "title is required", 400)
			return
		}
		projectID, err := n.ResolveProductID(req.ProjectID)
		if err != nil {
			writeError(w, err.Error(), 400)
			return
		}
		m, err := n.Manifests.Create(req.Title, req.Description, req.Content, req.Status, "dashboard", n.PeerID(), projectID, req.JiraRefs, req.Tags)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, m)
	}
}

func apiManifestGet(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		m, err := n.Manifests.Get(id)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		if m == nil {
			writeError(w, "not found", 404)
			return
		}
		writeJSON(w, m)
	}
}

func apiManifestUpdate(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		existing, err := n.Manifests.Get(id)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		if existing == nil {
			writeError(w, "not found", 404)
			return
		}
		var req struct {
			Title       *string  `json:"title"`
			Description *string  `json:"description"`
			Content     *string  `json:"content"`
			Status      *string  `json:"status"`
			ProjectID   *string  `json:"project_id"`
			JiraRefs    []string `json:"jira_refs"`
			Tags        []string `json:"tags"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		title := existing.Title
		if req.Title != nil {
			title = *req.Title
		}
		description := existing.Description
		if req.Description != nil {
			description = *req.Description
		}
		content := existing.Content
		if req.Content != nil {
			content = *req.Content
		}
		status := existing.Status
		if req.Status != nil {
			status = *req.Status
		}
		projectID := existing.ProjectID
		if req.ProjectID != nil {
			projectID, err = n.ResolveProductID(*req.ProjectID)
			if err != nil {
				writeError(w, err.Error(), 400)
				return
			}
		}
		jiraRefs := existing.JiraRefs
		if req.JiraRefs != nil {
			jiraRefs = req.JiraRefs
		}
		tags := existing.Tags
		if req.Tags != nil {
			tags = req.Tags
		}
		// Validate archive cascade: all tasks must be terminal first
		if status == "archive" && existing.Status != "archive" {
			if err := n.ValidateArchiveManifest(existing.ID); err != nil {
				http.Error(w, err.Error(), 409)
				return
			}
		}
		if err := n.Manifests.Update(existing.ID, title, description, content, status, projectID, jiraRefs, tags); err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		updated, _ := n.Manifests.Get(existing.ID)
		writeJSON(w, updated)
	}
}

func apiManifestDelete(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		if err := n.Manifests.Delete(id); err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": "deleted"})
	}
}

func apiManifestTasks(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		manifestID := mux.Vars(r)["id"]
		tasks, err := n.Tasks.ListByManifest(manifestID, 50)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		type taskWithMetrics struct {
			*task.Task
			Turns  int     `json:"turns"`
			Cost   float64 `json:"cost"`
			Reason string  `json:"reason"`
		}
		result := make([]taskWithMetrics, len(tasks))
		for i, t := range tasks {
			turns, cost, reason := parseTaskResultMetrics(t.LastOutput)
			result[i] = taskWithMetrics{Task: t, Turns: turns, Cost: cost, Reason: reason}
		}
		writeJSON(w, result)
	}
}

func apiManifestSearch(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Query == "" {
			http.Error(w, "query is required", 400)
			return
		}
		results, err := n.Manifests.Search(req.Query, 20)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, results)
	}
}

func apiManifestIdeas(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		refs, err := n.Manifests.IdeasForManifest(id)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, refs)
	}
}

func apiIdeasManifests(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		refs, err := n.Manifests.ManifestsForIdea(id)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, refs)
	}
}

func apiLink(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IdeaID     string `json:"idea_id"`
			ManifestID string `json:"manifest_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", 400)
			return
		}
		idea, _ := n.Ideas.Get(req.IdeaID)
		if idea == nil {
			http.Error(w, "idea not found", 404)
			return
		}
		m, _ := n.Manifests.Get(req.ManifestID)
		if m == nil {
			http.Error(w, "manifest not found", 404)
			return
		}
		if err := n.Manifests.LinkIdeaToManifest(idea.ID, m.ID); err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": "linked"})
	}
}

func apiUnlink(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IdeaID     string `json:"idea_id"`
			ManifestID string `json:"manifest_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", 400)
			return
		}
		if err := n.Manifests.UnlinkIdeaFromManifest(req.IdeaID, req.ManifestID); err != nil {
			slog.Warn("unlink idea from manifest failed", "error", err)
		}
		writeJSON(w, map[string]string{"status": "unlinked"})
	}
}
