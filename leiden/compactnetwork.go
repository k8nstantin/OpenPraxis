package leiden

import "fmt"

// CompactNetwork is an immutable, compressed-sparse-row (CSR) representation
// of a weighted, undirected graph.
//
// The structure is the working graph for the Leiden algorithm: it must be
// cache-friendly to traverse and cheap to copy by reference. Once
// constructed, no public method mutates its state.
//
// Storage layout:
//
//   - Each undirected edge (u, v) with u != v is stored as two directed
//     entries: v in u's neighbor list and u in v's neighbor list, with the
//     same weight.
//   - A self-loop (u, u) is stored as exactly one entry in u's neighbor
//     list with the edge's weight.
//   - Neighbors of node i are stored contiguously in the range
//     [firstNeighborIdx[i], firstNeighborIdx[i+1]).
//
// Node weights default to 1.0 when not supplied. Node strength — the sum of
// weights of incident directed entries — is precomputed on construction.
type CompactNetwork struct {
	nNodes           int
	nEdges           int
	nodeWeights      []float64
	nodeStrengths    []float64
	firstNeighborIdx []int
	neighbors        []int
	neighborWeights  []float64
	totalNodeWeight  float64
	totalEdgeWeight  float64
}

// NewCompactNetwork builds a CompactNetwork over nNodes nodes from the given
// undirected edge list. Each node defaults to unit weight.
//
// Returns an error if nNodes is non-positive, if any edge references a node
// outside [0, nNodes), or if any edge weight is negative.
//
// Edges may include duplicate pairs and self-loops; duplicates are summed
// implicitly because they each produce independent neighbor entries.
func NewCompactNetwork(nNodes int, edges []Edge) (*CompactNetwork, error) {
	return NewCompactNetworkWithNodeWeights(nNodes, nil, edges)
}

// NewCompactNetworkWithNodeWeights builds a CompactNetwork with explicit
// per-node weights. If nodeWeights is nil, each node defaults to weight 1.0.
//
// Returns an error if nNodes is non-positive, if len(nodeWeights) does not
// match nNodes when supplied, if any node weight is negative, if any edge
// references an out-of-range node, or if any edge weight is negative.
func NewCompactNetworkWithNodeWeights(nNodes int, nodeWeights []float64, edges []Edge) (*CompactNetwork, error) {
	if nNodes <= 0 {
		return nil, fmt.Errorf("%w: got %d", ErrInvalidNodeCount, nNodes)
	}

	weights := make([]float64, nNodes)
	if nodeWeights == nil {
		for i := range weights {
			weights[i] = 1.0
		}
	} else {
		if len(nodeWeights) != nNodes {
			return nil, fmt.Errorf("%w: got %d, want %d", ErrNodeWeightsLength, len(nodeWeights), nNodes)
		}
		for i, w := range nodeWeights {
			if w < 0 {
				return nil, fmt.Errorf("%w: node %d weight %g", ErrNegativeNodeWeight, i, w)
			}
			weights[i] = w
		}
	}

	totalNodeWeight := 0.0
	for _, w := range weights {
		totalNodeWeight += w
	}

	// Validate edges and compute degrees in one pass.
	degree := make([]int, nNodes)
	for i, e := range edges {
		if e.From < 0 || e.From >= nNodes {
			return nil, fmt.Errorf("%w: edge %d From=%d, nNodes=%d", ErrNodeOutOfRange, i, e.From, nNodes)
		}
		if e.To < 0 || e.To >= nNodes {
			return nil, fmt.Errorf("%w: edge %d To=%d, nNodes=%d", ErrNodeOutOfRange, i, e.To, nNodes)
		}
		if e.Weight < 0 {
			return nil, fmt.Errorf("%w: edge %d weight %g", ErrNegativeEdgeWeight, i, e.Weight)
		}
		degree[e.From]++
		if e.From != e.To {
			degree[e.To]++
		}
	}

	// CSR offsets via prefix sum.
	firstNeighborIdx := make([]int, nNodes+1)
	for i := 0; i < nNodes; i++ {
		firstNeighborIdx[i+1] = firstNeighborIdx[i] + degree[i]
	}
	totalEntries := firstNeighborIdx[nNodes]
	neighbors := make([]int, totalEntries)
	neighborWeights := make([]float64, totalEntries)

	// Per-node write cursor reusing degree as the running offset.
	cursor := make([]int, nNodes)
	copy(cursor, firstNeighborIdx[:nNodes])

	totalEdgeWeight := 0.0
	for _, e := range edges {
		neighbors[cursor[e.From]] = e.To
		neighborWeights[cursor[e.From]] = e.Weight
		cursor[e.From]++
		if e.From != e.To {
			neighbors[cursor[e.To]] = e.From
			neighborWeights[cursor[e.To]] = e.Weight
			cursor[e.To]++
		}
		totalEdgeWeight += e.Weight
	}

	strengths := make([]float64, nNodes)
	for i := 0; i < nNodes; i++ {
		s := 0.0
		for j := firstNeighborIdx[i]; j < firstNeighborIdx[i+1]; j++ {
			s += neighborWeights[j]
		}
		strengths[i] = s
	}

	return &CompactNetwork{
		nNodes:           nNodes,
		nEdges:           len(edges),
		nodeWeights:      weights,
		nodeStrengths:    strengths,
		firstNeighborIdx: firstNeighborIdx,
		neighbors:        neighbors,
		neighborWeights:  neighborWeights,
		totalNodeWeight:  totalNodeWeight,
		totalEdgeWeight:  totalEdgeWeight,
	}, nil
}

