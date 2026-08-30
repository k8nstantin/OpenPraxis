package leiden

import (
	"errors"
	"sort"
	"testing"
)

func TestAggregate_SingletonRefinement_PreservesGraph(t *testing.T) {
	// Refinement is the identity (singletons → singletons): the aggregated
	// network must be structurally equivalent to the input, with each node
	// keeping its parent cluster ID.
	edges := []Edge{
		{0, 1, 1}, {1, 2, 2}, {0, 2, 3},
	}
	net := mustNetwork(t, 3, edges)
	refined, _ := NewSingletonClustering(3)
	parent, _ := NewClusteringFromAssignment([]int{0, 0, 1})

	agg, init, err := aggregateNetwork(net, refined, parent)
	if err != nil {
		t.Fatalf("aggregateNetwork: %v", err)
	}
	if agg.NumNodes() != 3 {
		t.Errorf("agg.NumNodes=%d, want 3", agg.NumNodes())
	}
	if init.NumClusters() != 2 {
		t.Errorf("init.NumClusters=%d, want 2", init.NumClusters())
	}
	if got, want := agg.TotalEdgeWeight(), 6.0; !floatEq(got, want) {
		t.Errorf("agg.TotalEdgeWeight=%g, want %g", got, want)
	}
	// Node weights unchanged.
	for i := 0; i < 3; i++ {
		if w := agg.NodeWeight(i); w != 1.0 {
			t.Errorf("agg.NodeWeight(%d)=%g, want 1", i, w)
		}
	}
}

func TestAggregate_TriangleCollapsedToOneNode(t *testing.T) {
	// Triangle 0-1-2 (unit weights), refinement collapses everything into
	// cluster 0. Expected: 1 new node, self-loop weight = 3 (the three
	// internal edges), node weight = 3.
	edges := []Edge{
		{0, 1, 1}, {1, 2, 1}, {0, 2, 1},
	}
	net := mustNetwork(t, 3, edges)
	refined, _ := NewClustering(3) // all in cluster 0
	parent, _ := NewClustering(3)

	agg, init, err := aggregateNetwork(net, refined, parent)
	if err != nil {
		t.Fatalf("aggregateNetwork: %v", err)
	}
	if agg.NumNodes() != 1 {
		t.Fatalf("agg.NumNodes=%d, want 1", agg.NumNodes())
	}
	if w := agg.NodeWeight(0); w != 3 {
		t.Errorf("agg.NodeWeight(0)=%g, want 3", w)
	}
	if w := agg.TotalEdgeWeight(); w != 3 {
		t.Errorf("agg.TotalEdgeWeight=%g, want 3", w)
	}
	// Single self-loop edge, weight 3.
	nbrs := agg.Neighbors(0)
	ws := agg.NeighborWeights(0)
	if len(nbrs) != 1 || nbrs[0] != 0 || ws[0] != 3 {
		t.Errorf("agg neighbours=%v weights=%v, want self-loop weight 3", nbrs, ws)
	}
	if init.NumNodes() != 1 || init.Cluster(0) != 0 {
		t.Errorf("init clustering wrong: %+v", init.Assignment())
	}
}

func TestAggregate_PreservesCPMQuality(t *testing.T) {
	// CPM quality of a partition on the original network must equal the CPM
	// quality of the corresponding (initial) partition on the aggregated
	// network. This is the invariant that lets the algorithm iterate.
	//
	// Graph: two triangles bridged by one edge.
	edges := []Edge{
		{0, 1, 1}, {1, 2, 1}, {0, 2, 1},
		{3, 4, 1}, {4, 5, 1}, {3, 5, 1},
		{2, 3, 1},
	}
	net := mustNetwork(t, 6, edges)
	parent, _ := NewClusteringFromAssignment([]int{0, 0, 0, 1, 1, 1})
	// Refinement coincides with parent (one R-cluster per parent cluster).
	refined, _ := NewClusteringFromAssignment([]int{0, 0, 0, 1, 1, 1})
	q := cpmQuality{Gamma: 0.3}

	want := q.value(net, parent)
	agg, init, err := aggregateNetwork(net, refined, parent)
	if err != nil {
		t.Fatalf("aggregateNetwork: %v", err)
	}
	got := q.value(agg, init)
	if !floatEq(got, want) {
		t.Errorf("aggregated quality %g != original %g", got, want)
	}
}

