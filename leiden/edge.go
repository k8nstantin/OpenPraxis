package leiden

// Edge is a weighted, undirected edge between two nodes identified by
// zero-based integer IDs.
//
// Edges are treated as undirected by [CompactNetwork]: the pair (From, To)
// is equivalent to (To, From). Self-loops (From == To) are permitted and
// are stored as a single CSR entry that contributes their Weight once to
// the incident node's strength. See [CompactNetwork.TotalNodeStrength] for
// how this interacts with the 2m modularity normalisation.
//
// Weight must be non-negative; negative weights are rejected when
// constructing a [CompactNetwork].
type Edge struct {
	From   int
	To     int
	Weight float64
}