// NumNodes returns the number of nodes in the network.
func (n *CompactNetwork) NumNodes() int { return n.nNodes }

// NumEdges returns the number of input edges used to construct the network,
// counting duplicate pairs and self-loops exactly once each.
func (n *CompactNetwork) NumEdges() int { return n.nEdges }

// NodeWeight returns the weight of the given node. The caller is responsible
// for ensuring 0 <= node < NumNodes().
func (n *CompactNetwork) NodeWeight(node int) float64 { return n.nodeWeights[node] }

// NodeStrength returns the sum of incident edge weights for the given node,
// computed from the neighbor list as stored (self-loops contribute once).
func (n *CompactNetwork) NodeStrength(node int) float64 { return n.nodeStrengths[node] }

// TotalNodeWeight returns the sum of all node weights.
func (n *CompactNetwork) TotalNodeWeight() float64 { return n.totalNodeWeight }

// TotalEdgeWeight returns the sum of all input edge weights, counting each
// undirected edge once.
func (n *CompactNetwork) TotalEdgeWeight() float64 { return n.totalEdgeWeight }

// Degree returns the number of stored neighbor entries for the given node.
// This counts each non-self-loop incident edge once and each incident
// self-loop once.
func (n *CompactNetwork) Degree(node int) int {
	return n.firstNeighborIdx[node+1] - n.firstNeighborIdx[node]
}

// Neighbors returns the neighbor IDs of the given node as a slice view into
// the network's internal storage. The returned slice MUST NOT be mutated;
// it remains valid for the lifetime of the CompactNetwork.
//
// The slice is suitable for indexed iteration alongside [CompactNetwork.NeighborWeights]
// for the same node.
func (n *CompactNetwork) Neighbors(node int) []int {
	return n.neighbors[n.firstNeighborIdx[node]:n.firstNeighborIdx[node+1]]
}

// NeighborWeights returns the per-edge weights for the given node's
// neighbors, parallel to [CompactNetwork.Neighbors]. The returned slice MUST
// NOT be mutated.
func (n *CompactNetwork) NeighborWeights(node int) []float64 {
	return n.neighborWeights[n.firstNeighborIdx[node]:n.firstNeighborIdx[node+1]]
}
