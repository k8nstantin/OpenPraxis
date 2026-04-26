import { test, expect } from 'vitest';
import App from './app.svelte';

// Smoke test for the T1 build pipeline. We can't mount in node-only
// vitest without jsdom, so we just assert the Svelte plugin transformed
// the .svelte file into a callable component export. If the plugin
// regresses or the component fails to compile, this import throws and
// the test errors out before reaching expect().
test('app.svelte compiles and exports a component', () => {
  expect(typeof App).toBe('function');
});
