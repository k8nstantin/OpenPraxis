package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/k8nstantin/OpenPraxis/internal/entity"
	"github.com/k8nstantin/OpenPraxis/internal/node"

	"github.com/gorilla/mux"
)

// apiEntityTypesList handles GET /api/entity-types.
// Returns all current entity types as {"types": [...]}.
func apiEntityTypesList(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if n.EntityTypes == nil {
			http.Error(w, "entity types not available", http.StatusServiceUnavailable)
			return
		}
		types, err := n.EntityTypes.List(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if types == nil {
			types = []entity.EntityType{}
		}
		writeJSON(w, map[string]any{"types": types})
	}
}

// apiEntityTypesCreate handles POST /api/entity-types.
// Accepts {"name", "display_name", "description", "color", "icon"} and creates a new type.
func apiEntityTypesCreate(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if n.EntityTypes == nil {
			http.Error(w, "entity types not available", http.StatusServiceUnavailable)
			return
		}
		var body struct {
			Name        string `json:"name"`
			DisplayName string `json:"display_name"`
			Description string `json:"description"`
			Color       string `json:"color"`
			Icon        string `json:"icon"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		body.Name = strings.TrimSpace(body.Name)
		if body.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		if body.DisplayName == "" {
			body.DisplayName = body.Name
		}

		// Check types table first — do not allow duplicate type names.
		exists, err := n.EntityTypes.Exists(r.Context(), body.Name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if exists {
			http.Error(w, "entity type '"+body.Name+"' already exists", http.StatusConflict)
			return
		}

		et, err := n.EntityTypes.Create(r.Context(), body.Name, body.DisplayName, body.Description, body.Color, body.Icon, "operator")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, et)
	}
}

// apiEntityTypesUpdate handles PUT /api/entity-types/:name.
// Accepts {"display_name", "description", "color", "icon", "new_name"} and updates the type.
func apiEntityTypesUpdate(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if n.EntityTypes == nil {
			http.Error(w, "entity types not available", http.StatusServiceUnavailable)
			return
		}
		name := mux.Vars(r)["name"]
		if name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		var body struct {
			DisplayName string `json:"display_name"`
			Description string `json:"description"`
			Color       string `json:"color"`
			Icon        string `json:"icon"`
			NewName     string `json:"new_name"` // optional rename
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		// Verify the type exists
		exists, err := n.EntityTypes.Exists(r.Context(), name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !exists {
			http.Error(w, "entity type '"+name+"' not found", http.StatusNotFound)
			return
		}
		targetName := name
		if body.NewName != "" {
			targetName = strings.TrimSpace(body.NewName)
			// Reject rename if new_name already exists as a different type.
			if targetName != name {
				collision, cerr := n.EntityTypes.Exists(r.Context(), targetName)
				if cerr != nil {
					http.Error(w, cerr.Error(), http.StatusInternalServerError)
					return
				}
				if collision {
					http.Error(w, "entity type '"+targetName+"' already exists", http.StatusConflict)
					return
				}
			}
		}
		et, err := n.EntityTypes.Rename(r.Context(), name, targetName, body.DisplayName, body.Description, body.Color, body.Icon, "operator")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, et)
	}
}
