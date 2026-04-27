# OpenPraxis Dashboard v2

React 18 + Vite + TypeScript SPA embedded into the Go binary at
`/dashboard/`. The legacy UI at `/` is being migrated tab-by-tab; this
folder holds the new substrate.

## Layout

```
src/
  components/
    ui/         — design-system primitives (Button, Dialog, Tooltip, …)
    layout/     — global chrome (AppShell, Header, SidebarNav, …)
    desc/       — DescToggle + MarkdownRenderer (DV/M5 trust passthrough)
    comments/   — CommentsList + CommentEditor (read-only list, simple editor)
    cy/         — CytoscapeCanvas (generic Cytoscape lifecycle wrapper)
    forms/      — FormField, FormError, FormSection, FormActions
  lib/
    api/        — typed `fetchJSON` + `useApiQuery` / `useApiMutation`
    stores/     — Zustand UI store (sidebarCollapsed, descMode, theme)
    hooks/      — useTypeahead and friends
    queries/    — entity-specific query hooks (per tab)
    featureFlags.ts — `frontend_dashboard_v2_<tab>` reader
  schemas/      — zod schemas for forms (product, manifest, task)
  pages/        — one file per tab route
  routes.tsx    — lazy route registry consumed by App.tsx + SidebarNav
.storybook/     — Storybook config (dev-only; not embedded)
```

## Primitives — when NOT to invent per-tab

When a tab manifest needs:

- A button → `import { Button } from '@/components/ui'`. Don't write
  `<button className="my-tab-btn">`. Variants live in `Button.tsx`.
- A modal → `Dialog`. Radix-backed; do NOT roll a custom overlay.
- A status pill → `Badge` for text, `StatusDot` for the colored circle.
- An empty / error state → `EmptyState` (tone="error" for failures).
- A loading boundary → `<Suspense>` + `EmptyState` for the fallback.
- A toast → `import { toast } from '@/components/ui/Toast'`. The global
  `<Toaster />` is mounted in `App.tsx`.
- A description field → `MarkdownRenderer` + `DescToggle`. The Zustand
  store persists the user's mode preference.
- A form → `react-hook-form` + a zod schema in `src/schemas/`.
  Wrap inputs in `<FormField>` for label / hint / error consistency.
- A typeahead → `useTypeahead` from `@/lib/hooks/useTypeahead`.
- An API read/write → `useApiQuery` / `useApiMutation` from
  `@/lib/api`. Don't call `fetchJSON` directly from a component.

If a primitive is missing, EXTEND `src/components/ui/` rather than
shadowing it inside a tab folder. The whole point of M1 / Foundation is
that the eight migrated tabs share one component library.

## Per-tab cutover

Each migrated tab has a `frontend_dashboard_v2_<tab>` knob in the
settings catalog (`internal/settings/catalog.go`). When set to `"true"`:

1. The legacy app's `switchView(<tab>)` in `internal/web/ui/app.js`
   short-circuits to `window.location.assign('/dashboard/<tab>')`.
2. The React app's `routes.tsx` registers the route. (The lazy
   `routeRegistry` always declares all known routes; the flag controls
   the redirect from the legacy URL only.)

Operators flip the flag once a tab's parity is signed off in
Storybook + a manual side-by-side walkthrough.

## Storybook

`npm run storybook` (or `make storybook`) opens Storybook on port 6006.
Every primitive, chrome component, and cross-cutting component has at
least one story. axe-core a11y tests for the same surface live in
`src/test/axe.test.tsx` and run under `npm test` (so a regression in a
primitive's a11y is caught in CI without spinning up Storybook).

## Type generation

`make types` runs `tygo` against `internal/settings/catalog.go` (and
future packages declared in `tools/tygo/config.yaml`) and writes
`src/lib/api/generated.ts`. CI fails on uncommitted drift via
`git diff --exit-code` after `make types`.

## Dev workflow

```
make run            # rebuild Go binary + serve at :8765
make dev-frontend   # vite HMR server, proxies /api → :8765
make test-ui        # legacy JS regression + vitest + a11y
```
