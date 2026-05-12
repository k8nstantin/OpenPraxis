package leiden

import (
	"context"
	"encoding/binary"
	"math"
	"testing"
)

// decodeEdges reads (u, v, weight) triples from edgeBlob into [0, nNodes).
// Each edge consumes 6 bytes (u: uint16, v: uint16, weight-byte: uint8,
// padding: uint8). Bytes past the last full triple are ignored. The
// fixed-width layout lets the fuzzer mutate edges meaningfully without the
// engine having to learn a complex framing.
func decodeEdges(edgeBlob []byte, nNodes int) []Edge {
	if nNodes <= 0 {
		return nil
	}
	out := make([]Edge, 0, len(edgeBlob)/6)
	for i := 0; i+6 <= len(edgeBlob); i += 6 {
		u := int(binary.LittleEndian.Uint16(edgeBlob[i:i+2])) % nNodes
		v := int(binary.LittleEndian.Uint16(edgeBlob[i+2:i+4])) % nNodes
		// Weight in [0, 16); avoids NaN/inf and keeps quality computations bounded.
		w := float64(edgeBlob[i+4]) / 16.0
		out = append(out, Edge{From: u, To: v, Weight: w})
	}
	return out
}

// decodePartition maps each byte to a cluster ID in [0, nNodes).
func decodePartition(blob []byte, nNodes int) []int {
	if nNodes <= 0 {
		return nil
	}
	out := make([]int, nNodes)
	for i := 0; i < nNodes; i++ {
		if i < len(blob) {
			out[i] = int(blob[i]) % nNodes
		}
	}
	return out
}

// FuzzLeiden drives [Leiden] with random small graphs. The contract under
// fuzz:
//
//   - No panic on any input.
//   - On success: partition has length nNodes, IDs are in [0, NumClusters),
//     Quality is finite, Iterations is positive.
func FuzzLeiden(f *testing.F) {
	f.Add(int8(8), []byte{0, 0, 1, 0, 16, 0, 1, 0, 2, 0, 16, 0}, 0.1, int64(1))
	f.Add(int8(3), []byte{}, 0.05, int64(0))
	f.Add(int8(1), []byte{}, 0.0, int64(99))

	f.Fuzz(func(t *testing.T, nNodes int8, edgeBlob []byte, gamma float64, seed int64) {
		// Clamp inputs to the public contract surface; this is the role of a
		// caller validating user input.
		n := int(nNodes)
		if n <= 0 || n > 64 {
			return
		}
		if math.IsNaN(gamma) || math.IsInf(gamma, 0) || gamma < 0 || gamma > 10 {
			return
		}
		opts := DefaultOptions()
		opts.Resolution = gamma
		opts.Seed = seed
		opts.MaxIterations = 20 // bound work per fuzz iteration

		res, err := Leiden(context.Background(), n, decodeEdges(edgeBlob, n), opts)
		if err != nil {
			return
		}
		if len(res.Partition) != n {
			t.Fatalf("len(Partition)=%d, want %d", len(res.Partition), n)
		}
		for i, c := range res.Partition {
			if c < 0 || c >= res.NumClusters {
				t.Fatalf("Partition[%d]=%d, NumClusters=%d", i, c, res.NumClusters)
			}
		}
		if math.IsNaN(res.Quality) || math.IsInf(res.Quality, 0) {
			t.Fatalf("Quality=%g is not finite", res.Quality)
		}
		if res.Iterations <= 0 {
			t.Fatalf("Iterations=%d, want >0", res.Iterations)
		}
	})
}

// FuzzHierarchicalLeiden mirrors FuzzLeiden for the multi-level entry
// point. Asserts: no panic; on success Levels is non-empty, every level's
// partition has length nNodes, and Final's NumClusters matches the last
// level. The strict coarsening invariant is not asserted here — local-move
// on the aggregated network can in principle redistribute a sub-cluster
// across parents, and that property is covered by the curated tests in
// leiden_test.go where the input is well-conditioned.
func FuzzHierarchicalLeiden(f *testing.F) {
	f.Add(int8(8), []byte{0, 0, 1, 0, 16, 0, 1, 0, 2, 0, 16, 0}, 0.1, int64(1))
	f.Add(int8(4), []byte{}, 0.05, int64(0))

	f.Fuzz(func(t *testing.T, nNodes int8, edgeBlob []byte, gamma float64, seed int64) {
		n := int(nNodes)
		if n <= 0 || n > 32 {
			return
		}
		if math.IsNaN(gamma) || math.IsInf(gamma, 0) || gamma < 0 || gamma > 10 {
			return
		}
		opts := DefaultOptions()
		opts.Resolution = gamma
		opts.Seed = seed
		opts.MaxIterations = 10

		res, err := HierarchicalLeiden(context.Background(), n, decodeEdges(edgeBlob, n), opts)
		if err != nil {
			return
		}
		if len(res.Levels) == 0 {
			t.Fatalf("Levels is empty after successful HierarchicalLeiden")
		}
		last := res.Levels[len(res.Levels)-1]
		if res.Final.NumClusters != last.NumClusters {
			t.Fatalf("Final.NumClusters=%d != last level NumClusters=%d",
				res.Final.NumClusters, last.NumClusters)
		}
		for li, lvl := range res.Levels {
			if len(lvl.Partition) != n {
				t.Fatalf("level %d Partition length=%d, want %d", li, len(lvl.Partition), n)
			}
			for i, c := range lvl.Partition {
				if c < 0 || c >= lvl.NumClusters {
					t.Fatalf("level %d Partition[%d]=%d, NumClusters=%d", li, i, c, lvl.NumClusters)
				}
			}
			if math.IsNaN(lvl.Quality) || math.IsInf(lvl.Quality, 0) {
				t.Fatalf("level %d Quality=%g is not finite", li, lvl.Quality)
			}
		}
	})
}

// FuzzModularity drives [Modularity] with arbitrary graphs and partitions.
// Asserts no panic and finite result when the inputs satisfy the documented
// preconditions.
func FuzzModularity(f *testing.F) {
	f.Add(int8(4), []byte{0, 0, 1, 0, 16, 0}, []byte{0, 0, 1, 1}, 1.0)
	f.Add(int8(3), []byte{}, []byte{0, 1, 2}, 0.5)

	f.Fuzz(func(t *testing.T, nNodes int8, edgeBlob []byte, partBlob []byte, gamma float64) {
		n := int(nNodes)
		if n <= 0 || n > 64 {
			return
		}
		if math.IsNaN(gamma) || math.IsInf(gamma, 0) || gamma < -10 || gamma > 10 {
			return
		}
		q, err := Modularity(context.Background(), n, decodeEdges(edgeBlob, n), decodePartition(partBlob, n), gamma)
		if err != nil {
			return
		}
		if math.IsNaN(q) || math.IsInf(q, 0) {
			t.Fatalf("Modularity returned non-finite value: %g", q)
		}
	})
}
