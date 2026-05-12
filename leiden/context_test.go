package leiden

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLeiden_NilContext(t *testing.T) {
	//lint:ignore SA1012 deliberately passing nil to verify the guard.
	_, err := Leiden(nil, 3, nil, DefaultOptions())
	if !errors.Is(err, ErrNilContext) {
		t.Errorf("err=%v, want ErrNilContext", err)
	}
}

func TestHierarchicalLeiden_NilContext(t *testing.T) {
	//lint:ignore SA1012 deliberately passing nil to verify the guard.
	_, err := HierarchicalLeiden(nil, 3, nil, DefaultOptions())
	if !errors.Is(err, ErrNilContext) {
		t.Errorf("err=%v, want ErrNilContext", err)
	}
}

func TestModularity_NilContext(t *testing.T) {
	//lint:ignore SA1012 deliberately passing nil to verify the guard.
	_, err := Modularity(nil, 3, nil, []int{0, 0, 0}, 1.0)
	if !errors.Is(err, ErrNilContext) {
		t.Errorf("err=%v, want ErrNilContext", err)
	}
}

func TestLeiden_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Leiden(ctx, 8, twoCliqueBridgeEdges(), DefaultOptions())
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err=%v, want context.Canceled", err)
	}
}

func TestHierarchicalLeiden_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := HierarchicalLeiden(ctx, 8, twoCliqueBridgeEdges(), DefaultOptions())
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err=%v, want context.Canceled", err)
	}
}

func TestModularity_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Modularity(ctx, 3, nil, []int{0, 1, 2}, 1.0)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err=%v, want context.Canceled", err)
	}
}

// TestLeiden_DeadlineExceeded confirms that an already-expired deadline is
// surfaced via errors.Is(err, context.DeadlineExceeded), matching the
// idiomatic Go cancellation contract.
func TestLeiden_DeadlineExceeded(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer cancel()

	_, err := Leiden(ctx, 8, twoCliqueBridgeEdges(), DefaultOptions())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err=%v, want context.DeadlineExceeded", err)
	}
}
