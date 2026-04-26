/// <reference types="vitest" />
import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import path from 'node:path';

// Vite config for the React v2 dashboard.
//
// base: '/dashboard/' matches the Go embed mount point in handler.go.
// All emitted asset URLs get rewritten to /dashboard/assets/<hash>.<ext>
// so the served index.html points at the right paths under the embed.
export default defineConfig({
  plugins: [react()],
  base: '/dashboard/',
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    sourcemap: false,
    assetsDir: 'assets',
    rollupOptions: {
      output: {
        // Pin chunk filenames to assets/* so handler.go's immutable
        // cache rule (everything under assets/) covers JS, CSS, and
        // dynamic imports uniformly.
        entryFileNames: 'assets/[name]-[hash].js',
        chunkFileNames: 'assets/[name]-[hash].js',
        assetFileNames: 'assets/[name]-[hash].[ext]',
      },
    },
  },
  server: {
    // `make dev-frontend` runs alongside `make run`. Vite proxies
    // /api/* and /ws to the Go server on :8765 so the React app can
    // talk to live data while iterating with HMR.
    proxy: {
      '/api': 'http://localhost:8765',
      '/ws': { target: 'ws://localhost:8765', ws: true },
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    css: false,
  },
});
