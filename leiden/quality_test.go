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

func TestModularityQuality_AllSingletons_NoEdges(t *testing.T) {
	// No edges → internal = 0 and every node has strength 0 → penalty = 0
	// and twoM = 0. By definition the function returns 0 in this case.
	net := mustNetwork(t, 5, nil)
	cl, _ := NewSingletonClustering(5)
	q := modularityQuality{Gamma: 1.0}
	if got := q.value(net, cl); got != 0 {
		t.Errorf("value=%g, want 0", got)
	}
}

func TestModularityQuality_TriangleOneCluster(t *testing.T) {
	// Triangle 0-1-2, unit edges, single cluster.
	// 2m = sum of strengths = 6 (each node has strength 2).
	// CSR same-cluster sum = 2·3 = 6 (each undirected internal edge twice).
	// K = 6. Q = (6 − γ·36/6) / 6 = (6 − 6γ)/6 = 1 − γ.
	edges := []Edge{
		{From: 0, To: 1, Weight: 1},
		{From: 1, To: 2, Weight: 1},
		{From: 0, To: 2, Weight: 1},
	}
	net := mustNetwork(t, 3, edges)
	cl, _ := NewClustering(3)
	for gamma, want := range map[float64]float64{
		0.0: 1.0,
		0.5: 0.5,
		1.0: 0.0,
		1.5: -0.5,
	} {
		q := modularityQuality{Gamma: gamma}
		got := q.value(net, cl)
		if !floatEq(got, want) {
			t.Errorf("γ=%g: value=%g, want %g", gamma, got, want)
		}
	}
}

func TestModularityQuality_TwoCliquesBridged(t *testing.T) {
	// Two K_4 cliques bridged once: known modularity-friendly graph.
	// Singleton partition has Q = − γ · Σ k_u² / (2m)².
	// The natural two-community partition should have higher Q at γ = 1.
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

	singletons, _ := NewSingletonClustering(8)
	twoComm, _ := NewClusteringFromAssignment([]int{0, 0, 0, 0, 1, 1, 1, 1})
	allOne, _ := NewClustering(8)

	q := modularityQuality{Gamma: 1.0}
	qSing := q.value(net, singletons)
	qTwo := q.value(net, twoComm)
	qOne := q.value(net, allOne)

	if qTwo <= qSing {
		t.Errorf("two-community Q (%g) should exceed singleton Q (%g)", qTwo, qSing)
	}
	if qTwo <= qOne {
		t.Errorf("two-community Q (%g) should exceed single-cluster Q (%g)", qTwo, qOne)
	}
}

func TestModularityQuality_SelfLoopCountedOnce(t *testing.T) {
	// Single node with self-loop w=2.0; one cluster.
	// 2m = 2 (strength of the lone node = self-loop weight, stored once).
	// CSR same-cluster sum = 2 (one entry, weight 2).
	// K = 2. Q = (2 − γ·4/2)/2 = 1 − γ.
	net := mustNetwork(t, 1, []Edge{{From: 0, To: 0, Weight: 2.0}})
	cl, _ := NewClustering(1)
	q := modularityQuality{Gamma: 1.0}
	got := q.value(net, cl)
	want := 0.0
	if !floatEq(got, want) {
		t.Errorf("value=%g, want %g", got, want)
	}
}

func TestModularityQuality_MoveDeltaMatchesRecompute(t *testing.T) {
	// Move-delta must equal Q(after) − Q(before) for any valid move,
	// across every (from,to) pair node 2 could be reassigned to.
	edges := []Edge{
		{0, 1, 1}, {1, 2, 1}, {0, 2, 1},
		{3, 4, 1}, {4, 5, 1}, {3, 5, 1},
		{2, 3, 1},
	}
	net := mustNetwork(t, 6, edges)
	q := modularityQuality{Gamma: 0.6}

	for _, tc := range []struct {
		name       string
		assignment []int
		u, from, to int
	}{
		{"two-clusters-bridge", []int{0, 0, 0, 1, 1, 1}, 2, 0, 1},
		{"merge-into-other", []int{0, 0, 1, 1, 1, 1}, 2, 1, 0},
		{"new-singleton", []int{0, 0, 0, 1, 1, 1}, 2, 0, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cl, _ := NewClusteringFromAssignment(tc.assignment)
			before := q.value(net, cl)

			masses := make([]float64, cl.nClusters)
			for u := 0; u < net.NumNodes(); u++ {
				masses[cl.Cluster(u)] += net.nodeStrengths[u]
			}
			if tc.to >= len(masses) {
				grown := make([]float64, tc.to+1)
				copy(grown, masses)
				masses = grown
			}
			wToFrom, wToTo := 0.0, 0.0
			for i, v := range net.Neighbors(tc.u) {
				if v == tc.u {
					continue
				}
				w := net.NeighborWeights(tc.u)[i]
				switch cl.Cluster(v) {
				case tc.from:
					wToFrom += w
				case tc.to:
					wToTo += w
				}
			}
			delta := q.moveDelta(net, tc.u, tc.from, tc.to, wToFrom, wToTo, masses)

			_ = cl.SetCluster(tc.u, tc.to)
			after := q.value(net, cl)
			if !floatEq(delta, after-before) {
				t.Errorf("moveDelta=%.10g, recomputed=%.10g", delta, after-before)
			}
		})
	}
}

