package mcp

import (
	"context"
	"strings"
	"testing"
)

// TestTaskUpdate_TitleOnly — happy path, title replaced, description preserved.
func TestTaskUpdate_TitleOnly(t *testing.T) {
	s, _ := newTestServerWithAllStores(t)
	tk, err := s.node.Tasks.Create("", "original title", "original desc", "once", "claude-code", "", "", "")
	if err != nil {
		t.Fatalf("Create task: %v", err)
	}

	res, err := s.handleTaskUpdate(context.Background(), buildReq(map[string]any{
		"id":    tk.ID,
		"title": "new title",
	}))
	if err != nil {
		t.Fatalf("handleTaskUpdate: %v", err)
	}
	if isErrResult(res) {
		t.Fatalf("unexpected tool error: %q", toolResultText(res))
	}

	got, _ := s.node.Tasks.Get(tk.ID)
	if got.Title != "new title" {
		t.Errorf("title not updated: got %q want %q", got.Title, "new title")
	}
	if got.Description != "original desc" {
		t.Errorf("description clobbered: got %q want %q", got.Description, "original desc")
	}
}

// TestTaskUpdate_DescriptionOnly — title preserved, description replaced.
func TestTaskUpdate_DescriptionOnly(t *testing.T) {
	s, _ := newTestServerWithAllStores(t)
	tk, _ := s.node.Tasks.Create("", "keep title", "old desc", "once", "claude-code", "", "", "")

	res, err := s.handleTaskUpdate(context.Background(), buildReq(map[string]any{
		"id":          tk.ID,
		"description": "new desc",
	}))
	if err != nil {
		t.Fatalf("handleTaskUpdate: %v", err)
	}
	if isErrResult(res) {
		t.Fatalf("unexpected tool error: %q", toolResultText(res))
	}

	got, _ := s.node.Tasks.Get(tk.ID)
	if got.Title != "keep title" {
		t.Errorf("title clobbered: got %q", got.Title)
	}
	if got.Description != "new desc" {
		t.Errorf("description not updated: got %q", got.Description)
	}
}

// TestTaskUpdate_BothFields — both fields updated in a single call.
func TestTaskUpdate_BothFields(t *testing.T) {
	s, _ := newTestServerWithAllStores(t)
	tk, _ := s.node.Tasks.Create("", "a", "b", "once", "claude-code", "", "", "")

	res, _ := s.handleTaskUpdate(context.Background(), buildReq(map[string]any{
		"id":          tk.ID,
		"title":       "A",
		"description": "B",
	}))
	if isErrResult(res) {
		t.Fatalf("unexpected tool error: %q", toolResultText(res))
	}

	got, _ := s.node.Tasks.Get(tk.ID)
	if got.Title != "A" || got.Description != "B" {
		t.Errorf("both fields not updated: title=%q desc=%q", got.Title, got.Description)
	}
}

// TestTaskUpdate_MarkerResolved — agent passes the 8-char marker, handler
// resolves to full UUID before update.
func TestTaskUpdate_MarkerResolved(t *testing.T) {
	s, _ := newTestServerWithAllStores(t)
	tk, _ := s.node.Tasks.Create("", "by marker", "d", "once", "claude-code", "", "", "")

	shortMarker := tk.ID[:8]
	res, err := s.handleTaskUpdate(context.Background(), buildReq(map[string]any{
		"id":    shortMarker,
		"title": "updated via marker",
	}))
	if err != nil {
		t.Fatalf("handleTaskUpdate: %v", err)
	}
	if isErrResult(res) {
		t.Fatalf("unexpected tool error: %q", toolResultText(res))
	}

	got, _ := s.node.Tasks.Get(tk.ID)
	if got.Title != "updated via marker" {
		t.Errorf("marker path did not resolve: got %q", got.Title)
	}
}

// TestTaskUpdate_RejectsNoFields — empty update is a user error, not a no-op.
func TestTaskUpdate_RejectsNoFields(t *testing.T) {
	s, _ := newTestServerWithAllStores(t)
	tk, _ := s.node.Tasks.Create("", "x", "y", "once", "claude-code", "", "", "")

	res, _ := s.handleTaskUpdate(context.Background(), buildReq(map[string]any{
		"id": tk.ID,
	}))
	if !isErrResult(res) {
		t.Fatalf("expected empty-fields rejection, got %q", toolResultText(res))
	}
	if !strings.Contains(toolResultText(res), "at least one") {
		t.Errorf("unexpected error text: %q", toolResultText(res))
	}
}

// TestTaskUpdate_NonExistent — clean not-found error.
func TestTaskUpdate_NonExistent(t *testing.T) {
	s, _ := newTestServerWithAllStores(t)

	res, _ := s.handleTaskUpdate(context.Background(), buildReq(map[string]any{
		"id":    "deadbeef-nope",
		"title": "x",
	}))
	if !isErrResult(res) {
		t.Fatalf("expected not-found error, got %q", toolResultText(res))
	}
	if !strings.Contains(toolResultText(res), "not found") {
		t.Errorf("expected 'not found' in error, got %q", toolResultText(res))
	}
}

// TestTaskUpdate_MissingID — id is required.
func TestTaskUpdate_MissingID(t *testing.T) {
	s, _ := newTestServerWithAllStores(t)

	res, _ := s.handleTaskUpdate(context.Background(), buildReq(map[string]any{
		"title": "x",
	}))
	if !isErrResult(res) {
		t.Fatalf("expected id-required error, got %q", toolResultText(res))
	}
}
