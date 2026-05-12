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
	"errors"
	"reflect"
	"sort"
	"testing"
)

// twoCliqueBridgeEdges returns the canonical 8-node test graph: two K4
// cliques joined by a single bridge edge (3—4).
func twoCliqueBridgeEdges() []Edge {
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
	return edges
}

// partitionGroups groups node IDs by cluster, returning a slice of sorted
// member lists sorted lexicographically. This canonicalises partitions for
// comparison up to cluster-ID relabelling.
func partitionGroups(partition []int) [][]int {
	maxID := -1
	for _, c := range partition {
		if c > maxID {
			maxID = c
		}
	}
	out := make([][]int, maxID+1)
	for u, c := range partition {
		out[c] = append(out[c], u)
	}
	for _, g := range out {
		sort.Ints(g)
	}
	nonEmpty := out[:0]
	for _, g := range out {
		if len(g) > 0 {
			nonEmpty = append(nonEmpty, g)
		}
	}
	sort.Slice(nonEmpty, func(i, j int) bool {
		return nonEmpty[i][0] < nonEmpty[j][0]
	})
	return nonEmpty
}

func TestDefaultOptions_Values(t *testing.T) {
	opts := DefaultOptions()
	if opts.Resolution != 0.05 {
		t.Errorf("Resolution=%g, want 0.05", opts.Resolution)
	}
	if opts.Randomness != 0.01 {
		t.Errorf("Randomness=%g, want 0.01", opts.Randomness)
	}
	if opts.MaxIterations != defaultMaxIterations {
		t.Errorf("MaxIterations=%d, want %d", opts.MaxIterations, defaultMaxIterations)
	}
	if opts.Seed != 0 {
		t.Errorf("Seed=%d, want 0", opts.Seed)
	}
	if opts.NodeWeights != nil {
		t.Errorf("NodeWeights=%v, want nil", opts.NodeWeights)
	}
}

func TestLeiden_TwoCliqueBridge_FindsTwoCommunities(t *testing.T) {
	opts := DefaultOptions()
	opts.Resolution = 0.5
	res, err := Leiden(context.Background(), 8, twoCliqueBridgeEdges(), opts)
	if err != nil {
		t.Fatalf("Leiden: %v", err)
	}
	if res.NumClusters != 2 {
		t.Errorf("NumClusters=%d, want 2", res.NumClusters)
	}
	got := partitionGroups(res.Partition)
	want := [][]int{{0, 1, 2, 3}, {4, 5, 6, 7}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("partition=%v, want %v", got, want)
	}
	if res.Iterations <= 0 {
		t.Errorf("Iterations=%d, want >0", res.Iterations)
	}
	if len(res.Partition) != 8 {
		t.Errorf("len(Partition)=%d, want 8", len(res.Partition))
	}
}

func TestLeiden_EmptyEdges_AllSingletons(t *testing.T) {
	res, err := Leiden(context.Background(), 5, nil, DefaultOptions())
	if err != nil {
		t.Fatalf("Leiden: %v", err)
	}
	if res.NumClusters != 5 {
		t.Errorf("NumClusters=%d, want 5 (singletons)", res.NumClusters)
	}
	// One iteration: local-move makes 0 moves and the loop exits.
	if res.Iterations != 1 {
		t.Errorf("Iterations=%d, want 1", res.Iterations)
	}
}

func TestLeiden_SingleNode(t *testing.T) {
	res, err := Leiden(context.Background(), 1, nil, DefaultOptions())
	if err != nil {
		t.Fatalf("Leiden: %v", err)
	}
	if res.NumClusters != 1 || len(res.Partition) != 1 || res.Partition[0] != 0 {
		t.Errorf("Result=%+v, want one cluster with one node", res)
	}
}

