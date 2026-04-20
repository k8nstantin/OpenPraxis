package mcp

import (
	"strings"
	"testing"

	"github.com/k8nstantin/OpenPraxis/internal/memory"
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
	if !strings.Contains(out, "[019daac8-cdb]") {
		t.Errorf("12-char marker missing from output: %q", out)
	}
	if !strings.Contains(out, "/project/p/d/alpha") {
		t.Errorf("path missing from output: %q", out)
	}

	out = formatCandidates("session checkpoint", mems, true)
	if !strings.Contains(out, "Semantic search candidates") {
		t.Errorf("semantic label missing: %q", out)
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
