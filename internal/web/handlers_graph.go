package web

import (
	"net/http"
	"strconv"

	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/relationships"
)

// apiRelationshipsGraph returns a flat (nodes, edges) shape over the
// unified relationships SCD-2 table — one canonical source for the
// DAG tab. Walks every reachable node from the root via Walk(), then
// resolves titles + statuses per kind.
//
// GET /api/relationships/graph?root_id=X&root_kind=product&depth=10
//   ?edge_kinds=owns,depends_on   (default: both)
//
// Response:
//
//	{
//	  "nodes": [{"id":"...", "kind":"product", "title":"...", "status":"..."}],
//	  "edges": [{"id":"src->dst:kind", "source":"...", "target":"...", "kind":"owns"}]
//	}
func apiRelationshipsGraph(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rootID := r.URL.Query().Get("root_id")
		rootKind := r.URL.Query().Get("root_kind")
		if rootID == "" || rootKind == "" {
			writeError(w, "root_id and root_kind required", http.StatusBadRequest)
			return
		}

		// Skills are DAG roots governing many products. Cap depth at 1 so the
		// graph shows only direct product children — not the entire sub-tree
		// (which would be hundreds of manifests and tasks and unreadable).
		depth := 10
		if rootKind == relationships.KindSkill {
			depth = 1
		}
		if v := r.URL.Query().Get("depth"); v != "" {
			if d, err := strconv.Atoi(v); err == nil && d > 0 {
				depth = d
			}
		}
		edgeKinds := []string{relationships.EdgeOwns, relationships.EdgeDependsOn}
		if v := r.URL.Query().Get("edge_kinds"); v != "" {
			edgeKinds = splitCSV(v)
		}

		rows, err := n.Relationships.Walk(r.Context(), rootID, rootKind, edgeKinds, depth)
		if err != nil {
			writeError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Bucket node IDs by kind for batched title/status lookup.
		skillIDs := []string{}
		productIDs := []string{}
		manifestIDs := []string{}
		taskIDs := []string{}
		otherIDs := []string{}
		for _, row := range rows {
			switch row.Kind {
			case relationships.KindSkill:
				skillIDs = append(skillIDs, row.ID)
			case relationships.KindProduct:
				productIDs = append(productIDs, row.ID)
			case relationships.KindManifest:
				manifestIDs = append(manifestIDs, row.ID)
			case relationships.KindTask:
				taskIDs = append(taskIDs, row.ID)
			default:
				otherIDs = append(otherIDs, row.ID)
			}
		}

		type nodeOut struct {
			ID     string `json:"id"`
			Kind   string `json:"kind"`
			Title  string `json:"title"`
			Status string `json:"status"`
		}
		nodes := make([]nodeOut, 0, len(rows))

		// For products: walk UP one hop to find governing skill and prepend it
		// so the product DAG shows: skill → product → manifests → tasks.
		if rootKind == relationships.KindProduct && n.Relationships != nil {
			parentEdges, _ := n.Relationships.ListIncoming(r.Context(), rootID, relationships.EdgeOwns)
			for _, edge := range parentEdges {
				if edge.SrcKind == relationships.KindSkill {
					if skill, _ := n.Entities.Get(edge.SrcID); skill != nil {
						nodes = append(nodes, nodeOut{ID: skill.EntityUID, Kind: relationships.KindSkill, Title: skill.Title, Status: skill.Status})
						// Add the skill→product edge to the edge list below.
						skillIDs = append(skillIDs, edge.SrcID)
						// Inject a synthetic walk row so the edge is emitted.
						rows = append(rows, relationships.WalkRow{
							ID: rootID, Kind: rootKind,
							ViaSrc: edge.SrcID, ViaKind: relationships.EdgeOwns, Depth: 0,
						})
					}
					// no break — show ALL skills linked to this product
				}
			}
		}

		// Resolve title + status per entity.
		for _, id := range skillIDs {
			if e, _ := n.Entities.Get(id); e != nil {
				// Avoid duplicates — skill may already be added above.
				dup := false
				for _, existing := range nodes {
					if existing.ID == e.EntityUID {
						dup = true
						break
					}
				}
				if !dup {
					nodes = append(nodes, nodeOut{ID: e.EntityUID, Kind: relationships.KindSkill, Title: e.Title, Status: e.Status})
				}
			}
		}
		for _, id := range productIDs {
			if e, _ := n.Entities.Get(id); e != nil {
				nodes = append(nodes, nodeOut{ID: e.EntityUID, Kind: relationships.KindProduct, Title: e.Title, Status: e.Status})
			}
		}
		for _, id := range manifestIDs {
			if e, _ := n.Entities.Get(id); e != nil {
				nodes = append(nodes, nodeOut{ID: e.EntityUID, Kind: relationships.KindManifest, Title: e.Title, Status: e.Status})
			}
		}
		for _, id := range taskIDs {
			if e, _ := n.Entities.Get(id); e != nil {
				nodes = append(nodes, nodeOut{ID: e.EntityUID, Kind: relationships.KindTask, Title: e.Title, Status: e.Status})
			}
		}
		for _, id := range otherIDs {
			if e, _ := n.Entities.Get(id); e != nil {
				nodes = append(nodes, nodeOut{ID: e.EntityUID, Kind: e.Type, Title: e.Title, Status: e.Status})
			}
		}

		type edgeOut struct {
			ID     string `json:"id"`
			Source string `json:"source"`
			Target string `json:"target"`
			Kind   string `json:"kind"`
		}
		edges := make([]edgeOut, 0, len(rows))
		seen := make(map[string]bool)
		for _, row := range rows {
			if row.ViaSrc == "" {
				continue
			}
			id := row.ViaSrc + "->" + row.ID + ":" + row.ViaKind
			if seen[id] {
				continue
			}
			seen[id] = true
			edges = append(edges, edgeOut{ID: id, Source: row.ViaSrc, Target: row.ID, Kind: row.ViaKind})
		}

		// Second pass: emit depends_on edges between discovered nodes that the
		// Walk missed because the target was already visited via an owns edge.
		// Build the discovered node set first, then query each node's outgoing
		// depends_on edges and add any that connect two nodes already in the graph.
		if n.Relationships != nil {
			discovered := make(map[string]bool, len(rows)+1)
			discovered[rootID] = true
			for _, row := range rows {
				discovered[row.ID] = true
			}
			for nodeID := range discovered {
				depEdges, _ := n.Relationships.ListOutgoing(r.Context(), nodeID, relationships.EdgeDependsOn)
				for _, de := range depEdges {
					if !discovered[de.DstID] {
						continue
					}
					id := nodeID + "->" + de.DstID + ":" + relationships.EdgeDependsOn
					if seen[id] {
						continue
					}
					seen[id] = true
					edges = append(edges, edgeOut{ID: id, Source: nodeID, Target: de.DstID, Kind: relationships.EdgeDependsOn})
				}
			}
		}

		writeJSON(w, map[string]any{"nodes": nodes, "edges": edges})
	}
}

func splitCSV(s string) []string {
	out := []string{}
	cur := ""
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		cur += string(s[i])
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
