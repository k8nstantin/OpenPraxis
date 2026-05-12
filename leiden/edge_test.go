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
