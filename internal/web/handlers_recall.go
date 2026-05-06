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

		// Archived tasks from entities
		if n.Entities != nil {
			archived, _ := n.Entities.List("task", "archived", 50)
			for _, e := range archived {
				items = append(items, recallItem{ID: e.EntityUID, Type: "task", Title: e.Title})
			}
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
			if n.Entities != nil {
				_, err = n.Entities.Get(id)
				if err == nil {
					err = n.Entities.Update(id, "", "active", nil, "recall", "restored from archived")
				}
			}
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
