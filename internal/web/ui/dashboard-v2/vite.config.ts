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
  },
})
