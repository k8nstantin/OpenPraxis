// Portal V2 handler — dual-port architecture.
//
// Listens on its own port (default :9766, set by the --portal-v2-port
// flag in cmd/serve.go) and serves:
//
//   - The Vite-built React shell from `ui/dashboard-v2/dist` at `/`
//     (with SPA fallback so HashRouter deep-links survive reload).
//   - `/api/*` — the same dynamic backend Portal A exposes, mounted
//     via the shared mountAPI helper. Same Node, same DB, same data.
//   - `/ws` — the same WebSocket broadcast hub. Both portals see the
//     same event stream.
//
// `/mcp` is deliberately NOT mounted here. Agent configs (Claude Code,
// Cursor, etc.) point at :8765/mcp and there's no caller value in
// duplicating the agent endpoint on :9766.
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

// HandlerV2 returns the HTTP handler for Portal V2 on :9766. Same
// backend services as Handler() (the Portal A handler), routed onto a
// fresh mux so the React frontend at `/` doesn't fight the legacy
// vanilla-JS dashboard mounted at `/` on Portal A.
func HandlerV2(deps ServerDeps) http.Handler {
	r := mux.NewRouter()

	// /api/* — same routes as Portal A, same handlers, same data. The
	// React shell on :9766 fetches /api/* against its own port (no
	// CORS, no proxy) so requests resolve in-process via the same
	// handler chain that serves :8765/api/*.
	api := r.PathPrefix("/api").Subrouter()
	mountAPI(api, deps)

	// /ws — broadcast hub. Subscriptions opened from the React shell
	// land in the same hub Portal A subscribers use, so events emit
	// to both UIs simultaneously.
	mountWS(r, deps)

	// React shell — Vite build output. Hashed assets (assets/*) and
	// images (images/*) under their own paths; everything else falls
	// back to index.html so the TanStack hash router can take over.
	v2Content, err := fs.Sub(dashboardV2FS, "ui/dashboard-v2/dist")
	if err != nil {
		// Embed directive is statically validated, so this is unreachable.
		// Panic at startup is fine — fs.Sub failure here means the Go
		// binary itself is malformed.
		panic(fmt.Sprintf("dashboard-v2 embed sub: %v", err))
	}

	fileServer := http.FileServer(http.FS(v2Content))
	r.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// no-store on the entrypoint so updates land on hard-reload
		// without operators having to bust cache manually. Hashed asset
		// chunks (Vite default fingerprint contract) need long-cache
		// headers eventually but for now the FileServer's defaults are
		// fine while we iterate.
		if req.URL.Path == "/" || req.URL.Path == "/index.html" {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		}
		fileServer.ServeHTTP(w, req)
	})

	return r
}
