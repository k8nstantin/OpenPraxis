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
	"math"
	"math/rand"
	"sort"
)

// runRefinement performs the Leiden refinement phase on the partition
// produced by [runLocalMove].
//
// The refinement starts from singletons (each node in its own R-cluster)
// and only merges singleton nodes into existing R-clusters; established
// R-clusters are never split or relocated. A merge is gated by two
// well-connectedness conditions (per Traag, Waltman & van Eck, 2019):
//
//  1. The singleton u must be well-connected to its parent P-cluster:
//
//     Σ_{v ∈ P(u), v ≠ u} A_uv  ≥  γ · w_u · (w_{P(u)} − w_u)
//
//  2. The candidate R-cluster t (within the same P-cluster) must itself be
//     well-connected to the rest of the P-cluster:
//
//     Σ_{x ∈ t, y ∈ P(u)\t} A_xy  ≥  γ · w_t · (w_{P(u)} − w_t)
//
// Only candidates with ΔH > 0 are considered. Selection among them is
// controlled by theta:
//
//   - theta ≤ 0 : deterministic argmax over ΔH.
//   - theta > 0 : sample with probability ∝ exp(ΔH / theta) (Traag-style
//     refinement randomisation; smaller theta sharpens toward argmax).
//
// Returns a freshly allocated refined [Clustering]. parent is not mutated.
//
// rng is used both to shuffle the visit order and to sample candidates when
// theta > 0; deterministic seeding yields deterministic output.
func runRefinement(net *CompactNetwork, parent *Clustering, q qualityFunction, theta float64, rng *rand.Rand) *Clustering {
	n := net.nNodes
	gamma := q.resolution()

	// rAssign[u] is u's current R-cluster ID. We use the node ID itself as
	// the initial singleton ID so an R-cluster has a stable representative
	// even after dissolution (the singleton key is never reused).
	rAssign := make([]int, n)
	for i := range rAssign {
		rAssign[i] = i
	}

	// Per-R-cluster mass and external-to-parent-cluster weight.
	rMass := make([]float64, n)
	rExt := make([]float64, n)

	// Per-parent-cluster total mass.
	pMass := make([]float64, parent.nClusters)

	// Per-node weight of edges to other members of the same parent cluster.
	extToP := make([]float64, n)

	for u := 0; u < n; u++ {
		m := q.nodeMass(net, u)
		rMass[u] = m
		pMass[parent.assignment[u]] += m
	}
	for u := 0; u < n; u++ {
		nbrs := net.Neighbors(u)
		ws := net.NeighborWeights(u)
		pu := parent.assignment[u]
		for i, v := range nbrs {
			if v == u {
				continue
			}
			if parent.assignment[v] == pu {
				extToP[u] += ws[i]
			}
		}
		// Singleton: external-to-parent equals the node's parent-internal
		// neighbour weight.
		rExt[u] = extToP[u]
	}

	// Workspace, reused across iterations.
	edgeToR := make(map[int]float64)
	var candidates []cand

	perm := rng.Perm(n)
	for _, u := range perm {
		// Established (non-singleton) R-clusters are frozen.
		if rAssign[u] != u {
			continue
		}
		wu := q.nodeMass(net, u)
		pu := parent.assignment[u]
		if extToP[u] < gamma*wu*(pMass[pu]-wu) {
			continue
		}

		for k := range edgeToR {
			delete(edgeToR, k)
		}
		nbrs := net.Neighbors(u)
		ws := net.NeighborWeights(u)
		for i, v := range nbrs {
			if v == u || parent.assignment[v] != pu {
				continue
			}
			edgeToR[rAssign[v]] += ws[i]
		}

		candidates = candidates[:0]
		for t, wToT := range edgeToR {
			if t == u {
				continue
			}
			if rExt[t] < gamma*rMass[t]*(pMass[pu]-rMass[t]) {
				continue
			}
			// CPM ΔH for merging singleton {u} into R-cluster t (sizes are:
			// wFrom = wu, wTo = rMass[t], wToFrom = 0):
			//   ΔH = wToT − γ · wu · rMass[t]
			d := wToT - gamma*wu*rMass[t]
			if d <= 0 {
				continue
			}
			candidates = append(candidates, cand{id: t, delta: d, wToT: wToT})
		}
		if len(candidates) == 0 {
			continue
		}
		// edgeToR is a Go map, so iteration order above is randomised on
		// every loop. Sort by R-cluster id so argmax tiebreaking and
		// exponential sampling are reproducible for a given rng seed.
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].id < candidates[j].id
		})

		var pick cand
		if theta <= 0 {
			pick = candidates[0]
			for _, c := range candidates[1:] {
				if c.delta > pick.delta {
					pick = c
				}
			}
		} else {
			pick = sampleByExp(candidates, theta, rng)
		}

		rAssign[u] = pick.id
		rMass[pick.id] += wu
		rMass[u] = 0
		// Δ rExt[t] = extToP[u] − 2·wToT : u's edges to nodes in P\t become
		// either internal-to-t (the wToT going-to-t edges) or external-to-t
		// from t's perspective (now that u ∈ t). The 2× accounts for the
		// fact that the wToT edges were also previously counted in t's
		// external weight from t's side.
		rExt[pick.id] += extToP[u] - 2*pick.wToT
		rExt[u] = 0
	}

	cl, _ := NewClusteringFromAssignment(rAssign)
	return cl
}

// sampleByExp picks a candidate with probability proportional to
// exp(delta / theta), using max-shift for numerical stability so very large
// deltas don't overflow exp.
func sampleByExp(cs []cand, theta float64, rng *rand.Rand) cand {
	maxD := cs[0].delta
	for _, c := range cs[1:] {
		if c.delta > maxD {
			maxD = c.delta
		}
	}
	weights := make([]float64, len(cs))
	var sum float64
	for i, c := range cs {
		weights[i] = math.Exp((c.delta - maxD) / theta)
		sum += weights[i]
	}
	target := rng.Float64() * sum
	var acc float64
	for i, c := range cs {
		acc += weights[i]
		if target <= acc {
			return c
		}
	}
	return cs[len(cs)-1]
}

// cand is the unit of choice for refinement; declared at package scope so
// sampleByExp can take it by slice without redeclaring the type inside the
// runRefinement closure.
type cand struct {
	id    int
	delta float64
	wToT  float64
}
