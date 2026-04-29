package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/product"

	"github.com/gorilla/mux"
)

// pItemMenu is the products-by-peer menu row. SubProducts is filled when
// this product is detected as an umbrella (no manifests + has out-deps).
type pItemMenu struct {
	ID             string      `json:"id"`
	Marker         string      `json:"marker"`
	Title          string      `json:"title"`
	Status         string      `json:"status"`
	TotalManifests int         `json:"total_manifests"`
	TotalTasks     int         `json:"total_tasks"`
	TotalTurns     int         `json:"total_turns"`
	TotalCost      float64     `json:"total_cost"`
	CreatedAt      string      `json:"created_at"`
	UpdatedAt      string      `json:"updated_at"`
	SubProducts    []pItemMenu `json:"sub_products,omitempty"`
}

func apiProductsByPeer(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		products, err := n.Products.List("", 200)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		type peerGroup struct {
			PeerID   string      `json:"peer_id"`
			Count    int         `json:"count"`
			Products []pItemMenu `json:"products"`
		}

		// An "umbrella" is a product with zero manifests of its own that has
		// outgoing product_dep edges. Its dep targets render nested under it
		// in the menu and are filtered from the top-level list. Self-
		// documenting: anyone makes an umbrella by creating an empty product
		// + wiring sub-products via product_dep_add. First umbrella claiming
		// a sub wins (deterministic by product list order).
		ctx := r.Context()
		parentOf := map[string]string{}
		umbrellaSubs := map[string][]string{}
		for _, p := range products {
			if p.TotalManifests > 0 {
				continue
			}
			deps, _ := n.Products.ListDeps(ctx, p.ID)
			if len(deps) == 0 {
				continue
			}
			for _, d := range deps {
				if _, claimed := parentOf[d.ID]; !claimed {
					parentOf[d.ID] = p.ID
				}
				umbrellaSubs[p.ID] = append(umbrellaSubs[p.ID], d.ID)
			}
		}

		itemFor := func(p *product.Product) pItemMenu {
			return pItemMenu{
				ID: p.ID, Marker: p.Marker, Title: p.Title, Status: p.Status,
				TotalManifests: p.TotalManifests, TotalTasks: p.TotalTasks,
				TotalTurns: p.TotalTurns, TotalCost: p.TotalCost,
				CreatedAt: p.CreatedAt.Format(time.RFC3339),
				UpdatedAt: p.UpdatedAt.Format(time.RFC3339),
			}
		}
		productByID := map[string]*product.Product{}
		for _, p := range products {
			productByID[p.ID] = p
		}

		peers := make(map[string][]pItemMenu)
		peerOrder := []string{}
		for _, p := range products {
			if _, isSub := parentOf[p.ID]; isSub {
				continue
			}
			pid := p.SourceNode
			if pid == "" {
				pid = n.PeerID()
			}
			if _, ok := peers[pid]; !ok {
				peerOrder = append(peerOrder, pid)
			}
			item := itemFor(p)
			if subIDs, ok := umbrellaSubs[p.ID]; ok {
				for _, sid := range subIDs {
					if sub, ok := productByID[sid]; ok {
						item.SubProducts = append(item.SubProducts, itemFor(sub))
					}
				}
			}
			peers[pid] = append(peers[pid], item)
		}
		var result []peerGroup
		for _, pid := range peerOrder {
			items := peers[pid]
			result = append(result, peerGroup{PeerID: pid, Count: len(items), Products: items})
		}
		writeJSON(w, result)
	}
}

func apiProductList(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("status")
		// limit=0 → unbounded. The v2 list pane paginates client-side
		// (10 visible + Load more in 10-increments); a backend cap
		// silently truncates.
		products, err := n.Products.List(status, 0)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, products)
	}
}

