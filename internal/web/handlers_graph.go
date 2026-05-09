package web

import (
	"net/http"
	"strconv"

	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/relationships"
)

// apiRelationshipsGraph returns a flat (nodes, edges) shape over the
// unified relationships SCD-2 table — one canonical source for the
// DAG tab.
//
// Node discovery: Walk() from the root, following edgeKinds.
// Edge rendering: ListOutgoingForMany() on ALL discovered nodes — every
// edge whose src AND dst are both in the discovered set is emitted.
// This ensures depends_on edges between tasks render correctly regardless
// of the order nodes were discovered during the walk.
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
		edgeKinds := []string{relationships.EdgeOwns, relationships.EdgeDependsOn, relationships.EdgeLinksTo, relationships.EdgeReviews}
		if v := r.URL.Query().Get("edge_kinds"); v != "" {
			edgeKinds = splitCSV(v)
		}

		// ── Step 1: discover all reachable nodes via Walk ────────────────────
		walkRows, err := n.Relationships.Walk(r.Context(), rootID, rootKind, edgeKinds, depth)
		if err != nil {
			writeError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Build the full set of discovered node IDs (root + all walk results).
		nodeSet := make(map[string]bool, len(walkRows)+1)
		nodeSet[rootID] = true
		for _, row := range walkRows {
			nodeSet[row.ID] = true
		}

		// Bucket node IDs by kind for entity title/status resolution.
		skillIDs := []string{}
		productIDs := []string{}
		manifestIDs := []string{}
		taskIDs := []string{}
		otherIDs := []string{}
		for _, row := range walkRows {
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

		// ── Step 2: resolve nodes ─────────────────────────────────────────────
		type nodeOut struct {
			ID     string `json:"id"`
			Kind   string `json:"kind"`
			Title  string `json:"title"`
			Status string `json:"status"`
		}
		nodes := make([]nodeOut, 0, len(walkRows)+2)
		seenNodes := make(map[string]bool)

		addNode := func(id, kind, title, status string) {
			if seenNodes[id] {
				return
			}
			seenNodes[id] = true
			nodes = append(nodes, nodeOut{ID: id, Kind: kind, Title: title, Status: status})
		}

		// Root node itself.
		if root, _ := n.Entities.Get(rootID); root != nil {
			addNode(root.EntityUID, root.Type, root.Title, root.Status)
		}


		for _, id := range skillIDs {
			if e, _ := n.Entities.Get(id); e != nil {
				addNode(e.EntityUID, relationships.KindSkill, e.Title, e.Status)
			}
		}
		for _, id := range productIDs {
			if e, _ := n.Entities.Get(id); e != nil {
				addNode(e.EntityUID, relationships.KindProduct, e.Title, e.Status)
			}
		}
		for _, id := range manifestIDs {
			if e, _ := n.Entities.Get(id); e != nil {
				addNode(e.EntityUID, relationships.KindManifest, e.Title, e.Status)
			}
		}
		for _, id := range taskIDs {
			if e, _ := n.Entities.Get(id); e != nil {
				addNode(e.EntityUID, relationships.KindTask, e.Title, e.Status)
			}
		}
		for _, id := range otherIDs {
			if e, _ := n.Entities.Get(id); e != nil {
				addNode(e.EntityUID, e.Type, e.Title, e.Status)
			}
		}

		// ── Step 3: expand nodeSet with incoming neighbours ──────────────────
		// The Walk CTE only follows outgoing edges. Pull in nodes that point
		// TO anything already discovered so bidirectional links (links_to,
		// reviews) appear regardless of which end the viewer starts from.
		allNodeIDs := make([]string, 0, len(nodeSet))
		for id := range nodeSet {
			allNodeIDs = append(allNodeIDs, id)
		}
		incomingEdges, _ := n.Relationships.ListIncomingForMany(r.Context(), allNodeIDs, "")
		for dstID, dstEdges := range incomingEdges {
			_ = dstID
			for _, e := range dstEdges {
				if nodeSet[e.SrcID] {
					continue
				}
				nodeSet[e.SrcID] = true
				if src, _ := n.Entities.Get(e.SrcID); src != nil {
					addNode(src.EntityUID, src.Type, src.Title, src.Status)
				}
			}
		}

		// Rebuild allNodeIDs after expansion.
		allNodeIDs = allNodeIDs[:0]
		for id := range nodeSet {
			allNodeIDs = append(allNodeIDs, id)
		}

		// ── Step 4: render ALL edges between discovered nodes ─────────────────
		type edgeOut struct {
			ID     string `json:"id"`
			Source string `json:"source"`
			Target string `json:"target"`
			Kind   string `json:"kind"`
		}
		edges := make([]edgeOut, 0, len(nodeSet)*2)

		allEdges, _ := n.Relationships.ListOutgoingForMany(r.Context(), allNodeIDs, "")
		seenEdges := make(map[string]bool)
		for srcID, srcEdges := range allEdges {
			for _, e := range srcEdges {
				if !nodeSet[e.DstID] {
					continue
				}
				id := srcID + "->" + e.DstID + ":" + e.Kind
				if seenEdges[id] {
					continue
				}
				seenEdges[id] = true
				edges = append(edges, edgeOut{ID: id, Source: srcID, Target: e.DstID, Kind: e.Kind})
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
