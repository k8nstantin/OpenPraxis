# Changelog

Moved out of the main README to keep the landing page focused on what OpenPraxis **is** rather than what just changed. See the ["Changelog" link in the README](../README.md#deeper-references) to land here from the top of the repo.

## April 2026

Landed on `main` between 2026-04-20 and 2026-04-22.

### Reliability

- **Watcher is observer-only ([#149](https://github.com/k8nstantin/OpenPraxis/pull/149)).** Removed the gatekeeper path that was mutating task state + blocking `ActivateDependents` on gate failure. Audits still run; findings post as `watcher_finding` comments; the paired review task owns the verdict. Fixes the "ops-task with zero commits auto-downgraded to failed" bug that stranded INT MySQL backup chains.
- **Task `depends_on` display widened 8 → 14 chars ([#145](https://github.com/k8nstantin/OpenPraxis/pull/145)).** UUIDv7 tasks created in the same millisecond share an 8-char time prefix, so the old `task_get` output rendered every child as if it pointed at the same parent. Now unambiguous.
- **Scheduler cleanup rule for cancelled recurring tasks.** `task_cancel` on a task with `schedule: 30m` is now durable: status flips to `cancelled`, schedule collapses to `once`, `next_run_at` clears, so the runner can't re-fire it. (Operational tooling, not a PR — directly applied.)

### DAG renderer — one-and-done

- **Dagre layout ([#158](https://github.com/k8nstantin/OpenPraxis/pull/158)).** Deleted 80+ lines of hand-rolled column/row arithmetic + manual topo sort + per-manifest DFS. Replaced with `layout: { name: 'dagre', rankDir: 'TB', ... }`. Any DAG shape — linear chain, independent pairs, multi-parent fan-in, empty manifests — now renders correctly with no layout-specific code.
- **Edges from real `depends_on` ([#146](https://github.com/k8nstantin/OpenPraxis/pull/146), kept through #158).** Product → manifest (ownership), manifest → manifest (explicit deps), parent-task → child-task (`depends_on`), manifest → task (ownership for in-manifest roots).
- **Local vendor bundle ([#159](https://github.com/k8nstantin/OpenPraxis/pull/159)).** Cytoscape + dagre + cytoscape-dagre pinned and served from `/vendor/`, not a CDN. Dashboard works offline; no silent break when a CDN hiccups.
- **Extract + contract test ([#160](https://github.com/k8nstantin/OpenPraxis/pull/160)).** Diagram code moved out of the 900-line `products.js` into `views/product-dag.js`. Added `TestProductHierarchy_EmptyProduct / _LinearChain / _ParallelPairs` in `handlers_product_hierarchy_test.go` so the API contract dagre rides on is locked at build time.

### Search

- **Keyword-first search for conversations + actions ([#152](https://github.com/k8nstantin/OpenPraxis/pull/152)).** New envelope response `{ items, total, offset, limit, has_more, semantic? }`. Infinite scroll appends pages. `<mark>`-highlighted snippets around the first literal match, 80 chars of context each side. Conversations get an optional "Related by meaning" semantic tail on page 0 (capped at 10, deduped). Actions stay keyword-only (already LIKE-based).

### Execution controls

- **Model selector is an enum ([#151](https://github.com/k8nstantin/OpenPraxis/pull/151)).** `default_model` moved from free-form string to `KnobEnum` with the Claude family (`""` agent default, `claude-opus-4-7`, `claude-sonnet-4-6`, `claude-haiku-4-5`). Dashboard renders as a `<select>`; typos rejected at validation.

### Comments + target resolution

- **Short-marker target IDs resolve everywhere.** `comment_add` + HTTP comment endpoints accept 8–12 char markers or full UUIDs; both paths canonicalize via the entity stores before writing ([#136](https://github.com/k8nstantin/OpenPraxis/pull/136), [#139](https://github.com/k8nstantin/OpenPraxis/pull/139)). Sweep migration for legacy short-marker orphans shipped as `openpraxis admin migrate-comment-orphans` ([#141](https://github.com/k8nstantin/OpenPraxis/pull/141)).
- **`execution_review` enforcement ([#118](https://github.com/k8nstantin/OpenPraxis/pull/118)).** Task completion blocked unless the agent posted its post-run retrospective.

### Branding

- **OpenPraxis mark ([#161](https://github.com/k8nstantin/OpenPraxis/pull/161)).** Sidebar glyph swapped for a transparent 256×256 PNG served from `/assets/openpraxis-icon.png`. Favicon + apple-touch-icon wired at the same path.
