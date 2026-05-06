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
		depth := 10
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
		productIDs := []string{}
		manifestIDs := []string{}
		taskIDs := []string{}
		for _, w := range rows {
			switch w.Kind {
			case relationships.KindProduct:
				productIDs = append(productIDs, w.ID)
			case relationships.KindManifest:
				manifestIDs = append(manifestIDs, w.ID)
			case relationships.KindTask:
				taskIDs = append(taskIDs, w.ID)
			}
		}

		// Resolve title + status per entity. Loops are fine — node count
		// is bounded by walk depth × fan-out (typical: < 200 entities).
		type nodeOut struct {
			ID     string `json:"id"`
			Kind   string `json:"kind"`
			Title  string `json:"title"`
			Status string `json:"status"`
		}
		nodes := make([]nodeOut, 0, len(rows))
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

		// Edges: every WalkRow except the root carries the edge that
		// brought us there (ViaSrc → ID, kind = ViaKind).
		type edgeOut struct {
			ID     string `json:"id"`
			Source string `json:"source"`
			Target string `json:"target"`
			Kind   string `json:"kind"`
		}
		edges := make([]edgeOut, 0, len(rows))
		seen := make(map[string]bool)
		for _, w := range rows {
			if w.ViaSrc == "" {
				continue
			}
			id := w.ViaSrc + "->" + w.ID + ":" + w.ViaKind
			if seen[id] {
				continue
			}
			seen[id] = true
			edges = append(edges, edgeOut{ID: id, Source: w.ViaSrc, Target: w.ID, Kind: w.ViaKind})
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
