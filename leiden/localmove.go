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

import "math/rand"

// runLocalMove performs the Louvain-style local-moving phase: nodes are
// visited in shuffled order, and each is greedily reassigned to the
// neighbouring cluster offering the largest positive Δquality. When a node
// moves, its still-pending neighbours are re-enqueued. The loop terminates
// when the queue is drained — i.e. every node has been visited at least once
// since its last move and chose not to move.
//
// cl is mutated in place; its node-to-cluster assignments are updated but
// NumClusters never grows (local-move never creates new clusters).
//
// Returns the total number of moves performed.
//
// Pre-conditions:
//   - cl.NumNodes() == net.NumNodes()
//   - cl.NumClusters() correctly covers all cluster IDs that appear in the
//     assignment (the standard invariant of [NewClusteringFromAssignment]).
//
// rng is used only to shuffle the initial visit order; passing
// rand.New(rand.NewSource(seed)) gives deterministic runs.
func runLocalMove(net *CompactNetwork, cl *Clustering, q qualityFunction, rng *rand.Rand) int {
	n := net.nNodes
	if n == 0 {
		return 0
	}

	clusterMass := make([]float64, cl.nClusters)
	for u := 0; u < n; u++ {
		clusterMass[cl.assignment[u]] += q.nodeMass(net, u)
	}

	// Circular FIFO of pending nodes. inQueue dedups so the queue never
	// exceeds n elements.
	qbuf := make([]int, n)
	inQueue := make([]bool, n)
	perm := rng.Perm(n)
	copy(qbuf, perm)
	for _, p := range perm {
		inQueue[p] = true
	}
	head, qsize := 0, n

	// Workspace map: edge weight from u to each neighbouring cluster, keyed
	// by cluster ID. Reset per node by deletion (cheaper than reallocating
	// for sparse touches).
	edgeToCluster := make(map[int]float64)

	moves := 0
	for qsize > 0 {
		u := qbuf[head]
		head = (head + 1) % n
		qsize--
		inQueue[u] = false

		for k := range edgeToCluster {
			delete(edgeToCluster, k)
		}
		nbrs := net.Neighbors(u)
		ws := net.NeighborWeights(u)
		for i, v := range nbrs {
			if v == u {
				continue
			}
			edgeToCluster[cl.assignment[v]] += ws[i]
		}

		curr := cl.assignment[u]
		wToCurr := edgeToCluster[curr]

		bestCluster := curr
		bestDelta := 0.0
		for c, wToC := range edgeToCluster {
			if c == curr {
				continue
			}
			d := q.moveDelta(net, u, curr, c, wToCurr, wToC, clusterMass)
			if d > bestDelta {
				bestDelta = d
				bestCluster = c
			}
		}
		if bestCluster == curr {
			continue
		}

		mu := q.nodeMass(net, u)
		clusterMass[curr] -= mu
		clusterMass[bestCluster] += mu
		cl.assignment[u] = bestCluster
		moves++

		// Re-enqueue not-already-queued neighbours that are not already in
		// the new cluster: their best-move evaluation may have changed.
		for _, v := range nbrs {
			if v == u || inQueue[v] {
				continue
			}
			if cl.assignment[v] == bestCluster {
				continue
			}
			tail := (head + qsize) % n
			qbuf[tail] = v
			qsize++
			inQueue[v] = true
		}
	}
	return moves
}
