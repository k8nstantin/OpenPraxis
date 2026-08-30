package leiden

import "fmt"

// Clustering is a partition of nodes into clusters.
//
// Cluster IDs are non-negative integers. Cluster IDs are not required to be
// contiguous (i.e. a Clustering with nodes assigned to clusters 0, 2, 5 is
// valid); use [Clustering.Normalize] to renumber them into [0, k).
//
// The zero value of Clustering is not usable; construct one via
// [NewClustering], [NewSingletonClustering], or
// [NewClusteringFromAssignment].
type Clustering struct {
	nNodes     int
	nClusters  int
	assignment []int
}

// NewClustering returns a clustering with every node in cluster 0.
//
// Returns an error if nNodes is non-positive.
func NewClustering(nNodes int) (*Clustering, error) {
	if nNodes <= 0 {
		return nil, fmt.Errorf("%w: got %d", ErrInvalidNodeCount, nNodes)
	}
	return &Clustering{
		nNodes:     nNodes,
		nClusters:  1,
		assignment: make([]int, nNodes),
	}, nil
}

// NewSingletonClustering returns a clustering with each node in its own
// cluster: node i is assigned to cluster i.
//
// Returns an error if nNodes is non-positive.
func NewSingletonClustering(nNodes int) (*Clustering, error) {
	if nNodes <= 0 {
		return nil, fmt.Errorf("%w: got %d", ErrInvalidNodeCount, nNodes)
	}
	a := make([]int, nNodes)
	for i := range a {
		a[i] = i
	}
	return &Clustering{
		nNodes:     nNodes,
		nClusters:  nNodes,
		assignment: a,
	}, nil
}

// NewClusteringFromAssignment returns a clustering from an explicit
// node-to-cluster assignment. The returned clustering owns a copy of the
// input slice; the caller may safely mutate the original.
//
// Returns an error if the assignment is empty or contains a negative ID.
func NewClusteringFromAssignment(assignment []int) (*Clustering, error) {
	if len(assignment) == 0 {
		return nil, fmt.Errorf("%w: got 0", ErrInvalidNodeCount)
	}
	maxID := -1
	for i, c := range assignment {
		if c < 0 {
			return nil, fmt.Errorf("%w: node %d assigned to %d", ErrNegativeClusterID, i, c)
		}
		if c > maxID {
			maxID = c
		}
	}
	a := make([]int, len(assignment))
	copy(a, assignment)
	return &Clustering{
		nNodes:     len(assignment),
		nClusters:  maxID + 1,
		assignment: a,
	}, nil
}

// NumNodes returns the number of nodes in the clustering.
func (c *Clustering) NumNodes() int { return c.nNodes }

// NumClusters returns the cluster count: max(assignment)+1. Clusters in the
// range [0, NumClusters()) may be empty if [Clustering.Normalize] has not
// been called.
func (c *Clustering) NumClusters() int { return c.nClusters }

// Cluster returns the cluster ID assigned to the given node. The caller is
// responsible for ensuring 0 <= node < NumNodes().
func (c *Clustering) Cluster(node int) int { return c.assignment[node] }

// SetCluster assigns node to the given cluster, growing NumClusters if
// needed. Returns an error if the cluster ID is negative or the node is
// out of range.
func (c *Clustering) SetCluster(node, cluster int) error {
	if node < 0 || node >= c.nNodes {
		return fmt.Errorf("%w: node=%d, nNodes=%d", ErrNodeOutOfRange, node, c.nNodes)
	}
	if cluster < 0 {
		return fmt.Errorf("%w: cluster=%d", ErrNegativeClusterID, cluster)
	}
	c.assignment[node] = cluster
	if cluster+1 > c.nClusters {
		c.nClusters = cluster + 1
	}
	return nil
}

// Assignment returns a copy of the node-to-cluster assignment. The returned
// slice is owned by the caller and safe to mutate.
func (c *Clustering) Assignment() []int {
	out := make([]int, c.nNodes)
	copy(out, c.assignment)
	return out
}

// Sizes returns a copy of the per-cluster node counts. The returned slice
// has length [Clustering.NumClusters] and is owned by the caller.
func (c *Clustering) Sizes() []int {
	out := make([]int, c.nClusters)
	for _, cl := range c.assignment {
		out[cl]++
	}
	return out
}

// Nodes returns the node IDs assigned to the given cluster, in ascending
// order. The returned slice is owned by the caller. Returns an empty slice
// if the cluster ID is out of range or contains no nodes.
func (c *Clustering) Nodes(cluster int) []int {
	if cluster < 0 || cluster >= c.nClusters {
		return []int{}
	}
	var out []int
	for node, cl := range c.assignment {
		if cl == cluster {
			out = append(out, node)
		}
	}
	return out
}

// Normalize renumbers cluster IDs so they form a contiguous range [0, k),
// preserving the relative order of first appearance in the assignment.
// After normalization, [Clustering.NumClusters] equals the number of
// non-empty clusters.
func (c *Clustering) Normalize() {
	remap := make(map[int]int, c.nClusters)
	next := 0
	for i, cl := range c.assignment {
		newID, ok := remap[cl]
		if !ok {
			newID = next
			remap[cl] = newID
			next++
		}
		c.assignment[i] = newID
	}
	c.nClusters = next
}

// MergeClusters reassigns every node in cluster b to cluster a. Both IDs
// must be non-negative. Cluster b becomes empty but [Clustering.NumClusters]
// is not reduced until [Clustering.Normalize] is called.
//
// Returns an error if either ID is negative.
func (c *Clustering) MergeClusters(a, b int) error {
	if a < 0 || b < 0 {
		return fmt.Errorf("%w: a=%d, b=%d", ErrNegativeClusterID, a, b)
	}
	if a == b {
		return nil
	}
	for i, cl := range c.assignment {
		if cl == b {
			c.assignment[i] = a
		}
	}
	if a+1 > c.nClusters {
		c.nClusters = a + 1
	}
	return nil
}

// Clone returns an independent deep copy of the clustering.
func (c *Clustering) Clone() *Clustering {
	a := make([]int, c.nNodes)
	copy(a, c.assignment)
	return &Clustering{
		nNodes:     c.nNodes,
		nClusters:  c.nClusters,
		assignment: a,
	}
}