func TestLeiden_DeterministicSeed(t *testing.T) {
	edges := twoCliqueBridgeEdges()
	opts := DefaultOptions()
	opts.Resolution = 0.3
	opts.Seed = 12345
	a, err := Leiden(context.Background(), 8, edges, opts)
	if err != nil {
		t.Fatalf("Leiden (a): %v", err)
	}
	b, err := Leiden(context.Background(), 8, edges, opts)
	if err != nil {
		t.Fatalf("Leiden (b): %v", err)
	}
	if !reflect.DeepEqual(a.Partition, b.Partition) {
		t.Errorf("same seed produced different partitions:\n a=%v\n b=%v", a.Partition, b.Partition)
	}
	if a.Quality != b.Quality {
		t.Errorf("same seed produced different quality: a=%g b=%g", a.Quality, b.Quality)
	}
}

func TestLeiden_ReturnedSliceIsOwnedByCaller(t *testing.T) {
	res, err := Leiden(context.Background(), 8, twoCliqueBridgeEdges(), DefaultOptions())
	if err != nil {
		t.Fatalf("Leiden: %v", err)
	}
	// Mutating the returned partition must not break a subsequent call.
	for i := range res.Partition {
		res.Partition[i] = -999
	}
	res2, err := Leiden(context.Background(), 8, twoCliqueBridgeEdges(), DefaultOptions())
	if err != nil {
		t.Fatalf("Leiden (2): %v", err)
	}
	for _, c := range res2.Partition {
		if c < 0 {
			t.Errorf("second call partition contained negative ID: %v", res2.Partition)
			break
		}
	}
}

func TestLeiden_InvalidInputs(t *testing.T) {
	tests := []struct {
		name    string
		nNodes  int
		edges   []Edge
		opts    Options
		wantErr error
	}{
		{"zero nodes", 0, nil, DefaultOptions(), ErrInvalidNodeCount},
		{"negative nodes", -1, nil, DefaultOptions(), ErrInvalidNodeCount},
		{"out-of-range edge", 3, []Edge{{From: 0, To: 5, Weight: 1}}, DefaultOptions(), ErrNodeOutOfRange},
		{"negative edge weight", 3, []Edge{{From: 0, To: 1, Weight: -1}}, DefaultOptions(), ErrNegativeEdgeWeight},
		{"node weights length mismatch", 3, nil, Options{NodeWeights: []float64{1, 1}}, ErrNodeWeightsLength},
		{"negative node weight", 3, nil, Options{NodeWeights: []float64{1, -1, 1}}, ErrNegativeNodeWeight},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Leiden(context.Background(), tt.nNodes, tt.edges, tt.opts)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("err=%v, want errors.Is %v", err, tt.wantErr)
			}
		})
	}
}

func TestLeiden_NegativeMaxIterations(t *testing.T) {
	opts := DefaultOptions()
	opts.MaxIterations = -1
	_, err := Leiden(context.Background(), 3, nil, opts)
	if err == nil {
		t.Fatal("expected error for negative MaxIterations, got nil")
	}
}

// TestLeiden_ZeroValueOptions verifies that a zero-initialised Options
// (every field at its Go zero value) is accepted and produces a valid run.
// This is the forward-compatibility contract: a caller who constructs
// Options{} directly — without going through DefaultOptions — must still
// get a usable result, because adding fields to Options must never break
// callers who omit them.
//
// Concretely: MaxIterations=0 must fall back to the package default, a
// nil NodeWeights slice must mean unit weights, Seed=0 must be a valid
// RNG seed, and Resolution=0 / Randomness=0 must be accepted.
func TestLeiden_ZeroValueOptions(t *testing.T) {
	var opts Options // every field at its Go zero value
	res, err := Leiden(context.Background(), 8, twoCliqueBridgeEdges(), opts)
	if err != nil {
		t.Fatalf("Leiden with zero Options: %v", err)
	}
	if len(res.Partition) != 8 {
		t.Fatalf("len(Partition)=%d, want 8", len(res.Partition))
	}
	if res.NumClusters <= 0 {
		t.Errorf("NumClusters=%d, want >0", res.NumClusters)
	}
	for i, c := range res.Partition {
		if c < 0 || c >= res.NumClusters {
			t.Errorf("Partition[%d]=%d, out of range [0, %d)", i, c, res.NumClusters)
		}
	}
	if res.Iterations <= 0 {
		t.Errorf("Iterations=%d, want >0", res.Iterations)
	}
	// The zero-value run must be deterministic: a second call with the
	// same zero Options must reproduce the same partition.
	res2, err := Leiden(context.Background(), 8, twoCliqueBridgeEdges(), Options{})
	if err != nil {
		t.Fatalf("Leiden (second call): %v", err)
	}
	if !reflect.DeepEqual(res.Partition, res2.Partition) {
		t.Errorf("zero Options not deterministic:\n a=%v\n b=%v", res.Partition, res2.Partition)
	}
}

