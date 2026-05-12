package leiden

import (
	"math/rand"
	"reflect"
	"sort"
	"testing"
)

// clusterMembership returns a sorted slice of cluster IDs and a sorted member
// list per cluster, so a partition can be compared up to renaming of cluster
// IDs.
func clusterMembership(cl *Clustering) [][]int {
	cl.Normalize()
	out := make([][]int, cl.NumClusters())
	for u := 0; u < cl.NumNodes(); u++ {
		out[cl.Cluster(u)] = append(out[cl.Cluster(u)], u)
	}
	for _, m := range out {
		sort.Ints(m)
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i]) == 0 || len(out[j]) == 0 {
			return len(out[i]) < len(out[j])
		}
		return out[i][0] < out[j][0]
	})
	return out
}

func TestLocalMove_EmptyNetwork_NoCrash(t *testing.T) {
	net := mustNetwork(t, 1, nil)
	cl, _ := NewSingletonClustering(1)
	q := cpmQuality{Gamma: 0.1}
	moves := runLocalMove(net, cl, q, rand.New(rand.NewSource(1)))
	if moves != 0 {
		t.Errorf("moves=%d, want 0", moves)
	}
}

func TestLocalMove_NoBeneficialMove_StaysPut(t *testing.T) {
	// Two isolated nodes. Starting from singletons: any merge has Δ = 0 − γ·1·1 < 0
	// (no edge weight). So no moves.
	net := mustNetwork(t, 2, nil)
	cl, _ := NewSingletonClustering(2)
	q := cpmQuality{Gamma: 1.0}
	moves := runLocalMove(net, cl, q, rand.New(rand.NewSource(1)))
	if moves != 0 {
		t.Errorf("moves=%d, want 0", moves)
	}
	if cl.Cluster(0) == cl.Cluster(1) {
		t.Errorf("isolated nodes merged unexpectedly")
	}
}

func TestLocalMove_TwoCliquesBridge_SeparatesClusters(t *testing.T) {
	// Two 4-cliques connected by a single bridge edge. With γ = 0.5, internal
	// edges (weight 1) inside each clique dominate the penalty, and the bridge
	// is too weak to glue them together.
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
	edges = append(edges, Edge{From: 3, To: 4, Weight: 1}) // bridge
	net := mustNetwork(t, 8, edges)
	cl, _ := NewSingletonClustering(8)
	q := cpmQuality{Gamma: 0.5}
	rng := rand.New(rand.NewSource(42))

	if moves := runLocalMove(net, cl, q, rng); moves == 0 {
		t.Fatalf("expected moves; got 0")
	}

	want := [][]int{{0, 1, 2, 3}, {4, 5, 6, 7}}
	got := clusterMembership(cl)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("partition = %v, want %v", got, want)
	}
}

func TestLocalMove_MonotoneImprovement(t *testing.T) {
	// On any non-trivial graph, the local-moving phase must never decrease
	// quality. Use a 6-node graph and a seed-deterministic run.
	edges := []Edge{
		{0, 1, 1}, {1, 2, 1}, {0, 2, 1},
		{3, 4, 1}, {4, 5, 1}, {3, 5, 1},
		{2, 3, 0.2},
	}
	net := mustNetwork(t, 6, edges)
	cl, _ := NewSingletonClustering(6)
	q := cpmQuality{Gamma: 0.3}
	before := q.value(net, cl)

	runLocalMove(net, cl, q, rand.New(rand.NewSource(7)))

	after := q.value(net, cl)
	if after+floatTol < before {
		t.Errorf("quality decreased: before=%g, after=%g", before, after)
	}
}

func TestLocalMove_DeterministicWithSeed(t *testing.T) {
	edges := []Edge{
		{0, 1, 1}, {1, 2, 1}, {0, 2, 1},
		{3, 4, 1}, {4, 5, 1}, {3, 5, 1},
		{2, 3, 0.2},
	}
	net := mustNetwork(t, 6, edges)
	q := cpmQuality{Gamma: 0.3}

	run := func(seed int64) []int {
		cl, _ := NewSingletonClustering(6)
		runLocalMove(net, cl, q, rand.New(rand.NewSource(seed)))
		cl.Normalize()
		return cl.Assignment()
	}
	a := run(123)
	b := run(123)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("non-deterministic: %v vs %v", a, b)
	}
}
