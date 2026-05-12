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

package leiden_test

import (
	"context"
	"fmt"

	"github.com/k8nstantin/OpenPraxis/leiden"
)

// twoCliqueBridge builds the canonical 8-node test graph used throughout the
// examples: two K4 cliques joined by a single bridge edge (3—4).
func twoCliqueBridge() []leiden.Edge {
	var edges []leiden.Edge
	for i := 0; i < 4; i++ {
		for j := i + 1; j < 4; j++ {
			edges = append(edges, leiden.Edge{From: i, To: j, Weight: 1})
		}
	}
	for i := 4; i < 8; i++ {
		for j := i + 1; j < 8; j++ {
			edges = append(edges, leiden.Edge{From: i, To: j, Weight: 1})
		}
	}
	edges = append(edges, leiden.Edge{From: 3, To: 4, Weight: 1})
	return edges
}

// ExampleLeiden detects communities on a graph of two K4 cliques joined by a
// single bridge edge. The output is canonicalised through GroupBy so it is
// independent of internal cluster-ID assignment.
func ExampleLeiden() {
	opts := leiden.DefaultOptions()
	opts.Resolution = 0.5
	opts.Seed = 42

	res, err := leiden.Leiden(context.Background(), 8, twoCliqueBridge(), opts)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("communities:", res.NumClusters)
	for _, members := range leiden.GroupBy(res.Partition) {
		fmt.Println(members)
	}
	// Output:
	// communities: 2
	// [0 1 2 3]
	// [4 5 6 7]
}

// ExampleHierarchicalLeiden inspects the partition at every coarsening level.
func ExampleHierarchicalLeiden() {
	opts := leiden.DefaultOptions()
	opts.Resolution = 0.5
	opts.Seed = 42

	res, err := leiden.HierarchicalLeiden(context.Background(), 8, twoCliqueBridge(), opts)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	for i, lvl := range res.Levels {
		fmt.Printf("level %d: %d clusters\n", i, lvl.NumClusters)
	}
	fmt.Println("final clusters:", res.Final.NumClusters)
	// Output:
	// level 0: 2 clusters
	// level 1: 2 clusters
	// final clusters: 2
}

// ExampleModularity scores a hand-built partition with classical Newman
// modularity, independent of the Leiden algorithm itself.
func ExampleModularity() {
	edges := twoCliqueBridge()
	partition := []int{0, 0, 0, 0, 1, 1, 1, 1}

	q, err := leiden.Modularity(context.Background(), 8, edges, partition, 1.0)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("Q = %.4f\n", q)
	// Output:
	// Q = 0.4231
}

// ExampleCommunityCount counts the number of distinct cluster IDs in a
// partition slice.
func ExampleCommunityCount() {
	fmt.Println(leiden.CommunityCount([]int{2, 0, 2, 1, 0}))
	// Output: 3
}

// ExampleGroupBy projects a partition slice into per-community node lists.
func ExampleGroupBy() {
	for _, members := range leiden.GroupBy([]int{2, 0, 2, 1, 0}) {
		fmt.Println(members)
	}
	// Output:
	// [0 2]
	// [1 4]
	// [3]
}
