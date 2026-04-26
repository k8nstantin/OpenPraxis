package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// Tests for serveDashboard — the React v2 mount point at /dashboard/.
// Code paths covered:
//
//	1. /dashboard            (no slash)        → index.html, no-cache
//	2. /dashboard/           (trailing slash)  → index.html, no-cache
//	3. /dashboard/assets/<f> (real asset)      → file content, immutable cache
//	4. /dashboard/<spa>      (no such file)    → SPA fallback to index.html
//	5. /dashboard/index.html (must not infinite-loop — chunk-7 regression)
//	6. /dashboard/<root-static-file>           → no-cache (not content-addressed)
//	7. dist/ missing index.html                → 500 with helpful message

func newTestDashboardFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":              {Data: []byte("<!doctype html><html>app</html>")},
		"assets/index-abc123.js":  {Data: []byte("console.log('react');")},
		"assets/index-def456.css": {Data: []byte("body { color: red; }")},
		"openpraxis-icon.png":     {Data: []byte("\x89PNG\r\n\x1a\n")},
	}
}

func doDashReq(t *testing.T, h http.Handler, path string) *http.Response {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr.Result()
}

func bodyOf(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	resp.Body.Close()
	return string(b)
}

func TestServeDashboard_BareDashboardServesIndex(t *testing.T) {
	h := serveDashboard(newTestDashboardFS())
	resp := doDashReq(t, h, "/dashboard")
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(bodyOf(t, resp), "app") {
		t.Fatal("expected index.html body")
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-cache") {
		t.Errorf("Cache-Control: got %q, want no-cache", cc)
	}
}

func TestServeDashboard_TrailingSlashServesIndex(t *testing.T) {
	h := serveDashboard(newTestDashboardFS())
	resp := doDashReq(t, h, "/dashboard/")
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(bodyOf(t, resp), "app") {
		t.Fatal("expected index.html body")
	}
}

func TestServeDashboard_HashedAssetServedWithImmutableCache(t *testing.T) {
	h := serveDashboard(newTestDashboardFS())
	resp := doDashReq(t, h, "/dashboard/assets/index-abc123.js")
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	if got := bodyOf(t, resp); !strings.Contains(got, "console.log") {
		t.Fatalf("body: got %q, want js content", got)
	}
	cc := resp.Header.Get("Cache-Control")
	if !strings.Contains(cc, "immutable") {
		t.Errorf("Cache-Control: got %q, want immutable", cc)
	}
	if !strings.Contains(cc, "max-age=31536000") {
		t.Errorf("Cache-Control: got %q, want max-age=31536000", cc)
	}
}

func TestServeDashboard_SpaRouteFallsBackToIndex(t *testing.T) {
	h := serveDashboard(newTestDashboardFS())
	resp := doDashReq(t, h, "/dashboard/products/abc-route-not-in-embed")
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(bodyOf(t, resp), "app") {
		t.Fatal("expected index.html body for SPA fallback")
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-cache") {
		t.Errorf("Cache-Control: got %q, want no-cache", cc)
	}
}

func TestServeDashboard_ExplicitIndexHtmlNoRedirectLoop(t *testing.T) {
	// Regression for the chunk-7 bug: http.FileServer auto-redirects
	// any URL ending in `index.html` to its parent dir. If serveDashboard
	// went through FileServer for index.html, the path-rewrite trick
	// (req.URL.Path = "/index.html") would re-trigger that redirect on
	// every request → infinite 301 loop.
	h := serveDashboard(newTestDashboardFS())
	resp := doDashReq(t, h, "/dashboard/index.html")
	if resp.StatusCode == 301 || resp.StatusCode == 302 {
		t.Fatalf("status %d — index.html must not redirect (redirect-loop regression)", resp.StatusCode)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
}

func TestServeDashboard_RootLevelStaticFile(t *testing.T) {
	h := serveDashboard(newTestDashboardFS())
	resp := doDashReq(t, h, "/dashboard/openpraxis-icon.png")
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-cache") {
		t.Errorf("Cache-Control: got %q, want no-cache for non-asset file", cc)
	}
}

func TestServeDashboard_MissingIndexReturns500(t *testing.T) {
	emptyFS := fstest.MapFS{}
	h := serveDashboard(emptyFS)
	resp := doDashReq(t, h, "/dashboard/")
	if resp.StatusCode != 500 {
		t.Fatalf("status: got %d, want 500", resp.StatusCode)
	}
	body := bodyOf(t, resp)
	if !strings.Contains(body, "make build") {
		t.Errorf("error body: got %q, want hint to run make build", body)
	}
}
