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

// unlinkedBucketID is the synthetic id used for the "Unlinked Products"
// container that holds products without an idea parent.
const unlinkedBucketID = "unlinked"

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
		type adjEdge struct{ dst, dstKind, kind string }
		adj := make(map[string][]adjEdge)
		for srcID, edges := range allEdges {
			for _, e := range edges {
				adj[srcID] = append(adj[srcID], adjEdge{e.DstID, e.DstKind, e.Kind})
			}
		}

		var buildTask func(string) *treeNode
		var buildManifest func(string) *treeNode
		var buildProduct func(string) *treeNode
		var buildIdea func(string, map[string]bool) *treeNode

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
					if cn := buildTask(ed.dst); cn != nil {
						children = append(children, cn)
					}
				}
			}
			sort.Slice(children, func(i, j int) bool { return children[i].Name < children[j].Name })
			st := e.Status
			if len(children) > 0 {
				st = deriveTreeStatus(children)
			}
			return &treeNode{ID: e.EntityUID, Name: e.Title, Kind: entity.TypeManifest, Status: st, Children: children}
		}
		buildProduct = func(id string) *treeNode {
			e := entityByID[id]
			if e == nil {
				return nil
			}
			var children []*treeNode
			for _, ed := range adj[id] {
				if ed.kind == relationships.EdgeOwns && ed.dstKind == entity.TypeManifest {
					if cn := buildManifest(ed.dst); cn != nil {
						children = append(children, cn)
					}
				}
			}
			sort.Slice(children, func(i, j int) bool { return children[i].Name < children[j].Name })
			st := e.Status
			if len(children) > 0 {
				st = deriveTreeStatus(children)
			}
			return &treeNode{ID: e.EntityUID, Name: e.Title, Kind: entity.TypeProduct, Status: st, Children: children}
		}
		buildIdea = func(id string, linked map[string]bool) *treeNode {
			e := entityByID[id]
			if e == nil {
				return nil
			}
			var children []*treeNode
			for _, ed := range adj[id] {
				if ed.kind == relationships.EdgeLinksTo && ed.dstKind == entity.TypeProduct {
					linked[ed.dst] = true
					if cn := buildProduct(ed.dst); cn != nil {
						children = append(children, cn)
					}
				}
			}
			sort.Slice(children, func(i, j int) bool { return children[i].Name < children[j].Name })
			st := e.Status
			if len(children) > 0 {
				st = deriveTreeStatus(children)
			}
			nd := &treeNode{ID: e.EntityUID, Name: e.Title, Kind: entity.TypeIdea, Status: st}
			if len(children) > 0 {
				nd.Children = children
			}
			return nd
		}

		skills := make([]*treeNode, 0)
		for _, s := range byKind[entity.TypeSkill] {
			if s.Status != entity.StatusArchived {
				skills = append(skills, &treeNode{ID: s.EntityUID, Name: s.Title, Kind: entity.TypeSkill, Status: s.Status})
			}
		}
		sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })

		linked := make(map[string]bool)
		lifecycle := make([]*treeNode, 0)
		for _, i := range byKind[entity.TypeIdea] {
			if nd := buildIdea(i.EntityUID, linked); nd != nil {
				lifecycle = append(lifecycle, nd)
			}
		}
		sort.Slice(lifecycle, func(i, j int) bool { return lifecycle[i].Name < lifecycle[j].Name })

		var unlinked []*treeNode
		for _, p := range byKind[entity.TypeProduct] {
			if !linked[p.EntityUID] && p.Status != entity.StatusArchived {
				if nd := buildProduct(p.EntityUID); nd != nil {
					unlinked = append(unlinked, nd)
				}
			}
		}
		sort.Slice(unlinked, func(i, j int) bool { return unlinked[i].Name < unlinked[j].Name })
		if len(unlinked) > 0 {
			lifecycle = append(lifecycle, &treeNode{
				ID:       unlinkedBucketID,
				Name:     "Unlinked Products",
				Kind:     entity.TypeIdea,
				Status:   deriveTreeStatus(unlinked),
				Children: unlinked,
			})
		}
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
