package task

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeProductChecker struct {
	responses map[string]fakeProductResult
	errFor    string
}

type fakeProductResult struct {
	satisfied   bool
	unsatisfied []string
}

func (f *fakeProductChecker) IsSatisfied(_ context.Context, productID string) (bool, []string, error) {
	if f.errFor == productID {
		return false, nil, errors.New("fake product lookup failure")
	}
	if r, ok := f.responses[productID]; ok {
		return r.satisfied, r.unsatisfied, nil
	}
	return true, nil, nil
}

// seedManifestForProduct inserts a minimal manifest row. task.Create's
// product lookup joins manifests.project_id; internal/manifest import
// from tests here would produce a cycle, so raw SQL it is.
func seedManifestForProduct(t *testing.T, s *Store, manifestID, productID string) {
	t.Helper()
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS manifests (
		id TEXT PRIMARY KEY, title TEXT, description TEXT, content TEXT,
		status TEXT, jira_refs TEXT, tags TEXT, author TEXT, source_node TEXT,
		project_id TEXT, depends_on TEXT, version INT, created_at TEXT,
		updated_at TEXT, deleted_at TEXT DEFAULT ''
	)`); err != nil {
		t.Fatalf("create manifests fixture: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(`INSERT OR REPLACE INTO manifests
		(id, title, description, content, status, jira_refs, tags, author, source_node, project_id, depends_on, version, created_at, updated_at, deleted_at)
		VALUES (?, 'm', '', '', 'open', '[]', '[]', 't', '', ?, '', 1, ?, ?, '')`,
		manifestID, productID, now, now); err != nil {
		t.Fatalf("seed manifest row: %v", err)
	}
}

// TestCreate_ProductUnsatisfied_SeedsWaitingWithProductReason — task
// whose product has open deps lands waiting with a product-prefix
// block_reason.
func TestCreate_ProductUnsatisfied_SeedsWaitingWithProductReason(t *testing.T) {
	s := openRepoTestStore(t)
	seedManifestForProduct(t, s, "mf-x", "prod-blocked")
	s.SetProductChecker(&fakeProductChecker{
		responses: map[string]fakeProductResult{
			"prod-blocked": {satisfied: false, unsatisfied: []string{"prod-dep-1"}},
		},
	})

	task, err := s.Create("mf-x", "t", "", "once", "claude-code", "node", "user", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := readStatus(t, s, task.ID); got != "waiting" {
		t.Fatalf("status = %q, want waiting", got)
	}
	br := readBlockReason(t, s, task.ID)
	if !strings.HasPrefix(br, "product not satisfied") {
		t.Errorf("block_reason = %q, want 'product not satisfied' prefix", br)
	}
	if !strings.Contains(br, "prod-dep-1") {
		t.Errorf("block_reason = %q, want marker prod-dep-1", br)
	}
}

// TestCreate_ManifestBlockTrumpsProduct — precedence: innermost
// blocker is named first. Manifest block wins over product block.
func TestCreate_ManifestBlockTrumpsProduct(t *testing.T) {
	s := openRepoTestStore(t)
	seedManifestForProduct(t, s, "mf-y", "prod-blocked")
	s.SetManifestChecker(&fakeChecker{
		responses: map[string]fakeCheckResult{"mf-y": {satisfied: false, unsatisfied: []string{"mf-dep"}}},
	})
	s.SetProductChecker(&fakeProductChecker{
		responses: map[string]fakeProductResult{"prod-blocked": {satisfied: false, unsatisfied: []string{"prod-dep"}}},
	})

	task, err := s.Create("mf-y", "t", "", "once", "claude-code", "node", "user", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	br := readBlockReason(t, s, task.ID)
	if !strings.HasPrefix(br, "manifest not satisfied") {
		t.Errorf("block_reason = %q, want manifest-level prefix", br)
	}
	if strings.Contains(br, "product not satisfied") {
		t.Errorf("block_reason = %q, must not mention product when manifest is closer blocker", br)
	}
}

// TestFlipProductBlockedTasks_ScopedByProduct — flip touches only the
// given product's manifests. Other products' product-blocked tasks
// stay put.
func TestFlipProductBlockedTasks_ScopedByProduct(t *testing.T) {
	s := openRepoTestStore(t)
	seedManifestForProduct(t, s, "mf-A", "prod-A")
	seedManifestForProduct(t, s, "mf-B", "prod-B")

	mkBlocked := func(manifestID, title string) string {
		t.Helper()
		tsk, err := s.Create(manifestID, title, "", "once", "claude-code", "node", "t", "")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if _, err := s.db.Exec(
			`UPDATE tasks SET status = 'waiting', block_reason = ? WHERE id = ?`,
			"product not satisfied — blocked by: prod-dep", tsk.ID); err != nil {
			t.Fatalf("force waiting: %v", err)
		}
		return tsk.ID
	}
	inA := mkBlocked("mf-A", "A")
	inB := mkBlocked("mf-B", "B")

	n, err := s.FlipProductBlockedTasks(context.Background(), "prod-A", StatusScheduled)
	if err != nil {
		t.Fatalf("FlipProductBlockedTasks: %v", err)
	}
	if n != 1 {
		t.Fatalf("flipped = %d, want 1 (only prod-A)", n)
	}
	if got := readStatus(t, s, inA); got != "scheduled" {
		t.Errorf("inA = %q, want scheduled", got)
	}
	if got := readStatus(t, s, inB); got != "waiting" {
		t.Errorf("inB = %q, want waiting (untouched, different product)", got)
	}
}

// TestFlipProductBlockedTasks_SkipsOtherBlockPrefixes — only tasks
// with the product-prefix block_reason get flipped. Task-level and
// manifest-level blocks stay put.
func TestFlipProductBlockedTasks_SkipsOtherBlockPrefixes(t *testing.T) {
	s := openRepoTestStore(t)
	seedManifestForProduct(t, s, "mf-P", "prod-P")

	mk := func(title, blockReason string) string {
		tsk, err := s.Create("mf-P", title, "", "once", "claude-code", "node", "t", "")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if _, err := s.db.Exec(
			`UPDATE tasks SET status = 'waiting', block_reason = ? WHERE id = ?`,
			blockReason, tsk.ID); err != nil {
			t.Fatalf("force waiting: %v", err)
		}
		return tsk.ID
	}
	productBlocked := mk("P", "product not satisfied — blocked by: other")
	manifestBlocked := mk("M", "manifest not satisfied — blocked by: dep-M")
	taskBlocked := mk("T", "task xyz not completed")

	n, err := s.FlipProductBlockedTasks(context.Background(), "prod-P", StatusScheduled)
	if err != nil {
		t.Fatalf("FlipProductBlockedTasks: %v", err)
	}
	if n != 1 {
		t.Fatalf("flipped = %d, want 1", n)
	}
	if got := readStatus(t, s, productBlocked); got != "scheduled" {
		t.Errorf("product-blocked = %q, want scheduled", got)
	}
	if got := readStatus(t, s, manifestBlocked); got != "waiting" {
		t.Errorf("manifest-blocked was flipped: %q", got)
	}
	if got := readStatus(t, s, taskBlocked); got != "waiting" {
		t.Errorf("task-blocked was flipped: %q", got)
	}
}

// TestPropagateProductClosed_ActivatesDownstream — end-to-end walk.
// prod-A depends on prod-B. Close prod-B → prod-A satisfied → tasks
// in prod-A flip to scheduled.
func TestPropagateProductClosed_ActivatesDownstream(t *testing.T) {
	s := openRepoTestStore(t)
	seedManifestForProduct(t, s, "mf-A", "prod-A")

	tsk, err := s.Create("mf-A", "downstream", "", "once", "claude-code", "node", "t", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := s.db.Exec(
		`UPDATE tasks SET status = 'waiting', block_reason = 'product not satisfied — blocked by: prod-B' WHERE id = ?`,
		tsk.ID); err != nil {
		t.Fatalf("force waiting: %v", err)
	}

	depsFor := func(_ context.Context, id string) ([]string, error) {
		if id == "prod-B" {
			return []string{"prod-A"}, nil
		}
		return nil, nil
	}
	satisfiedFor := func(_ context.Context, _ string) (bool, error) { return true, nil }

	activated, err := s.PropagateProductClosed(context.Background(), "prod-B", depsFor, satisfiedFor)
	if err != nil {
		t.Fatalf("PropagateProductClosed: %v", err)
	}
	if activated != 1 {
		t.Fatalf("activated = %d, want 1", activated)
	}
	if got := readStatus(t, s, tsk.ID); got != "scheduled" {
		t.Errorf("task = %q, want scheduled", got)
	}
}

// TestPropagateProductClosed_CycleSafe — visited set guarantees
// termination even if the dep graph contains a back-edge.
func TestPropagateProductClosed_CycleSafe(t *testing.T) {
	s := openRepoTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	depsFor := func(_ context.Context, id string) ([]string, error) {
		switch id {
		case "prod-A":
			return []string{"prod-B"}, nil
		case "prod-B":
			return []string{"prod-A"}, nil
		}
		return nil, nil
	}
	satisfiedFor := func(_ context.Context, _ string) (bool, error) { return true, nil }

	done := make(chan struct{})
	go func() {
		_, _ = s.PropagateProductClosed(ctx, "prod-A", depsFor, satisfiedFor)
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("PropagateProductClosed did not terminate within 2s on cyclical graph")
	}
}
