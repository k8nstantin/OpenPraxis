import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

// Svelte 5 config. vitePreprocess lets us write TypeScript inside
// <script lang="ts"> blocks. No SvelteKit — we're a pure Vite-built
// SPA embedded in the Go binary.
export default {
  preprocess: vitePreprocess(),
};
