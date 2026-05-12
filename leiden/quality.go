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

// modularityQuality is the Newman-Girvan modularity with a generalised
// resolution parameter (Reichardt-Bornholdt, Arenas et al.):
//
//	Q = (1/2m) Σ_{ij} [ A_ij − γ · (k_i · k_j) / (2m) ] · δ(c_i, c_j)
//	  = (1/2m) [ Σ_c (2·e_c^≠ + e_c^◦) − (γ/2m) · Σ_c K_c² ]
//
// where A is the (symmetric) weighted adjacency matrix, k_i is node i's
// strength, 2m = Σ_i k_i, e_c^≠ is the sum of weights of non-self-loop
// edges with both endpoints in cluster c (counted once per edge), e_c^◦
// is the sum of weights of self-loops in c (counted once), and K_c is
// the sum of strengths over c. γ is the resolution parameter — larger γ
// favours smaller clusters; γ = 1 recovers classic modularity.
//
// The node mass used by [qualityFunction] is the node's strength, so the
// running clusterMass maintained by the algorithm phases is K_c.
//
// modularityQuality is not invariant under [aggregateNetwork] because the
// aggregation stores each internal edge once as a self-loop (the convention
// that preserves [cpmQuality]). Carrying modularity across coarsening
// levels therefore requires the algorithm to maintain the original-level
// node strengths through aggregation; that is the algorithm runner's
// responsibility, not this quality function's.
type modularityQuality struct {
	Gamma float64
}

func (q modularityQuality) value(net *CompactNetwork, cl *Clustering) float64 {
	twoM := net.totalNodeStrength
	if twoM == 0 {
		return 0
	}
	// Σ over CSR entries in same cluster: counts each internal non-self-loop
	// edge twice (u→v and v→u) and each internal self-loop once.
	var internal float64
	for u := 0; u < net.nNodes; u++ {
		cu := cl.assignment[u]
		nbrs := net.Neighbors(u)
		ws := net.NeighborWeights(u)
		for i, v := range nbrs {
			if cl.assignment[v] != cu {
				continue
			}
			internal += ws[i]
		}
	}
	masses := make([]float64, cl.nClusters)
	for u := 0; u < net.nNodes; u++ {
		masses[cl.assignment[u]] += net.nodeStrengths[u]
	}
	var penalty float64
	for _, k := range masses {
		penalty += k * k
	}
	return (internal - q.Gamma*penalty/twoM) / twoM
}

func (q modularityQuality) moveDelta(net *CompactNetwork, u, from, to int, wToFrom, wToTo float64, clusterMass []float64) float64 {
	if from == to {
		return 0
	}
	twoM := net.totalNodeStrength
	if twoM == 0 {
		return 0
	}
	ku := net.nodeStrengths[u]
	// Δ internal-CSR-sum: each non-self-loop edge from u contributes 2·w
	// because it appears in u's CSR and the neighbour's CSR. u's self-loop
	// stays with u and contributes 0 change.
	deltaInternal := 2.0 * (wToTo - wToFrom)
	// Δ Σ_c K_c² = (K_from − k_u)² + (K_to + k_u)² − K_from² − K_to²
	//            = 2·k_u·(K_to − K_from + k_u).
	// clusterMass[from] still includes u; clusterMass[to] does not.
	deltaPenalty := 2.0 * ku * (clusterMass[to] - clusterMass[from] + ku)
	return (deltaInternal - q.Gamma*deltaPenalty/twoM) / twoM
}

func (q modularityQuality) nodeMass(net *CompactNetwork, u int) float64 {
	return net.nodeStrengths[u]
}

func (q modularityQuality) resolution() float64 {
	return q.Gamma
}
