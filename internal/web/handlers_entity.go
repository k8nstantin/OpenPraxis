package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
	"github.com/k8nstantin/OpenPraxis/internal/entity"
	execution "github.com/k8nstantin/OpenPraxis/internal/execution"
	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/relationships"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// entityID extracts and validates the {id} path parameter as a UUID.
// Returns (id, true) on success; writes 400 and returns ("", false) on failure.
func entityID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := mux.Vars(r)["id"]
	if _, err := uuid.Parse(id); err != nil {
		http.Error(w, "invalid entity id", http.StatusBadRequest)
		return "", false
	}
	return id, true
}

// runUID extracts and validates the {runUid} path parameter as a UUID.
func runUID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := mux.Vars(r)["runUid"]
	if _, err := uuid.Parse(id); err != nil {
		http.Error(w, "invalid run uid", http.StatusBadRequest)
		return "", false
	}
	return id, true
}

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
			Type       string   `json:"type"`
			Title      string   `json:"title"`
			Status     string   `json:"status"`
			Tags       []string `json:"tags"`
			ProjectID  string   `json:"project_id"`   // manifest → parent product
			ManifestID string   `json:"manifest_id"`  // task → parent manifest
			Prompt     string   `json:"prompt"`       // optional initial prompt — posted as TypePrompt comment
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
		// Wire the owns edge from parent to the newly created entity.
		if n.Relationships != nil {
			if req.ProjectID != "" && req.Type == "manifest" {
				if err := n.Relationships.Create(r.Context(), relationships.Edge{
					SrcKind:   relationships.KindProduct,
					SrcID:     req.ProjectID,
					DstKind:   relationships.KindManifest,
					DstID:     e.EntityUID,
					Kind:      relationships.EdgeOwns,
					CreatedBy: "http-api",
				}); err != nil {
					slog.Error("entity create: wire owns edge failed", "src", req.ProjectID, "dst", e.EntityUID, "err", err)
				}
			}
			if req.ManifestID != "" && req.Type == "task" {
				if err := n.Relationships.Create(r.Context(), relationships.Edge{
					SrcKind:   relationships.KindManifest,
					SrcID:     req.ManifestID,
					DstKind:   relationships.KindTask,
					DstID:     e.EntityUID,
					Kind:      relationships.EdgeOwns,
					CreatedBy: "http-api",
				}); err != nil {
					slog.Error("entity create: wire owns edge failed", "src", req.ManifestID, "dst", e.EntityUID, "err", err)
				}
			}
		}
		// Persist optional prompt as a TypePrompt comment so it's versioned
		// and never silently dropped. Before this fix the field was not in the
		// request struct and Go's JSON decoder dropped it without error.
		if req.Prompt != "" && n.Comments != nil {
			if _, err := n.Comments.Add(r.Context(), comments.TargetEntity, e.EntityUID,
				"http-api", comments.TypePrompt, req.Prompt); err != nil {
				slog.Warn("entity create: failed to save prompt comment", "entity", e.EntityUID, "error", err)
			}
		}
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, e)
	}
}

