# OpenPraxis Dashboard v2 — React + Vite

Single-page React dashboard. Tabs migrate one at a time off the legacy
vanilla-JS UI. Built with **Vite + React 18 + TypeScript** and embedded
into the Go binary via `go:embed all:ui/dashboard/dist`.

## Scripts

```bash
npm run dev               # Vite HMR dev server (proxies /api → :8765)
npm run build             # tsc --noEmit && vite build
npm test                  # vitest run
npm run check             # tsc --noEmit (no build)
npm run storybook         # dev-only component playground (port 6006)
npm run build-storybook   # static Storybook export
```

The Make wrappers in the repo root drive the same scripts:

```bash
make frontend       # vite build
make dev-frontend   # vite dev
make test-ui        # vitest + legacy JS tests
make types          # regenerate TS types from Go via tygo
make types-check    # CI gate — fails on uncommitted drift
make storybook      # dev-only Storybook server
```

## Architecture

```
src/
  components/
    ui/             Design-system primitives (Button, Dialog, Tooltip, …)
    layout/         AppShell + Header + SidebarNav + Breadcrumb + PageWrapper
    forms/          FormField + FormError + FormSection + FormActions
    DescToggle.tsx
    MarkdownRenderer.tsx
    CommentsList.tsx + CommentEditor.tsx
    CytoscapeCanvas.tsx + ProductDag.tsx
    Tree.tsx
  lib/
    api/            fetchJSON + ApiError + useApiQuery + useApiMutation + errorBus
    stores/         Zustand UI store (sidebarCollapsed, descMode, theme)
    hooks/          useTypeahead
    queries/        Per-tab React Query hooks
    types.ts        Hand-written narrow shapes
    types.gen.*.ts  GENERATED — `make types` regenerates from Go
    dag.ts          Pure DAG layout helper
  schemas/          zod schemas + inferred TS types
  pages/            One file per migrated tab
  routes.tsx        Lazy route registry + per-tab feature flag map
  App.tsx           Router + provider + AppShell
  main.tsx          Entry
.storybook/         Storybook 8 config
```

## When NOT to invent per-tab

Always reach for the substrate first — only invent locally when the tab
genuinely needs a one-off shape.

| You need…                       | Use…                                                  |
| ------------------------------- | ----------------------------------------------------- |
| A button                        | `<Button>` from `components/ui`                       |
| Anything with only an icon      | `<IconButton>` (requires `aria-label`)                |
| A modal                         | `<Dialog>` (radix-based, focus-trapped, ESC closes)   |
| A tooltip                       | `<Tooltip>` — wraps a focusable trigger               |
| A popover menu                  | `<Popover>`                                           |
| A status pill                   | `<Badge tone="success\|info\|…">`                      |
| A status indicator              | `<StatusDot status="running">`                        |
| Empty / loading / error state   | `<EmptyState tone="neutral\|loading\|error">`         |
| To catch a render crash         | `<ErrorBoundary>` — already wraps every page          |
| Toast notifications             | `import { toast }` — `<Toaster />` is already mounted |
| HTTP read                       | `useApiQuery<T>(['key'], '/api/path')`                |
| HTTP write                      | `useApiMutation<T, V>(v => ({ path, init }))`         |
| Form                            | `react-hook-form` + `zodResolver` + `<FormField>`     |
| Persistent UI state             | `useUiStore` from `lib/stores/ui`                     |
| A tree                          | `<TreeRow<T>>`                                        |
| A Cytoscape graph               | `<CytoscapeCanvas elements style layout>`             |
| A description with markup/render| `<DescToggle raw={text} rendered={html}>`             |
| Markdown / trusted HTML         | `<MarkdownRenderer source={text} trustHtml>`          |

## Data layer

Every read goes through `useApiQuery`. Every write goes through
`useApiMutation`. Both type the row shape once and infer through —
`useApiQuery<Product>(...)` returns `data: Product | undefined`.

Errors thrown from `fetchJSON` are typed as `ApiError` and broadcast
to the global error bus → `Toaster` renders them as red toasts. Pass
`silent: true` in `FetchOptions` to opt out.

Each request stamps an `X-Request-ID` header so server logs can be
correlated with client errors.

## Forms

Schemas live in `src/schemas/` and are the single source of truth for
shape + validation. The Go side is the runtime authority; `make types`
regenerates `lib/types.gen.*.ts` on every build for read shapes.

```tsx
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { productCreateSchema } from '../schemas';

const { register, handleSubmit, formState: { errors } } = useForm({
  resolver: zodResolver(productCreateSchema),
});
```

## Routing + feature flags

Tab routes live in `src/routes.tsx`. Each declares a `flagKey` that
maps to a settings catalog entry (`frontend_dashboard_v2_<tab>`). When
the flag resolves to `"true"` at any scope (system / product / manifest
/ task), the legacy nav redirects to `/dashboard/<tab>`. Tabs migrate
one at a time — flipping the global `frontend_dashboard_v2` flag is
no longer necessary.

URL shape lock: `/dashboard/<tab>` and `/dashboard/<tab>/:id`. Anything
deeper belongs in query params.

## Testing

- `vitest` runs every `*.test.ts(x)` under `src/**`
- A11y tests use `axe-core` against rendered primitives
- `tsc --noEmit` is the strict TS gate (CI runs it)
- Storybook a11y addon catches color-contrast in the real browser
  (jsdom can't lay out colors)
- `make types-check` regenerates types and fails CI on drift

## Adding a new tab

1. Create `src/pages/MyTab.tsx`
2. Register it in `src/routes.tsx`:
   ```ts
   {
     key: 'mytab',
     path: '/mytab',
     label: 'MyTab',
     flagKey: 'frontend_dashboard_v2_mytab',
     component: lazy(() => import('./pages/MyTab')),
   }
   ```
3. Add the catalog flag in `internal/settings/catalog.go`
4. Bump the catalog count in `internal/settings/catalog_test.go`
5. Reuse primitives — do not reinvent `<Button>`, `<Dialog>`, etc.
