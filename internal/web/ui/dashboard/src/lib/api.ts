// Backward-compat shim. PR #240 placed the API client at `src/lib/api.ts`.
// PR #245 (M1 / Foundation) moved it under `src/lib/api/` so query
// wrappers, request-ID propagation, and toast surfacing share a folder.
// Re-exporting from the new module path keeps existing imports green
// while migration finishes.
export { fetchJSON, ApiError, setApiErrorToasts, type FetchOptions } from './api/client';
