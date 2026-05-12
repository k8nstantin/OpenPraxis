// Package leiden provides a pure-Go implementation of the Leiden algorithm for
// community detection in weighted, undirected graphs.
//
// The Leiden algorithm (Traag, Waltman, van Eck, 2019) refines the Louvain
// method to guarantee well-connected communities. This package targets the
// same primitives as the reference implementation in networkanalysis-java
// (CWTS) and leidenalg (Python), while remaining idiomatic Go with zero
// external dependencies.
//
// The core data structures are:
//
//   - [Edge]: an undirected, weighted edge used as input.
//   - [CompactNetwork]: an immutable CSR-style representation of the graph.
//   - [Clustering]: a partition of nodes into clusters.
//
// The user-facing entry points are:
//
//   - [Options] and [DefaultOptions]: forward-compatible configuration.
//   - [Leiden]: run the algorithm to convergence and return a flat [Result].
//   - [HierarchicalLeiden]: return the partition at every coarsening level
//     as a [HierarchicalResult].
//   - [Modularity]: score an arbitrary partition with the generalised
//     Newman modularity, independent of the algorithm itself.
//   - [CommunityCount] and [GroupBy]: post-process a partition slice
//     without constructing a [Clustering].
//
// All long-running entry points accept a [context.Context]; pass
// [context.Background] when cancellation is not needed.
package leiden
