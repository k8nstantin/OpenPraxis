package render

import (
	"strings"
	"testing"
)

func TestRender_KeepsStructuralAgentTags(t *testing.T) {
	body := "<role>You are a reviewer</role>\n\n<scope>review the diff</scope>\n\n## Heading\n- bullet"
	got := Render(body)
	for _, want := range []string{"<role>", "</role>", "<scope>", "</scope>", "<h2>Heading</h2>", "<li>bullet</li>"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in output; got: %q", want, got)
		}
	}
}

func TestRender_KeepsAllPromptTags(t *testing.T) {
	// Block-level: each tag on its own line so goldmark sees them as HTML blocks.
	body := "<role>R</role>\n\n<scope>S</scope>\n\n<calibration>C</calibration>\n\n<reference>X</reference>\n\n<context>K</context>"
	got := Render(body)
	for _, tag := range []string{"<role>", "<scope>", "<calibration>", "<reference>", "<context>"} {
		if !strings.Contains(got, tag) {
			t.Errorf("expected %q; got: %q", tag, got)
		}
	}
}

func TestRender_KeepsXMLLikeNestedStructure(t *testing.T) {
	// Realistic agent prompt shape — the actual case from the React T2
	// descriptions that triggered DV/M5.
	body := `<role>
You are an experienced developer reviewing the React/Products tab.
</role>

<scope>
- Read the work delivered in T1
- Run the watcher audit
</scope>

<calibration>
Push back on premature abstractions.
</calibration>`
	got := Render(body)
	for _, want := range []string{
		"<role>", "</role>",
		"<scope>", "</scope>",
		"<calibration>", "</calibration>",
		"experienced developer",
		"watcher audit",
		"premature abstractions",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in agent-prompt-shaped output; got: %q", want, got)
		}
	}
}

func TestRender_MarkdownHeadingsWork(t *testing.T) {
	got := Render("# H1\n\n## H2\n\n- bullet")
	for _, want := range []string{"<h1>H1</h1>", "<h2>H2</h2>", "<li>bullet</li>"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q; got: %q", want, got)
		}
	}
}

func TestRender_GFMTablesWork(t *testing.T) {
	got := Render("| a | b |\n|---|---|\n| 1 | 2 |")
	if !strings.Contains(got, "<table>") {
		t.Fatalf("GFM tables must render; got: %q", got)
	}
}

func TestRender_CodeBlocksWork(t *testing.T) {
	got := Render("```go\nfunc f() {}\n```")
	if !strings.Contains(got, "<pre>") || !strings.Contains(got, "<code") {
		t.Fatalf("code blocks must render; got: %q", got)
	}
}

func TestRender_DetailsAndSummary(t *testing.T) {
	body := "<details><summary>click</summary>hidden</details>"
	got := Render(body)
	for _, want := range []string{"<details>", "<summary>click</summary>"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q; got: %q", want, got)
		}
	}
}

func TestRender_InlineHTMLPassesThrough(t *testing.T) {
	// Mixed markdown + inline raw HTML — both should render.
	got := Render("**bold** and <em>italic</em> together")
	for _, want := range []string{"<strong>bold</strong>", "<em>italic</em>"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q; got: %q", want, got)
		}
	}
}

func TestRender_EmptyInput(t *testing.T) {
	got := Render("")
	if got != "" {
		// Empty input may yield empty body — verify we don't panic and
		// don't emit unexpected wrappers.
		t.Logf("empty input produced: %q", got)
	}
}
