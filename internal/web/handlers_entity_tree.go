package web

import (
	"net/http"
	"sort"

	"github.com/k8nstantin/OpenPraxis/internal/entity"
	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/relationships"
)

// treeNode is the wire shape for a single node in the entity tree response.
type treeNode struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Kind     string      `json:"kind"`
	Status   string      `json:"status"`
	Children []*treeNode `json:"children,omitempty"`
}

// Tree-level execution status constants — distinct from entity.Status*
// because tasks carry runtime states (running/failed/completed) that
// don't map 1:1 to entity lifecycle status.
const (
	treeStatusRunning   = "running"
	treeStatusFailed    = "failed"
	treeStatusCompleted = "completed"
	treeStatusActive    = "active"
	treeStatusDraft     = "draft"
)

// GET /api/entities/tree
//
// Returns the full sidebar entity tree in one response — no client-side
// relationship fan-out required. Queries:
//   1. List all entities by type (5 parallel goroutines)
//   2. ListOutgoingForMany on all IDs (one SQL IN query)
//   3. Assemble tree in Go
//
// Response: { "skills": [treeNode, ...], "lifecycle": [treeNode, ...] }
func apiEntityTree(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if n.Entities == nil || n.Relationships == nil {
			writeJSON(w, map[string]any{"skills": []any{}, "lifecycle": []any{}})
			return
		}

		// ── 1. Fetch all entities by type in parallel ────────────────────
		type listResult struct {
			kind  string
			items []*entity.Entity
			err   error
		}
		kinds := []string{
			entity.TypeSkill,
			entity.TypeIdea,
			entity.TypeProduct,
			entity.TypeManifest,
			entity.TypeTask,
		}
		ch := make(chan listResult, len(kinds))
		for _, k := range kinds {
			go func(kind string) {
				items, err := n.Entities.List(kind, "", 500)
				ch <- listResult{kind: kind, items: items, err: err}
			}(k)
		}
		byKind := make(map[string][]*entity.Entity, len(kinds))
		for range kinds {
			res := <-ch
			if res.err == nil {
				byKind[res.kind] = res.items
			}
		}

		// ── 2. Build entity lookup + collect all IDs ─────────────────────
		entityByID := make(map[string]*entity.Entity)
		var allIDs []string
		for _, items := range byKind {
			for _, e := range items {
				entityByID[e.EntityUID] = e
				allIDs = append(allIDs, e.EntityUID)
			}
		}

		// ── 3. One bulk outgoing-relationship query ───────────────────────
		allEdges, err := n.Relationships.ListOutgoingForMany(ctx, allIDs, "")
		if err != nil {
			writeError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Adjacency: srcID → []edge
		type adjEdge struct{ dst, dstKind, kind string }
		adj := make(map[string][]adjEdge, len(allEdges))
		for srcID, edges := range allEdges {
			for _, e := range edges {
				adj[srcID] = append(adj[srcID], adjEdge{
					dst:     e.DstID,
					dstKind: e.DstKind,
					kind:    e.Kind,
				})
			}
		}

		// ── 4. Assemble tree ─────────────────────────────────────────────

		var buildTask func(id string) *treeNode
		var buildManifest func(id string) *treeNode
		var buildProduct func(id string) *treeNode
		var buildIdea func(id string, linkedProducts map[string]bool) *treeNode

		buildTask = func(id string) *treeNode {
			e := entityByID[id]
			if e == nil {
				return nil
			}
			return &treeNode{ID: e.EntityUID, Name: e.Title, Kind: entity.TypeTask, Status: e.Status}
		}

		buildManifest = func(id string) *treeNode {
			e := entityByID[id]
			if e == nil {
				return nil
			}
			var children []*treeNode
			for _, ed := range adj[id] {
				if ed.kind == relationships.EdgeOwns && ed.dstKind == entity.TypeTask {
					if t := buildTask(ed.dst); t != nil {
						children = append(children, t)
					}
				}
			}
			sort.Slice(children, func(i, j int) bool { return children[i].Name < children[j].Name })
			status := e.Status
			if len(children) > 0 {
				status = deriveTreeStatus(children)
			}
			return &treeNode{ID: e.EntityUID, Name: e.Title, Kind: entity.TypeManifest, Status: status, Children: children}
		}

		buildProduct = func(id string) *treeNode {
			e := entityByID[id]
			if e == nil {
				return nil
			}
			var children []*treeNode
			for _, ed := range adj[id] {
				if ed.kind == relationships.EdgeOwns && ed.dstKind == entity.TypeManifest {
					if m := buildManifest(ed.dst); m != nil {
						children = append(children, m)
					}
				}
			}
			sort.Slice(children, func(i, j int) bool { return children[i].Name < children[j].Name })
			status := e.Status
			if len(children) > 0 {
				status = deriveTreeStatus(children)
			}
			return &treeNode{ID: e.EntityUID, Name: e.Title, Kind: entity.TypeProduct, Status: status, Children: children}
		}

		buildIdea = func(id string, linkedProducts map[string]bool) *treeNode {
			e := entityByID[id]
			if e == nil {
				return nil
			}
			var children []*treeNode
			for _, ed := range adj[id] {
				if ed.kind == relationships.EdgeLinksTo && ed.dstKind == entity.TypeProduct {
					linkedProducts[ed.dst] = true
					if p := buildProduct(ed.dst); p != nil {
						children = append(children, p)
					}
				}
			}
			sort.Slice(children, func(i, j int) bool { return children[i].Name < children[j].Name })
			status := e.Status
			if len(children) > 0 {
				status = deriveTreeStatus(children)
			}
			nd := &treeNode{ID: e.EntityUID, Name: e.Title, Kind: entity.TypeIdea, Status: status}
			if len(children) > 0 {
				nd.Children = children
			}
			return nd
		}

		// Skills — flat, sorted, exclude archived
		skills := make([]*treeNode, 0, len(byKind[entity.TypeSkill]))
		for _, s := range byKind[entity.TypeSkill] {
			if s.Status == entity.StatusArchived {
				continue
			}
			skills = append(skills, &treeNode{ID: s.EntityUID, Name: s.Title, Kind: entity.TypeSkill, Status: s.Status})
		}
		sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })

		// Lifecycle: ideas (with linked products), then unlinked products
		linkedProducts := make(map[string]bool)
		lifecycle := make([]*treeNode, 0)
		for _, i := range byKind[entity.TypeIdea] {
			if nd := buildIdea(i.EntityUID, linkedProducts); nd != nil {
				lifecycle = append(lifecycle, nd)
			}
		}
		sort.Slice(lifecycle, func(i, j int) bool { return lifecycle[i].Name < lifecycle[j].Name })

		var unlinked []*treeNode
		for _, p := range byKind[entity.TypeProduct] {
			if !linkedProducts[p.EntityUID] && p.Status != entity.StatusArchived {
				if nd := buildProduct(p.EntityUID); nd != nil {
					unlinked = append(unlinked, nd)
				}
			}
		}
		sort.Slice(unlinked, func(i, j int) bool { return unlinked[i].Name < unlinked[j].Name })
		if len(unlinked) > 0 {
			lifecycle = append(lifecycle, &treeNode{
				ID:       "unlinked-products",
				Name:     "Unlinked Products",
				Kind:     entity.TypeIdea,
				Status:   deriveTreeStatus(unlinked),
				Children: unlinked,
			})
		}

		writeJSON(w, map[string]any{"skills": skills, "lifecycle": lifecycle})
	}
}

// deriveTreeStatus rolls up children execution states.
// Priority: running > failed > completed > active > draft.
// Kind-agnostic — works for any internal node.
func deriveTreeStatus(children []*treeNode) string {
	for _, c := range children {
		if c.Status == treeStatusRunning {
			return treeStatusRunning
		}
	}
	for _, c := range children {
		if c.Status == treeStatusFailed {
			return treeStatusFailed
		}
	}
	allDone, anyDone := true, false
	for _, c := range children {
		if c.Status == treeStatusCompleted {
			anyDone = true
		} else {
			allDone = false
		}
	}
	if allDone && anyDone {
		return treeStatusCompleted
	}
	if anyDone {
		return treeStatusActive
	}
	return treeStatusDraft
}
