package web

import (
	"net/http"

	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/watcher"

	"github.com/gorilla/mux"
)

func apiWatcherList(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("status")
		audits, err := n.Watcher.List(status, 50)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if audits == nil {
			audits = make([]*watcher.Audit, 0)
		}
		writeJSON(w, audits)
	}
}

func apiWatcherStats(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats, err := n.Watcher.Stats()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, stats)
	}
}

func apiWatcherGet(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		audit, err := n.Watcher.Get(id)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if audit == nil {
			http.Error(w, "audit not found", 404)
			return
		}
		writeJSON(w, audit)
	}
}

func apiWatcherForTask(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskID := mux.Vars(r)["id"]
		audit, err := n.Watcher.GetByTask(taskID)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if audit == nil {
			http.Error(w, "no audit for this task", 404)
			return
		}
		writeJSON(w, audit)
	}
}

func apiWatcherTrigger(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskID := mux.Vars(r)["id"]
		t, err := n.Entities.Get(taskID)
		if err != nil || t == nil {
			http.Error(w, "task not found", 404)
			return
		}

		// Resolve manifest via relationships
		var manifestTitle, manifestContent string
		if n.Relationships != nil {
			if edges, _ := n.Relationships.ListIncoming(r.Context(), t.EntityUID, "owns"); len(edges) > 0 {
				if e, _ := n.Entities.Get(edges[0].SrcID); e != nil && e.Type == "manifest" {
					manifestTitle = e.Title
					manifestContent = e.Title
				}
			}
		}

		// Count actions
		actions, _ := n.Actions.ListByTask(taskID, 1000)
		actionCount := len(actions)

		// Cost from execution_log
		var costUSD float64
		if n.ExecutionLog != nil {
			if rows, err := n.ExecutionLog.ListByEntity(r.Context(), taskID, 1); err == nil && len(rows) > 0 {
				costUSD = rows[0].CostUSD
			}
		}

		// Subsystem torn out per operator request 2026-04-30.
		// Endpoint kept for routing compatibility but returns a stub.
		_ = manifestTitle
		_ = manifestContent
		_ = actionCount
		_ = costUSD
		_ = t
		writeJSON(w, map[string]string{
			"status": "skipped",
			"reason": "post-task audit subsystem torn out — to be redesigned",
		})
	}
}
