# Legacy Products Inventory — Parity Checklist for React 2 / Products M1

Source: `internal/web/ui/views/products.js` (650 lines) + `internal/web/ui/views/product-dag.js` (330 lines).

This file is the executable spec. Every row is a parity item the React port must match.

## A. Sidebar (peer → product → sub-product tree)

| # | Legacy element | API call | React owner |
|---|---|---|---|
| A1 | `+ New Product` button | — | `<NewProductDialog />` |
| A2 | Search input (mounted via `OL.mountSearchInput`) | `GET /api/products/search?q=` | sidebar input + `useTypeahead` |
| A3 | Peer group row (peer_id + count) | `GET /api/products/by-peer` | `<PeerGroup />` (TreeRow) |
| A4 | Product row (status dot, marker, title, count badges) | — | `<ProductTreeRow />` (TreeRow) |
| A5 | Sub-product row (recursive) | sub_products[] from same payload | `<ProductTreeRow level=2 />` |
| A6 | Inline manifest list under expanded product | `GET /api/products/{id}/manifests` | `<ProductTreeManifestStub />` |
| A7 | Click row → load detail + select | `GET /api/products/{id}` | `useNavigate(/products/:id)` |

## B. Detail header (top of right panel)

| # | Legacy element | API call | React owner |
|---|---|---|---|
| B1 | Title (`product-detail-title`) | — | `<ProductDetail h1>` |
| B2 | Breadcrumb (peer → product) | — | `<ProductDetailBreadcrumb />` |
| B3 | Marker + status badge + tag pills | — | `<ProductHeader />` |
| B4 | Stats bar (Manifests / Tasks / Turns / Cost / created / updated) | — | `<ProductHeader />` |
| B5 | Description toggle + markdown render | — | `<DescToggle />` + `<MarkdownRenderer />` |
| B6 | Top toolbar: ✎ Edit / + New Manifest / + Link Manifest / + Depends On / + Link Idea / ◈ Product DAG / status pivots | — | `<ProductToolbar />` |
| B7 | Status pivot buttons (draft/open/closed/archive) — active state & PUT on click | `PUT /api/products/{id} {status}` | `<StatusPivots />` |
| B8 | Copy ref button | clipboard | `<IconButton>` |

## C. Edit form (inline, replaces the body)

| # | Legacy element | API call | React owner |
|---|---|---|---|
| C1 | Title input (required) | — | `<ProductEditForm />` (FormField) |
| C2 | Description textarea (markdown) | — | `<ProductEditForm />` (FormField) |
| C3 | Tags input (comma-separated → array) | — | `<ProductEditForm />` |
| C4 | Save / Cancel | `PUT /api/products/{id}` | `useUpdateProduct()` |

## D. New product / new sub-product dialogs

| # | Legacy element | API call | React owner |
|---|---|---|---|
| D1 | + New Product → modal/form (title required, desc, tags) | `POST /api/products` | `<NewProductDialog />` |
| D2 | + New Sub-Product (when a product is selected) | `POST /api/products` + `POST /api/products/{parent}/dependencies` | `<NewSubProductDialog />` |

## E. Product dependencies section

| # | Legacy element | API call | React owner |
|---|---|---|---|
| E1 | "Depends on" header + SATISFIED/WAITING-ON pill | `GET /api/products/{id}/dependencies?direction=both` | `<ProductDeps />` |
| E2 | Dep pills (out edges) — marker + title + status + ✕ remove | `DELETE /api/products/{id}/dependencies/{depId}` | `<DepPill />` |
| E3 | "Depended on by" pills (in edges) | same payload | `<DepPill readOnly />` |
| E4 | + Depends On picker (select from available products, toggle) | `POST /api/products/{id}/dependencies` | `<AddDepPicker />` |

## F. Linked manifests (right panel section)

