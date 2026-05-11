package web

import (
	"net/http"

	"github.com/k8nstantin/OpenPraxis/internal/entity"
	"github.com/k8nstantin/OpenPraxis/internal/node"

	"github.com/gorilla/mux"
)

// --- Recall (soft-deleted items) ---

func apiRecall(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type recallItem struct {
			ID    string `json:"id"`
			Type  string `json:"type"` // memory, <entity-type>
			Title string `json:"title"`
		}
		var items []recallItem

		// Deleted memories
		mems, _ := n.Index.ListDeleted(50)
		for _, m := range mems {
			items = append(items, recallItem{ID: m.ID, Type: "memory", Title: m.L0})
		}

		// Archived entities — single ListByTypes call across all known types so
		// dynamic/custom types are included without N+1 queries.
		if n.Entities != nil {
			typeNames := []string{entity.TypeTask}
			if n.EntityTypes != nil {
				if etypes, err := n.EntityTypes.List(r.Context()); err == nil && len(etypes) > 0 {
					typeNames = make([]string, 0, len(etypes))
					for _, et := range etypes {
						typeNames = append(typeNames, et.Name)
					}
				}
			}
			archived, _ := n.Entities.ListByTypes(typeNames, entity.StatusArchived, 50)
			// Results are already sorted DESC by created_at (store ORDER BY).
			for _, e := range archived {
				items = append(items, recallItem{ID: e.EntityUID, Type: e.Type, Title: e.Title})
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
		default:
			if n.Entities != nil {
				e, getErr := n.Entities.Get(id)
				if getErr != nil || e == nil {
					http.Error(w, "not found: "+id, http.StatusNotFound)
					return
				}
				if err = n.Entities.Update(id, e.Title, entity.StatusActive, e.Tags, "operator", "restored from archive"); err != nil {
					writeError(w, err.Error(), http.StatusInternalServerError)
					return
				}
				writeJSON(w, map[string]any{"ok": true, "id": id, "type": itemType})
				return
			}
			http.Error(w, "unknown type: "+itemType, http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": "restored"})
	}
}
