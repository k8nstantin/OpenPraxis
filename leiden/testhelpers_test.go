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

import (
	"bufio"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"
)

// mustLoadEdges parses an edge list (one "u v [w]" per line, '#'-comment
// lines and blanks ignored) and returns the implied node count plus the
// edge slice. Missing weights default to 1.0. The caller's t.Fatal is
// invoked on parse failure.
func mustLoadEdges(t testing.TB, path string) (int, []Edge) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	var edges []Edge
	maxNode := -1
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			t.Fatalf("malformed edge line %q in %s", line, path)
		}
		u, err1 := strconv.Atoi(parts[0])
		v, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			t.Fatalf("non-integer node in line %q: %v / %v", line, err1, err2)
		}
		w := 1.0
		if len(parts) >= 3 {
			pw, err := strconv.ParseFloat(parts[2], 64)
			if err != nil {
				t.Fatalf("bad weight %q in line %q: %v", parts[2], line, err)
			}
			w = pw
		}
		if u > maxNode {
			maxNode = u
		}
		if v > maxNode {
			maxNode = v
		}
		edges = append(edges, Edge{From: u, To: v, Weight: w})
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	if maxNode < 0 {
		t.Fatalf("empty edge list in %s", path)
	}
	return maxNode + 1, edges
}

// mustLoadPartition reads a partition file (one cluster ID per line) and
// returns the slice. Comment ('#') and blank lines are skipped.
func mustLoadPartition(t testing.TB, path string) []int {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	var out []int
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		c, err := strconv.Atoi(line)
		if err != nil {
			t.Fatalf("bad cluster ID %q in %s: %v", line, path, err)
		}
		out = append(out, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return out
}

// plantedPartition builds a planted-partition (stochastic block model)
// graph with k blocks of nPerBlock nodes each, intra-block edge probability
// pIn, inter-block edge probability pOut. Edges are unit-weight.
//
// The returned `truth` slice is the planted partition: truth[i] is the
// block (in [0, k)) of node i.
func plantedPartition(k, nPerBlock int, pIn, pOut float64, rng *rand.Rand) (n int, edges []Edge, truth []int) {
	n = k * nPerBlock
	truth = make([]int, n)
	for i := 0; i < n; i++ {
		truth[i] = i / nPerBlock
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			p := pOut
			if truth[i] == truth[j] {
				p = pIn
			}
			if rng.Float64() < p {
				edges = append(edges, Edge{From: i, To: j, Weight: 1})
			}
		}
	}
	return n, edges, truth
}

// ringEdges builds a ring of n nodes (n edges, each unit-weight) connecting
// i to (i+1) mod n. Useful as a sparse benchmark fixture.
func ringEdges(n int) []Edge {
	edges := make([]Edge, n)
	for i := 0; i < n; i++ {
		edges[i] = Edge{From: i, To: (i + 1) % n, Weight: 1}
	}
	return edges
}

// adjustedRandIndex computes the Adjusted Rand Index of two partitions
// over the same n nodes. Returns 1.0 on identical partitions (up to
// relabelling) and ≈0.0 on independent random partitions.
//
// Both slices must have the same length; cluster IDs may be any
// non-negative integers (need not be contiguous). Panics on length
// mismatch.
//
// ARI = (Σ C(n_ij,2) − [Σ C(a_i,2) · Σ C(b_j,2)] / C(n,2))
//
//	/ (½[Σ C(a_i,2) + Σ C(b_j,2)] − [Σ C(a_i,2) · Σ C(b_j,2)] / C(n,2))
func adjustedRandIndex(a, b []int) float64 {
	if len(a) != len(b) {
		panic("adjustedRandIndex: length mismatch")
	}
	n := len(a)
	if n < 2 {
		return 1.0
	}
	// Contingency table as map keyed by (a,b).
	type pair struct{ x, y int }
	tab := make(map[pair]int)
	rowSum := make(map[int]int)
	colSum := make(map[int]int)
	for i := 0; i < n; i++ {
		tab[pair{a[i], b[i]}]++
		rowSum[a[i]]++
		colSum[b[i]]++
	}
	c2 := func(x int) float64 { return float64(x) * float64(x-1) / 2.0 }
	sumN := 0.0
	for _, c := range tab {
		sumN += c2(c)
	}
	sumA := 0.0
	for _, c := range rowSum {
		sumA += c2(c)
	}
	sumB := 0.0
	for _, c := range colSum {
		sumB += c2(c)
	}
	cN := c2(n)
	expected := sumA * sumB / cN
	maxIdx := 0.5 * (sumA + sumB)
	denom := maxIdx - expected
	if denom == 0 {
		// Both partitions are degenerate (e.g. all-singleton vs all-singleton).
		if sumN == expected {
			return 1.0
		}
		return 0.0
	}
	return (sumN - expected) / denom
}

// normalizedMutualInformation returns the NMI of two partitions over the
// same n nodes using the arithmetic-mean normalisation:
//
//	NMI = I(A;B) / ((H(A) + H(B)) / 2)
//
// Returns 1.0 on identical partitions; 0.0 when both partitions are
// trivial (one cluster each). Panics on length mismatch.
func normalizedMutualInformation(a, b []int) float64 {
	if len(a) != len(b) {
		panic("normalizedMutualInformation: length mismatch")
	}
	n := len(a)
	if n == 0 {
		return 1.0
	}
	type pair struct{ x, y int }
	tab := make(map[pair]int)
	rowSum := make(map[int]int)
	colSum := make(map[int]int)
	for i := 0; i < n; i++ {
		tab[pair{a[i], b[i]}]++
		rowSum[a[i]]++
		colSum[b[i]]++
	}
	nF := float64(n)
	xlog := func(p float64) float64 {
		if p <= 0 {
			return 0
		}
		return p * math.Log(p)
	}
	hA := 0.0
	for _, c := range rowSum {
		p := float64(c) / nF
		hA -= xlog(p)
	}
	hB := 0.0
	for _, c := range colSum {
		p := float64(c) / nF
		hB -= xlog(p)
	}
	mi := 0.0
	for p, c := range tab {
		pij := float64(c) / nF
		pi := float64(rowSum[p.x]) / nF
		pj := float64(colSum[p.y]) / nF
		if pij > 0 {
			mi += pij * math.Log(pij/(pi*pj))
		}
	}
	denom := 0.5 * (hA + hB)
	if denom == 0 {
		return 0.0
	}
	return mi / denom
}
