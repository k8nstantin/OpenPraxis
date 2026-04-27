// Per-tab feature-flag client. The legacy UI calls
// `tabIsOnReactV2('products')` to decide whether to redirect to
// /dashboard/products; the React app calls the same helper to decide
// whether to register a route at all (defensive — flags are only flipped
// once a tab actually exists).
import { fetchJSON } from './api/client';
import type { ResolvedKnob } from './types';

export type DashboardTab =
  | 'products'
  | 'manifests'
  | 'tasks'
  | 'memories'
  | 'conversations'
  | 'settings'
  | 'compliance'
  | 'overview';

export function flagKey(tab: DashboardTab): string {
  return `frontend_dashboard_v2_${tab}`;
}

let cache: Map<string, boolean> | null = null;

/**
 * Returns true when the per-tab cutover knob resolves to "true" at
 * system scope. Memoized for the page lifetime — the Zustand store is
 * the right place to listen for live changes; this helper covers the
 * initial-boot redirect decision.
 */
export async function tabIsOnReactV2(tab: DashboardTab, options?: { silent?: boolean }): Promise<boolean> {
  if (!cache) {
    try {
      const knobs = await fetchJSON<ResolvedKnob[]>('/api/settings/resolve?scope=system', {
        silent: options?.silent ?? true,
      });
      cache = new Map(knobs.map((k) => [k.key, String(k.value) === 'true']));
    } catch {
      cache = new Map();
    }
  }
  return cache.get(flagKey(tab)) ?? false;
}

/** Test helper — clears the cache so subsequent calls re-fetch. */
export function _resetFlagCache() {
  cache = null;
}
