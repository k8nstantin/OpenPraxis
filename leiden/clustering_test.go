package leiden

import (
	"errors"
	"reflect"
	"testing"
)

func TestNewClustering_AllZero(t *testing.T) {
	c, err := NewClustering(4)
	if err != nil {
		t.Fatal(err)
	}
	if c.NumNodes() != 4 {
		t.Errorf("NumNodes=%d, want 4", c.NumNodes())
	}
	if c.NumClusters() != 1 {
		t.Errorf("NumClusters=%d, want 1", c.NumClusters())
	}
	for i := 0; i < 4; i++ {
		if c.Cluster(i) != 0 {
			t.Errorf("Cluster(%d)=%d, want 0", i, c.Cluster(i))
		}
	}
}

func TestNewSingletonClustering(t *testing.T) {
	c, err := NewSingletonClustering(3)
	if err != nil {
		t.Fatal(err)
	}
	if c.NumClusters() != 3 {
		t.Errorf("NumClusters=%d, want 3", c.NumClusters())
	}
	for i := 0; i < 3; i++ {
		if c.Cluster(i) != i {
			t.Errorf("Cluster(%d)=%d, want %d", i, c.Cluster(i), i)
		}
	}
}

func TestNewClusteringFromAssignment(t *testing.T) {
	in := []int{0, 0, 2, 1, 2}
	c, err := NewClusteringFromAssignment(in)
	if err != nil {
		t.Fatal(err)
	}
	if c.NumNodes() != 5 {
		t.Errorf("NumNodes=%d", c.NumNodes())
	}
	if c.NumClusters() != 3 {
		t.Errorf("NumClusters=%d, want 3 (max+1)", c.NumClusters())
	}
	// Caller mutating input must not affect clustering.
	in[0] = 99
	if c.Cluster(0) != 0 {
		t.Errorf("clustering shared input slice: Cluster(0)=%d", c.Cluster(0))
	}
}

func TestNewClustering_Errors(t *testing.T) {
	tests := []struct {
		name string
		fn   func() error
		want error
	}{
		{
			name: "zero nodes",
			fn:   func() error { _, e := NewClustering(0); return e },
			want: ErrInvalidNodeCount,
		},
		{
			name: "negative nodes singleton",
			fn:   func() error { _, e := NewSingletonClustering(-1); return e },
			want: ErrInvalidNodeCount,
		},
		{
			name: "empty assignment",
			fn:   func() error { _, e := NewClusteringFromAssignment(nil); return e },
			want: ErrInvalidNodeCount,
		},
		{
			name: "negative cluster id",
			fn:   func() error { _, e := NewClusteringFromAssignment([]int{0, -1}); return e },
			want: ErrNegativeClusterID,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); !errors.Is(err, tt.want) {
				t.Fatalf("got %v, want errors.Is %v", err, tt.want)
			}
		})
	}
}

func TestClustering_SetCluster(t *testing.T) {
	c, _ := NewClustering(3)
	if err := c.SetCluster(1, 5); err != nil {
		t.Fatal(err)
	}
	if c.Cluster(1) != 5 {
		t.Errorf("Cluster(1)=%d, want 5", c.Cluster(1))
	}
	if c.NumClusters() != 6 {
		t.Errorf("NumClusters=%d, want 6", c.NumClusters())
	}

	if err := c.SetCluster(-1, 0); !errors.Is(err, ErrNodeOutOfRange) {
		t.Errorf("expected ErrNodeOutOfRange, got %v", err)
	}
	if err := c.SetCluster(0, -1); !errors.Is(err, ErrNegativeClusterID) {
		t.Errorf("expected ErrNegativeClusterID, got %v", err)
	}
}

func TestClustering_Assignment_ReturnsCopy(t *testing.T) {
	c, _ := NewClusteringFromAssignment([]int{0, 1, 0, 1})
	a := c.Assignment()
	a[0] = 99
	if c.Cluster(0) != 0 {
		t.Errorf("Assignment() shared internal slice; Cluster(0)=%d", c.Cluster(0))
	}
}

func TestClustering_Sizes(t *testing.T) {
	c, _ := NewClusteringFromAssignment([]int{0, 1, 0, 2, 2, 2})
	sizes := c.Sizes()
	want := []int{2, 1, 3}
	if !reflect.DeepEqual(sizes, want) {
		t.Errorf("Sizes=%v, want %v", sizes, want)
	}
}

func TestClustering_Nodes(t *testing.T) {
	c, _ := NewClusteringFromAssignment([]int{2, 0, 2, 1, 0})
	if got, want := c.Nodes(0), []int{1, 4}; !reflect.DeepEqual(got, want) {
		t.Errorf("Nodes(0)=%v, want %v", got, want)
	}
	if got, want := c.Nodes(2), []int{0, 2}; !reflect.DeepEqual(got, want) {
		t.Errorf("Nodes(2)=%v, want %v", got, want)
	}
	if got := c.Nodes(99); len(got) != 0 {
		t.Errorf("Nodes(99) should be empty, got %v", got)
	}
	if got := c.Nodes(-1); len(got) != 0 {
		t.Errorf("Nodes(-1) should be empty, got %v", got)
	}
}

func TestClustering_Normalize(t *testing.T) {
	c, _ := NewClusteringFromAssignment([]int{2, 5, 2, 5, 9})
	c.Normalize()
	if c.NumClusters() != 3 {
		t.Errorf("NumClusters=%d, want 3", c.NumClusters())
	}
	// First-appearance order: 2→0, 5→1, 9→2.
	want := []int{0, 1, 0, 1, 2}
	got := c.Assignment()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Assignment=%v, want %v", got, want)
	}
}

func TestClustering_MergeClusters(t *testing.T) {
	c, _ := NewClusteringFromAssignment([]int{0, 1, 2, 1, 0})
	if err := c.MergeClusters(0, 1); err != nil {
		t.Fatal(err)
	}
	want := []int{0, 0, 2, 0, 0}
	if got := c.Assignment(); !reflect.DeepEqual(got, want) {
		t.Errorf("after merge: %v, want %v", got, want)
	}
	// NumClusters retains the upper bound until Normalize.
	if c.NumClusters() != 3 {
		t.Errorf("NumClusters=%d, want 3 (pre-normalize)", c.NumClusters())
	}
	c.Normalize()
	if c.NumClusters() != 2 {
		t.Errorf("NumClusters=%d, want 2 (post-normalize)", c.NumClusters())
	}

	if err := c.MergeClusters(-1, 0); !errors.Is(err, ErrNegativeClusterID) {
		t.Errorf("got %v, want ErrNegativeClusterID", err)
	}

	// a == b is a no-op.
	before := c.Assignment()
	if err := c.MergeClusters(1, 1); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.Assignment(), before) {
		t.Errorf("merging cluster with itself changed assignment")
	}
}

func TestClustering_Clone_Independent(t *testing.T) {
	a, _ := NewClusteringFromAssignment([]int{0, 1, 0, 1})
	b := a.Clone()
	if err := b.SetCluster(0, 2); err != nil {
		t.Fatal(err)
	}
	if a.Cluster(0) != 0 {
		t.Errorf("Clone is not independent: original mutated to %d", a.Cluster(0))
	}
	if b.Cluster(0) != 2 {
		t.Errorf("Clone mutation failed: got %d", b.Cluster(0))
	}
}
