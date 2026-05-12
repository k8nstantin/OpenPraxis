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

// Edge is a weighted, undirected edge between two nodes identified by
// zero-based integer IDs.
//
// Edges are treated as undirected by [CompactNetwork]: the pair (From, To)
// is equivalent to (To, From). Self-loops (From == To) are permitted and
// are stored as a single CSR entry that contributes their Weight once to
// the incident node's strength. See [CompactNetwork.TotalNodeStrength] for
// how this interacts with the 2m modularity normalisation.
//
// Weight must be non-negative; negative weights are rejected when
// constructing a [CompactNetwork].
type Edge struct {
	From   int
	To     int
	Weight float64
}
