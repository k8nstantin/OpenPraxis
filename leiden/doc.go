// Copyright 2026 Constantin Alexander
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
