package web

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
	"github.com/k8nstantin/OpenPraxis/internal/entity"
	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/relationships"

	"github.com/gorilla/mux"
)

func apiEntityList(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		entityType := q.Get("type")
		status := q.Get("status")
		limit := 50
		if l := q.Get("limit"); l != "" {
			if v, err := strconv.Atoi(l); err == nil && v > 0 {
				limit = v
			}
		}
		entities, err := n.Entities.List(entityType, status, limit)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if entities == nil {
			entities = []*entity.Entity{}
		}
		writeJSON(w, entities)
	}
}

func apiEntityCreate(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Type   string   `json:"type"`
			Title  string   `json:"title"`
			Status string   `json:"status"`
			Tags   []string `json:"tags"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Type == "" || req.Title == "" {
			http.Error(w, "type and title are required", 400)
			return
		}
		e, err := n.Entities.Create(req.Type, req.Title, req.Status, req.Tags, "http-api", "")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, e)
	}
}

func apiEntityGet(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		e, err := n.Entities.Get(id)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if e == nil {
			http.Error(w, "not found", 404)
			return
		}
		writeJSON(w, e)
	}
}

func apiEntityUpdate(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		existing, err := n.Entities.Get(id)
		if err != nil || existing == nil {
			http.Error(w, "not found", 404)
			return
		}
		var req struct {
			Title       *string  `json:"title"`
			Status      *string  `json:"status"`
			Tags        []string `json:"tags"`
			Description *string  `json:"description"` // product/task description body
			Content     *string  `json:"content"`     // manifest spec body
			ProjectID   *string  `json:"project_id"`  // manifest → parent product link
			ManifestID  *string  `json:"manifest_id"` // task → parent manifest link
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		title := existing.Title
		if req.Title != nil {
			title = *req.Title
		}
		status := existing.Status
		if req.Status != nil {
			status = *req.Status
		}
		tags := existing.Tags
		if req.Tags != nil {
			tags = req.Tags
		}

		// Record a description revision when description/content body changes.
		descBody := ""
		if req.Description != nil {
			descBody = *req.Description
		} else if req.Content != nil {
			descBody = *req.Content
		}
		if descBody != "" {
			if _, err := n.RecordDescriptionChange(r.Context(), comments.TargetEntity, existing.EntityUID, descBody, ""); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
		}

		// Handle project_id (manifest → product ownership edge).
		if req.ProjectID != nil && n.Relationships != nil {
			newProjectID := *req.ProjectID
			// Remove old owns edge(s) pointing to this manifest from any product.
			incoming, _ := n.Relationships.ListIncoming(r.Context(), existing.EntityUID, relationships.EdgeOwns)
			for _, e := range incoming {
				if e.SrcKind == relationships.KindProduct {
					_ = n.Relationships.Remove(r.Context(), e.SrcID, existing.EntityUID, relationships.EdgeOwns, "http-api", "re-parent")
				}
			}
			// Add new owns edge if non-empty project_id.
			if newProjectID != "" {
				_ = n.Relationships.Create(r.Context(), relationships.Edge{
					SrcKind:   relationships.KindProduct,
					SrcID:     newProjectID,
					DstKind:   relationships.KindManifest,
					DstID:     existing.EntityUID,
					Kind:      relationships.EdgeOwns,
					CreatedBy: "http-api",
				})
			}
		}

		// Handle manifest_id (task → manifest ownership edge).
		if req.ManifestID != nil && n.Relationships != nil {
			newManifestID := *req.ManifestID
			incoming, _ := n.Relationships.ListIncoming(r.Context(), existing.EntityUID, relationships.EdgeOwns)
			for _, e := range incoming {
				if e.SrcKind == relationships.KindManifest {
					_ = n.Relationships.Remove(r.Context(), e.SrcID, existing.EntityUID, relationships.EdgeOwns, "http-api", "re-parent")
				}
			}
			if newManifestID != "" {
				_ = n.Relationships.Create(r.Context(), relationships.Edge{
					SrcKind:   relationships.KindManifest,
					SrcID:     newManifestID,
					DstKind:   relationships.KindTask,
					DstID:     existing.EntityUID,
					Kind:      relationships.EdgeOwns,
					CreatedBy: "http-api",
				})
			}
		}

		if err := n.Entities.Update(existing.EntityUID, title, status, tags, "http-api", ""); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		updated, _ := n.Entities.Get(existing.EntityUID)
		writeJSON(w, updated)
	}
}

func apiEntityHistory(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		history, err := n.Entities.History(id)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, history)
	}
}

func apiEntitySearch(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		entityType := r.URL.Query().Get("type")
		entities, err := n.Entities.Search(q, entityType, 50)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, entities)
	}
}

func apiExecutionOutput(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runUID := mux.Vars(r)["runUid"]
		chunks, err := n.ExecutionLog.ListOutput(r.Context(), runUID)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, chunks)
	}
}

func apiExecutionLog(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runUID := mux.Vars(r)["runUid"]
		rows, err := n.ExecutionLog.ListByRun(r.Context(), runUID)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, rows)
	}
}

func apiEntityExecutionLog(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		limit := 50
		if l := r.URL.Query().Get("limit"); l != "" {
			if v, err := strconv.Atoi(l); err == nil && v > 0 {
				limit = v
			}
		}
		rows, err := n.ExecutionLog.ListByEntity(r.Context(), id, limit)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, rows)
	}
}

// apiEntityCommentsList handles GET /api/entities/{id}/comments.
// Lists comments for any entity using the TargetEntity comment type.
func apiEntityCommentsList(n *node.Node) http.HandlerFunc {
	return listComments(n.Comments, comments.TargetEntity, nodeTargetResolver(n))
}

// apiEntityCommentsAdd handles POST /api/entities/{id}/comments.
// Adds a comment to any entity using the TargetEntity comment type.
func apiEntityCommentsAdd(n *node.Node) http.HandlerFunc {
	return addComment(n.Comments, comments.TargetEntity, nodeTargetResolver(n))
}

// depRow is the wire shape for /api/{products|manifests}/{id}/dependencies.
type depRow struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// apiEntityDependencies handles GET /api/products/{id}/dependencies
// and GET /api/manifests/{id}/dependencies.
// Products: returns downstream sub-products (products that depend on this one).
// Manifests: returns upstream manifests this one depends on (direction=out default).
func apiEntityDependencies(n *node.Node, srcKind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		e, err := n.Entities.Get(id)
		if err != nil || e == nil {
			http.Error(w, "not found", 404)
			return
		}

		var rows []depRow
		if srcKind == relationships.KindProduct {
			// Products: list other products that depend on this one (downstream/sub-products).
			// These are edges where this product is the dependency target (dst).
			incoming, _ := n.Relationships.ListIncoming(r.Context(), e.EntityUID, relationships.EdgeDependsOn)
			for _, edge := range incoming {
				if edge.SrcKind != relationships.KindProduct {
					continue
				}
				dep, depErr := n.Entities.Get(edge.SrcID)
				if depErr != nil || dep == nil {
					continue
				}
				rows = append(rows, depRow{ID: dep.EntityUID, Title: dep.Title, Status: dep.Status})
			}
		} else {
			// Manifests: list upstream manifests this one depends on (outgoing deps).
			direction := r.URL.Query().Get("direction")
			if direction == "" {
				direction = "out"
			}
			if direction == "out" {
				outgoing, _ := n.Relationships.ListOutgoing(r.Context(), e.EntityUID, relationships.EdgeDependsOn)
				for _, edge := range outgoing {
					if edge.DstKind != relationships.KindManifest {
						continue
					}
					dep, depErr := n.Entities.Get(edge.DstID)
					if depErr != nil || dep == nil {
						continue
					}
					rows = append(rows, depRow{ID: dep.EntityUID, Title: dep.Title, Status: dep.Status})
				}
			} else {
				incoming, _ := n.Relationships.ListIncoming(r.Context(), e.EntityUID, relationships.EdgeDependsOn)
				for _, edge := range incoming {
					dep, depErr := n.Entities.Get(edge.SrcID)
					if depErr != nil || dep == nil {
						continue
					}
					rows = append(rows, depRow{ID: dep.EntityUID, Title: dep.Title, Status: dep.Status})
				}
			}
		}
		if rows == nil {
			rows = []depRow{}
		}
		writeJSON(w, rows)
	}
}

// apiEntityAddDependency handles POST /api/products/{id}/dependencies
// and POST /api/manifests/{id}/dependencies.
// Body: { "depends_on_id": "<entity_uid>" }
// Products: adds this product as a dependency of the sub-product (X depends on THIS).
// Manifests: adds an upstream dep (THIS depends on X).
func apiEntityAddDependency(n *node.Node, srcKind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		e, err := n.Entities.Get(id)
		if err != nil || e == nil {
			http.Error(w, "not found", 404)
			return
		}
		var req struct {
			DependsOnID string `json:"depends_on_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DependsOnID == "" {
			http.Error(w, "depends_on_id required", 400)
			return
		}
		dep, err := n.Entities.Get(req.DependsOnID)
		if err != nil || dep == nil {
			http.Error(w, "dependency entity not found", 404)
			return
		}

		var edge relationships.Edge
		if srcKind == relationships.KindProduct {
			// Sub-product depends on this product: edge is X(src) → this(dst)
			edge = relationships.Edge{
				SrcKind:   relationships.KindProduct,
				SrcID:     e.EntityUID,
				DstKind:   relationships.KindProduct,
				DstID:     dep.EntityUID,
				Kind:      relationships.EdgeDependsOn,
				CreatedBy: "http-api",
			}
		} else {
			// This manifest depends on upstream manifest
			edge = relationships.Edge{
				SrcKind:   relationships.KindManifest,
				SrcID:     e.EntityUID,
				DstKind:   relationships.KindManifest,
				DstID:     dep.EntityUID,
				Kind:      relationships.EdgeDependsOn,
				CreatedBy: "http-api",
			}
		}
		if createErr := n.Relationships.Create(r.Context(), edge); createErr != nil {
			// 409 on duplicate — already linked
			w.WriteHeader(http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// apiEntityRemoveDependency handles DELETE /api/products/{id}/dependencies/{dep_id}
// and DELETE /api/manifests/{id}/dependencies/{dep_id}.
func apiEntityRemoveDependency(n *node.Node, srcKind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["id"]
		depID := vars["dep_id"]
		e, err := n.Entities.Get(id)
		if err != nil || e == nil {
			http.Error(w, "not found", 404)
			return
		}
		dep, err := n.Entities.Get(depID)
		if err != nil || dep == nil {
			http.Error(w, "dependency entity not found", 404)
			return
		}

		var removeErr error
		if srcKind == relationships.KindProduct {
			removeErr = n.Relationships.Remove(r.Context(), e.EntityUID, dep.EntityUID, relationships.EdgeDependsOn, "http-api", "removed")
		} else {
			removeErr = n.Relationships.Remove(r.Context(), e.EntityUID, dep.EntityUID, relationships.EdgeDependsOn, "http-api", "removed")
		}
		if removeErr != nil {
			// Not found is acceptable (idempotent delete)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// apiRelationshipCreate handles POST /api/relationships.
// Creates a relationship edge between two entities.
func apiRelationshipCreate(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SrcID    string `json:"src_id"`
			SrcKind  string `json:"src_kind"`
			DstID    string `json:"dst_id"`
			DstKind  string `json:"dst_kind"`
			Kind     string `json:"kind"`
			Metadata string `json:"metadata"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body", 400)
			return
		}
		edge := relationships.Edge{
			SrcKind: body.SrcKind, SrcID: body.SrcID,
			DstKind: body.DstKind, DstID: body.DstID,
			Kind: body.Kind, Metadata: body.Metadata,
			CreatedBy: "http-api",
		}
		if err := n.Relationships.Create(r.Context(), edge); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(201)
		writeJSON(w, edge)
	}
}

// apiRelationshipDelete handles DELETE /api/relationships?src_id=&dst_id=&kind=
// Removes a relationship edge between two entities.
func apiRelationshipDelete(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		srcID := r.URL.Query().Get("src_id")
		dstID := r.URL.Query().Get("dst_id")
		kind := r.URL.Query().Get("kind")
		if srcID == "" || dstID == "" || kind == "" {
			http.Error(w, "src_id, dst_id, and kind are required", 400)
			return
		}
		if err := n.Relationships.Remove(r.Context(), srcID, dstID, kind, "http-api", ""); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// hierarchyNode is the recursive shape returned by /api/products/{id}/hierarchy.
type hierarchyNode struct {
	ID          string           `json:"id"`
	Title       string           `json:"title"`
	Type        string           `json:"type"`
	Status      string           `json:"status"`
	SubProducts []*hierarchyNode `json:"sub_products,omitempty"`
	Children    []*hierarchyNode `json:"children,omitempty"`
}

// apiProductHierarchy handles GET /api/products/{id}/hierarchy.
// Returns a recursive tree of sub-products and manifests rooted at this product.
func apiProductHierarchy(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		e, err := n.Entities.Get(id)
		if err != nil || e == nil {
			http.Error(w, "not found", 404)
			return
		}
		root := buildHierarchy(r, n, e.EntityUID, relationships.KindProduct, 0)
		writeJSON(w, root)
	}
}

// buildHierarchy recursively constructs the hierarchy tree up to maxDepth.
func buildHierarchy(r *http.Request, n *node.Node, entityID, kind string, depth int) *hierarchyNode {
	if depth > 10 {
		return nil
	}
	e, err := n.Entities.Get(entityID)
	if err != nil || e == nil {
		return nil
	}
	node := &hierarchyNode{
		ID:     e.EntityUID,
		Title:  e.Title,
		Type:   e.Type,
		Status: e.Status,
	}
	if kind == relationships.KindProduct {
		// Sub-products: products that depend on this one
		incoming, _ := n.Relationships.ListIncoming(r.Context(), entityID, relationships.EdgeDependsOn)
		for _, edge := range incoming {
			if edge.SrcKind != relationships.KindProduct {
				continue
			}
			sub := buildHierarchy(r, n, edge.SrcID, relationships.KindProduct, depth+1)
			if sub != nil {
				node.SubProducts = append(node.SubProducts, sub)
			}
		}
		// Owned manifests: products own manifests via EdgeOwns
		owned, _ := n.Relationships.ListOutgoing(r.Context(), entityID, relationships.EdgeOwns)
		for _, edge := range owned {
			if edge.DstKind != relationships.KindManifest {
				continue
			}
			mani := buildHierarchy(r, n, edge.DstID, relationships.KindManifest, depth+1)
			if mani != nil {
				node.Children = append(node.Children, mani)
			}
		}
	}
	return node
}
