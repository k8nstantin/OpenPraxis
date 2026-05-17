package leiden

// aggregateNetwork builds the next coarsening level of the Leiden algorithm.
//
// Each cluster of `refined` becomes one node in the returned network:
//
//   - The new node's weight is the sum of [CompactNetwork.NodeWeight] over its
//     members.
//   - Edges between original nodes within the same refined cluster contribute
//     to a self-loop on the corresponding new node, preserving the cluster's
//     internal edge weight (each undirected edge counted once).
//   - Edges between original nodes in different refined clusters become a
//     weighted edge between the corresponding new nodes; multi-edges are
//     summed.
//
// `aggInit` is the initial clustering for the next algorithm iteration: two
// new nodes (refined clusters) are placed in the same bucket iff their
// original members were in the same cluster of `parent`. This is the
// invariant that distinguishes Leiden from Louvain — the next local-move
// phase resumes from the (pre-refinement) partition.
//
// `refined` is normalized (cluster IDs renumbered into a contiguous range)
// as a side effect.
//
// With this construction, the CPM quality of `parent` on `net` equals the CPM
// quality of `aggInit` on the returned aggNet, so the algorithm's monotone
// improvement property holds across coarsening levels.
//
// Errors:
//   - [ErrInvalidNodeCount] if `refined` or `net` is nil or has zero nodes.
func aggregateNetwork(net *CompactNetwork, refined, parent *Clustering) (aggNet *CompactNetwork, aggInit *Clustering, err error) {
	if net == nil || refined == nil || parent == nil {
		return nil, nil, ErrInvalidNodeCount
	}
	if net.nNodes == 0 || refined.nNodes == 0 || parent.nNodes == 0 {
		return nil, nil, ErrInvalidNodeCount
	}
	refined.Normalize()
	k := refined.nClusters

	newWeights := make([]float64, k)
	for u := 0; u < net.nNodes; u++ {
		newWeights[refined.assignment[u]] += net.nodeWeights[u]
	}

	// Accumulators. Each undirected edge in the original network is stored
	// twice in CSR (once on each endpoint), so we count only the src <= dst
	// direction. Self-loops appear once (src == dst) and naturally fall in
	// the same-cluster branch.
	selfLoop := make([]float64, k)
	type pair struct{ i, j int }
	cross := make(map[pair]float64)

	for src := 0; src < net.nNodes; src++ {
		i := refined.assignment[src]
		nbrs := net.Neighbors(src)
		ws := net.NeighborWeights(src)
		for idx, dst := range nbrs {
			if src > dst {
				continue
			}
			j := refined.assignment[dst]
			w := ws[idx]
			if i == j {
				selfLoop[i] += w
				continue
			}
			a, b := i, j
			if a > b {
				a, b = b, a
			}
			cross[pair{a, b}] += w
		}
	}

	edges := make([]Edge, 0, len(cross)+k)
	for i, w := range selfLoop {
		if w > 0 {
			edges = append(edges, Edge{From: i, To: i, Weight: w})
		}
	}
	for p, w := range cross {
		edges = append(edges, Edge{From: p.i, To: p.j, Weight: w})
	}

	aggNet, err = NewCompactNetworkWithNodeWeights(k, newWeights, edges)
	if err != nil {
		return nil, nil, err
	}

	// New-node i (refined cluster i) inherits its parent cluster ID. All
	// members of a refined cluster share the same parent cluster — refinement
	// only merges within a parent cluster — so taking the first-seen member
	// is well-defined.
	aggAssign := make([]int, k)
	seen := make([]bool, k)
	for u := 0; u < net.nNodes; u++ {
		ri := refined.assignment[u]
		if seen[ri] {
			continue
		}
		seen[ri] = true
		aggAssign[ri] = parent.assignment[u]
	}
	aggInit, err = NewClusteringFromAssignment(aggAssign)
	if err != nil {
		return nil, nil, err
	}
	return aggNet, aggInit, nil
}
