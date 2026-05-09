import path from 'path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { tanstackRouter } from '@tanstack/router-plugin/vite'

// OpenPraxis Portal V2 — Vite + React 19 + Tailwind v4 + TanStack Router.
// Built into `dist/` which is `go:embed`'d by `internal/web/handler_v2.go`
// and served on :9766 alongside Portal A on :8765.
export default defineConfig({
  plugins: [
    tanstackRouter({
      target: 'react',
      autoCodeSplitting: true,
    }),
    react(),
    tailwindcss(),
  ],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  build: {
    // Vite's default. Listed explicitly so the contract with the Go
    // `go:embed all:ui/dashboard-v2/dist` directive in handler_v2.go is
    // visible from this file too.
    outDir: 'dist',
    emptyOutDir: true,
    // Force BlockNote and react-icons into isolated chunks. Without this,
    // rolldown rc.17 can hoist BlockNote toolbar-icon defs into a lazy
    // FloatingThreadController chunk that the eager index chunk also
    // imports — producing a circular ESM import and a TDZ
    // `require_react is not a function` blank-screen at runtime.
    rolldownOptions: {
      output: {
        manualChunks: (id: string) => {
          if (id.includes('node_modules/@blocknote/')) return 'blocknote'
          if (id.includes('node_modules/react-icons/')) return 'react-icons'
          return undefined
        },
      },
    },
  },
  server: {
    // Dev-mode proxy. `npm run dev` hosts the React app on :5173 with
    // HMR; production-mode `npm run build` ships dist/ which the Go
    // binary serves on :9766 alongside /api + /ws (HandlerV2). To keep
    // dev-mode parity, route /api + /ws through the same backend so
    // fetch('/api/...') and new WebSocket('/ws') work identically in
    // both modes. ws:true flips the proxy to handle WebSocket upgrade
    // frames (otherwise /ws would return 404 in dev).
    proxy: {
      '/api': {
        target: 'http://localhost:9766',
        changeOrigin: true,
      },
      '/ws': {
        target: 'ws://localhost:9766',
        ws: true,
        changeOrigin: true,
      },
    },
  },
})
