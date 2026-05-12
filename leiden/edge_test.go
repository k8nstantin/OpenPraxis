package leiden

import "testing"

func TestEdgeZeroValue(t *testing.T) {
	var e Edge
	if e.From != 0 || e.To != 0 || e.Weight != 0 {
		t.Fatalf("zero Edge should be {0,0,0}, got %+v", e)
	}
}

func TestEdgeLiteral(t *testing.T) {
	e := Edge{From: 1, To: 2, Weight: 0.5}
	if e.From != 1 || e.To != 2 || e.Weight != 0.5 {
		t.Fatalf("literal mismatch: %+v", e)
	}
}
