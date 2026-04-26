// Package render is the markdown → HTML pipeline used by every dashboard
// surface (product / manifest / task / idea / comment descriptions).
//
// Single-user, single-node policy: render whatever the operator + their
// agents wrote. Raw HTML and custom XML-shaped tags (<role>, <scope>,
// <calibration>, …) pass through verbatim so the dashboard shows EXACTLY
// what the agent will receive — no render divergence between human and
// agent view of the same body.
//
// Threat model: there isn't one in v1. There's one user (the operator) on
// a local node; everything in the database is content the operator or
// their agents wrote. If multi-user / multi-tenant ever lands, we'll add a
// trust-aware sanitizer back (see DV/M5 manifest 019dc953-b12 for the
// design we deferred).
package render

import (
	"bytes"
	"strings"
	"sync"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	rendererhtml "github.com/yuin/goldmark/renderer/html"
)

var (
	once sync.Once
	md   goldmark.Markdown
)

func initRenderer() {
	md = goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(
			rendererhtml.WithXHTML(),
			rendererhtml.WithUnsafe(), // raw HTML / custom tags pass through
		),
	)
}

// Render converts a markdown body to HTML, preserving raw HTML and custom
// XML-shaped tags verbatim. On goldmark error falls back to the body
// wrapped in an escaped <p> so the caller never has to handle a
// partial-render path.
func Render(body string) string {
	once.Do(initRenderer)

	var buf bytes.Buffer
	if err := md.Convert([]byte(body), &buf); err != nil {
		return "<p>" + escape(body) + "</p>"
	}
	return buf.String()
}

func escape(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	).Replace(s)
}
