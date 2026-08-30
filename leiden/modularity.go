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

package leiden

import (
	"context"
	"fmt"
)

// Modularity computes the generalised Newman modularity Q of a partition on
// a weighted, undirected graph:
//
//	Q = (1/2m) · Σ_{ij} [ A_ij − γ · k_i k_j / 2m ] · δ(c_i, c_j)
//
// where A is the symmetric weighted adjacency matrix, k_i is node i's
// strength (sum of incident edge weights, self-loops counted once), 2m is
// the total node strength, and δ is the Kronecker delta over the partition.
//
// gamma = 1 recovers the classical Newman-Girvan modularity. gamma > 1
// favours smaller clusters; 0 < gamma < 1 favours larger ones.
//
// partition must have length nNodes. Cluster IDs must be non-negative; they
// need not be contiguous. The returned value is in [-1, 1] for connected
// graphs.
//
// Returns 0 with a nil error for the degenerate case of a graph with zero
// total edge weight (no edges and no self-loops).
//
// ctx is checked once before scoring. If it is already cancelled, Modularity
// returns 0 and ctx.Err(). Pass [context.Background] for an uncancelable
// call.
//
// Errors:
//   - [ErrNilContext] if ctx is nil.
//   - [ErrInvalidNodeCount] if nNodes is non-positive.
//   - [ErrAssignmentLength] if len(partition) != nNodes.
//   - [ErrNegativeClusterID] if any partition entry is negative.
//   - [ErrNodeOutOfRange] or [ErrNegativeEdgeWeight] if an edge is invalid.
func Modularity(ctx context.Context, nNodes int, edges []Edge, partition []int, gamma float64) (float64, error) {
	if ctx == nil {
		return 0, ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if len(partition) != nNodes {
		return 0, fmt.Errorf("%w: got %d, want %d", ErrAssignmentLength, len(partition), nNodes)
	}
	net, err := NewCompactNetwork(nNodes, edges)
	if err != nil {
		return 0, err
	}
	cl, err := NewClusteringFromAssignment(partition)
	if err != nil {
		return 0, err
	}
	q := modularityQuality{Gamma: gamma}
	return q.value(net, cl), nil
}
