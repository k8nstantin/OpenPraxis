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
	"errors"
	"math"
	"sort"
	"testing"
)

// neighborSet returns the (neighbor, weight) entries for a node sorted by
// neighbor ID so tests can compare without depending on input edge order.
func neighborSet(n *CompactNetwork, node int) []Edge {
	nbrs := n.Neighbors(node)
	weights := n.NeighborWeights(node)
	out := make([]Edge, len(nbrs))
	for i := range nbrs {
		out[i] = Edge{From: node, To: nbrs[i], Weight: weights[i]}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].To < out[j].To })
	return out
}

func mustNetwork(t *testing.T, nNodes int, edges []Edge) *CompactNetwork {
	t.Helper()
	n, err := NewCompactNetwork(nNodes, edges)
	if err != nil {
		t.Fatalf("NewCompactNetwork: %v", err)
	}
	return n
}

func TestNewCompactNetwork_Triangle(t *testing.T) {
	edges := []Edge{
		{From: 0, To: 1, Weight: 1},
		{From: 1, To: 2, Weight: 1},
		{From: 2, To: 0, Weight: 1},
	}
	n := mustNetwork(t, 3, edges)

	if got, want := n.NumNodes(), 3; got != want {
		t.Errorf("NumNodes=%d, want %d", got, want)
	}
	if got, want := n.NumEdges(), 3; got != want {
		t.Errorf("NumEdges=%d, want %d", got, want)
	}
	if got, want := n.TotalEdgeWeight(), 3.0; got != want {
		t.Errorf("TotalEdgeWeight=%g, want %g", got, want)
	}
	if got, want := n.TotalNodeWeight(), 3.0; got != want {
		t.Errorf("TotalNodeWeight=%g, want %g", got, want)
	}

	for node := 0; node < 3; node++ {
		if d := n.Degree(node); d != 2 {
			t.Errorf("Degree(%d)=%d, want 2", node, d)
		}
		if s := n.NodeStrength(node); s != 2.0 {
			t.Errorf("NodeStrength(%d)=%g, want 2", node, s)
		}
	}

	want0 := []Edge{{From: 0, To: 1, Weight: 1}, {From: 0, To: 2, Weight: 1}}
	got0 := neighborSet(n, 0)
	if len(got0) != len(want0) {
		t.Fatalf("node 0 neighbors len=%d, want %d", len(got0), len(want0))
	}
	for i := range want0 {
		if got0[i] != want0[i] {
			t.Errorf("node 0 neighbor %d: got %+v, want %+v", i, got0[i], want0[i])
		}
	}
}

func TestNewCompactNetwork_SelfLoop(t *testing.T) {
	edges := []Edge{
		{From: 0, To: 0, Weight: 2.5},
		{From: 0, To: 1, Weight: 1.0},
	}
	n := mustNetwork(t, 2, edges)

	if d := n.Degree(0); d != 2 {
		t.Errorf("Degree(0)=%d, want 2 (self-loop + edge to 1)", d)
	}
	if d := n.Degree(1); d != 1 {
		t.Errorf("Degree(1)=%d, want 1", d)
	}
	if got, want := n.NodeStrength(0), 3.5; got != want {
		t.Errorf("NodeStrength(0)=%g, want %g", got, want)
	}
	if got, want := n.NodeStrength(1), 1.0; got != want {
		t.Errorf("NodeStrength(1)=%g, want %g", got, want)
	}
	if got, want := n.TotalEdgeWeight(), 3.5; got != want {
		t.Errorf("TotalEdgeWeight=%g, want %g", got, want)
	}
}

func TestNewCompactNetwork_Isolated(t *testing.T) {
	n := mustNetwork(t, 5, nil)
	for i := 0; i < 5; i++ {
		if d := n.Degree(i); d != 0 {
			t.Errorf("Degree(%d)=%d, want 0", i, d)
		}
		if s := n.NodeStrength(i); s != 0 {
			t.Errorf("NodeStrength(%d)=%g, want 0", i, s)
		}
		if w := n.NodeWeight(i); w != 1.0 {
			t.Errorf("NodeWeight(%d)=%g, want default 1", i, w)
		}
	}
	if n.TotalEdgeWeight() != 0 {
		t.Errorf("TotalEdgeWeight=%g, want 0", n.TotalEdgeWeight())
	}
	if n.TotalNodeWeight() != 5 {
		t.Errorf("TotalNodeWeight=%g, want 5", n.TotalNodeWeight())
	}
}

func TestNewCompactNetwork_Duplicates(t *testing.T) {
	edges := []Edge{
		{From: 0, To: 1, Weight: 0.5},
		{From: 0, To: 1, Weight: 0.25},
	}
	n := mustNetwork(t, 2, edges)

	if got, want := n.NumEdges(), 2; got != want {
		t.Errorf("NumEdges=%d, want %d", got, want)
	}
	if got, want := n.Degree(0), 2; got != want {
		t.Errorf("Degree(0)=%d, want %d", got, want)
	}
	if got, want := n.NodeStrength(0), 0.75; math.Abs(got-want) > 1e-12 {
		t.Errorf("NodeStrength(0)=%g, want %g", got, want)
	}
	if got, want := n.NodeStrength(1), 0.75; math.Abs(got-want) > 1e-12 {
		t.Errorf("NodeStrength(1)=%g, want %g", got, want)
	}
}

