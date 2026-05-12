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
	"math"
	"testing"
)

func TestModularity_OneClusterIsZero(t *testing.T) {
	// On any graph, placing all nodes in one cluster gives Q = 0 because
	// Σ A_ij = 2m and Σ k_i k_j / 2m = 2m, so the bracket sums to 0.
	edges := []Edge{
		{From: 0, To: 1, Weight: 1},
		{From: 1, To: 2, Weight: 2},
		{From: 0, To: 2, Weight: 3},
	}
	q, err := Modularity(context.Background(), 3, edges, []int{0, 0, 0}, 1.0)
	if err != nil {
		t.Fatalf("Modularity: %v", err)
	}
	if !floatEq(q, 0) {
		t.Errorf("Q=%g, want 0", q)
	}
}

func TestModularity_SingletonsInCompleteGraph(t *testing.T) {
	// For K_n with unit weights and singletons: Q = -1/n.
	const n = 4
	var edges []Edge
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			edges = append(edges, Edge{From: i, To: j, Weight: 1})
		}
	}
	part := identityPartition(n)
	q, err := Modularity(context.Background(), n, edges, part, 1.0)
	if err != nil {
		t.Fatalf("Modularity: %v", err)
	}
	want := -1.0 / float64(n)
	if math.Abs(q-want) > 1e-9 {
		t.Errorf("Q=%g, want %g", q, want)
	}
}

func TestModularity_TwoCliqueBridge_OptimalBeatsSingletons(t *testing.T) {
	edges := twoCliqueBridgeEdges()
	twoCommunities := []int{0, 0, 0, 0, 1, 1, 1, 1}
	qOpt, err := Modularity(context.Background(), 8, edges, twoCommunities, 1.0)
	if err != nil {
		t.Fatalf("Modularity (opt): %v", err)
	}
	qSing, err := Modularity(context.Background(), 8, edges, identityPartition(8), 1.0)
	if err != nil {
		t.Fatalf("Modularity (singleton): %v", err)
	}
	if qOpt <= qSing {
		t.Errorf("two-community Q=%g not > singleton Q=%g", qOpt, qSing)
	}
	if qOpt <= 0 {
		t.Errorf("two-community Q=%g, want >0", qOpt)
	}
}

func TestModularity_NoEdgesIsZero(t *testing.T) {
	q, err := Modularity(context.Background(), 3, nil, []int{0, 1, 2}, 1.0)
	if err != nil {
		t.Fatalf("Modularity: %v", err)
	}
	if q != 0 {
		t.Errorf("Q=%g, want 0 (degenerate empty graph)", q)
	}
}

func TestModularity_GammaScalesPenalty(t *testing.T) {
	// Larger gamma should make the one-cluster Q monotonically smaller
	// (more penalty) than singletons for a graph with structure.
	edges := twoCliqueBridgeEdges()
	one := []int{0, 0, 0, 0, 0, 0, 0, 0}
	q1, err := Modularity(context.Background(), 8, edges, one, 1.0)
	if err != nil {
		t.Fatalf("Modularity (gamma=1): %v", err)
	}
	q2, err := Modularity(context.Background(), 8, edges, one, 2.0)
	if err != nil {
		t.Fatalf("Modularity (gamma=2): %v", err)
	}
	if !floatEq(q1, 0) {
		t.Errorf("Q(gamma=1, one-cluster)=%g, want 0", q1)
	}
	if q2 >= q1 {
		t.Errorf("Q(gamma=2)=%g should be < Q(gamma=1)=%g for whole-graph cluster", q2, q1)
	}
}

func TestModularity_InvalidInputs(t *testing.T) {
	tests := []struct {
		name      string
		nNodes    int
		edges     []Edge
		partition []int
		wantErr   error
	}{
		{"partition length mismatch", 3, nil, []int{0, 0}, ErrAssignmentLength},
		{"negative cluster id", 3, nil, []int{0, -1, 0}, ErrNegativeClusterID},
		{"zero nodes", 0, nil, []int{0}, ErrAssignmentLength},
		{"zero nodes empty partition", 0, nil, []int{}, ErrInvalidNodeCount},
		{"out-of-range edge", 3, []Edge{{From: 0, To: 5, Weight: 1}}, []int{0, 0, 0}, ErrNodeOutOfRange},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Modularity(context.Background(), tt.nNodes, tt.edges, tt.partition, 1.0)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("err=%v, want errors.Is %v", err, tt.wantErr)
			}
		})
	}
}