func apiEntityGet(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := entityID(w, r)
		if !ok {
			return
		}
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
		id, ok := entityID(w, r)
		if !ok {
			return
		}
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
			incoming, err := n.Relationships.ListIncoming(r.Context(), existing.EntityUID, relationships.EdgeOwns)
			if err != nil {
				slog.Error("re-parent: list incoming failed", "entity", existing.EntityUID, "err", err)
				http.Error(w, "failed to list relationships", 500)
				return
			}
			for _, e := range incoming {
				if e.SrcKind == relationships.KindProduct {
					if err := n.Relationships.Remove(r.Context(), e.SrcID, existing.EntityUID, relationships.EdgeOwns, "http-api", "re-parent"); err != nil {
						slog.Error("re-parent: remove old edge failed", "src", e.SrcID, "dst", existing.EntityUID, "err", err)
						http.Error(w, "failed to remove old relationship", 500)
						return
					}
				}
			}
			// Add new owns edge if non-empty project_id.
			if newProjectID != "" {
				if err := n.Relationships.Create(r.Context(), relationships.Edge{
					SrcKind:   relationships.KindProduct,
					SrcID:     newProjectID,
					DstKind:   relationships.KindManifest,
					DstID:     existing.EntityUID,
					Kind:      relationships.EdgeOwns,
					CreatedBy: "http-api",
				}); err != nil {
					slog.Error("re-parent: create new edge failed", "src", newProjectID, "dst", existing.EntityUID, "err", err)
					http.Error(w, "failed to create relationship", 500)
					return
				}
			}
		}

		// Handle manifest_id (task → manifest ownership edge).
		if req.ManifestID != nil && n.Relationships != nil {
			newManifestID := *req.ManifestID
			incoming, err := n.Relationships.ListIncoming(r.Context(), existing.EntityUID, relationships.EdgeOwns)
			if err != nil {
				slog.Error("re-parent: list incoming failed", "entity", existing.EntityUID, "err", err)
				http.Error(w, "failed to list relationships", 500)
				return
			}
			for _, e := range incoming {
				if e.SrcKind == relationships.KindManifest {
					if err := n.Relationships.Remove(r.Context(), e.SrcID, existing.EntityUID, relationships.EdgeOwns, "http-api", "re-parent"); err != nil {
						slog.Error("re-parent: remove old edge failed", "src", e.SrcID, "dst", existing.EntityUID, "err", err)
						http.Error(w, "failed to remove old relationship", 500)
						return
					}
				}
			}
			if newManifestID != "" {
				if err := n.Relationships.Create(r.Context(), relationships.Edge{
					SrcKind:   relationships.KindManifest,
					SrcID:     newManifestID,
					DstKind:   relationships.KindTask,
					DstID:     existing.EntityUID,
					Kind:      relationships.EdgeOwns,
					CreatedBy: "http-api",
				}); err != nil {
					slog.Error("re-parent: create new edge failed", "src", newManifestID, "dst", existing.EntityUID, "err", err)
					http.Error(w, "failed to create relationship", 500)
					return
				}
			}
		}

		if err := n.Entities.Update(existing.EntityUID, title, status, tags, "http-api", ""); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		updated, err := n.Entities.Get(existing.EntityUID)
		if err != nil || updated == nil {
			http.Error(w, "entity not found after update", 500)
			return
		}
		writeJSON(w, updated)
	}
}

func apiEntityHistory(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := entityID(w, r)
		if !ok {
			return
		}
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
		runUID, ok := runUID(w, r)
		if !ok {
			return
		}
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
		runUID, ok := runUID(w, r)
		if !ok {
			return
		}
		rows, err := n.ExecutionLog.ListByRun(r.Context(), runUID)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		// Recompute cost_per_turn / cost_per_action (and other ratios) so the
		// response is correct even for legacy rows persisted before
		// ComputeDerived was wired into the write path. Idempotent.
		for i := range rows {
			execution.ComputeDerived(&rows[i])
		}
		writeJSON(w, rows)
	}
}