func TestAggregate_PreservesQuality_RefinementSubdividesParent(t *testing.T) {
	// Same graph, but refinement subdivides each parent cluster into two
	// sub-clusters. Quality must still be preserved.
	edges := []Edge{
		{0, 1, 1}, {1, 2, 1}, {0, 2, 1},
		{3, 4, 1}, {4, 5, 1}, {3, 5, 1},
		{2, 3, 1},
	}
	net := mustNetwork(t, 6, edges)
	parent, _ := NewClusteringFromAssignment([]int{0, 0, 0, 1, 1, 1})
	refined, _ := NewClusteringFromAssignment([]int{0, 0, 2, 3, 3, 5})
	q := cpmQuality{Gamma: 0.3}

	want := q.value(net, parent)
	agg, init, err := aggregateNetwork(net, refined, parent)
	if err != nil {
		t.Fatalf("aggregateNetwork: %v", err)
	}
	got := q.value(agg, init)
	if !floatEq(got, want) {
		t.Errorf("aggregated quality %g != original %g", got, want)
	}
	// Sanity: aggregated network has 4 new nodes (one per refined cluster).
	if agg.NumNodes() != 4 {
		t.Errorf("agg.NumNodes=%d, want 4", agg.NumNodes())
	}
}

func TestAggregate_PreservesTotalEdgeWeight(t *testing.T) {
	// Sum of input edge weights is invariant under aggregation regardless of
	// the refined clustering used.
	edges := []Edge{
		{0, 1, 0.5}, {1, 2, 1.5}, {2, 3, 2.0}, {0, 3, 0.25},
		{1, 1, 0.7}, // self-loop
	}
	net := mustNetwork(t, 4, edges)
	parent, _ := NewClustering(4)
	refined, _ := NewClusteringFromAssignment([]int{0, 0, 1, 1})

	agg, _, err := aggregateNetwork(net, refined, parent)
	if err != nil {
		t.Fatalf("aggregateNetwork: %v", err)
	}
	if !floatEq(agg.TotalEdgeWeight(), net.TotalEdgeWeight()) {
		t.Errorf("TotalEdgeWeight: agg=%g, orig=%g", agg.TotalEdgeWeight(), net.TotalEdgeWeight())
	}
}

func TestAggregate_NormalisesRefinedClusters(t *testing.T) {
	// Refined cluster IDs need not be contiguous; aggregateNetwork should
	// normalize them transparently.
	edges := []Edge{{0, 1, 1}, {1, 2, 1}}
	net := mustNetwork(t, 3, edges)
	parent, _ := NewClustering(3)
	refined, _ := NewClusteringFromAssignment([]int{7, 7, 99})
	agg, init, err := aggregateNetwork(net, refined, parent)
	if err != nil {
		t.Fatalf("aggregateNetwork: %v", err)
	}
	if agg.NumNodes() != 2 {
		t.Errorf("agg.NumNodes=%d, want 2", agg.NumNodes())
	}
	if init.NumNodes() != 2 {
		t.Errorf("init.NumNodes=%d, want 2", init.NumNodes())
	}
	// Both new nodes inherited parent cluster 0.
	got := init.Assignment()
	sort.Ints(got)
	if got[0] != 0 || got[1] != 0 {
		t.Errorf("init assignment=%v, want [0,0]", got)
	}
}

func TestAggregate_NilInputs_Error(t *testing.T) {
	cl, _ := NewSingletonClustering(2)
	net := mustNetwork(t, 2, nil)
	tests := []struct {
		name string
		fn   func() error
	}{
		{"nil network", func() error { _, _, e := aggregateNetwork(nil, cl, cl); return e }},
		{"nil refined", func() error { _, _, e := aggregateNetwork(net, nil, cl); return e }},
		{"nil parent", func() error { _, _, e := aggregateNetwork(net, cl, nil); return e }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); !errors.Is(err, ErrInvalidNodeCount) {
				t.Errorf("got %v, want errors.Is %v", err, ErrInvalidNodeCount)
			}
		})
	}
}
