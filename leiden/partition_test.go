package leiden

import (
	"context"
	"reflect"
	"testing"
)

func TestCommunityCount(t *testing.T) {
	tests := []struct {
		name      string
		partition []int
		want      int
	}{
		{"empty", nil, 0},
		{"single", []int{7}, 1},
		{"all same", []int{2, 2, 2, 2}, 1},
		{"singletons", []int{0, 1, 2, 3}, 4},
		{"sparse ids", []int{0, 5, 0, 5, 9}, 3},
		{"with negative", []int{-1, 0, -1, 1}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CommunityCount(tt.partition); got != tt.want {
				t.Errorf("CommunityCount(%v) = %d, want %d", tt.partition, got, tt.want)
			}
		})
	}
}

func TestCommunityCount_MatchesResultOnNormalizedPartition(t *testing.T) {
	res, err := Leiden(context.Background(), 8, twoCliqueBridgeEdges(), withResolution(0.5))
	if err != nil {
		t.Fatalf("Leiden: %v", err)
	}
	if got, want := CommunityCount(res.Partition), res.NumClusters; got != want {
		t.Errorf("CommunityCount=%d, want NumClusters=%d", got, want)
	}
}

func TestGroupBy(t *testing.T) {
	tests := []struct {
		name      string
		partition []int
		want      [][]int
	}{
		{"empty", nil, nil},
		{"singletons", []int{0, 1, 2}, [][]int{{0}, {1}, {2}}},
		{"first-appearance order", []int{2, 0, 2, 1, 0}, [][]int{{0, 2}, {1, 4}, {3}}},
		{"sparse ids", []int{5, 9, 5, 9}, [][]int{{0, 2}, {1, 3}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GroupBy(tt.partition)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GroupBy(%v) = %v, want %v", tt.partition, got, tt.want)
			}
		})
	}
}

func TestGroupBy_ReturnsOwnedSlices(t *testing.T) {
	part := []int{0, 1, 0, 1}
	groups := GroupBy(part)
	// Mutating returned groups must not affect input.
	groups[0][0] = 99
	if part[0] != 0 {
		t.Errorf("GroupBy aliased input slice")
	}
	// Mutating input must not affect returned groups.
	part[2] = 5
	if groups[0][1] != 2 {
		t.Errorf("GroupBy aliased input data; group[0][1]=%d", groups[0][1])
	}
}

func TestGroupBy_CoversAllNodes(t *testing.T) {
	res, err := Leiden(context.Background(), 8, twoCliqueBridgeEdges(), withResolution(0.5))
	if err != nil {
		t.Fatalf("Leiden: %v", err)
	}
	groups := GroupBy(res.Partition)
	total := 0
	for _, g := range groups {
		total += len(g)
	}
	if total != len(res.Partition) {
		t.Errorf("GroupBy emitted %d total nodes, want %d", total, len(res.Partition))
	}
	if len(groups) != CommunityCount(res.Partition) {
		t.Errorf("len(GroupBy)=%d != CommunityCount=%d", len(groups), CommunityCount(res.Partition))
	}
}

// withResolution returns DefaultOptions with Resolution overridden.
func withResolution(gamma float64) Options {
	o := DefaultOptions()
	o.Resolution = gamma
	return o
}
