// Shared dashboard constants. Lives at src/lib/ so subsequent
// migrations (Tasks, Manifests, …) can import without dragging in
// Svelte component code.

export const APP_NAME = 'OpenPraxis Dashboard v2';

// Resolved API origin for fetch wrappers. Empty string in production
// (same-origin against the Go server); the dev-mode HMR proxy will
// override this in T2 when the API wrapper lands.
export function apiBase(): string {
  return '';
}