func TestLeiden_MaxIterationsOneStopsEarly(t *testing.T) {
	opts := DefaultOptions()
	opts.Resolution = 0.5
	opts.MaxIterations = 1
	res, err := Leiden(context.Background(), 8, twoCliqueBridgeEdges(), opts)
	if err != nil {
		t.Fatalf("Leiden: %v", err)
	}
	if res.Iterations != 1 {
		t.Errorf("Iterations=%d, want 1 (capped by MaxIterations)", res.Iterations)
	}
}

func TestHierarchicalLeiden_TwoCliqueBridge(t *testing.T) {
	opts := DefaultOptions()
	opts.Resolution = 0.5
	res, err := HierarchicalLeiden(context.Background(), 8, twoCliqueBridgeEdges(), opts)
	if err != nil {
		t.Fatalf("HierarchicalLeiden: %v", err)
	}
	if len(res.Levels) == 0 {
		t.Fatal("Levels is empty")
	}
	// Final partition matches the last level.
	last := res.Levels[len(res.Levels)-1]
	if !reflect.DeepEqual(res.Final.Partition, last.Partition) {
		t.Errorf("Final.Partition=%v != Levels[last].Partition=%v", res.Final.Partition, last.Partition)
	}
	if res.Final.NumClusters != last.NumClusters {
		t.Errorf("Final.NumClusters=%d != last.NumClusters=%d", res.Final.NumClusters, last.NumClusters)
	}
	// Each level must be a coarsening (or equal) refinement of the next:
	// if two nodes share a cluster at level k, they share at all later levels.
	for k := 0; k+1 < len(res.Levels); k++ {
		for u := 0; u < 8; u++ {
			for v := u + 1; v < 8; v++ {
				if res.Levels[k].Partition[u] == res.Levels[k].Partition[v] &&
					res.Levels[k+1].Partition[u] != res.Levels[k+1].Partition[v] {
					t.Errorf("level %d→%d split (%d,%d) — not a coarsening", k, k+1, u, v)
				}
			}
		}
	}
	// Final should be the 2-clique partition.
	if g := partitionGroups(res.Final.Partition); !reflect.DeepEqual(g, [][]int{{0, 1, 2, 3}, {4, 5, 6, 7}}) {
		t.Errorf("Final partition=%v, want two cliques", g)
	}
}

func TestLeiden_QualityImprovesOverSingleton(t *testing.T) {
	edges := twoCliqueBridgeEdges()
	opts := DefaultOptions()
	opts.Resolution = 0.5

	// CPM quality of the all-singleton partition.
	singletonQ, err := scoreCPM(8, edges, identityPartition(8), opts.Resolution)
	if err != nil {
		t.Fatalf("scoreCPM (singleton): %v", err)
	}

	res, err := Leiden(context.Background(), 8, edges, opts)
	if err != nil {
		t.Fatalf("Leiden: %v", err)
	}
	if res.Quality <= singletonQ {
		t.Errorf("Leiden Q=%g is not greater than singleton Q=%g", res.Quality, singletonQ)
	}
}

// scoreCPM is a test helper that scores a partition under CPM via the same
// path the algorithm uses, but exposed only here so the public surface
// remains minimal.
func scoreCPM(nNodes int, edges []Edge, partition []int, gamma float64) (float64, error) {
	net, err := NewCompactNetwork(nNodes, edges)
	if err != nil {
		return 0, err
	}
	cl, err := NewClusteringFromAssignment(partition)
	if err != nil {
		return 0, err
	}
	return cpmQuality{Gamma: gamma}.value(net, cl), nil
}

func identityPartition(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}
