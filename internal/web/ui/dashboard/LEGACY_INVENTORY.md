# React 2 / Products M1 — Legacy Inventory

Audit of `internal/web/ui/views/products.js` (650 lines) and
`internal/web/ui/views/product-dag.js` (330 lines). Every fetch +
UI element + interaction enumerated so the React port hits parity.

## A. Fetches (every endpoint exercised)

| Method | Path | Where | What it drives |
|---|---|---|---|
| GET | `/api/products/by-peer` | `loadProducts`, `loadProductDetail` (typeahead source) | Sidebar tree + dep picker candidates |
| GET | `/api/products/search?q=…` | `loadProducts` (search box) | Search-results list rendered in sidebar |
| GET | `/api/products/{id}` | `loadProductDetail`, `editProduct` | Detail header + edit form preload |
| POST | `/api/products` | `createProduct` | Create top-level product |
| PUT | `/api/products/{id}` | `editProduct`, `updateProductStatus` | Save edits + cycle status |
| GET | `/api/products/{id}/dependencies?direction=both` | `loadProductDetail` | "Depends on" + "Depended on by" pills |
| POST | `/api/products/{id}/dependencies` `{depends_on_id}` | dep picker | Add product dep |
| DELETE | `/api/products/{id}/dependencies/{otherId}` | dep pill ✕ | Remove product dep |
| GET | `/api/products/{id}/manifests` | `loadProductDetail`, sidebar-expand | Linked manifests list |
| GET | `/api/manifests` | `linkManifestToProduct` | Picker — manifests not yet linked |
| PUT | `/api/manifests/{id}` `{project_id}` | link / unlink | Attach / detach from product |
| GET | `/api/products/{id}/ideas` | `loadProductDetail` | Linked ideas list |
| GET | `/api/ideas` | `linkIdeaToProduct` | Picker — ideas not yet linked |
| PUT | `/api/ideas/{id}` `{project_id, …}` | link / unlink | Attach / detach idea |
| GET | `/api/products/{id}/hierarchy` | `showProductDiagram` (DAG overlay) | Cytoscape elements |

Cross-references via `OL.*` (out-of-tree but called by these files):
`OL.descToggle`, `OL.renderTree`, `OL.mountSearchInput`, `OL.mountEditor`,
`OL.renderKnobSection` (settings catalog scope = product),
`OL.renderRevisionsSection` (DV/M5),
`OL.renderCommentsSection` (POST/GET `/api/products/{id}/comments`),
`OL.createManifest({productId})` (POST `/api/manifests`),
`OL.copy`, `OL.switchView('manifests'|'tasks')`, `OL.loadManifest`,
`OL.loadTaskDetail`.

## B. UI Elements

### Sidebar (`loadProducts`)
1. Search input (`mountSearchInput`) — placeholder
   `Search products by id, marker, tag, or keyword...`. Empties the list and
   re-renders search hits as flat `manifest-item.clickable` rows.
2. `+ New Product` button — fires `createProduct()`.
3. Tree (`renderTree`) — three tiers:
   - **L0** peer group `peer_id` + `count`.
   - **L1** product `title`, marker, manifest count, sub-count purple chip.
     Status dot color: open → green, closed/draft → red/yellow, archive → red.
     Click expands + loads detail + lazy-fetches that product's manifests.
   - **L2** sub-products (recursive — same `productLevel` config).
4. Empty fallback: `No products yet. Create one to group your manifests.`

### Detail panel (`loadProductDetail`) — under `#product-detail`
1. **Breadcrumb** `<peer> → <marker> <title>`, peer click → tab top.
2. **Metadata bar**: large marker, status badge, tag pills, copy-ref button
   (`OL.copy('get product <marker>')`).
3. **Stats bar**: Manifests, Tasks, Turns, Cost ($x.xx), Created, Updated.
4. **Top toolbar** — single row of action buttons:
   - ✎ Edit
   - + New Manifest (preselects `productId`)
   - + Link Manifest
   - + Depends On (toggles dep picker)
   - + Link Idea
   - ◈ Product DAG (full-screen Cytoscape overlay)
   - Status pivots: draft / open / closed / archive (active = filled)
5. **Revisions mount** — DV/M5 history list.
6. **Description** — `descToggle` markdown ↔ rendered with persisted mode.
7. **Deps section**:
   - "Depends on" label + status pill: `✓ SATISFIED` (all terminal) or
     `⏳ WAITING ON N` (any non-terminal).
   - Outgoing pills with marker (clickable nav), title, status, ✕ remove.
   - Inline picker (hidden by default; toggle from toolbar):
     `<select>` with all candidate products excluding self / archive / dups.
     Inline error span below the select.
   - "Depended on by" pills (no remove control).
8. **Linked Manifests section** with row count and clickable rows:
   marker + status badge + `N tasks · N turns · $cost` strip + ✕ unlink.
9. **Knobs mount** — `renderKnobSection({type:'product', id})`.
10. **Comments mount** — `renderCommentsSection({type:'product', id})`.
11. **Linked Ideas section** with priority dot + ✕ unlink.

