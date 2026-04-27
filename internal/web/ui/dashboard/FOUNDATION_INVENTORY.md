# React 2 / Foundation T1 — Inventory

Audit of what PR #240 + #246 already shipped and what this PR (M1/T1) adds on top.

## Already in main (do NOT rebuild)

- Vite + React 18 + TypeScript pipeline (`vite.config.ts`, `tsconfig.json`)
- `make frontend` / `make build` / `make dev-frontend` Makefile pipeline
- `go:embed all:ui/dashboard/dist` wiring in `internal/web/handler.go`
- `@tanstack/react-query` provider + `BrowserRouter` lazy routes (App.tsx)
- `src/lib/api.ts` — minimal `fetchJSON<T>` + `ApiError`
- `src/lib/types.ts` — `Product`, `Manifest`, `Task`, `ProductHierarchy`
- `src/lib/queries/products.ts` — Products tab queries
- `src/components/Tree.tsx` — generic recursive `TreeRow<T>`
- `src/components/ProductDag.tsx` — Cytoscape lifecycle wrapper
- `src/pages/{Home,Products}.tsx`
- `src/styles.css` — port of legacy visual language (#246)
- Vitest + Testing Library setup
- Settings catalog flag `frontend_dashboard_v2` (system scope)

## What this PR adds

### A. Design system primitives — `src/components/ui/`
Button, IconButton, Dialog (radix), Tooltip (react-tooltip), Popover (radix), Badge, StatusDot, EmptyState, ErrorBoundary, Toast (sonner). All accessible, keyboardable, story + axe test.

### B. Global chrome — `src/components/layout/`
AppShell (header + sidebar + main), Header, SidebarNav (per-tab links), Breadcrumb, PageWrapper.

### C. Data layer — extend `src/lib/api/`
`fetchJSON<T>` adds `X-Request-ID` outbound + global error → toast bus. `useApiQuery<T>` and `useApiMutation<T>` typed wrappers around React Query with optimistic + rollback. `make types` regenerates `src/lib/types.gen.ts` from Go via tygo (devDep, run via npx).

### D. State — `src/lib/stores/ui.ts`
Zustand store: `sidebarCollapsed`, `descMode` (`markup` | `rendered`), `theme` (`dark`). `persist` middleware → `localStorage:openpraxis.ui`.

### E. Cross-cutting — `src/components/`
- `DescToggle` — port of `OL.descToggle` (#239), reads `descMode` from store, persists.
- `MarkdownRenderer` — react-markdown w/ DV/M5 trust passthrough policy.
- `CommentsList` + `CommentEditor` — reusable for any entity.
- `CytoscapeCanvas` — generic lifecycle wrapper, ProductDag now uses it.
- `Tree` — already extracted in #240, story added.

### F. Forms — `src/components/forms/`
FormField, FormError, FormSection, FormActions. `react-hook-form` + `zod` + `@hookform/resolvers/zod`. Schemas in `src/schemas/` (productSchema, manifestSchema, taskSchema). `useTypeahead` hook for picker UIs.

### G. Routes registry + cutover
`src/routes.tsx` lazy registry. Per-tab feature flags added to settings catalog: `frontend_dashboard_v2_products`, `..._manifests`, `..._tasks`, `..._ideas`, `..._memories`, `..._sessions`. Legacy `/products` redirects to `/dashboard/products` when flag enabled (handled in legacy nav). URL shape: `/dashboard/<tab>/:id`.

### H. Storybook
`.storybook/` config (Storybook 8 + addon-essentials + addon-a11y). Stories co-located with components. Dev-only (gitignored `storybook-static/`). `npm run storybook`, `npm run build-storybook`. Not embedded.

## Bundle budget

PR #240's `dist/assets/` baseline (gzipped): ~210 KB.
M1/T1 cap: +70 KB gzipped.
React Query + radix-dialog + radix-popover + sonner + zustand + clsx + react-hook-form + zod ≈ ~62 KB gzipped (rough estimate; verified post-build).