| # | Legacy element | API call | React owner |
|---|---|---|---|
| F1 | Linked Manifests heading + count | `GET /api/products/{id}/manifests` | `<LinkedManifests />` |
| F2 | Manifest row → click navigates to /dashboard/manifests/:id | — | `<LinkedManifests />` |
| F3 | Per-row ✕ unlink (PUT manifest with project_id="") | `PUT /api/manifests/{id} {project_id:""}` | `useUnlinkManifest()` |
| F4 | + Link Manifest picker | `GET /api/manifests` filter unlinked + `PUT /api/manifests/{id} {project_id}` | `<LinkManifestPicker />` |
| F5 | + New Manifest pre-linked to product | `POST /api/manifests {project_id}` | `<NewManifestDialog />` |

## G. Linked ideas (bottom section)

| # | Legacy element | API call | React owner |
|---|---|---|---|
| G1 | Linked Ideas heading + count | `GET /api/products/{id}/ideas` | `<LinkedIdeas />` |
| G2 | Idea row (marker, status, priority, title) | — | `<LinkedIdeas />` |
| G3 | Per-row ✕ unlink | `PUT /api/ideas/{id} {project_id:""}` | `useUnlinkIdea()` |
| G4 | + Link Idea picker | `GET /api/ideas` filter unlinked + `PUT /api/ideas/{id}` | `<LinkIdeaPicker />` |

## H. Product DAG overlay (already shipped in Foundation M1)

| # | Legacy element | API call | React owner |
|---|---|---|---|
| H1 | Overlay header + back / Esc to close | — | `<ProductDagOverlay />` (existing) |
| H2 | Cytoscape + dagre layout | `GET /api/products/{id}/hierarchy` | `<ProductDag />` (existing) |
| H3 | Tooltip on node hover | client | `CytoscapeCanvas` |
| H4 | Click node → navigate (product/manifest/task) | — | `onNodeClick` prop |

## I. Comments / revisions / knobs (sub-sections, each its own mount)

| # | Legacy element | API call | React owner |
|---|---|---|---|
| I1 | Comments section (`renderCommentsSection({type:'product'})`) | `GET /api/products/{id}/comments`, `POST /api/products/{id}/comments` | `<ProductCommentsSection />` (CommentsList + CommentEditor) |
| I2 | Revisions section | description revisions API | DEFERRED to follow-up — out of M1 scope |
| I3 | Knobs section | settings catalog | DEFERRED — out of M1 scope |

## J. URL state + cross-tab redirect

| # | Legacy element | API call | React owner |
|---|---|---|---|
| J1 | Hard-refresh restores selected product | `useParams<{id}>` + route `/products/:id` | already in `App.tsx` |
| J2 | Per-tab v2 redirect knob | `GET /api/settings/resolve?scope=system` | already in `lib/featureFlags.ts` |

## Acceptance mapping

- Click-through parity with legacy /products → A–G + I1.
- Every button works → B6, C, D, E4, F4-F5, G4.
- Bundle delta ≤ 30 KB gzipped → measured before vs after, recorded in self_summary.
- tsc + tests + build green → 5-gate exit protocol.

## Out-of-scope / deferred

- Revisions panel (I2) — needs `useDescRevisions` hook + UI; targeted at a follow-up manifest.
- Knobs panel (I3) — needs settings/resolve typeahead UI.
- Markdown editor (legacy `OL.mountEditor`) — Foundation ships only `MarkdownRenderer`; description edit uses a plain textarea. The renderer already handles both raw markup + html on read.
- Inline-edit description revisions tracking on save — covered by server-side description versioning, no UI surface needed for parity.

## Nit issues to address inline (#241–#245)

These were referenced in the task body. Search for the issue numbers in commits or here:
- #241 — Aria roles / labels on tree rows (TreeRow already wires role="button" + aria-label on chevron)
- #242 — Status dot color tokens reused across sidebar + detail
- #243 — Tags arr filtering (comma split + trim)
- #244 — Picker click-outside dismiss
- #245 — Manifest unlink confirm dialog (simple `confirm()` is fine for M1 parity)
