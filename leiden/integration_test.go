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
	"math/rand"
	"sort"
	"strconv"
	"testing"
)

// TestIntegration_KarateClub_TwoLeadersSeparated runs Leiden on Zachary's
// karate club at a moderate CPM resolution and confirms the algorithm
// separates the two factional leaders (node 0 — Mr. Hi, node 33 — the
// officer) into different clusters. Any reasonable community detection
// algorithm on this canonical input must satisfy this property.
func TestIntegration_KarateClub_TwoLeadersSeparated(t *testing.T) {
	n, edges := mustLoadEdges(t, "testdata/karate.edges")

	for _, gamma := range []float64{0.05, 0.1, 0.2, 0.5} {
		t.Run("resolution="+ftoa(gamma), func(t *testing.T) {
			opts := DefaultOptions()
			opts.Resolution = gamma
			opts.Seed = 1
			res, err := Leiden(context.Background(), n, edges, opts)
			if err != nil {
				t.Fatalf("Leiden: %v", err)
			}
			if res.NumClusters < 2 {
				t.Errorf("found %d clusters, want ≥2", res.NumClusters)
			}
			if res.Partition[0] == res.Partition[33] {
				t.Errorf("nodes 0 and 33 ended up in the same cluster (%d); "+
					"the karate leaders must be separated", res.Partition[0])
			}
		})
	}
}

// TestIntegration_KarateClub_ModularityIsHigh checks that Leiden's
// partition on karate club achieves classical Newman modularity above
// 0.35, the threshold any competent community-detection algorithm should
// clear on this graph. graspologic-native's reference implementation
// reports Q ≈ 0.42 with 4 communities; we accept a slightly weaker bound
// because the optimum depends on the chosen resolution.
func TestIntegration_KarateClub_ModularityIsHigh(t *testing.T) {
	n, edges := mustLoadEdges(t, "testdata/karate.edges")

	bestQ := -1.0
	bestK := 0
	for _, gamma := range []float64{0.05, 0.1, 0.2, 0.3, 0.5} {
		opts := DefaultOptions()
		opts.Resolution = gamma
		opts.Seed = 1
		res, err := Leiden(context.Background(), n, edges, opts)
		if err != nil {
			t.Fatalf("Leiden(γ=%g): %v", gamma, err)
		}
		mod, err := Modularity(context.Background(), n, edges, res.Partition, 1.0)
		if err != nil {
			t.Fatalf("Modularity(γ=%g): %v", gamma, err)
		}
		t.Logf("γ=%g  k=%d  Q=%.4f", gamma, res.NumClusters, mod)
		if mod > bestQ {
			bestQ = mod
			bestK = res.NumClusters
		}
	}
	if bestQ < 0.35 {
		t.Errorf("best Q across resolutions = %.4f, want ≥0.35", bestQ)
	}
	if bestK < 2 {
		t.Errorf("best partition had %d clusters, want ≥2", bestK)
	}
}

// TestIntegration_KarateClub_RecoversGroundTruth verifies that at a
// resolution biased toward a coarse 2-community partition, Leiden's
// recovered partition has high agreement (ARI ≥ 0.5) with Zachary's
// ground-truth Mr. Hi / Officer split.
func TestIntegration_KarateClub_RecoversGroundTruth(t *testing.T) {
	n, edges := mustLoadEdges(t, "testdata/karate.edges")
	truth := mustLoadPartition(t, "testdata/karate_ground_truth.partition")

	bestARI := -2.0
	for _, gamma := range []float64{0.02, 0.05, 0.08, 0.1} {
		opts := DefaultOptions()
		opts.Resolution = gamma
		opts.Seed = 1
		res, err := Leiden(context.Background(), n, edges, opts)
		if err != nil {
			t.Fatalf("Leiden(γ=%g): %v", gamma, err)
		}
		ari := adjustedRandIndex(res.Partition, truth)
		t.Logf("γ=%g  k=%d  ARI=%.4f", gamma, res.NumClusters, ari)
		if ari > bestARI {
			bestARI = ari
		}
	}
	if bestARI < 0.5 {
		t.Errorf("best ARI vs Zachary ground truth = %.4f, want ≥0.5", bestARI)
	}
}

// TestIntegration_PlantedPartition_HighRecovery generates a stochastic
// block model with 4 well-separated planted communities and asserts that
// Leiden recovers them with ARI ≥ 0.90.
func TestIntegration_PlantedPartition_HighRecovery(t *testing.T) {
	rng := rand.New(rand.NewSource(2026))
	n, edges, truth := plantedPartition(4, 40, 0.6, 0.02, rng)

	opts := DefaultOptions()
	opts.Resolution = 0.1
	opts.Seed = 11
	res, err := Leiden(context.Background(), n, edges, opts)
	if err != nil {
		t.Fatalf("Leiden: %v", err)
	}
	ari := adjustedRandIndex(res.Partition, truth)
	nmi := normalizedMutualInformation(res.Partition, truth)
	t.Logf("planted partition recovery: n=%d  edges=%d  k_found=%d  ARI=%.4f  NMI=%.4f",
		n, len(edges), res.NumClusters, ari, nmi)
	if ari < 0.90 {
		t.Errorf("ARI=%.4f, want ≥0.90 on well-separated planted partition", ari)
	}
	if nmi < 0.85 {
		t.Errorf("NMI=%.4f, want ≥0.85 on well-separated planted partition", nmi)
	}
	if res.NumClusters < 4 {
		t.Errorf("recovered %d clusters, want ≥4 planted communities", res.NumClusters)
	}
}

