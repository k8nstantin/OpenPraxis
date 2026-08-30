# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-05-12

Initial public release. A pure-Go implementation of the Leiden algorithm
(Traag, Waltman & van Eck, 2019) for community detection in weighted,
undirected graphs. Zero external dependencies; stdlib only.

### Added

- Core data structures:
  - `Edge` — undirected, weighted edge input type.
  - `CompactNetwork` — immutable CSR-style graph representation, with
    `NewCompactNetwork` and `NewCompactNetworkWithNodeWeights` constructors
    that validate inputs and return typed sentinel errors.
  - `Clustering` — node-to-cluster partition with `NewSingletonClustering`,
    `NewClusteringFromAssignment`, `Assignment`, `NumClusters`, and
    `Normalize`.
- Algorithm phases (Traag et al. 2019):
  - Local-move phase that converges to a local CPM optimum.
  - Refinement phase with deterministic argmax tiebreak and θ-sampling
    when `Options.Randomness > 0`.
  - Aggregation phase that builds the coarsened network for the next
    iteration.
- Public API:
  - `Leiden(ctx, nNodes, edges, opts) (Result, error)` — runs to
    convergence and returns the final flat partition.
  - `HierarchicalLeiden(ctx, nNodes, edges, opts) (HierarchicalResult, error)`
    — returns the partition at every coarsening level.
  - `Options` and `DefaultOptions()` for forward-compatible configuration
    (resolution, randomness, max iterations, seed, optional node weights).
  - `Modularity(nNodes, edges, partition) (float64, error)` — generalised
    Newman modularity, independent of the algorithm itself.
  - `CommunityCount(partition) int` and `GroupBy(partition) [][]int`
    post-processing helpers that operate on a flat `[]int` partition
    without constructing a `Clustering`.
- Quality functions:
  - Constant Potts Model (CPM) — the optimisation objective.
  - Newman modularity — for post-hoc scoring.
- Typed sentinel errors for all input-validation failures
  (`ErrInvalidNodeCount`, `ErrNodeOutOfRange`, `ErrNegativeEdgeWeight`,
  `ErrNegativeNodeWeight`, `ErrNodeWeightsLength`, `ErrAssignmentLength`,
  `ErrNegativeClusterID`, `ErrNilContext`). Compare with `errors.Is`.
- `context.Context` cancellation on every long-running public entry
  point. Cancelled runs return `ctx.Err()` (test with `errors.Is` against
  `context.Canceled` / `context.DeadlineExceeded`).
- Seed-deterministic runs: two calls with the same `Options.Seed` on the
  same input produce identical output.
- Test suite:
  - Unit tests for every exported function.
  - Integration tests on Zachary's karate-club graph and planted-partition
    graphs.
  - Benchmarks at multiple scales.
  - Fuzz tests on every constructor.
  - Cross-check tests against the `graspologic-native` reference output
    on the karate club partition.
- Documentation:
  - `doc.go` package overview.
  - Apache 2.0 copyright headers on every source file.
  - `example_test.go` runnable godoc example.

### Notes

- Module path: `github.com/k8nstantin/OpenPraxis/leiden`.
- Minimum Go version: 1.22.
- Zero external dependencies — stdlib only.
- Reference: Traag, V.A., Waltman, L. & van Eck, N.J. *From Louvain to
  Leiden: guaranteeing well-connected communities*. Sci Rep 9, 5233 (2019).

[Unreleased]: https://github.com/k8nstantin/OpenPraxis/compare/leiden/v0.1.0...HEAD
[0.1.0]: https://github.com/k8nstantin/OpenPraxis/releases/tag/leiden%2Fv0.1.0
