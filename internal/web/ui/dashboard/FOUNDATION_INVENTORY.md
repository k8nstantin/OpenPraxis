# React 2 / Foundation M1 — Inventory

Audit of what PR #240 already shipped in `internal/web/ui/dashboard/` so this
PR (M1) extends rather than reinvents.

## Already shipped by #240 (keep, extend)

- `package.json` — react 18.3, react-dom, react-router-dom 6.27,
  @tanstack/react-query 5.59, react-markdown 9, cytoscape + cytoscape-dagre,
  vitest 2.1, @testing-library/react 16
- `vite.config.ts` — base `/dashboard/`, alias `@/*`, Vite proxy for /api +
  /ws to :8765, vitest jsdom environment
- `tsconfig.json` — strict, paths `@/*`, vitest globals
- `index.html` + `src/main.tsx` + `src/App.tsx` — basic SPA shell with
  BrowserRouter basename `/dashboard` and a tiny lazy route registry for
  Home + Products
- `src/lib/api.ts` — `fetchJSON<T>` + `ApiError`
- `src/lib/types.ts` — Product / Manifest / Task / ProductHierarchy
- `src/lib/queries/products.ts` — `useProductsByPeer`, `useProduct`,
  `useProductManifests`, `useProductHierarchy`
- `src/lib/dag.ts` + tests — Cytoscape element builder
- `src/components/Tree.tsx` — generic TreeRow (replaces views/tree.js)
- `src/components/ProductDag.tsx` — Cytoscape lifecycle wrapper
- `src/pages/Home.tsx`, `src/pages/Products.tsx`
- `src/styles.css` — 415 lines of base styling
- Vitest setup at `src/test/setup.ts`
- Makefile pipeline (`frontend`, `dev-frontend`, `test-ui`)
- `internal/web/handler.go` SPA fallback at `/dashboard`

## Added by this PR (M1 / Foundation)

### A. Primitives — `src/components/ui/`

- `Button.tsx`, `IconButton.tsx`
- `Dialog.tsx` (Radix wrapper)
- `Tooltip.tsx` (Radix wrapper)
- `Popover.tsx` (Radix wrapper)
- `Badge.tsx`, `StatusDot.tsx`
- `EmptyState.tsx`
- `ErrorBoundary.tsx` (class component)
- `Toast.tsx` — sonner re-export
- `index.ts` barrel
- `*.stories.tsx` for each
- `*.test.tsx` axe-core a11y tests

### B. Chrome — `src/components/layout/`

- `AppShell.tsx` — outer flex shell with header + sidebar + content
- `Header.tsx` — top bar
- `SidebarNav.tsx` — vertical tab list driven by route registry
- `Breadcrumb.tsx`
- `PageWrapper.tsx` — per-route content frame

### C. Data layer — `src/lib/api/`

- `client.ts` — `fetchJSON<T>` extended with X-Request-ID propagation +
  global error → toast on `ApiError`
- `queries.ts` — `useApiQuery<T>(key, path)` + `useApiMutation<T,V>` with
  optimistic update + rollback
- `types.ts` — re-exports `lib/types.ts` plus generated `lib/api/generated.ts`
- `index.ts` barrel

`src/lib/api.ts` is kept as a thin re-export so PR #240's queries continue
to compile while migration finishes.

### D. State — `src/lib/stores/ui.ts`

Zustand store with `persist` middleware:
- `sidebarCollapsed: boolean`
- `descMode: 'markup' | 'rendered'`
- `theme: 'dark' | 'light'`

### E. Cross-cutting components

- `src/components/desc/DescToggle.tsx` — mirrors PR #239 `OL.descToggle`
- `src/components/desc/MarkdownRenderer.tsx` — DV/M5 trust passthrough
- `src/components/comments/CommentsList.tsx` + `CommentEditor.tsx`
- `src/components/cy/CytoscapeCanvas.tsx` — ProductDag's lifecycle wrapper
  hoisted into a generic component

### F. Forms — `src/components/forms/`

- `FormField.tsx`, `FormError.tsx`, `FormSection.tsx`, `FormActions.tsx`
- `src/lib/hooks/useTypeahead.ts`

### G. Schemas — `src/schemas/`

- `product.ts`, `manifest.ts`, `task.ts` (zod)

### H. Routing — `src/routes.tsx`

Lazy-loaded route registry consumed by `App.tsx` and `SidebarNav`.
Per-tab feature flag check: a tab whose `frontend_dashboard_v2_<tab>` is
false in `/api/settings/resolve` falls through to the legacy UI.

### I. Backend — `frontend_dashboard_v2_<tab>` knobs

Added in `internal/settings/catalog.go`. One knob per migrated tab so
operators flip them on as parity is reached.

### J. Storybook — `.storybook/`

- `main.ts`, `preview.ts`
- `package.json` adds `storybook` + `build-storybook` scripts
- Dev-only; not embedded into the binary.

### K. Type generation — `tools/tygo`

- `tools/tygo/config.yaml`
- `Makefile` `types` target; CI fails on uncommitted drift via
  `git diff --exit-code`.

## Out of scope (per manifest spec)

Tab-specific surfaces, comments soft-delete UI, markdown editor toolbar,
theme switching beyond dark.
