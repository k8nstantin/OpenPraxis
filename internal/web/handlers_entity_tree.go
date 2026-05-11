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

		// Build the kinds slice from entity_types table when available;
		// fall back to the built-in set if the store is nil or returns an error.
		builtinKinds := []string{
			entity.TypeSkill,
			entity.TypeIdea,
			entity.TypeProduct,
			entity.TypeManifest,
			entity.TypeTask,
			entity.TypeRAG,
		}
		var kinds []string
		if n.EntityTypes != nil {
			if etypes, err := n.EntityTypes.List(ctx); err == nil && len(etypes) > 0 {
				kinds = make([]string, 0, len(etypes))
				for _, et := range etypes {
					kinds = append(kinds, et.Name)
				}
			}
		}
		if len(kinds) == 0 {
			kinds = builtinKinds
		}

		type res struct {
			kind  string
			items []*entity.Entity
			err   error
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
		// ownedByOwns tracks IDs owned by a same-or-lower-tier entity.
		// A product owned by a SKILL is still a root — skills are governance,
		// not hierarchy parents. Only exclude from root when owned by another
		// product, manifest, or task (i.e. a non-skill entity).
		ownedByOwns := make(map[string]bool)
		for srcID, edges := range allEdges {
			for _, e := range edges {
				adj[srcID] = append(adj[srcID], adjEdge{e.DstID, e.Kind})
				if e.Kind == relationships.EdgeOwns {
					srcEntity := entityByID[srcID]
					if srcEntity == nil || srcEntity.Type != entity.TypeSkill {
						ownedByOwns[e.DstID] = true
					}
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
		// Products first (newest → oldest), then ideas (newest → oldest).
		// ULID is lexicographically time-sortable so string comparison = creation order.
		sort.Slice(lifecycle, func(i, j int) bool {
			pi := lifecycle[i].Kind == entity.TypeProduct
			pj := lifecycle[j].Kind == entity.TypeProduct
			if pi != pj {
				return pi // products before ideas
			}
			return lifecycle[i].ID > lifecycle[j].ID // newest first within each kind
		})

		// Manifests and tasks: buildNode applied so they expand their owned children.
		manifests := make([]*treeNode, 0)
		for _, m := range byKind[entity.TypeManifest] {
			if m.Status != entity.StatusArchived {
				if nd := buildNode(m.EntityUID, make(map[string]bool)); nd != nil {
					manifests = append(manifests, nd)
				}
			}
		}
		sort.Slice(manifests, func(i, j int) bool { return manifests[i].ID > manifests[j].ID })

		tasks := make([]*treeNode, 0)
		for _, t := range byKind[entity.TypeTask] {
			if t.Status != entity.StatusArchived {
				if nd := buildNode(t.EntityUID, make(map[string]bool)); nd != nil {
					tasks = append(tasks, nd)
				}
			}
		}
		sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID > tasks[j].ID })

		rags := make([]*treeNode, 0)
		for _, r := range byKind[entity.TypeRAG] {
			if r.Status != entity.StatusArchived {
				if nd := buildNode(r.EntityUID, make(map[string]bool)); nd != nil {
					rags = append(rags, nd)
				}
			}
		}
		sort.Slice(rags, func(i, j int) bool { return rags[i].ID > rags[j].ID })

		// knownSpecial tracks the kinds that are already handled in the named
		// top-level sections. Any kind NOT in this set is included in by_type
		// only and as an extra section in the response.
		knownSpecial := map[string]bool{
			entity.TypeSkill:    true,
			entity.TypeIdea:     true,
			entity.TypeProduct:  true,
			entity.TypeManifest: true,
			entity.TypeTask:     true,
			entity.TypeRAG:      true,
		}

		// by_type: all entity types keyed by name — for forwards-compatible frontend consumption.
		byType := make(map[string][]*treeNode, len(kinds))
		for _, k := range kinds {
			nodes := make([]*treeNode, 0)
			for _, e := range byKind[k] {
				if e.Status != entity.StatusArchived {
					if nd := buildNode(e.EntityUID, make(map[string]bool)); nd != nil {
						nodes = append(nodes, nd)
					}
				}
			}
			sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID > nodes[j].ID })
			byType[k] = nodes
		}

		// extra_types: unknown/custom entity kinds not handled as named sections.
		extraTypes := make(map[string][]*treeNode)
		for _, k := range kinds {
			if !knownSpecial[k] {
				extraTypes[k] = byType[k]
			}
		}

		writeJSON(w, map[string]any{
			"skills":      skills,
			"lifecycle":   lifecycle,
			"manifests":   manifests,
			"tasks":       tasks,
			"rags":        rags,
			"by_type":     byType,
			"extra_types": extraTypes,
		})
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
