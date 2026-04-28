package web

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
	"github.com/k8nstantin/OpenPraxis/internal/manifest"
	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/task"

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
			ID              string   `json:"id"`
			Marker          string   `json:"marker"`
			Title           string   `json:"title"`
			Status          string   `json:"status"`
			ProjectID       string   `json:"project_id"`
			DependsOn       string   `json:"depends_on"`
			DependsOnTitles []string `json:"depends_on_titles"`
			TotalTasks      int      `json:"total_tasks"`
			TotalTurns      int      `json:"total_turns"`
			TotalCost       float64  `json:"total_cost"`
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
			peers[pid] = append(peers[pid], mItem{ID: m.ID, Marker: m.Marker, Title: m.Title, Status: m.Status, ProjectID: m.ProjectID, DependsOn: m.DependsOn, DependsOnTitles: n.ResolveDependsOnTitles(m.DependsOn), TotalTasks: m.TotalTasks, TotalTurns: m.TotalTurns, TotalCost: m.TotalCost})
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
		writeJSON(w, enrichManifests(n, manifests))
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
			DependsOn   string   `json:"depends_on"`
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
		dependsOn, err := n.ResolveManifestDependsOn(req.DependsOn, "")
		if err != nil {
			writeError(w, err.Error(), 400)
			return
		}
		m, err := n.Manifests.Create(req.Title, req.Description, req.Content, req.Status, "dashboard", n.PeerID(), projectID, dependsOn, req.JiraRefs, req.Tags)
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, enrichManifest(n, m))
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
		// Pull turns + cost + actions + tokens from this manifest's
		// task_runs. Mirrors the apiProductGet enrichment so the Main
		// tab gauges have something to render. List endpoints stay lean
		// (cheaper batched EnrichWithCosts only).
		n.Manifests.EnrichRecursiveCosts(m)
		// Single GET enriches with rendered HTML for the body fields so
		// the dashboard renders formatted markdown. List endpoints stay
		// lean (no HTML render per row).
		writeJSON(w, EnrichWithHTML(enrichManifest(n, m), map[string]string{
			"content":     m.Content,
			"description": m.Description,
		}))
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
			DependsOn   *string  `json:"depends_on"`
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
		dependsOn := existing.DependsOn
		if req.DependsOn != nil {
			dependsOn, err = n.ResolveManifestDependsOn(*req.DependsOn, existing.ID)
			if err != nil {
				writeError(w, err.Error(), 400)
				return
			}
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
		// Record append-only description_revision on content changes, before
		// the denormalised UPDATE (DV/M2). Manifest's spec text lives in
		// Content; Description is the short summary line and is not tracked.
		if req.Content != nil {
			if _, err := n.RecordDescriptionChange(r.Context(), comments.TargetManifest, existing.ID, content, ""); err != nil {
				writeError(w, err.Error(), 500)
				return
			}
		}
		if err := n.Manifests.Update(existing.ID, title, description, content, status, projectID, dependsOn, jiraRefs, tags); err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		updated, _ := n.Manifests.Get(existing.ID)
		writeJSON(w, enrichManifest(n, updated))
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
		writeJSON(w, enrichManifests(n, results))
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

// enrichedManifest wraps a Manifest with resolved DependsOnTitles for API responses.
type enrichedManifest struct {
	*manifest.Manifest
	DependsOnTitles []string `json:"depends_on_titles"`
}

func enrichManifest(n *node.Node, m *manifest.Manifest) enrichedManifest {
	return enrichedManifest{
		Manifest:        m,
		DependsOnTitles: n.ResolveDependsOnTitles(m.DependsOn),
	}
}

func enrichManifests(n *node.Node, manifests []*manifest.Manifest) []enrichedManifest {
	result := make([]enrichedManifest, len(manifests))
	for i, m := range manifests {
		result[i] = enrichManifest(n, m)
	}
	return result
}

// resolveManifestID accepts either a 12-char marker or a full UUID and
// returns the full UUID via Manifests.Get (which already handles prefix
// matching). Returns "" + a 404-worthy error message when missing.
func resolveManifestID(n *node.Node, idOrMarker string) (string, string) {
	m, _ := n.Manifests.Get(idOrMarker)
	if m == nil {
		return "", "manifest not found: " + idOrMarker
	}
	return m.ID, ""
}

// apiManifestDepList — GET /api/manifests/{id}/dependencies?direction=out|in|both
//
// Default direction is "out". Response body keys are omitted when the
// requested direction filters them out, so the UI can dispatch on which
// key is present without reading query params.
func apiManifestDepList(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, msg := resolveManifestID(n, mux.Vars(r)["id"])
		if msg != "" {
			writeError(w, msg, http.StatusNotFound)
			return
		}
		direction := r.URL.Query().Get("direction")
		if direction == "" {
			direction = "out"
		}
		out := map[string]any{}
		switch direction {
		case "out", "both":
			deps, err := n.Manifests.ListDeps(r.Context(), id)
			if err != nil {
				writeError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			out["deps"] = deps
			if direction != "both" {
				break
			}
			fallthrough
		case "in":
			dependents, err := n.Manifests.ListDependents(r.Context(), id)
			if err != nil {
				writeError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			out["dependents"] = dependents
		default:
			writeError(w, "direction must be out, in, or both", http.StatusBadRequest)
			return
		}
		writeJSON(w, out)
	}
}

// apiManifestDepAdd — POST /api/manifests/{id}/dependencies
//
// Body: {"depends_on_id": "..."}.
// 201 on success with the denormalized dep row echoed back.
// 400 for self-loop / missing body fields.
// 409 for cycle detection — the body carries the specific rejected pair.
// 404 when either manifest doesn't exist.
func apiManifestDepAdd(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		srcID, msg := resolveManifestID(n, mux.Vars(r)["id"])
		if msg != "" {
			writeError(w, msg, http.StatusNotFound)
			return
		}
		var body struct {
			DependsOnID string `json:"depends_on_id"`
			CreatedBy   string `json:"created_by"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		if body.DependsOnID == "" {
			writeError(w, "depends_on_id is required", http.StatusBadRequest)
			return
		}
		dstID, msg := resolveManifestID(n, body.DependsOnID)
		if msg != "" {
			writeError(w, msg, http.StatusNotFound)
			return
		}
		createdBy := body.CreatedBy
		if createdBy == "" {
			createdBy = "http-api"
		}

		if err := n.Manifests.AddDep(r.Context(), srcID, dstID, createdBy); err != nil {
			// Map domain errors to HTTP status so UI can branch.
			switch {
			case errors.Is(err, manifest.ErrCycle):
				writeError(w, err.Error(), http.StatusConflict)
			case errors.Is(err, manifest.ErrSelfLoop):
				writeError(w, err.Error(), http.StatusBadRequest)
			default:
				writeError(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		// Echo back the created edge so the UI doesn't need a refetch.
		deps, _ := n.Manifests.ListDeps(r.Context(), srcID)
		for _, d := range deps {
			if d.ID == dstID {
				w.WriteHeader(http.StatusCreated)
				writeJSON(w, d)
				return
			}
		}
		// Fallback — write 201 with a minimal body if the list lookup
		// didn't find it (shouldn't happen; defensive).
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, map[string]string{"manifest_id": srcID, "depends_on_id": dstID})
	}
}

// apiManifestDepRemove — DELETE /api/manifests/{id}/dependencies/{depId}
//
// Idempotent: 204 whether or not the edge existed. The RemoveDep path
// already fires the rehab handler (see #79) that flips any newly-
// unblocked waiting tasks to 'pending', so callers don't need a
// follow-up action.
func apiManifestDepRemove(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		srcID, msg := resolveManifestID(n, mux.Vars(r)["id"])
		if msg != "" {
			writeError(w, msg, http.StatusNotFound)
			return
		}
		dstID, msg := resolveManifestID(n, mux.Vars(r)["depId"])
		if msg != "" {
			writeError(w, msg, http.StatusNotFound)
			return
		}
		if err := n.Manifests.RemoveDep(r.Context(), srcID, dstID); err != nil {
			writeError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

