package leiden

import (
	"math/rand"
	"testing"
)

// TestAlgorithmLoop_ThreePhasesCompose drives the local-move →
// refinement → aggregation pipeline end-to-end and verifies the
// across-level invariants that future milestones will rely on:
//
//   - Refinement never breaks parent-cluster boundaries.
//   - Aggregation preserves CPM quality between levels.
//   - One full iteration on a known graph reaches the expected
//     two-community partition for moderately-connected planted clusters.
func TestAlgorithmLoop_ThreePhasesCompose(t *testing.T) {
	// Two 4-cliques bridged by a single edge (3—4).
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

	q := cpmQuality{Gamma: 0.5}
	rng := rand.New(rand.NewSource(42))

	// Phase 1: local-move from singletons.
	parent, _ := NewSingletonClustering(8)
	qBefore := q.value(net, parent)
	runLocalMove(net, parent, q, rng)
	qAfterLocalMove := q.value(net, parent)
	if qAfterLocalMove+floatTol < qBefore {
		t.Fatalf("local-move decreased quality: %g → %g", qBefore, qAfterLocalMove)
	}

	// Phase 2: refinement.
	refined := runRefinement(net, parent, q, 0, rng)
	for u := 0; u < net.NumNodes(); u++ {
		for v := u + 1; v < net.NumNodes(); v++ {
			if refined.Cluster(u) == refined.Cluster(v) && parent.Cluster(u) != parent.Cluster(v) {
				t.Errorf("refined cluster crosses parent boundary: nodes %d,%d", u, v)
			}
		}
	}

	// Phase 3: aggregation.
	agg, init, err := aggregateNetwork(net, refined, parent)
	if err != nil {
		t.Fatalf("aggregateNetwork: %v", err)
	}
	qAgg := q.value(agg, init)
	// CPM-quality invariant: H(parent on net) == H(init on agg).
	if !floatEq(qAgg, qAfterLocalMove) {
		t.Errorf("aggregated quality %g != original %g", qAgg, qAfterLocalMove)
	}

	// Expected partition (parent): two communities split at the bridge.
	want := [][]int{{0, 1, 2, 3}, {4, 5, 6, 7}}
	got := clusterMembership(parent)
	if len(got) != len(want) {
		t.Fatalf("partition = %v, want %v", got, want)
	}
	for i := range want {
		if len(got[i]) != len(want[i]) {
			t.Errorf("cluster %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestAlgorithmLoop_AggregatedNextIteration_Stable(t *testing.T) {
	// After one full iteration the aggregated network's local-move pass
	// should make no further moves: the partition is at a local optimum.
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

	q := cpmQuality{Gamma: 0.5}
	rng := rand.New(rand.NewSource(42))

	parent, _ := NewSingletonClustering(8)
	runLocalMove(net, parent, q, rng)
	refined := runRefinement(net, parent, q, 0, rng)
	agg, init, err := aggregateNetwork(net, refined, parent)
	if err != nil {
		t.Fatalf("aggregateNetwork: %v", err)
	}

	moves := runLocalMove(agg, init, q, rng)
	if moves != 0 {
		t.Errorf("local-move on aggregated network made %d moves; expected 0 at local optimum", moves)
	}
}
