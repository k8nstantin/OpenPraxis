# Changelog

Moved out of the main README to keep the landing page focused on what OpenPraxis **is** rather than what just changed. See the ["Changelog" link in the README](../README.md#deeper-references) to land here from the top of the repo.

## v0.9.0 — May 2026

### Trace-Grounded Feedback Loop (Meta-Harness Pattern)

Three phases that close the retry loop — agents now see their own history, operators can query pass rates, and the system can improve its own prompt scaffold autonomously.

#### Phase 1 — Prior Context Injection

Every task prompt includes a `<prior_context>` block (between `<manifest_spec>` and `<task>`) with digests of prior runs and prior agent review comments. Agents stop repeating mistakes on retries.

- 5 new **Prompt Context** knobs: `prompt_prior_runs_limit` (5), `prompt_prior_comments_limit` (3), `prompt_build_timeout_seconds` (5s), `prompt_max_comment_chars` (2000), `prompt_max_context_pct` (0.40)
- Budget enforcement: oldest entries drop when context window limit approached
- Bounded DB timeout on history queries — no dispatcher stalls

#### Phase 2 — Frontier Queries and Pass-Rate API

- `GET /api/execution/frontier?manifest_id=X` — per-task pass rates, avg cost, avg turns, best task
- Single `GROUP BY` SQL query — no N+1 across large manifests
- `cost_per_turn` and `cost_per_action` now in run detail responses
- `frontier_window_days` knob controls lookback (default 30 days)

#### Phase 3 — Proposer Loop

Autonomous prompt scaffold evolution — **off by default** (`proposer_enabled=false`).

- Monitors manifest failure streaks and per-run cost thresholds
- Auto-fires a proposer agent; proposer reads frontier and changes one template section
- Evaluator accepts or tombstones the change based on pass-rate delta
- Full rollback via `template_tombstone` on regression
- 5 new **Proposer Loop** knobs: `proposer_enabled`, `proposer_trigger_failure_streak` (3), `proposer_trigger_cost_usd` (0), `proposer_min_pass_rate_delta` (0.05), `proposer_max_candidates` (3)

### DAG — Type-Agnostic and Bidirectional

- Graph handler renders whatever is in the relationships table — no hardcoded entity type checks
- Bidirectional: incoming edges expand the discovered node set, so `links_to` and `reviews` edges appear from either end
- `AllEdgeKinds()` single source of truth for all edge kind enumerations
- `rel_create` MCP tool now accepts `skill` and `idea` as valid `src_kind`/`dst_kind`

### Entity Description Unified

- `entity_create` and `entity_update` accept a `description` param for all entity types (product, manifest, task, skill, idea)
- `idea_add` / `idea_update` MCP tools removed — superseded by unified `entity_*` path
- All descriptions stored as append-only `type=prompt` comments with full revision history

### DAG Chain Recovery Fix

- Server restart no longer auto-fires tasks from manifests with old historical completions
- `dag_chain_recovery_window_minutes` knob controls recovery window (default 60 min, 0 = disabled)

### Visceral Rule — No Hardcoding

Added system-enforced rule: no entity kinds, edge kinds, status values, or enumerated domain constants may appear as string literals in handler or business logic code. All sessions load this rule before any work.

---

## v0.8.0 — May 2026

### ContentBlock — unified text + attachment element

Single reusable component replaces the per-entity description editors. Same element across all types, just with different labels:

| Entity | Label |
|--------|-------|
| Product | Description |
| Manifest | Declaration |
| Task | Instructions |
| Skill / Idea | Description |

- `BlockNoteComposer` with native drag/drop/paste file attachments (paperclip button in toolbar)
- Version history built-in — collapsible revision list
- Comments tab uses same composer; only latest `description_revision` highlighted (older dimmed)

### System metrics consolidated into execution_log

`system_host_samples` table dropped. All system metrics now live exclusively in `execution_log` sample rows:

- 7 new `NOT NULL` columns: `net_rx_mbps`, `net_tx_mbps`, `disk_read_mbps`, `disk_write_mbps`, `mem_used_mb`, `mem_total_mb`, `load_avg_1m`
- MCP session sampler and hook writer capture full OS state on every tick
- `NOT NULL` with no default — missing data fails at the DB level, not silently

### Charts

- **Activity chart**: turns + actions on shared Y-axis; lines/files/net in separate System chart
- **SystemStatsChart**: self-contained with full range selector (1d default, up to All)
- **Y-axis pinned**: max set from data × 1.1, no rescaling between ticks
- **All charts**: `boundaryGap: false`, smooth monotone curves, ET timezone labels

### Runner

- All 63 `execution_log` columns populated on every task run
- Git stats (lines+/-, files, commits, branch, SHA) captured after each run
- Compactions, errors, reasoning tokens parsed from stream-json output
- Host sampler extended with network and disk I/O rates

## v0.7.0 — May 2026

### Stats page — full execution history with per-chart zoom

New `/stats` route in the Governance sidebar. All data from `execution_log`, no joins.

**5 tabs, all charts independently zoomable** — each chart header has `1d · 2d · 3d · 1w · 2w · 1m · 3m · All`. Change the range on one chart without affecting others.

| Tab | Charts |
|-----|--------|
| **Runs** | Daily runs (completed/failed), avg duration trend, terminal reasons donut, retry distribution |
| **Efficiency** | Avg turns/run, cache hit rate %, context window %, tokens/turn, actions/turn, compactions |
| **Tokens** | Daily token volumes (stacked), cache read/write ratio, output tokens, reasoning tokens |
| **Productivity** | Lines added/removed, commits + files changed, tests run/passed/failed — backfilled from git history |
| **Agents** | Runs by model, runs by agent runtime, interactive vs autonomous split |

**History backfill** — `started_at` timestamps used for real run dates (going back to April 11), not `created_at` which was stamped at migration time. Git log merged into Productivity for lines/commits data.

| Runs | Efficiency | Tokens |
|------|------------|--------|
| ![](images/stats-runs-v1.png) | ![](images/stats-efficiency-v1.png) | ![](images/stats-tokens-v1.png) |

## v0.6.0 — May 2026

### Entity Unification + Execution Log Architecture

**The most significant architectural change since v0.1.** Collapses five separate entity tables into one unified `entities` table and rewrites all execution tracking into a single append-only `execution_log`.

#### Core architecture changes

- **`entities` table (SCD-2)** — all entity types in one table: `product | manifest | task | idea | skill`. Every change creates a new row; time travel is native.
- **`execution_log` (append-only, event-sourced)** — every run writes `event=started → sample (every 5s) → completed|failed` with full metrics: turns, actions, tokens, CPU, RSS, lines, commits, tests.
- **13 legacy tables dropped** at startup: `products`, `manifests`, `ideas`, `task_runs`, `task_run_host_samples`, `task_dependency`, `product_dependencies`, `manifest_dependencies`, `idea_manifest_links`, `task_manifests`, `task_runtime_state`, `execution_log_samples`, `execution_log_legacy`, `model_pricing`.
- **`relationships` table** is the single source for all edges (owns, depends_on, links_to) across all entity types.
- **`comments` table** stores all text content (descriptions, notes, decisions) for every entity type.
- **`schedules` table (SCD-2)** drives when entities run — the scheduler reads this, not the tasks table.

#### Interactive session tracking

Claude Code sessions now write to `execution_log` in real time:
- MCP `AfterInitialize` → `event=started` row
- Every 5 seconds → `event=sample` row (CPU/RSS from system sampler)
- Every 30 seconds → `event=sample` row with full transcript metrics (turns, tokens, cache hit rate)
- `SessionEnd` hook → `event=completed` row with final transcript metrics

#### New portal features

- **Overview page** — 8 stat cards + 8 ECharts pulled from `execution_log` and git history: runs per hour, cache hit rate trend, avg turns/run, cache reuse ratio, lines added/removed, commits per hour, terminal reasons donut, interactive vs autonomous split.
- **Unified entity menus** — all 5 entity types (Products, Manifests, Tasks, Skills, Ideas) use one identical component: expand arrow, `+ New` button, status dot, full UUID, latest first.
- **Skills + Ideas** — added to sidebar with dedicated routes and full entity detail (Main · Execution · Comments · Dependencies · DAG).
- **Schedules page** — standalone tab with Active / History tabs and a create form.
- **Git productivity** — `GET /api/stats/git` reads real commit history and merges with execution_log for lines/commits/files charts.
- **Cost removed** — all hardcoded pricing rates deleted. Cost will be redesigned with a proper billing integration.

#### Breaking changes

- `/api/products`, `/api/manifests`, `/api/tasks` CRUD endpoints removed. Use `/api/entities?type=<kind>`.
- `product_create`, `manifest_create`, `task_create` MCP tools removed. Use `entity_create`.
- Stats and Schedule tabs removed from entity detail. Will be rebuilt from `execution_log`.

#### Screenshots

| Overview | Products | Schedules |
|----------|----------|-----------|
| ![](images/overview-v3.png) | ![](images/products-v3.png) | ![](images/schedules-v3.png) |

## v0.5.0 — May 2026

### Breaking changes

- **`:9766` retired.** Portal V2 is now the only dashboard and serves on `:8765`. Any bookmark or script pointing at port 9766 must be updated.
- **`--portal-v2-port` flag removed.** Passing this flag to `openpraxis serve` now errors. Remove it from start scripts.

### Legacy portal removed ([#325](https://github.com/k8nstantin/OpenPraxis/pull/325))

17,000+ lines of legacy vanilla-JS dashboard code deleted: `views/`, `components/`, `vendor/`, `assets/`, `app.js`, `api.js`, `style.css`, `index.html`, `tree.js`, `lifecycle.js`, `task-status.js`. The dual-port architecture is gone — one binary, one port (`:8765`), one dashboard.

The React dashboard (Vite + React 19 + Tailwind v4 + TanStack Router + shadcn/ui) is now the canonical UI. It is embedded directly in the Go binary via `go:embed all:ui/dashboard-v2/dist` and served by the primary `Handler()` function.

`handler_v2.go` has been deleted — its functionality is merged into `handler.go`. The `Makefile` build pipeline now builds only the React dashboard before `go build`.

### Dashboard layout

Portal V2 organises the sidebar into three groups:

- **Operations** — Overview, Actions Log, Products, Manifests, Tasks, Inbox, Recall
- **Governance** — Productivity, Audit, Activity
- **Configuration** — Settings

Execution controls are presented as semicircle dial gauges (not sliders) at every scope level (task → manifest → product → system).

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
