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

import "sort"

// CommunityCount returns the number of distinct cluster IDs present in
// partition.
//
// Unlike [Result.NumClusters], which reports max(partition)+1, CommunityCount
// counts only IDs that appear at least once. For a normalized partition the
// two agree; for a sparse partition (e.g. one with unused IDs) CommunityCount
// is the meaningful "number of communities" measure.
//
// An empty partition returns 0. Negative cluster IDs are counted as distinct
// values; no validation is performed.
func CommunityCount(partition []int) int {
	if len(partition) == 0 {
		return 0
	}
	seen := make(map[int]struct{}, len(partition))
	for _, c := range partition {
		seen[c] = struct{}{}
	}
	return len(seen)
}

// GroupBy returns the per-community membership of partition: groups[k] is
// the slice of node IDs whose cluster ID, ordered by first appearance in
// partition, is the k-th distinct value.
//
// Within each group, node IDs are in ascending order. The returned slices
// are freshly allocated and owned by the caller.
//
// For an empty partition, GroupBy returns nil. Negative cluster IDs are
// permitted; no validation is performed.
//
// Example:
//
//	GroupBy([]int{2, 0, 2, 1, 0}) // [][]int{{0, 2}, {1, 4}, {3}}
func GroupBy(partition []int) [][]int {
	if len(partition) == 0 {
		return nil
	}
	// First-appearance order matches partitionGroups in this package's
	// existing tests and is stable across cluster-ID relabelling.
	order := make(map[int]int, len(partition))
	next := 0
	for _, c := range partition {
		if _, ok := order[c]; !ok {
			order[c] = next
			next++
		}
	}
	groups := make([][]int, next)
	for node, c := range partition {
		idx := order[c]
		groups[idx] = append(groups[idx], node)
	}
	for _, g := range groups {
		sort.Ints(g)
	}
	return groups
}
