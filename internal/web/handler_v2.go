// Portal V2 handler — dual-port plumbing stub.
//
// Listens on its own port (default :8766, set by the --portal-v2-port flag
// in cmd/serve.go) and serves the Vite-style static bundle from
// `ui/dashboard-v2/dist`. Same Go binary, same database, same MCP server —
// different frontend tree.
//
// Foundation V2 hasn't shipped yet; for now this returns a hand-rolled
// stub at `/` so the dual-port architecture can be verified end-to-end
// before any React work lands.
//
// When the real shadcn-admin scaffold lands in a later chunk, the API +
// /mcp + /ws routes from `Handler(...)` get factored into a shared mux
// builder so both portals expose the full backend on their own port.
package web

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/gorilla/mux"
)

//go:embed all:ui/dashboard-v2/dist
var dashboardV2FS embed.FS

// HandlerV2 returns the HTTP handler for Portal V2. Static-only for the
// chunk-1 plumbing milestone — no API surface yet. Future chunks add the
// shared API + MCP + WebSocket mux once the React app starts making real
// requests.
func HandlerV2() http.Handler {
	r := mux.NewRouter()

	v2Content, err := fs.Sub(dashboardV2FS, "ui/dashboard-v2/dist")
	if err != nil {
		// Embed directive is statically validated, so this is unreachable.
		// Panic at startup is fine — fs.Sub failure here means the Go
		// binary itself is malformed.
		panic(fmt.Sprintf("dashboard-v2 embed sub: %v", err))
	}

	// SPA fallback: any path that doesn't resolve to a static file falls
	// back to index.html so HashRouter deep-links (#overview, #active,
	// etc.) reload cleanly. The hash never reaches the server, so this is
	// belt-and-suspenders for chunk 1; the real router lands later.
	fileServer := http.FileServer(http.FS(v2Content))
	r.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// no-store on the entrypoint so updates land on hard-reload
		// without operators having to bust cache manually. Hashed asset
		// chunks (when Vite ships real ones) get long-cache headers via
		// Vite's default fingerprint contract — they don't need this.
		if req.URL.Path == "/" || req.URL.Path == "/index.html" {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		}
		fileServer.ServeHTTP(w, req)
	})

	return r
}
