package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/embedding"
	"github.com/k8nstantin/OpenPraxis/internal/memory"
	"github.com/k8nstantin/OpenPraxis/internal/node"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

func TestLooksPathy(t *testing.T) {
	cases := map[string]bool{
		"/project/openpraxis/sessions/":          true,
		"/project/openpraxis":                    true,
		"project/openpraxis/foo":                 true,
		"019daac8-cdb3-7e77-b995-8706b3414128":   false,
		"019daac8":                               false,
		"deadbeef-nope":                          false,
	}
	for in, want := range cases {
		if got := looksPathy(in); got != want {
			t.Errorf("looksPathy(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestFormatCandidates_PrefixVsSearch(t *testing.T) {
	mems := []*memory.Memory{
		{ID: "019daac8-cdb3-7e77-b995-8706b3414128", Path: "/project/p/d/alpha", L0: "session checkpoint 1"},
		{ID: "019daac8-fff0-7000-aaaa-000000000001", Path: "/project/p/d/beta", L0: "session checkpoint 2"},
	}

	out := formatCandidates("019daac8-cdb", mems, false)
	if !strings.Contains(out, "Closest candidates") {
		t.Errorf("prefix candidate header missing: %q", out)
	}
	if !strings.Contains(out, "[019daac8-cdb3-7e77-b995-8706b3414128]") {
		t.Errorf("full UUID missing from output: %q", out)
	}
	if !strings.Contains(out, "/project/p/d/alpha") {
		t.Errorf("path missing from output: %q", out)
	}

	out = formatCandidates("session checkpoint", mems, true)
	if !strings.Contains(out, "Semantic search candidates") {
		t.Errorf("semantic label missing: %q", out)
	}
}

func TestHandleRecall_Rung5SemanticFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"embeddings": [][]float32{{0.1, 0.2, 0.3, 0.4}},
		})
	}))
	defer srv.Close()

	idx, err := memory.NewIndex(t.TempDir(), 4)
	if err != nil {
		t.Fatalf("NewIndex: %v", err)
	}
	t.Cleanup(func() { idx.Close() })

	now := time.Now().UTC().Format(time.RFC3339)
	seedOne := func(id, path, l0 string) {
		m := &memory.Memory{
			ID: id, Path: path, L0: l0, L1: l0, L2: l0,
			Type: "insight", Scope: "project", Project: "p", Domain: "d",
			Tags:      []string{},
			CreatedAt: now, UpdatedAt: now, AccessedAt: now,
		}
		if err := idx.Upsert(m, []float32{0.1, 0.2, 0.3, 0.4}); err != nil {
			t.Fatalf("Upsert %s: %v", id, err)
		}
	}
	seedOne("aaaaaaaa-1111-7000-aaaa-000000000001", "/project/p/d/alpha", "session checkpoint alpha")
	seedOne("bbbbbbbb-2222-7000-aaaa-000000000002", "/project/p/d/beta", "session checkpoint beta")

	n := &node.Node{
		Index:    idx,
		Embedder: embedding.NewEngine(srv.URL, "test-model", 4),
	}
	s := &Server{node: n}

	req := mcplib.CallToolRequest{}
	req.Params.Arguments = map[string]any{"id": "session checkpoint"}
	res, err := s.handleRecall(context.Background(), req)
	if err != nil {
		t.Fatalf("handleRecall: %v", err)
	}
	if len(res.Content) == 0 {
		t.Fatalf("expected content, got none")
	}
	tc, ok := res.Content[0].(mcplib.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", res.Content[0])
	}
	if !strings.Contains(tc.Text, "Semantic search candidates") {
		t.Errorf("expected rung-5 header, got: %q", tc.Text)
	}
	if !strings.Contains(tc.Text, "/project/p/d/") {
		t.Errorf("expected a seeded path in output, got: %q", tc.Text)
	}
}

func TestFormatCandidates_ShortID(t *testing.T) {
	// Ids shorter than 12 chars shouldn't panic when sliced.
	mems := []*memory.Memory{
		{ID: "abc", Path: "/x", L0: "short"},
	}
	out := formatCandidates("abc", mems, false)
	if !strings.Contains(out, "[abc]") {
		t.Errorf("short id not rendered: %q", out)
	}
}
