package web

import (
	"encoding/json"
	"net/http"

	"github.com/k8nstantin/OpenPraxis/internal/entity"
	"github.com/k8nstantin/OpenPraxis/internal/node"
)

// apiEntityTypesList handles GET /api/entity-types.
// Returns all current entity types as {"types": [...]}.
func apiEntityTypesList(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
