// Lazy route registry. Single source of truth for the React v2 dashboard:
// SidebarNav renders entries with `nav: true`, App.tsx renders the
// <Route> elements, the per-tab feature flag check (useV2TabFlag) decides
// whether the legacy URL redirects here.
import { lazy, type LazyExoticComponent, type ComponentType, type ReactNode } from 'react';

const Home = lazy(() => import('./pages/Home'));
const Products = lazy(() => import('./pages/Products'));

export interface RouteEntry {
  path: string;
  label: string;
  icon?: ReactNode;
  /** Show in the primary sidebar nav. Detail-only routes set this false. */
  nav: boolean;
  /**
   * Settings catalog key for the per-tab cutover flag. When the resolved
   * value is "true" the legacy nav redirects here; when missing or "false"
   * the legacy UI keeps serving this tab. Home/index is always on.
   */
  flagKey?: string;
  /** Legacy URL that should redirect here when flag is on. */
  legacyPath?: string;
  component: LazyExoticComponent<ComponentType>;
}

export const routeRegistry: RouteEntry[] = [
  {
    path: '/',
    label: 'Home',
    icon: '⌂',
    nav: true,
    component: Home,
  },
  {
    path: '/products',
    label: 'Products',
    icon: '◫',
    nav: true,
    flagKey: 'frontend_dashboard_v2_products',
    legacyPath: '/products',
    component: Products,
  },
];

export function findRouteForLegacyPath(legacyPath: string): RouteEntry | undefined {
  return routeRegistry.find((r) => r.legacyPath === legacyPath);
}
