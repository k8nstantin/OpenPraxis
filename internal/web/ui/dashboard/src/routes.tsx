import { lazy, type LazyExoticComponent, type ComponentType, type ReactNode } from 'react';

// Per-tab feature-flag key in the settings catalog. Tabs migrate one
// at a time: when `frontend_dashboard_v2_<tab>` resolves to "true",
// the legacy nav redirects to the React route. Until then both UIs
// stay alive and the operator can hop back via the "Legacy UI" link
// in the header.
export interface TabRoute {
  key: string;
  path: string;
  label: string;
  icon?: ReactNode;
  /** Catalog flag key. When the resolved value is "true" the legacy app should redirect here. */
  flagKey: string;
  /** Lazy-loaded page component. */
  component: LazyExoticComponent<ComponentType>;
  /** Optional sub-paths. Each gets registered in App.tsx. */
  subPaths?: string[];
}

const Home = lazy(() => import('./pages/Home'));
const Products = lazy(() => import('./pages/Products'));

export const TAB_ROUTES: TabRoute[] = [
  {
    key: 'home',
    path: '/',
    label: 'Home',
    icon: '◆',
    flagKey: 'frontend_dashboard_v2',
    component: Home,
  },
  {
    key: 'products',
    path: '/products',
    label: 'Products',
    icon: '◈',
    flagKey: 'frontend_dashboard_v2_products',
    component: Products,
    subPaths: ['/products/:id'],
  },
];

// Convenience map for legacy nav redirects.
export const FLAG_TO_PATH: Record<string, string> = TAB_ROUTES.reduce((acc, t) => {
  acc[t.flagKey] = t.path;
  return acc;
}, {} as Record<string, string>);
