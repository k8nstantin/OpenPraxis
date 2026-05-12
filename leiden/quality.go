package leiden

// qualityFunction abstracts the optimisation objective used by the Leiden
// algorithm phases. Implementations evaluate the absolute quality of a
// clustering on a network and the change in quality for a proposed single-node
// move.
//
// Positive deltas are improvements; the algorithm only commits moves with
// delta > 0.
//
// All implementations must produce the same value invariant under
// aggregation: the quality of a partition on the original network equals the
// quality of the corresponding partition on the aggregated network produced
// by [aggregateNetwork].
type qualityFunction interface {
	// value returns H(P) for clustering cl over net.
	value(net *CompactNetwork, cl *Clustering) float64

	// moveDelta returns ΔH for moving node u from cluster `from` to cluster
	// `to`, given precomputed:
	//
	//   wToFrom = Σ_{v ∈ from, v ≠ u} A_uv   (edges from u to other from-members)
	//   wToTo   = Σ_{v ∈ to}          A_uv   (edges from u to to-members; u ∉ to)
	//
	// clusterMass[c] is the total nodeMass currently assigned to cluster c,
	// with u still in `from`. Implementations must not retain references to
	// clusterMass.
	moveDelta(net *CompactNetwork, u, from, to int, wToFrom, wToTo float64, clusterMass []float64) float64

	// nodeMass returns the contribution of node u to its cluster's mass for
	// this quality function. The local-move and refinement phases maintain
	// clusterMass as the running sum of nodeMass per cluster.
	nodeMass(net *CompactNetwork, u int) float64

	// resolution returns the γ parameter, used by the refinement phase to
	// gate well-connectedness of singletons and candidate sub-clusters.
	resolution() float64
}

// cpmQuality is the Constant Potts Model (CPM) quality function recommended
// by Traag et al. for Leiden:
//
//	H(P) = Σ_c [ e_c − (γ/2) · w_c² ]
//
// where e_c is the total weight of edges with both endpoints in cluster c
// (each undirected edge counted once; self-loops counted once), and w_c is
// the sum of node weights in c.
//
// γ is the resolution parameter: larger γ favours smaller clusters. The
// formula is invariant under [aggregateNetwork], so iteration across
// coarsening levels preserves quality.
type cpmQuality struct {
	Gamma float64
}

func (q cpmQuality) value(net *CompactNetwork, cl *Clustering) float64 {
	var internal float64
	for u := 0; u < net.nNodes; u++ {
		cu := cl.assignment[u]
		nbrs := net.Neighbors(u)
		ws := net.NeighborWeights(u)
		for i, v := range nbrs {
			if cl.assignment[v] != cu {
				continue
			}
			// Each non-self edge is stored twice in CSR (u→v and v→u); halve
			// to count once. Self-loops are stored once and counted as-is.
			if v == u {
				internal += ws[i]
			} else {
				internal += ws[i] / 2.0
			}
		}
	}
	masses := make([]float64, cl.nClusters)
	for u := 0; u < net.nNodes; u++ {
		masses[cl.assignment[u]] += net.nodeWeights[u]
	}
	var penalty float64
	for _, w := range masses {
		penalty += w * w
	}
	return internal - q.Gamma*penalty/2.0
}

func (q cpmQuality) moveDelta(net *CompactNetwork, u, from, to int, wToFrom, wToTo float64, clusterMass []float64) float64 {
	if from == to {
		return 0
	}
	wu := net.nodeWeights[u]
	// Edge term: each side of the inequality counts non-self-loop weights once
	// (the caller's accumulator is over the directed CSR entries from u, so
	// the self-loop is excluded by construction and cancels between from/to).
	deltaEdge := wToTo - wToFrom
	// Penalty term: Δ(w_from² + w_to²) = 2·w_u·(w_to − w_from + w_u),
	// scaled by γ/2 gives γ·w_u·(w_to − w_from + w_u). clusterMass[from]
	// still includes u, clusterMass[to] does not.
	deltaPenalty := q.Gamma * wu * (clusterMass[to] - clusterMass[from] + wu)
	return deltaEdge - deltaPenalty
}

func (q cpmQuality) nodeMass(net *CompactNetwork, u int) float64 {
	return net.nodeWeights[u]
}

func (q cpmQuality) resolution() float64 {
	return q.Gamma
}
