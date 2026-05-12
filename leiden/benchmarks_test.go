package leiden

import (
	"math/rand"
	"testing"
)

// benchPlantedFixture is a cached planted-partition fixture for benchmarks.
// Generation cost is intentionally outside the timed loop.
type benchPlantedFixture struct {
	nNodes int
	edges  []Edge
}

func makeBenchFixture(b *testing.B, nPerBlock int, pIn, pOut float64) benchPlantedFixture {
	b.Helper()
	const blocks = 4
	rng := rand.New(rand.NewSource(int64(nPerBlock)))
	n, edges, _ := plantedPartition(blocks, nPerBlock, pIn, pOut, rng)
	return benchPlantedFixture{nNodes: n, edges: edges}
}

// BenchmarkLeiden_PlantedPartition_100 covers the small-graph hot path:
// 100 nodes, dense intra-block connectivity, light inter-block noise.
func BenchmarkLeiden_PlantedPartition_100(b *testing.B) {
	fx := makeBenchFixture(b, 25, 0.6, 0.05)
	benchmarkLeiden(b, fx)
}

// BenchmarkLeiden_PlantedPartition_1k covers a medium graph used to stress
// the local-move / refinement / aggregation loop interactions.
func BenchmarkLeiden_PlantedPartition_1k(b *testing.B) {
	fx := makeBenchFixture(b, 250, 0.05, 0.001)
	benchmarkLeiden(b, fx)
}

// BenchmarkLeiden_PlantedPartition_10k stresses the large-graph regime;
// edge density is kept modest so each benchmark iteration completes in
// seconds rather than minutes.
func BenchmarkLeiden_PlantedPartition_10k(b *testing.B) {
	fx := makeBenchFixture(b, 2500, 0.01, 0.0001)
	benchmarkLeiden(b, fx)
}

// BenchmarkHierarchicalLeiden_PlantedPartition_1k measures the hierarchical
// entry point at medium scale.
func BenchmarkHierarchicalLeiden_PlantedPartition_1k(b *testing.B) {
	fx := makeBenchFixture(b, 250, 0.05, 0.001)
	opts := DefaultOptions()
	opts.Resolution = 0.05
	opts.Seed = 1
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := HierarchicalLeiden(fx.nNodes, fx.edges, opts)
		if err != nil {
			b.Fatalf("HierarchicalLeiden: %v", err)
		}
	}
}

// BenchmarkNewCompactNetwork_PlantedPartition_10k measures graph
// construction (CSR build + degree pass + strength accumulation) on the
// large fixture.
func BenchmarkNewCompactNetwork_PlantedPartition_10k(b *testing.B) {
	fx := makeBenchFixture(b, 2500, 0.01, 0.0001)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := NewCompactNetwork(fx.nNodes, fx.edges)
		if err != nil {
			b.Fatalf("NewCompactNetwork: %v", err)
		}
	}
}

// BenchmarkModularity_PlantedPartition_1k scores a fixed singleton
// partition; the reported cost is the modularity evaluation, not the
// algorithm.
func BenchmarkModularity_PlantedPartition_1k(b *testing.B) {
	fx := makeBenchFixture(b, 250, 0.05, 0.001)
	partition := make([]int, fx.nNodes)
	for i := range partition {
		partition[i] = i % 8
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Modularity(fx.nNodes, fx.edges, partition, 1.0)
		if err != nil {
			b.Fatalf("Modularity: %v", err)
		}
	}
}

// BenchmarkLeiden_KarateClub measures the canonical small-graph hot path
// — repeated runs over Zachary's karate club at the default resolution.
func BenchmarkLeiden_KarateClub(b *testing.B) {
	n, edges := mustLoadEdges(b, "testdata/karate.edges")
	opts := DefaultOptions()
	opts.Resolution = 0.1
	opts.Seed = 1
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Leiden(n, edges, opts)
		if err != nil {
			b.Fatalf("Leiden: %v", err)
		}
	}
}

func benchmarkLeiden(b *testing.B, fx benchPlantedFixture) {
	opts := DefaultOptions()
	opts.Resolution = 0.05
	opts.Seed = 1
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Leiden(fx.nNodes, fx.edges, opts)
		if err != nil {
			b.Fatalf("Leiden: %v", err)
		}
	}
}
