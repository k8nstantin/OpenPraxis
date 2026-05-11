package web

import (
	"net/http"
	"sort"

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

		// Archived entities — fetch all types when the EntityTypes registry is
		// available so dynamic/custom types are included. Fall back to tasks only
		// when the registry is nil (pre-migration databases).
		if n.Entities != nil {
			var archived []*entity.Entity
			if n.EntityTypes != nil {
				etypes, err := n.EntityTypes.List(r.Context())
				if err == nil {
					for _, et := range etypes {
						items_, _ := n.Entities.List(et.Name, entity.StatusArchived, 50)
						archived = append(archived, items_...)
					}
				} else {
					archived, _ = n.Entities.List(entity.TypeTask, entity.StatusArchived, 50)
				}
			} else {
				archived, _ = n.Entities.List(entity.TypeTask, entity.StatusArchived, 50)
			}
			// Sort combined results by created_at descending and cap at 50.
			sort.Slice(archived, func(i, j int) bool {
				return archived[i].CreatedAt > archived[j].CreatedAt
			})
			if len(archived) > 50 {
				archived = archived[:50]
			}
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
