package leiden

import (
	"math"
	"math/rand"
	"testing"
)

func TestAdjustedRandIndex_Identical(t *testing.T) {
	a := []int{0, 0, 1, 1, 2, 2}
	if got := adjustedRandIndex(a, a); math.Abs(got-1.0) > 1e-12 {
		t.Errorf("ARI(identical)=%g, want 1", got)
	}
}

func TestAdjustedRandIndex_RelabellingInvariant(t *testing.T) {
	a := []int{0, 0, 1, 1, 2, 2}
	b := []int{7, 7, 3, 3, 5, 5}
	if got := adjustedRandIndex(a, b); math.Abs(got-1.0) > 1e-12 {
		t.Errorf("ARI(relabelled)=%g, want 1", got)
	}
}

func TestAdjustedRandIndex_AllSameVsAllSingleton(t *testing.T) {
	a := []int{0, 0, 0, 0}
	b := []int{0, 1, 2, 3}
	// Both partitions are "trivial" w.r.t. each other; ARI is 0 by
	// convention when the denominator collapses.
	if got := adjustedRandIndex(a, b); math.Abs(got) > 1e-12 {
		t.Errorf("ARI(all-same vs singletons)=%g, want 0", got)
	}
}

func TestAdjustedRandIndex_RandomNearZero(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	const n = 500
	a := make([]int, n)
	b := make([]int, n)
	for i := range a {
		a[i] = rng.Intn(4)
		b[i] = rng.Intn(4)
	}
	got := adjustedRandIndex(a, b)
	if math.Abs(got) > 0.05 {
		t.Errorf("ARI(independent random)=%g, want |ARI|<0.05", got)
	}
}

func TestAdjustedRandIndex_PartialAgreement(t *testing.T) {
	a := []int{0, 0, 0, 0, 1, 1, 1, 1}
	b := []int{0, 0, 0, 1, 1, 1, 1, 1} // one node mislabelled
	got := adjustedRandIndex(a, b)
	if got <= 0 || got >= 1 {
		t.Errorf("ARI(one mismatch)=%g, want in (0,1)", got)
	}
}

func TestNormalizedMutualInformation_Identical(t *testing.T) {
	a := []int{0, 0, 1, 1, 2, 2}
	if got := normalizedMutualInformation(a, a); math.Abs(got-1.0) > 1e-12 {
		t.Errorf("NMI(identical)=%g, want 1", got)
	}
}

func TestNormalizedMutualInformation_Independent(t *testing.T) {
	a := []int{0, 0, 0, 0, 0, 0, 0, 0}
	b := []int{0, 1, 0, 1, 0, 1, 0, 1}
	// H(a)=0 → MI=0 → NMI=0 (per arithmetic-mean normalisation).
	if got := normalizedMutualInformation(a, b); got != 0 {
		t.Errorf("NMI(degenerate)=%g, want 0", got)
	}
}

func TestPlantedPartition_StructuralInvariants(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	n, edges, truth := plantedPartition(3, 10, 0.6, 0.1, rng)
	if n != 30 {
		t.Errorf("n=%d, want 30", n)
	}
	if len(truth) != n {
		t.Errorf("len(truth)=%d, want %d", len(truth), n)
	}
	for i, c := range truth {
		want := i / 10
		if c != want {
			t.Errorf("truth[%d]=%d, want %d", i, c, want)
		}
	}
	intra, inter := 0, 0
	for _, e := range edges {
		if truth[e.From] == truth[e.To] {
			intra++
		} else {
			inter++
		}
	}
	if intra == 0 || inter == 0 {
		t.Errorf("expected mix of intra/inter edges, got intra=%d inter=%d", intra, inter)
	}
	if intra <= inter {
		t.Errorf("planted partition: intra=%d should exceed inter=%d for pIn>pOut", intra, inter)
	}
}

func TestLoadEdges_KarateClub(t *testing.T) {
	n, edges := mustLoadEdges(t, "testdata/karate.edges")
	if n != 34 {
		t.Errorf("karate n=%d, want 34", n)
	}
	if len(edges) != 78 {
		t.Errorf("karate edges=%d, want 78", len(edges))
	}
}

func TestLoadPartition_KarateGroundTruth(t *testing.T) {
	p := mustLoadPartition(t, "testdata/karate_ground_truth.partition")
	if len(p) != 34 {
		t.Errorf("karate ground truth length=%d, want 34", len(p))
	}
	// Mr. Hi (cluster 0) and the officer (cluster 1) must be on opposite sides.
	if p[0] == p[33] {
		t.Errorf("nodes 0 and 33 share a cluster in ground truth; should differ")
	}
}