func TestNewCompactNetwork_CustomNodeWeights(t *testing.T) {
	weights := []float64{0.5, 1.5, 3.0}
	n, err := NewCompactNetworkWithNodeWeights(3, weights, []Edge{{From: 0, To: 1, Weight: 1}})
	if err != nil {
		t.Fatalf("ctor: %v", err)
	}
	for i, w := range weights {
		if got := n.NodeWeight(i); got != w {
			t.Errorf("NodeWeight(%d)=%g, want %g", i, got, w)
		}
	}
	if n.TotalNodeWeight() != 5.0 {
		t.Errorf("TotalNodeWeight=%g, want 5", n.TotalNodeWeight())
	}
}

func TestNewCompactNetwork_Errors(t *testing.T) {
	tests := []struct {
		name    string
		nNodes  int
		weights []float64
		edges   []Edge
		wantErr error
	}{
		{
			name:    "zero nodes",
			nNodes:  0,
			wantErr: ErrInvalidNodeCount,
		},
		{
			name:    "negative nodes",
			nNodes:  -1,
			wantErr: ErrInvalidNodeCount,
		},
		{
			name:    "node weights length mismatch",
			nNodes:  3,
			weights: []float64{1, 1},
			wantErr: ErrNodeWeightsLength,
		},
		{
			name:    "negative node weight",
			nNodes:  2,
			weights: []float64{1, -0.5},
			wantErr: ErrNegativeNodeWeight,
		},
		{
			name:    "edge From out of range",
			nNodes:  2,
			edges:   []Edge{{From: 2, To: 1, Weight: 1}},
			wantErr: ErrNodeOutOfRange,
		},
		{
			name:    "edge To out of range",
			nNodes:  2,
			edges:   []Edge{{From: 0, To: -1, Weight: 1}},
			wantErr: ErrNodeOutOfRange,
		},
		{
			name:    "negative edge weight",
			nNodes:  2,
			edges:   []Edge{{From: 0, To: 1, Weight: -0.1}},
			wantErr: ErrNegativeEdgeWeight,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewCompactNetworkWithNodeWeights(tt.nNodes, tt.weights, tt.edges)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err=%v, want errors.Is %v", err, tt.wantErr)
			}
		})
	}
}

func TestCompactNetwork_NeighborsAndWeights_LineGraph(t *testing.T) {
	// Line: 0 - 1 - 2 - 3, weights 1, 2, 3.
	edges := []Edge{
		{From: 0, To: 1, Weight: 1},
		{From: 1, To: 2, Weight: 2},
		{From: 2, To: 3, Weight: 3},
	}
	n := mustNetwork(t, 4, edges)

	if got, want := n.NodeStrength(1), 3.0; got != want {
		t.Errorf("strength(1)=%g, want %g", got, want)
	}
	if got, want := n.NodeStrength(2), 5.0; got != want {
		t.Errorf("strength(2)=%g, want %g", got, want)
	}
	want3 := []Edge{{From: 3, To: 2, Weight: 3}}
	got3 := neighborSet(n, 3)
	if len(got3) != 1 || got3[0] != want3[0] {
		t.Errorf("node 3 neighbors=%+v, want %+v", got3, want3)
	}
}

func BenchmarkNewCompactNetwork_Ring1k(b *testing.B) {
	const n = 1000
	edges := make([]Edge, n)
	for i := 0; i < n; i++ {
		edges[i] = Edge{From: i, To: (i + 1) % n, Weight: 1}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := NewCompactNetwork(n, edges); err != nil {
			b.Fatal(err)
		}
	}
}

func FuzzNewCompactNetwork(f *testing.F) {
	f.Add(4, []byte{0, 1, 1, 2, 2, 3})
	f.Fuzz(func(t *testing.T, nNodes int, raw []byte) {
		if nNodes <= 0 || nNodes > 64 {
			return
		}
		edges := make([]Edge, 0, len(raw)/2)
		for i := 0; i+1 < len(raw); i += 2 {
			from := int(raw[i]) % nNodes
			to := int(raw[i+1]) % nNodes
			edges = append(edges, Edge{From: from, To: to, Weight: 1})
		}
		n, err := NewCompactNetwork(nNodes, edges)
		if err != nil {
			t.Fatalf("unexpected error on valid input: %v", err)
		}
		if n.NumNodes() != nNodes {
			t.Fatalf("nNodes mismatch")
		}
		if n.NumEdges() != len(edges) {
			t.Fatalf("nEdges mismatch")
		}
	})
}
