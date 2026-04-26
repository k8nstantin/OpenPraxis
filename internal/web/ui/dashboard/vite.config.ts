import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

// OpenPraxis dashboard v2 build config.
//
// Output goes to internal/web/ui/dashboard/dist/ which is then embedded
// into the Go binary via `go:embed all:internal/web/ui/dashboard/dist`
// (see internal/web/handler.go). The Go side serves dist/ at the
// /dashboard/ URL prefix.
//
// base: '/dashboard/' — the embedded asset URLs resolve against the
// /dashboard/ prefix (Go embed mounts there). If we change this, the
// hashed-asset fingerprints in index.html will all 404.
//
// build.outDir: explicit dist path so the Go embed pattern above
// continues to work. Don't change without updating the embed directive.
//
// build.assetsDir: 'assets' — keeps hashed JS/CSS in dist/assets/ so
// the embed can serve them under /dashboard/assets/*.
//
// build.sourcemap: 'inline' for dev builds, false for prod. Source maps
// in production embed bloat the binary; dev mode wants them for stack
// traces. We rely on Vite's default-on-dev / default-off-prod here,
// but spell it out so a future config change is visible.
export default defineConfig(({ mode }) => ({
  base: '/dashboard/',
  plugins: [svelte()],
  build: {
    outDir: 'dist',
    assetsDir: 'assets',
    emptyOutDir: true,
    sourcemap: mode === 'development',
  },
  // Vitest config lives here too so we don't need a separate file.
  // jsdom would let us mount components in tests; for T1 we only need
  // the smoke test which runs node-only.
  test: {
    environment: 'node',
    globals: false,
    include: ['src/**/*.test.ts'],
  },
}));