### `createProduct` (in-place form, no dialog)
Title (required), Description (textarea), Tags (csv). `Create Product` /
`Cancel` buttons. On save → `loadProducts()` + `loadProductDetail(p.id)`.

### `editProduct` (in-place form using `mountEditor`)
Title, Description (resizable monospace, Cmd-S to save), Tags csv. Cancel
returns to detail view. Save = `PUT /api/products/{id}` then re-render.

### `linkManifestToProduct` (in-place picker)
List unlinked manifests; click → `PUT /api/manifests/{id} {project_id}`.

### `linkIdeaToProduct` (in-place picker)
Same shape as manifest picker but for ideas; preserves all idea fields when
patching.

## C. DAG overlay (`product-dag.js`)

`OL.showProductDiagram(productId, productTitle)` mounts a fullscreen
overlay (`#product-diagram-overlay`, z-index 1000). Header has Back
button + ESC key handler + legend (hierarchy / manifest dep / task dep
edge styles). Body is `#product-cytoscape`.

`OL.buildDagElements(data)` (exposed for tests):
- Recursive `addProduct(p)` — emits product node, manifest children,
  task grandchildren, then sub-products (depth-bounded by hierarchy
  endpoint; defense via `seenProducts`).
- Edge types: `product_link` (purple), `manifest_dep` (blue solid),
  `ownership` (blue dashed), `task_dep` (amber dashed).
- Manifests with no in-product deps get a `product_link` edge from
  the product. Tasks with `depends_on` get `task_dep`; otherwise an
  `ownership` edge from manifest.
- Title shortener strips `QA `, `OpenPraxis `, ` — …` suffix.

`renderProductDiagram(productId)`:
- Layout `dagre` rankDir=TB, nodeSep=40, rankSep=90, edgeSep=25,
  padding=32, fit=true.
- Node sizes: product 180×60 purple bold, manifest 140×50 blue bold,
  task 120×44 base. Status colors: completed green, running yellow,
  failed red.
- Tooltip on hover: title, status, marker, type-specific stats
  (manifests/tasks count, depends-on titles, turn count, cost).
- Click product → `OL.loadProductDetail`. Click manifest → switch to
  manifests tab + `OL.loadManifest`. Click task → tasks tab +
  `OL.loadTaskDetail`. ESC + Back close the overlay.

## D. Already shipped by Foundation M1 (PR #248)

Re-used directly:
- `Tree`, `TreeRow` — recursive sidebar.
- `CytoscapeCanvas`, `ProductDag`, `lib/dag.ts` — DAG plumbing.
- `Button`, `IconButton`, `Badge`, `StatusDot`, `EmptyState`, `Dialog`,
  `Tooltip`, `Popover`, `Toast`.
- `FormField`, `FormError`, `FormSection`, `FormActions`, `useTypeahead`.
- `MarkdownRenderer`, `DescToggle`.
- `CommentsList`, `CommentEditor`.
- `lib/api/client.ts` (`fetchJSON` w/ X-Request-ID + toast),
  `lib/api/queries.ts` (`useApiQuery` / `useApiMutation`).
- `lib/queries/products.ts` (read hooks; this PR extends with mutations).
- `lib/featureFlags.ts` — `tabIsOnReactV2('products')` already present.
- `routes.tsx` — route entry already maps `/products` →
  `frontend_dashboard_v2_products` knob.

## E. Out of scope (per manifest spec)

- Markdown editor toolbar (legacy `mountEditor`) — react-markdown
  preview is sufficient for description editing in M1.
- Search box with debounce — covered for parity by simple text input
  filtering the same `/api/products/search` endpoint.
- Knob mount + Revisions mount — these primitives don't exist yet in
  React land; bullets remain marked PARTIAL with a follow-up issue.

## F. Acceptance map (manifest `<scope>` ↔ this PR)

| Scope item | Implementation |
|---|---|
| Sidebar peer → product → sub tree | `Products.tsx` — `useProductsByPeer` + `TreeRow` |
| Header: title, marker, status, tags, stats, toolbar | `ProductHeader` |
| Edit toggle inline form (FormField) | `ProductEditForm` |
| Status pivots draft/open/closed/archive | `StatusPivot` buttons in toolbar |
| + New Product (top-level + sub via dep) | `NewProductDialog` (Radix Dialog) |
| + New Manifest (pre-linked) | `NewManifestDialog` posts `project_id` |
| + Link Manifest / Idea / Depends On (typeahead) | `LinkPicker` w/ `useTypeahead` |
| Manifests list clickable → /manifests/:id | `ProductManifests` |
| Ideas list, Comments | `ProductIdeas`, `useProductComments` + `CommentsList` |
| DAG overlay (lazy CytoscapeCanvas) | `ProductDagOverlay` |
| URL state via useParams; refresh restores | `Routes` for `/products/:id` |
| Per-tab flag wires legacy redirect | already in `routes.tsx` + `lifecycle.js` |
