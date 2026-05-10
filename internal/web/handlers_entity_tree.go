package web

import (
	"net/http"
	"sort"

	"github.com/k8nstantin/OpenPraxis/internal/entity"
	"github.com/k8nstantin/OpenPraxis/internal/execution"
	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/relationships"
)

type treeNode struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Kind     string      `json:"kind"`
	Status   string      `json:"status"`
	Children []*treeNode `json:"children,omitempty"`
}

// Tree-display status values. Derived from entity status + child rollup; not
// queried, not filtered. Aliased to canonical constants where one exists so
// adding a new entity/event status flows through.
const (
	treeStatusRunning   = "running"
	treeStatusFailed    = execution.EventFailed
	treeStatusCompleted = execution.EventCompleted
	treeStatusActive    = entity.StatusActive
	treeStatusDraft     = entity.StatusDraft
)

func apiEntityTree(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if n.Entities == nil || n.Relationships == nil {
			writeJSON(w, map[string]any{"skills": []any{}, "lifecycle": []any{}})
			return
		}

		type res struct {
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
		ch := make(chan res, len(kinds))
		for _, k := range kinds {
			go func(kind string) {
				items, err := n.Entities.List(kind, "", 500)
				ch <- res{kind: kind, items: items, err: err}
			}(k)
		}
		byKind := make(map[string][]*entity.Entity)
		for range kinds {
			r := <-ch
			if r.err == nil {
				byKind[r.kind] = r.items
			}
		}

		entityByID := make(map[string]*entity.Entity)
		var allIDs []string
		for _, items := range byKind {
			for _, e := range items {
				entityByID[e.EntityUID] = e
				allIDs = append(allIDs, e.EntityUID)
			}
		}

		allEdges, _ := n.Relationships.ListOutgoingForMany(ctx, allIDs, "")
		type adjEdge struct{ dst, kind string }
		adj := make(map[string][]adjEdge)
		// ownedByOwns tracks IDs that have a parent via owns — used to find roots.
		ownedByOwns := make(map[string]bool)
		for srcID, edges := range allEdges {
			for _, e := range edges {
				adj[srcID] = append(adj[srcID], adjEdge{e.DstID, e.Kind})
				if e.Kind == relationships.EdgeOwns {
					ownedByOwns[e.DstID] = true
				}
			}
		}

		// Generic recursive builder — follows owns edges from the relationship table.
		// No hardcoded kind hierarchy: whatever owns what is what the tree shows.
		var buildNode func(id string, visited map[string]bool) *treeNode
		buildNode = func(id string, visited map[string]bool) *treeNode {
			if visited[id] {
				return nil
			}
			visited[id] = true
			e := entityByID[id]
			if e == nil {
				return nil
			}
			var children []*treeNode
			for _, ed := range adj[id] {
				if ed.kind == relationships.EdgeOwns {
					if cn := buildNode(ed.dst, visited); cn != nil {
						children = append(children, cn)
					}
				}
			}
			sort.Slice(children, func(i, j int) bool { return children[i].Name < children[j].Name })
			st := e.Status
			if len(children) > 0 {
				st = deriveTreeStatus(children)
			}
			nd := &treeNode{ID: e.EntityUID, Name: e.Title, Kind: e.Type, Status: st}
			if len(children) > 0 {
				nd.Children = children
			}
			return nd
		}

		// Skills: flat governance section — not part of the lifecycle hierarchy.
		skills := make([]*treeNode, 0)
		for _, s := range byKind[entity.TypeSkill] {
			if s.Status != entity.StatusArchived {
				skills = append(skills, &treeNode{ID: s.EntityUID, Name: s.Title, Kind: s.Type, Status: s.Status})
			}
		}
		sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })

		// Lifecycle roots: products + ideas that have no owns-parent.
		// Orphan manifests/tasks (no product parent) are excluded from the nav tree —
		// they're accessible via the Entities list but would clutter root-level navigation.
		// TypeProduct and TypeIdea come from the entities table — not hardcoded strings.
		rootTypes := map[string]bool{entity.TypeProduct: true, entity.TypeIdea: true}
		lifecycle := make([]*treeNode, 0)
		for _, items := range byKind {
			for _, e := range items {
				if !rootTypes[e.Type] || e.Status == entity.StatusArchived {
					continue
				}
				if ownedByOwns[e.EntityUID] {
					continue
				}
				if nd := buildNode(e.EntityUID, make(map[string]bool)); nd != nil {
					lifecycle = append(lifecycle, nd)
				}
			}
		}
		// Sort root entities chronologically — entity_uid is a ULID, lexicographic = creation order.
		sort.Slice(lifecycle, func(i, j int) bool { return lifecycle[i].ID > lifecycle[j].ID })

		writeJSON(w, map[string]any{"skills": skills, "lifecycle": lifecycle})
	}
}

func deriveTreeStatus(ch []*treeNode) string {
	for _, c := range ch {
		if c.Status == treeStatusRunning {
			return treeStatusRunning
		}
	}
	for _, c := range ch {
		if c.Status == treeStatusFailed {
			return treeStatusFailed
		}
	}
	allDone, anyDone := true, false
	for _, c := range ch {
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