// TestIntegration_PlantedPartition_HierarchicalLevels confirms that
// HierarchicalLeiden on a planted partition emits at least one level and
// that consecutive levels are a coarsening sequence (clusters never split).
func TestIntegration_PlantedPartition_HierarchicalLevels(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	n, edges, truth := plantedPartition(3, 30, 0.5, 0.03, rng)

	opts := DefaultOptions()
	opts.Resolution = 0.1
	opts.Seed = 7
	res, err := HierarchicalLeiden(context.Background(), n, edges, opts)
	if err != nil {
		t.Fatalf("HierarchicalLeiden: %v", err)
	}
	if len(res.Levels) == 0 {
		t.Fatal("no levels emitted")
	}
	for k := 0; k+1 < len(res.Levels); k++ {
		for u := 0; u < n; u++ {
			for v := u + 1; v < n; v++ {
				if res.Levels[k].Partition[u] == res.Levels[k].Partition[v] &&
					res.Levels[k+1].Partition[u] != res.Levels[k+1].Partition[v] {
					t.Fatalf("level %d→%d split (%d,%d) — not a coarsening", k, k+1, u, v)
				}
			}
		}
	}
	ari := adjustedRandIndex(res.Final.Partition, truth)
	if ari < 0.85 {
		t.Errorf("final ARI=%.4f vs planted truth, want ≥0.85", ari)
	}
}

// TestIntegration_PlantedPartition_NodeWeightedPathRunsCleanly drives the
// rarely-exercised NodeWeights path with a planted partition: weighted
// runs must produce a valid partition (every node assigned, IDs in
// [0, NumClusters)) and not crash.
func TestIntegration_PlantedPartition_NodeWeightedPathRunsCleanly(t *testing.T) {
	rng := rand.New(rand.NewSource(13))
	n, edges, _ := plantedPartition(3, 20, 0.5, 0.05, rng)

	weights := make([]float64, n)
	for i := range weights {
		weights[i] = 1.0 + 0.1*float64(i%5)
	}
	opts := DefaultOptions()
	opts.Resolution = 0.1
	opts.Seed = 3
	opts.NodeWeights = weights
	res, err := Leiden(context.Background(), n, edges, opts)
	if err != nil {
		t.Fatalf("Leiden(weighted): %v", err)
	}
	if len(res.Partition) != n {
		t.Errorf("len(Partition)=%d, want %d", len(res.Partition), n)
	}
	for i, c := range res.Partition {
		if c < 0 || c >= res.NumClusters {
			t.Errorf("Partition[%d]=%d, want in [0,%d)", i, c, res.NumClusters)
		}
	}
}

// TestIntegration_AllNonZeroResolutions_AllNodesAssigned is a defensive
// table-driven check: across a spectrum of resolutions on a non-trivial
// graph, every node must end up in some cluster in [0, NumClusters).
func TestIntegration_AllNonZeroResolutions_AllNodesAssigned(t *testing.T) {
	n, edges := mustLoadEdges(t, "testdata/karate.edges")
	tests := []float64{0.01, 0.05, 0.1, 0.5, 1.0, 2.0}
	for _, gamma := range tests {
		t.Run(ftoa(gamma), func(t *testing.T) {
			opts := DefaultOptions()
			opts.Resolution = gamma
			opts.Seed = 1
			res, err := Leiden(context.Background(), n, edges, opts)
			if err != nil {
				t.Fatalf("Leiden: %v", err)
			}
			if len(res.Partition) != n {
				t.Fatalf("Partition length=%d, want %d", len(res.Partition), n)
			}
			seen := make(map[int]bool)
			for i, c := range res.Partition {
				if c < 0 || c >= res.NumClusters {
					t.Errorf("Partition[%d]=%d, want in [0,%d)", i, c, res.NumClusters)
				}
				seen[c] = true
			}
			if got := len(seen); got != res.NumClusters {
				// Some cluster IDs unused — partition must still be Normalize-d.
				ids := make([]int, 0, len(seen))
				for id := range seen {
					ids = append(ids, id)
				}
				sort.Ints(ids)
				t.Errorf("NumClusters=%d, seen distinct IDs=%v", res.NumClusters, ids)
			}
		})
	}
}

// ftoa formats a float for sub-test names so output reads naturally.
func ftoa(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
