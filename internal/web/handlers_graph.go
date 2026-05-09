package web

import (
	"net/http"
	"strconv"

	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/relationships"
)

// apiRelationshipsGraph returns a flat (nodes, edges) shape rooted at any
// entity. It does not care about entity types — it walks whatever is in the
// relationships table and renders it.
//
// GET /api/relationships/graph?root_id=X&root_kind=Y&depth=10
//
// Response:
//
//	{
//	  "nodes": [{"id":"...", "kind":"...", "title":"...", "status":"..."}],
//	  "edges": [{"id":"src->dst:kind", "source":"...", "target":"...", "kind":"..."}]
//	}
func apiRelationshipsGraph(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rootID := r.URL.Query().Get("root_id")
		rootKind := r.URL.Query().Get("root_kind")
		if rootID == "" || rootKind == "" {
			writeError(w, "root_id and root_kind required", http.StatusBadRequest)
			return
		}

		depth := 10
		if v := r.URL.Query().Get("depth"); v != "" {
			if d, err := strconv.Atoi(v); err == nil && d > 0 {
				depth = d
			}
		}

		// All edge kinds by default — the DAG renders whatever is in the table.
		edgeKinds := relationships.AllEdgeKinds()
		if v := r.URL.Query().Get("edge_kinds"); v != "" {
			edgeKinds = splitCSV(v)
		}

		// ── Step 1: walk outgoing edges from root ────────────────────────────
		walkRows, err := n.Relationships.Walk(r.Context(), rootID, rootKind, edgeKinds, depth)
		if err != nil {
			writeError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		nodeSet := make(map[string]bool, len(walkRows)+1)
		nodeSet[rootID] = true
		for _, row := range walkRows {
			nodeSet[row.ID] = true
		}

		// ── Step 2: resolve nodes — type-agnostic ────────────────────────────
		type nodeOut struct {
			ID     string `json:"id"`
			Kind   string `json:"kind"`
			Title  string `json:"title"`
			Status string `json:"status"`
		}
		nodes := make([]nodeOut, 0, len(nodeSet))
		seenNodes := make(map[string]bool)

		addNode := func(id string) {
			if seenNodes[id] {
				return
			}
			seenNodes[id] = true
			if e, _ := n.Entities.Get(id); e != nil {
				nodes = append(nodes, nodeOut{ID: e.EntityUID, Kind: e.Type, Title: e.Title, Status: e.Status})
			}
		}

		for id := range nodeSet {
			addNode(id)
		}

		// ── Step 3: expand with incoming neighbours ──────────────────────────
		// Walk only follows outgoing edges. Pull in nodes that point TO any
		// discovered node so the graph is bidirectional.
		allNodeIDs := make([]string, 0, len(nodeSet))
		for id := range nodeSet {
			allNodeIDs = append(allNodeIDs, id)
		}
		incomingEdges, _ := n.Relationships.ListIncomingForMany(r.Context(), allNodeIDs, "")
		for _, srcEdges := range incomingEdges {
			for _, e := range srcEdges {
				if nodeSet[e.SrcID] {
					continue
				}
				nodeSet[e.SrcID] = true
				addNode(e.SrcID)
			}
		}

		// ── Step 4: emit all edges between discovered nodes ──────────────────
		allNodeIDs = allNodeIDs[:0]
		for id := range nodeSet {
			allNodeIDs = append(allNodeIDs, id)
		}

		type edgeOut struct {
			ID     string `json:"id"`
			Source string `json:"source"`
			Target string `json:"target"`
			Kind   string `json:"kind"`
		}
		edges := make([]edgeOut, 0, len(nodeSet)*2)
		seenEdges := make(map[string]bool)

		allEdges, _ := n.Relationships.ListOutgoingForMany(r.Context(), allNodeIDs, "")
		for srcID, srcEdges := range allEdges {
			for _, e := range srcEdges {
				if !nodeSet[e.DstID] {
					continue
				}
				eid := srcID + "->" + e.DstID + ":" + e.Kind
				if seenEdges[eid] {
					continue
				}
				seenEdges[eid] = true
				edges = append(edges, edgeOut{ID: eid, Source: srcID, Target: e.DstID, Kind: e.Kind})
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
