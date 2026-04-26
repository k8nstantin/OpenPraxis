import { test, expect } from 'vitest';
import { APP_NAME, apiBase } from './lib/util';

// Smoke test for the T1 build pipeline. Verifies the TS + vitest
// toolchain runs against this config — module resolution, ESM imports,
// type stripping. Real Svelte component tests land in T2/T3 once we
// have logic worth testing AND a DOM environment (happy-dom).
//
// We deliberately avoid importing .svelte files here: vitest's vite
// preprocessing pipeline fails on `<style>` blocks in node-only mode,
// and that's not what this smoke test is checking.
test('lib/util compiles and exports', () => {
  expect(APP_NAME).toBe('OpenPraxis Dashboard v2');
  expect(apiBase()).toBe('');
});
