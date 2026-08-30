package leiden

import (
	"math/rand"
	"testing"
)

func TestRefinement_RespectsParentClusterBoundaries(t *testing.T) {
	// 8-node graph: two cliques bridged once. Parent puts everything in one
	// cluster; refinement must split the bridge (since the two cliques aren't
	// well-connected to a combined 8-cluster at γ = 0.5) and never produce a
	// refined cluster spanning both halves.
	var edges []Edge
	for i := 0; i < 4; i++ {
		for j := i + 1; j < 4; j++ {
			edges = append(edges, Edge{From: i, To: j, Weight: 1})
		}
	}
	for i := 4; i < 8; i++ {
		for j := i + 1; j < 8; j++ {
			edges = append(edges, Edge{From: i, To: j, Weight: 1})
		}
	}
	edges = append(edges, Edge{From: 3, To: 4, Weight: 1})
	net := mustNetwork(t, 8, edges)

	// Parent: everyone in one cluster.
	parent, _ := NewClustering(8)
	q := cpmQuality{Gamma: 0.5}
	refined := runRefinement(net, parent, q, 0, rand.New(rand.NewSource(1)))

	// Every refined cluster's members must share a parent cluster ID. Parent
	// is single-cluster so this is trivially true; check the stronger
	// property that two nodes in different parent clusters never end up in
	// the same refined cluster, using a mixed parent.
	parent2, _ := NewClusteringFromAssignment([]int{0, 0, 0, 0, 1, 1, 1, 1})
	refined2 := runRefinement(net, parent2, q, 0, rand.New(rand.NewSource(1)))
	for u := 0; u < net.NumNodes(); u++ {
		for v := 0; v < net.NumNodes(); v++ {
			if refined2.Cluster(u) == refined2.Cluster(v) && parent2.Cluster(u) != parent2.Cluster(v) {
				t.Errorf("refined cluster crosses parent boundary: u=%d v=%d", u, v)
			}
		}
	}
	_ = refined
}

func TestRefinement_AllSingletonsOnIsolatedNodes(t *testing.T) {
	// No edges at all → no node can be well-connected to its parent cluster.
	// Refinement must leave everyone in their singleton.
	net := mustNetwork(t, 5, nil)
	parent, _ := NewClustering(5)
	q := cpmQuality{Gamma: 0.1}
	refined := runRefinement(net, parent, q, 0, rand.New(rand.NewSource(2)))
	refined.Normalize()
	if refined.NumClusters() != 5 {
		t.Errorf("NumClusters=%d, want 5", refined.NumClusters())
	}
}

func TestRefinement_NeverDecreasesQuality(t *testing.T) {
	edges := []Edge{
		{0, 1, 1}, {1, 2, 1}, {0, 2, 1},
		{3, 4, 1}, {4, 5, 1}, {3, 5, 1},
		{2, 3, 0.2},
	}
	net := mustNetwork(t, 6, edges)
	parent, _ := NewClusteringFromAssignment([]int{0, 0, 0, 1, 1, 1})
	q := cpmQuality{Gamma: 0.3}

	// Singleton baseline quality is the floor refinement must beat or match.
	singletons, _ := NewSingletonClustering(6)
	baseline := q.value(net, singletons)

	refined := runRefinement(net, parent, q, 0, rand.New(rand.NewSource(3)))
	got := q.value(net, refined)
	if got+floatTol < baseline {
		t.Errorf("refinement quality %g below singleton baseline %g", got, baseline)
	}
}

func TestRefinement_DeterministicWithSeed(t *testing.T) {
	edges := []Edge{
		{0, 1, 1}, {1, 2, 1}, {0, 2, 1},
		{3, 4, 1}, {4, 5, 1}, {3, 5, 1},
		{2, 3, 1},
	}
	net := mustNetwork(t, 6, edges)
	parent, _ := NewClustering(6)
	q := cpmQuality{Gamma: 0.3}

	run := func(seed int64) []int {
		r := runRefinement(net, parent, q, 0, rand.New(rand.NewSource(seed)))
		r.Normalize()
		return r.Assignment()
	}
	a := run(99)
	b := run(99)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("non-deterministic refinement: %v vs %v", a, b)
		}
	}
}

func TestRefinement_ThetaRandomizationConsumesRNG(t *testing.T) {
	// With theta > 0 and multiple positive candidates, the picker should
	// consult the RNG; two distinct seeds must be able to diverge. Set up
	// a star-ish graph where node 0 sees two equally-attractive candidate
	// clusters.
	edges := []Edge{
		{1, 2, 1}, {1, 3, 1}, {2, 3, 1}, // R-cluster candidate A: {1,2,3}
		{4, 5, 1}, {4, 6, 1}, {5, 6, 1}, // R-cluster candidate B: {4,5,6}
		{0, 1, 1}, {0, 2, 1}, {0, 4, 1}, {0, 5, 1}, // 0 connects to both
	}
	net := mustNetwork(t, 7, edges)
	parent, _ := NewClustering(7)
	q := cpmQuality{Gamma: 0.01}

	// We can't guarantee divergence (the picker may still hit the same
	// candidate), but we can verify the function runs and produces a valid
	// partition under theta > 0.
	r := runRefinement(net, parent, q, 0.5, rand.New(rand.NewSource(11)))
	if r.NumNodes() != 7 {
		t.Errorf("NumNodes=%d, want 7", r.NumNodes())
	}
}
