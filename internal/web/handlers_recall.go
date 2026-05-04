package web

import (
	"net/http"

	"github.com/k8nstantin/OpenPraxis/internal/node"

	"github.com/gorilla/mux"
)

// --- Recall (soft-deleted items) ---

func apiRecall(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type recallItem struct {
			ID    string `json:"id"`
			Type  string `json:"type"` // memory, task
			Title string `json:"title"`
		}
		var items []recallItem

		// Deleted memories
		mems, _ := n.Index.ListDeleted(50)
		for _, m := range mems {
			items = append(items, recallItem{ID: m.ID, Type: "memory", Title: m.L0})
		}

		// Deleted tasks
		tasks, _ := n.Tasks.ListDeleted(50)
		for _, t := range tasks {
			items = append(items, recallItem{ID: t.ID, Type: "task", Title: t.Title})
		}

		writeJSON(w, items)
	}
}

func apiRestore(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		itemType := mux.Vars(r)["type"]
		id := mux.Vars(r)["id"]
		var err error
		switch itemType {
		case "memory":
			err = n.Index.Restore(id)
		case "task":
			err = n.Tasks.Restore(id)
		default:
			http.Error(w, "unknown type: "+itemType, 400)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": "restored"})
	}
}
