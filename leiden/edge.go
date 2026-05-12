package leiden

// Edge is a weighted, undirected edge between two nodes identified by
// zero-based integer IDs.
//
// Edges are treated as undirected by [CompactNetwork]: the pair (From, To)
// is equivalent to (To, From). Self-loops (From == To) are permitted and
// contribute twice their Weight to the incident node's strength, matching
// the convention used by the Leiden reference implementation.
//
// Weight must be non-negative; negative weights are rejected when
// constructing a [CompactNetwork].
type Edge struct {
	From   int
	To     int
	Weight float64
}
