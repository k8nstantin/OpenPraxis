package web

import (
	"net/http"

	"openpraxis/internal/node"

	"github.com/gorilla/mux"
)

// --- Recall (soft-deleted items) ---

func apiRecall(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type recallItem struct {
			ID     string `json:"id"`
			Marker string `json:"marker"`
			Type   string `json:"type"` // memory, manifest, idea, task
			Title  string `json:"title"`
		}
		var items []recallItem

		// Deleted memories
		mems, _ := n.Index.ListDeleted(50)
		for _, m := range mems {
			marker := ""
			if len(m.ID) >= 12 {
				marker = m.ID[:12]
			}
			items = append(items, recallItem{ID: m.ID, Marker: marker, Type: "memory", Title: m.L0})
		}

		// Deleted manifests
		manifests, _ := n.Manifests.ListDeleted(50)
		for _, m := range manifests {
			items = append(items, recallItem{ID: m.ID, Marker: m.Marker, Type: "manifest", Title: m.Title})
		}

		// Deleted ideas
		ideas, _ := n.Ideas.ListDeleted(50)
		for _, i := range ideas {
			items = append(items, recallItem{ID: i.ID, Marker: i.Marker, Type: "idea", Title: i.Title})
		}

		// Deleted tasks
		tasks, _ := n.Tasks.ListDeleted(50)
		for _, t := range tasks {
			items = append(items, recallItem{ID: t.ID, Marker: t.Marker, Type: "task", Title: t.Title})
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
		case "manifest":
			err = n.Manifests.Restore(id)
		case "idea":
			err = n.Ideas.Restore(id)
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
