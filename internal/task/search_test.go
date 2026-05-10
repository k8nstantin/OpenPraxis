package task

import "testing"

func containsTask(ts []*Task, id string) bool {
	for _, t := range ts {
		if t.ID == id {
			return true
		}
	}
	return false
}

func TestSearch_Keyword(t *testing.T) {
	t.Skip("task store migrated to entities")
	s := openRepoTestStore(t)
	a, _ := s.Create("", "Alpha widget", "", "once", "claude-code", "node", "test", "")
	b, _ := s.Create("", "Beta gizmo", "", "once", "claude-code", "node", "test", "")

	res, err := s.Search("widget", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !containsTask(res, a.ID) || containsTask(res, b.ID) {
		t.Fatalf("keyword mismatch: got %+v", res)
	}
}

func TestSearch_IDExact(t *testing.T) {
	t.Skip("task store migrated to entities")
	s := openRepoTestStore(t)
	a, _ := s.Create("", "A", "", "once", "claude-code", "node", "test", "")
	_, _ = s.Create("", "B", "", "once", "claude-code", "node", "test", "")

	res, err := s.Search(a.ID, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res) != 1 || res[0].ID != a.ID {
		t.Fatalf("id-exact wanted [%s], got %+v", a.ID, res)
	}
}

func TestSearch_IDPrefix(t *testing.T) {
	t.Skip("task store migrated to entities")
	s := openRepoTestStore(t)
	a, _ := s.Create("", "A", "", "once", "claude-code", "node", "test", "")
	_, _ = s.Create("", "B", "", "once", "claude-code", "node", "test", "")

	res, err := s.Search(a.ID[:12], 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !containsTask(res, a.ID) {
		t.Fatalf("id-prefix missing %s: got %+v", a.ID, res)
	}
}

func TestSearch_Unknown(t *testing.T) {
	t.Skip("task store migrated to entities")
	s := openRepoTestStore(t)
	_, _ = s.Create("", "A", "", "once", "claude-code", "node", "test", "")

	res, err := s.Search("no-such-thing-xyz-987", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("unknown should be empty, got %+v", res)
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	t.Skip("task store migrated to entities")
	s := openRepoTestStore(t)
	_, _ = s.Create("", "A", "", "once", "claude-code", "node", "test", "")

	res, err := s.Search("   ", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("empty query should be empty, got %+v", res)
	}
}