func apiProductCreate(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Title       string   `json:"title"`
			Description string   `json:"description"`
			Status      string   `json:"status"`
			Tags        []string `json:"tags"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Title == "" {
			http.Error(w, "title is required", 400)
			return
		}
		p, err := n.Products.Create(req.Title, req.Description, req.Status, n.PeerID(), req.Tags)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, p)
	}
}

func apiProductGet(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		p, err := n.Products.Get(id)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if p == nil {
			http.Error(w, "not found", 404)
			return
		}
		// Pull turns + cost from descendant manifests → tasks → task_runs.
		// Recursive: walks sub-products via product_dependencies so an
		// umbrella whose tasks live under sub-product manifests still
		// surfaces the cumulative cost on its dashboard. Without this
		// the single-product GET returned struct zero values regardless
		// of what work had actually run.
		n.Products.EnrichRecursiveCosts(p)
		writeJSON(w, EnrichWithHTML(p, map[string]string{"description": p.Description}))
	}
}

func apiProductUpdate(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		existing, err := n.Products.Get(id)
		if err != nil || existing == nil {
			http.Error(w, "not found", 404)
			return
		}
		var req struct {
			Title       *string  `json:"title"`
			Description *string  `json:"description"`
			Status      *string  `json:"status"`
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
		status := existing.Status
		if req.Status != nil {
			status = *req.Status
		}
		tags := existing.Tags
		if req.Tags != nil {
			tags = req.Tags
		}
		// Validate archive cascade: all manifests must be archived first
		if status == "archive" && existing.Status != "archive" {
			if err := n.ValidateArchiveProduct(existing.ID); err != nil {
				http.Error(w, err.Error(), 409)
				return
			}
		}
		// Record an append-only description_revision comment BEFORE the
		// denormalised UPDATE so edit history is preserved. No-op when
		// description unchanged (DV/M2).
		if req.Description != nil {
			if _, err := n.RecordDescriptionChange(r.Context(), comments.TargetProduct, existing.ID, description, ""); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
		}
		if err := n.Products.Update(existing.ID, title, description, status, tags); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		updated, _ := n.Products.Get(existing.ID)
		writeJSON(w, updated)
	}
}

func apiProductDelete(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		if err := n.Products.Delete(id); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": "deleted"})
	}
}

func apiProductManifests(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		p, err := n.Products.Get(id)
		if err != nil || p == nil {
			http.Error(w, "product not found", 404)
			return
		}
		manifests, err := n.Manifests.ListByProject(p.ID, 50)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, manifests)
	}
}

type hierarchyTaskNode struct {
	ID        string         `json:"id"`
	Marker    string         `json:"marker"`
	Title     string         `json:"title"`
	Type      string         `json:"type"`
	Status    string         `json:"status"`
	DependsOn string         `json:"depends_on"`
	Meta      map[string]any `json:"meta"`
}

type hierarchyManifestNode struct {
	ID              string              `json:"id"`
	Marker          string              `json:"marker"`
	Title           string              `json:"title"`
	Type            string              `json:"type"`
	Status          string              `json:"status"`
	DependsOn       string              `json:"depends_on"`
	DependsOnTitles []string            `json:"depends_on_titles"`
	Meta            map[string]any      `json:"meta"`
	Children        []hierarchyTaskNode `json:"children"`
}

type hierarchyProductNode struct {
	ID          string                  `json:"id"`
	Marker      string                  `json:"marker"`
	Title       string                  `json:"title"`
	Type        string                  `json:"type"`
	Status      string                  `json:"status"`
	Meta        map[string]any          `json:"meta"`
	Children    []hierarchyManifestNode `json:"children"`
	SubProducts []hierarchyProductNode  `json:"sub_products,omitempty"`
}

// buildHierarchyProduct returns a productNode with its manifest children.
// When depth > 0 it also recurses into product→product dep edges, exposing
// sub-products as SubProducts so the umbrella product (which owns no manifests
// directly) still renders its full subsystem tree in the DAG view. Cycles
// are already rejected at AddDep time; depth cap is defense in depth.
func buildHierarchyProduct(ctx context.Context, n *node.Node, p *product.Product, depth int, seen map[string]bool) hierarchyProductNode {
	if seen[p.ID] {
		return hierarchyProductNode{ID: p.ID, Marker: p.Marker, Title: p.Title, Type: "product", Status: p.Status}
	}
	seen[p.ID] = true

	manifests, _ := n.Manifests.ListByProject(p.ID, 200)
	mNodes := make([]hierarchyManifestNode, 0, len(manifests))
	totalTasks := 0
	for _, m := range manifests {
		tasks, _ := n.Tasks.ListByManifest(m.ID, 200)
		tNodes := make([]hierarchyTaskNode, 0, len(tasks))
		for _, t := range tasks {
			tNodes = append(tNodes, hierarchyTaskNode{
				ID: t.ID, Marker: t.Marker, Title: t.Title,
				Type: "task", Status: t.Status, DependsOn: t.DependsOn,
				Meta: map[string]any{
					"cost_usd":  t.TotalCost,
					"turns":     t.TotalTurns,
					"run_count": t.RunCount,
				},
			})
		}
		totalTasks += len(tasks)
		mNodes = append(mNodes, hierarchyManifestNode{
			ID: m.ID, Marker: m.Marker, Title: m.Title,
			Type: "manifest", Status: m.Status,
			DependsOn: m.DependsOn, DependsOnTitles: n.ResolveDependsOnTitles(m.DependsOn),
			Meta: map[string]any{
				"total_cost":  m.TotalCost,
				"total_tasks": len(tasks),
				"total_turns": m.TotalTurns,
			},
			Children: tNodes,
		})
	}

	var subNodes []hierarchyProductNode
	if depth > 0 {
		deps, _ := n.Products.ListDeps(ctx, p.ID)
		for _, d := range deps {
			sub, err := n.Products.Get(d.ID)
			if err != nil || sub == nil {
				continue
			}
			subNodes = append(subNodes, buildHierarchyProduct(ctx, n, sub, depth-1, seen))
		}
	}

	return hierarchyProductNode{
		ID: p.ID, Marker: p.Marker, Title: p.Title,
		Type: "product", Status: p.Status,
		Meta: map[string]any{
			"total_cost":      p.TotalCost,
			"total_manifests": len(manifests),
			"total_tasks":     totalTasks,
		},
		Children:    mNodes,
		SubProducts: subNodes,
	}
}

func apiProductHierarchy(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		p, err := n.Products.Get(id)
		if err != nil || p == nil {
			http.Error(w, "product not found", 404)
			return
		}
		// Depth 1: umbrella shows its direct sub-products; each sub-product
		// returns only its manifests, NOT its own subsystem deps. Depth 2
		// caused the 2026-04-24 "tangled mess" where Execution appeared as
		// a child of both Agentic OS (rollup) AND Observability (subsystem
		// dep), with both edges drawn on top of each other in the same
		// product layer. Subsystem deps only render when that product is
		// the root of the view, not when it's a nested child.
		result := buildHierarchyProduct(r.Context(), n, p, 1, map[string]bool{})
		writeJSON(w, result)
	}
}

func apiProductIdeas(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		p, err := n.Products.Get(id)
		if err != nil || p == nil {
			http.Error(w, "product not found", 404)
			return
		}
		ideas, err := n.Ideas.ListByProject(p.ID, 50)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, ideas)
	}
}

// resolveProductID accepts a marker or full UUID and returns the full
// UUID via Products.Get (which handles prefix matching). Empty return
// + error message means 404-worthy.
func resolveProductID(n *node.Node, idOrMarker string) (string, string) {
	p, _ := n.Products.Get(idOrMarker)
	if p == nil {
		return "", "product not found: " + idOrMarker
	}
	return p.ID, ""
}

// apiProductDepList — GET /api/products/{id}/dependencies?direction=out|in|both
//
// Default direction is 'out'. Response body omits keys that the
// direction filter excluded so the UI can dispatch on key presence.
func apiProductDepList(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, msg := resolveProductID(n, mux.Vars(r)["id"])
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
			deps, err := n.Products.ListDeps(r.Context(), id)
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
			dependents, err := n.Products.ListDependents(r.Context(), id)
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

// apiProductDepAdd — POST /api/products/{id}/dependencies
//
// Body: {"depends_on_id": "..."}.
// 201 with the added edge on success, 409 on cycle, 400 on
// self-loop or bad body, 404 on missing product.
func apiProductDepAdd(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		srcID, msg := resolveProductID(n, mux.Vars(r)["id"])
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
		dstID, msg := resolveProductID(n, body.DependsOnID)
		if msg != "" {
			writeError(w, msg, http.StatusNotFound)
			return
		}
		createdBy := body.CreatedBy
		if createdBy == "" {
			createdBy = "http-api"
		}
		if err := n.Products.AddDep(r.Context(), srcID, dstID, createdBy); err != nil {
			switch {
			case errors.Is(err, product.ErrCycle):
				writeError(w, err.Error(), http.StatusConflict)
			case errors.Is(err, product.ErrSelfLoop):
				writeError(w, err.Error(), http.StatusBadRequest)
			default:
				writeError(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		// Echo the added edge back — the UI won't need a refetch.
		deps, _ := n.Products.ListDeps(r.Context(), srcID)
		for _, d := range deps {
			if d.ID == dstID {
				w.WriteHeader(http.StatusCreated)
				writeJSON(w, d)
				return
			}
		}
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, map[string]string{"product_id": srcID, "depends_on_id": dstID})
	}
}

// apiProductDepRemove — DELETE /api/products/{id}/dependencies/{depId}
//
// Idempotent: 204 whether the edge existed or not.
func apiProductDepRemove(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		srcID, msg := resolveProductID(n, mux.Vars(r)["id"])
		if msg != "" {
			writeError(w, msg, http.StatusNotFound)
			return
		}
		dstID, msg := resolveProductID(n, mux.Vars(r)["depId"])
		if msg != "" {
			writeError(w, msg, http.StatusNotFound)
			return
		}
		if err := n.Products.RemoveDep(r.Context(), srcID, dstID); err != nil {
			writeError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
