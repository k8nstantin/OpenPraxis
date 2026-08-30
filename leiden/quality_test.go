package leiden

import (
	"math"
	"testing"
)

const floatTol = 1e-9

func floatEq(a, b float64) bool {
	return math.Abs(a-b) <= floatTol
}

func TestCPMQuality_AllSingletons_NoEdges(t *testing.T) {
	// 5 isolated nodes, all singletons: e_c = 0, w_c = 1 for each cluster.
	// H = 0 − γ/2 · 5·1² = −2.5γ
	net := mustNetwork(t, 5, nil)
	cl, _ := NewSingletonClustering(5)
	q := cpmQuality{Gamma: 1.0}
	got := q.value(net, cl)
	want := -2.5
	if !floatEq(got, want) {
		t.Errorf("value=%g, want %g", got, want)
	}
}

func TestCPMQuality_TriangleOneCluster(t *testing.T) {
	// Triangle 0-1-2 with unit edges; all in one cluster.
	// e_c = 3, w_c = 3. H = 3 − γ/2 · 9 = 3 − 4.5γ.
	edges := []Edge{
		{From: 0, To: 1, Weight: 1},
		{From: 1, To: 2, Weight: 1},
		{From: 0, To: 2, Weight: 1},
	}
	net := mustNetwork(t, 3, edges)
	cl, _ := NewClustering(3)
	for gamma, want := range map[float64]float64{
		0.0: 3.0,
		0.5: 0.75,
		1.0: -1.5,
	} {
		q := cpmQuality{Gamma: gamma}
		got := q.value(net, cl)
		if !floatEq(got, want) {
			t.Errorf("γ=%g: value=%g, want %g", gamma, got, want)
		}
	}
}

func TestCPMQuality_SelfLoopCountedOnce(t *testing.T) {
	// Single node with self-loop w=2.0; one cluster.
	// e_c = 2.0, w_c = 1. H = 2 − γ/2 = 2 − 0.5γ.
	net := mustNetwork(t, 1, []Edge{{From: 0, To: 0, Weight: 2.0}})
	cl, _ := NewClustering(1)
	q := cpmQuality{Gamma: 1.0}
	got := q.value(net, cl)
	want := 1.5
	if !floatEq(got, want) {
		t.Errorf("value=%g, want %g", got, want)
	}
}

func TestCPMQuality_MoveDeltaMatchesRecompute(t *testing.T) {
	// Move-delta must equal H(after) - H(before) for any valid move.
	// Use a 6-node graph with two triangles connected by a single edge.
	edges := []Edge{
		{0, 1, 1}, {1, 2, 1}, {0, 2, 1}, // triangle A
		{3, 4, 1}, {4, 5, 1}, {3, 5, 1}, // triangle B
		{2, 3, 1}, // bridge
	}
	net := mustNetwork(t, 6, edges)

	// Start with two natural clusters: {0,1,2}=0, {3,4,5}=1.
	cl, _ := NewClusteringFromAssignment([]int{0, 0, 0, 1, 1, 1})
	q := cpmQuality{Gamma: 0.4}

	before := q.value(net, cl)

	// Compute the helpers required by moveDelta: edges from node 2 to each cluster.
	u := 2
	wToFrom := 0.0
	wToTo := 0.0
	for i, v := range net.Neighbors(u) {
		if v == u {
			continue
		}
		w := net.NeighborWeights(u)[i]
		switch cl.Cluster(v) {
		case 0:
			wToFrom += w
		case 1:
			wToTo += w
		}
	}
	clusterMass := []float64{3, 3}
	delta := q.moveDelta(net, u, 0, 1, wToFrom, wToTo, clusterMass)

	// Apply move and recompute.
	_ = cl.SetCluster(u, 1)
	after := q.value(net, cl)

	if !floatEq(delta, after-before) {
		t.Errorf("moveDelta=%g, recomputed=%g", delta, after-before)
	}
}

func TestCPMQuality_MoveDeltaSelfTargetZero(t *testing.T) {
	net := mustNetwork(t, 2, []Edge{{0, 1, 1}})
	q := cpmQuality{Gamma: 1.0}
	cm := []float64{1, 1}
	if d := q.moveDelta(net, 0, 0, 0, 0, 0, cm); d != 0 {
		t.Errorf("moveDelta(from==to)=%g, want 0", d)
	}
}

func TestCPMQuality_NodeMassReturnsNodeWeight(t *testing.T) {
	net, _ := NewCompactNetworkWithNodeWeights(3, []float64{0.5, 1.0, 2.0}, nil)
	q := cpmQuality{Gamma: 1.0}
	for i, want := range []float64{0.5, 1.0, 2.0} {
		if got := q.nodeMass(net, i); got != want {
			t.Errorf("nodeMass(%d)=%g, want %g", i, got, want)
		}
	}
}

func TestCPMQuality_ResolutionGetter(t *testing.T) {
	q := cpmQuality{Gamma: 0.37}
	if got := q.resolution(); got != 0.37 {
		t.Errorf("resolution=%g, want 0.37", got)
	}
}
