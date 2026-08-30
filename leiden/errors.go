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

import "errors"

// Errors returned by constructors in this package. Callers compare with
// [errors.Is] rather than string matching.
var (
	// ErrInvalidNodeCount is returned when a non-positive node count is supplied.
	ErrInvalidNodeCount = errors.New("leiden: node count must be positive")

	// ErrNodeOutOfRange is returned when an edge or assignment references a
	// node ID outside [0, nNodes).
	ErrNodeOutOfRange = errors.New("leiden: node id out of range")

	// ErrNegativeEdgeWeight is returned when an edge weight is negative.
	ErrNegativeEdgeWeight = errors.New("leiden: edge weight must be non-negative")

	// ErrNegativeNodeWeight is returned when a node weight is negative.
	ErrNegativeNodeWeight = errors.New("leiden: node weight must be non-negative")

	// ErrNodeWeightsLength is returned when the node-weights slice length does
	// not match the declared node count.
	ErrNodeWeightsLength = errors.New("leiden: node weights length does not match node count")

	// ErrAssignmentLength is returned when a cluster assignment slice length
	// does not match the declared node count.
	ErrAssignmentLength = errors.New("leiden: assignment length does not match node count")

	// ErrNegativeClusterID is returned when a cluster assignment contains a
	// negative cluster ID.
	ErrNegativeClusterID = errors.New("leiden: cluster id must be non-negative")

	// ErrNilContext is returned when a nil [context.Context] is passed to a
	// public function that accepts one. Callers should pass
	// [context.Background] when they do not need cancellation.
	ErrNilContext = errors.New("leiden: nil context")
)