func apiEntityExecutionLog(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id, ok := entityID(w, r)
		if !ok {
			return
		}
		limit := 50
		if l := r.URL.Query().Get("limit"); l != "" {
			if v, err := strconv.Atoi(l); err == nil && v > 0 {
				limit = v
			}
		}

		// For task entities return the flat atomic run list (existing behaviour).
		// For manifests and products walk the DAG and return a hierarchical shape:
		//   manifest → [{task_id, task_title, runs:[...ExecutionRow]}]
		//   product  → [{manifest_id, manifest_title, tasks:[{task_id, task_title, runs:[...]}]}]
		e, _ := n.Entities.Get(id)
		if e == nil || e.Type == "task" || n.Relationships == nil {
			rows, err := n.ExecutionLog.ListByEntity(ctx, id, limit)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			writeJSON(w, rows)
			return
		}

		type taskRuns struct {
			TaskID    string      `json:"task_id"`
			TaskTitle string      `json:"task_title"`
			Runs      interface{} `json:"runs"`
		}
		type manifestGroup struct {
			ManifestID    string     `json:"manifest_id"`
			ManifestTitle string     `json:"manifest_title"`
			Tasks         []taskRuns `json:"tasks"`
		}

		// Helper: collect runs for a task
		taskRunsFor := func(taskID, taskTitle string) taskRuns {
			rows, _ := n.ExecutionLog.ListByEntity(ctx, taskID, limit)
			if rows == nil {
				rows = []execution.Row{}
			}
			return taskRuns{TaskID: taskID, TaskTitle: taskTitle, Runs: rows}
		}

		if e.Type == "manifest" {
			// manifest → tasks
			edges, _ := n.Relationships.ListOutgoing(ctx, id, "owns")

			// Collect all task IDs first, then batch-fetch titles.
			var taskIDs []string
			for _, edge := range edges {
				if edge.DstKind == "task" {
					taskIDs = append(taskIDs, edge.DstID)
				}
			}
			titleByID := make(map[string]string, len(taskIDs))
			if len(taskIDs) > 0 {
				entities, err := n.Entities.ListByIDs(taskIDs)
				if err != nil {
					slog.Warn("apiEntityExecutionLog: ListByIDs failed", "error", err)
				} else {
					for _, te := range entities {
						titleByID[te.EntityUID] = te.Title
					}
				}
			}

			result := []taskRuns{}
			for _, edge := range edges {
				if edge.DstKind == "task" {
					title := edge.DstID
					if t, ok := titleByID[edge.DstID]; ok {
						title = t
					}
					result = append(result, taskRunsFor(edge.DstID, title))
				}
			}
			writeJSON(w, result)
			return
		}

		// product → manifests → tasks
		manifEdges, _ := n.Relationships.ListOutgoing(ctx, id, "owns")

		// Pass 1: collect manifest IDs and, per manifest, their task edges.
		// This is O(manifests) relationship queries — unavoidable without a
		// new bulk-walk API. Entity title lookups are batched below.
		type manifData struct {
			id        string
			taskEdges []relationships.Edge
		}
		var manifList []manifData
		var allTaskIDs []string
		var manifIDs []string
		for _, me := range manifEdges {
			if me.DstKind != "manifest" {
				continue
			}
			manifIDs = append(manifIDs, me.DstID)
			tEdges, _ := n.Relationships.ListOutgoing(ctx, me.DstID, "owns")
			md := manifData{id: me.DstID, taskEdges: tEdges}
			for _, te := range tEdges {
				if te.DstKind == "task" {
					allTaskIDs = append(allTaskIDs, te.DstID)
				}
			}
			manifList = append(manifList, md)
		}

		// Pass 2: two ListByIDs calls — one for all manifests, one for all tasks.
		manifTitleByID := make(map[string]string, len(manifIDs))
		if len(manifIDs) > 0 {
			if ents, err := n.Entities.ListByIDs(manifIDs); err != nil {
				slog.Warn("apiEntityExecutionLog: ListByIDs manifests failed", "error", err)
			} else {
				for _, e := range ents {
					manifTitleByID[e.EntityUID] = e.Title
				}
			}
		}
		taskTitleByID := make(map[string]string, len(allTaskIDs))
		if len(allTaskIDs) > 0 {
			if ents, err := n.Entities.ListByIDs(allTaskIDs); err != nil {
				slog.Warn("apiEntityExecutionLog: ListByIDs tasks failed", "error", err)
			} else {
				for _, e := range ents {
					taskTitleByID[e.EntityUID] = e.Title
				}
			}
		}

		// Pass 3: assemble result from pre-fetched data.
		groups := []manifestGroup{}
		for _, md := range manifList {
			manTitle := md.id
			if t, ok := manifTitleByID[md.id]; ok {
				manTitle = t
			}
			tasks := []taskRuns{}
			for _, te := range md.taskEdges {
				if te.DstKind != "task" {
					continue
				}
				tTitle := te.DstID
				if t, ok := taskTitleByID[te.DstID]; ok {
					tTitle = t
				}
				tasks = append(tasks, taskRunsFor(te.DstID, tTitle))
			}
			groups = append(groups, manifestGroup{
				ManifestID: md.id, ManifestTitle: manTitle, Tasks: tasks,
			})
		}
		writeJSON(w, groups)
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
		id, ok := entityID(w, r)
		if !ok {
			return
		}
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
		id, ok := entityID(w, r)
		if !ok {
			return
		}
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
		id, ok := entityID(w, r)
		if !ok {
			return
		}
		depID := mux.Vars(r)["dep_id"]
		if _, err := uuid.Parse(depID); err != nil {
			http.Error(w, "invalid dep_id", http.StatusBadRequest)
			return
		}
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
		id, ok := entityID(w, r)
		if !ok {
			return
		}
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

// apiEntityActions returns the actions (tool calls) for an entity, newest first.
// GET /api/entities/{id}/actions?limit=100&run_uid=<runUID>
//
// When run_uid is provided, actions are filtered to only those created after
// the run's started_at timestamp — ensures live output shows only the current
// run's tool calls, not historical ones from prior runs of the same task.
func apiEntityActions(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id, ok := entityID(w, r)
		if !ok {
			return
		}
		limit := 100
		if l := r.URL.Query().Get("limit"); l != "" {
			if v, err := strconv.Atoi(l); err == nil && v > 0 {
				limit = v
			}
		}

		// If run_uid provided, filter to actions created at/after run start.
		runUID := r.URL.Query().Get("run_uid")
		var sinceTime string
		if runUID != "" && n.ExecutionLog != nil {
			rows, err := n.ExecutionLog.ListByRun(ctx, runUID)
			if err == nil {
				for _, row := range rows {
					if row.Event == "started" {
						sinceTime = row.CreatedAt
						break
					}
				}
			}
		}

		actions, err := n.Actions.ListByTask(id, limit)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if actions == nil {
			writeJSON(w, []any{})
			return
		}

		// Filter by run start time when run_uid was specified.
		if sinceTime != "" {
			filtered := actions[:0]
			for _, a := range actions {
				if a.CreatedAt.Format("2006-01-02T15:04:05Z07:00") >= sinceTime {
					filtered = append(filtered, a)
				}
			}
			actions = filtered
		}

		writeJSON(w, actions)
	}
}