func TestModularityQuality_MoveDeltaSelfTargetZero(t *testing.T) {
	net := mustNetwork(t, 2, []Edge{{0, 1, 1}})
	q := modularityQuality{Gamma: 1.0}
	cm := []float64{1, 1}
	if d := q.moveDelta(net, 0, 0, 0, 0, 0, cm); d != 0 {
		t.Errorf("moveDelta(from==to)=%g, want 0", d)
	}
}

func TestModularityQuality_MoveDeltaEmptyGraph(t *testing.T) {
	// Edgeless graph: 2m = 0; moveDelta is defined to return 0.
	net := mustNetwork(t, 3, nil)
	q := modularityQuality{Gamma: 1.0}
	cm := []float64{0, 0, 0}
	if d := q.moveDelta(net, 0, 0, 1, 0, 0, cm); d != 0 {
		t.Errorf("moveDelta on edgeless net=%g, want 0", d)
	}
}

func TestModularityQuality_NodeMassReturnsStrength(t *testing.T) {
	// strength = sum of incident edge weights. Node 0: 0.5+1.0 = 1.5,
	// node 1: 0.5+2.0 = 2.5, node 2: 1.0+2.0 = 3.0.
	edges := []Edge{
		{0, 1, 0.5}, {0, 2, 1.0}, {1, 2, 2.0},
	}
	net := mustNetwork(t, 3, edges)
	q := modularityQuality{Gamma: 1.0}
	for u, want := range []float64{1.5, 2.5, 3.0} {
		if got := q.nodeMass(net, u); !floatEq(got, want) {
			t.Errorf("nodeMass(%d)=%g, want %g", u, got, want)
		}
	}
}

func TestModularityQuality_ResolutionGetter(t *testing.T) {
	q := modularityQuality{Gamma: 0.73}
	if got := q.resolution(); got != 0.73 {
		t.Errorf("resolution=%g, want 0.73", got)
	}
}

func TestModularityQuality_MatchesTextbookFormula(t *testing.T) {
	// Independently compute modularity from the A_ij/2m formula and compare
	// to value(). This guards against off-by-one factors in the rearranged
	// implementation.
	edges := []Edge{
		{0, 1, 1.0}, {0, 2, 1.0}, {1, 2, 1.0},
		{2, 3, 0.5},
		{3, 4, 1.0}, {3, 5, 1.0}, {4, 5, 1.0},
	}
	net := mustNetwork(t, 6, edges)
	cl, _ := NewClusteringFromAssignment([]int{0, 0, 0, 1, 1, 1})
	q := modularityQuality{Gamma: 1.25}

	// Build a dense weighted adjacency matrix from the CSR storage so the
	// textbook formula sums each ordered pair (i,j) exactly once.
	n := net.NumNodes()
	A := make([][]float64, n)
	for i := range A {
		A[i] = make([]float64, n)
	}
	for u := 0; u < n; u++ {
		nbrs := net.Neighbors(u)
		ws := net.NeighborWeights(u)
		for i, v := range nbrs {
			A[u][v] = ws[i]
		}
	}
	twoM := net.TotalNodeStrength()
	var want float64
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if cl.Cluster(i) != cl.Cluster(j) {
				continue
			}
			ki := net.nodeStrengths[i]
			kj := net.nodeStrengths[j]
			want += A[i][j] - q.Gamma*ki*kj/twoM
		}
	}
	want /= twoM

	got := q.value(net, cl)
	if !floatEq(got, want) {
		t.Errorf("value=%.10g, textbook=%.10g", got, want)
	}
}
