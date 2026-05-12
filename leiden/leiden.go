package leiden

import (
	"fmt"
	"math/rand"
)

// Options configures a [Leiden] or [HierarchicalLeiden] run.
//
// Construct an Options via [DefaultOptions] and modify the fields you need;
// constructing a literal directly is permitted but will not pick up future
// fields' defaults.
type Options struct {
	// Resolution is the γ parameter of the Constant Potts Model quality
	// function. Larger values favour smaller, more numerous clusters.
	// Must be ≥ 0.
	Resolution float64

	// Randomness is the θ parameter governing refinement-phase candidate
	// selection. θ ≤ 0 chooses the deterministic argmax of ΔH; θ > 0 samples
	// a candidate with probability ∝ exp(ΔH/θ). Larger θ produces more
	// varied sub-clusters; smaller θ sharpens toward argmax.
	Randomness float64

	// MaxIterations caps the number of coarsening iterations. The run also
	// terminates as soon as a local-move pass makes zero moves. A value of
	// zero means "use the package default" (currently 100). Negative values
	// are rejected.
	MaxIterations int

	// Seed initialises the deterministic pseudo-random number generator
	// used to shuffle node visit orders and (when Randomness > 0) to sample
	// refinement candidates. Two runs with the same Seed on the same input
	// produce identical output.
	Seed int64

	// NodeWeights, if non-nil, overrides the default unit weight for each
	// node. Length must equal nNodes. Weights must be non-negative.
	NodeWeights []float64
}

// DefaultOptions returns recommended Leiden settings:
//
//   - Resolution: 0.05
//   - Randomness: 0.01
//   - MaxIterations: 100
//   - Seed: 0
//   - NodeWeights: nil (each node weight defaults to 1)
//
// These match the defaults of the reference networkanalysis-java
// implementation by CWTS.
func DefaultOptions() Options {
	return Options{
		Resolution:    0.05,
		Randomness:    0.01,
		MaxIterations: defaultMaxIterations,
		Seed:          0,
	}
}

// Result is the outcome of a single [Leiden] run.
type Result struct {
	// Partition[i] is the cluster ID assigned to node i. Cluster IDs are
	// contiguous in [0, NumClusters). The slice is owned by the caller.
	Partition []int

	// NumClusters is the number of distinct clusters in Partition.
	NumClusters int

	// Quality is the CPM quality of Partition under the configured
	// Resolution.
	Quality float64

	// Iterations is the number of coarsening iterations the algorithm
	// performed before converging or reaching MaxIterations.
	Iterations int
}

// LevelResult is the partition observed at one coarsening level of a
// [HierarchicalLeiden] run.
type LevelResult struct {
	// Partition[i] is the cluster ID of original node i at this level.
	// The slice is owned by the caller.
	Partition []int

	// NumClusters is the number of distinct clusters in Partition.
	NumClusters int

	// Quality is the CPM quality of Partition on the input network under
	// the configured Resolution.
	Quality float64
}

// HierarchicalResult is the outcome of a single [HierarchicalLeiden] run.
//
// Levels are ordered from finest (most clusters) to coarsest (fewest
// clusters). Every cluster at level k is a union of clusters at level k-1.
type HierarchicalResult struct {
	// Levels is the per-coarsening-iteration partition of the original
	// nodes. Levels has at least one element.
	Levels []LevelResult

	// Final is the converged partition (the last element of Levels).
	Final Result
}

// defaultMaxIterations is used when [Options].MaxIterations is zero.
const defaultMaxIterations = 100

// Leiden runs the Leiden community-detection algorithm on a weighted,
// undirected graph and returns the final flat partition.
//
// The optimised objective is the Constant Potts Model (CPM) with the
// configured resolution. To score a partition with classical modularity
// after the fact, use [Modularity].
//
// nNodes must be positive. edges may be empty, may contain self-loops, and
// may contain duplicate node pairs (whose weights sum). Edge weights and
// node weights must be non-negative.
//
// Reference: Traag, Waltman & van Eck, "From Louvain to Leiden: guaranteeing
// well-connected communities", Scientific Reports 9:5233 (2019).
func Leiden(nNodes int, edges []Edge, opts Options) (Result, error) {
	res, err := runLeiden(nNodes, edges, opts)
	if err != nil {
		return Result{}, err
	}
	return res.Final, nil
}

// HierarchicalLeiden runs the Leiden algorithm and returns the partition at
// every coarsening level, not only the final converged partition. Use this
// when the multi-level structure of the graph is informative (e.g. building
// a community dendrogram or selecting a resolution after the fact).
//
// HierarchicalLeiden has the same input contract as [Leiden].
func HierarchicalLeiden(nNodes int, edges []Edge, opts Options) (HierarchicalResult, error) {
	return runLeiden(nNodes, edges, opts)
}

func runLeiden(nNodes int, edges []Edge, opts Options) (HierarchicalResult, error) {
	if opts.MaxIterations < 0 {
		return HierarchicalResult{}, fmt.Errorf("leiden: MaxIterations must be non-negative, got %d", opts.MaxIterations)
	}
	maxIter := opts.MaxIterations
	if maxIter == 0 {
		maxIter = defaultMaxIterations
	}

	net, err := NewCompactNetworkWithNodeWeights(nNodes, opts.NodeWeights, edges)
	if err != nil {
		return HierarchicalResult{}, err
	}
	q := cpmQuality{Gamma: opts.Resolution}
	rng := rand.New(rand.NewSource(opts.Seed))

	parent, err := NewSingletonClustering(nNodes)
	if err != nil {
		return HierarchicalResult{}, err
	}

	// origToCurrent[u] is the index of original node u in the current
	// (possibly aggregated) network. Initially the identity map.
	origToCurrent := make([]int, nNodes)
	for i := range origToCurrent {
		origToCurrent[i] = i
	}
	currentNet := net

	var levels []LevelResult
	for iter := 0; iter < maxIter; iter++ {
		moves := runLocalMove(currentNet, parent, q, rng)

		levelPart := projectToOriginal(origToCurrent, parent, nNodes)
		levels = append(levels, LevelResult{
			Partition:   levelPart.Assignment(),
			NumClusters: levelPart.NumClusters(),
			Quality:     q.value(net, levelPart),
		})

		if moves == 0 {
			break
		}

		refined := runRefinement(currentNet, parent, q, opts.Randomness, rng)
		aggNet, aggInit, err := aggregateNetwork(currentNet, refined, parent)
		if err != nil {
			return HierarchicalResult{}, err
		}

		for u := 0; u < nNodes; u++ {
			origToCurrent[u] = refined.assignment[origToCurrent[u]]
		}
		currentNet = aggNet
		parent = aggInit
	}

	last := levels[len(levels)-1]
	finalPart := make([]int, len(last.Partition))
	copy(finalPart, last.Partition)
	return HierarchicalResult{
		Levels: levels,
		Final: Result{
			Partition:   finalPart,
			NumClusters: last.NumClusters,
			Quality:     last.Quality,
			Iterations:  len(levels),
		},
	}, nil
}

// projectToOriginal returns a normalized clustering of the original nodes
// induced by the current (possibly aggregated) partition.
//
// For each original node u, its cluster ID is the cluster ID of its current
// network node origToCurrent[u] under parent.
func projectToOriginal(origToCurrent []int, parent *Clustering, nNodes int) *Clustering {
	a := make([]int, nNodes)
	for u := 0; u < nNodes; u++ {
		a[u] = parent.assignment[origToCurrent[u]]
	}
	cl, _ := NewClusteringFromAssignment(a)
	cl.Normalize()
	return cl
}