// apiRelationshipsIncoming handles GET /api/relationships/incoming?dst_id=...&kind=...
// Returns all current edges pointing TO dst_id with the given kind.
// Agents use this to walk UP the DAG — find the manifest that owns a task, etc.
func apiRelationshipsIncoming(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if n.Relationships == nil {
			writeJSON(w, []any{})
			return
		}
		dstID := r.URL.Query().Get("dst_id")
		kind := r.URL.Query().Get("kind")
		if dstID == "" || kind == "" {
			writeError(w, "dst_id and kind are required", http.StatusBadRequest)
			return
		}
		edges, err := n.Relationships.ListIncoming(r.Context(), dstID, kind)
		if err != nil {
			writeError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if edges == nil {
			edges = []relationships.Edge{}
		}
		writeJSON(w, edges)
	}
}

// apiRelationshipsOutgoing handles GET /api/relationships/outgoing?src_id=...&kind=...
// Returns all current edges pointing FROM src_id with the given kind.
// Agents use this to walk DOWN the DAG — list tasks owned by a manifest, etc.
func apiRelationshipsOutgoing(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if n.Relationships == nil {
			writeJSON(w, []any{})
			return
		}
		srcID := r.URL.Query().Get("src_id")
		kind := r.URL.Query().Get("kind")
		if srcID == "" || kind == "" {
			writeError(w, "src_id and kind are required", http.StatusBadRequest)
			return
		}
		edges, err := n.Relationships.ListOutgoing(r.Context(), srcID, kind)
		if err != nil {
			writeError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if edges == nil {
			edges = []relationships.Edge{}
		}
		writeJSON(w, edges)
	}
}
