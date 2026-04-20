# Peer Review — Task `019dab06-120` (M3+M5/T4)

**PR:** [#131](https://github.com/k8nstantin/OpenPraxis/pull/131) — `feat(ui): M3+M5/T4 — shared per-tab search input with 250ms debounce`
**Manifest:** `019daafb-b5e` — Per-Tab Search (M3 per-tab input UX, M5 250 ms debounce lock)
**Product:** `019daaf5-5c4` — OpenPraxis Per-Tab Search
**Reviewer task:** `019dab06-e41d-70fd-9088-e07b7816c4b4`
**Verdict:** **APPROVE**

## Summary of the change

Adds `OL.mountSearchInput(container, opts)` in `internal/web/ui/views/search.js`
— a 101-line shared component rendering `<input>` + clear-X + results-count
badge, with a 250 ms debounce (cancel-on-new-input), a stale-response guard,
Enter-fires-immediately, and Escape-clears. Wires it into all seven list tabs
(products, manifests, tasks, memories, ideas, conversations, actions) via
dedicated `*-search-mount` divs declared in `index.html`. Each tab owns its own
`onSearch(q)` → `GET /api/<type>/search?q=...` + flat renderer, and `onClear()`
→ reload the full tree. Legacy `#manifest-search-input`/`#manifest-search-btn`
and `#conv-search-input`/`#conv-search-btn` (plus their handlers) are removed
from `index.html` and the views. In-context `tc-manifest-search` picker inside
the task-create form is untouched, per spec.

## Acceptance-criteria trace (manifest §Acceptance criteria)

| AC | Satisfied where |
|----|----|
| 1. Every listed tab (products, manifests, tasks, memories, ideas, conversations, actions) has a working search input | `internal/web/ui/index.html:237,256,276,297,328,533,552` — seven `*-search-mount` divs; `views/{memories,products,manifests,ideas,tasks,conversations,actions}.js` each call `OL.mountSearchInput` |
| 2. Typing a marker or UUID-prefix into any tab's search returns the matching entity of that type | Frontend hits `GET /api/<type>/search?q=` (`handler.go:83-103`); id-ladder behavior lives in each store and was reviewed in M1/T2 + M2/T3 PRs #126/#128 |
| 3. Typing a keyword returns entities whose title/description/body matches (LIKE) | Same endpoints; same backend coverage |
| 4. Empty input restores the full tab list | `views/search.js:47-51` (`restore()` fires `onClear`), `views/search.js:73-79` (input listener clears and restores on empty). Each tab's `onClear` re-invokes its root loader (`OL.loadProducts`, `OL.loadManifests`, etc.) |
| 5. Manifests search is fixed | M6/T1 shipped in PR #121; this PR keeps the fix and migrates the UI to the shared component (`views/manifests.js:11-22`) |
| 6. Global `#search-input` repurposed as jump-by-id | **Out of scope for T4 — M4 is a separate task.** Not blocking. |
| 7. Each store has unit tests for `Search(q, limit)` | Shipped in M1/T2 (PR #126) and M2/T3 (PR #128); `go test ./...` green on this branch |
| 8. `go build ./... && go vet ./... && go test ./...` green; CI green | Verified locally (all pass); GitHub Actions CI reports SUCCESS on commit `339af86` |

## Quality / best practices

- **Pattern consistency.** All seven tabs follow the same 9-line mount pattern
  (`mount + OL.mountSearchInput` guard; `placeholder`, `onSearch`, `onClear`).
  Easy to read, trivial to extend. No dead branches, no variance.
- **Debounce + stale-response guard.** `views/search.js:56-63` captures the
  query into `lastQ` and drops late responses whose `q !== lastQ`. Prevents
  the classic "fast typing → wrong results render last" race. Good taste.
- **Escape and Enter keyboard handling** (`views/search.js:81-89`) are the
  minimal expected UX for a search input. Clear-X focuses the input after
  clearing (`views/search.js:70`) — good keyboard flow.
- **Per-tab placeholders** are tailored — "Search products by id, marker,
  tag, or keyword...", "Search manifests by id, marker, Jira ref, tag, or
  keyword...", etc. Matches the manifest's intent (A2 — no catch-all bar;
  each tab's search is scoped).
- **Reuse of `OL.esc` and `OL.onView`** throughout — no raw event listeners,
  no XSS-risky innerHTML paths (placeholder and ariaLabel are both escaped).
- **No new dependencies.** Pure vanilla JS, consistent with the rest of the
  `ui/views/*.js` tree.
- **Count-badge semantics.** `setCount(null)` hides the badge, `setCount(n)`
  shows "N result" or "N results" — handles the singular correctly.
- **DocBlock on `search.js`** (`views/search.js:1-4`) explains M3+M5 scope
  and the 250 ms lock — future maintainers know why the value is a constant.

## Architectural decisions compliance

- **A1 (no `internal/search/` in v1):** respected — search.js is a UI helper,
  not a new shared package. Good.
- **A2 (no catch-all bar):** respected — each tab mounts its own scoped input.
  The global `#search-input` is left untouched pending M4.
- **A3 (global bar → jump-by-id):** correctly deferred to M4. PR #131 does
  not touch the header search, which is the right scope discipline.
- **A4 (250 ms debounce everywhere):** respected —
  `views/search.js:11` defaults `debounceMs = 250` and every call site uses
  the default. Locked.
- **A5 (MCP comment_add for closing comment):** applies to runner, not this
  UI code — reviewer posts via MCP below.

## Scope discipline

PR #131 modifies exactly 9 files, all listed in the manifest M3+M5 scope:

- `internal/web/ui/index.html` (+9/-7) — add seven `*-search-mount` divs,
  remove legacy ids + buttons.
- `internal/web/ui/views/search.js` (+101 new) — shared component.
- `internal/web/ui/views/{actions,conversations,ideas,manifests,memories,products,tasks}.js`
  — wire the mount per tab.

No drive-bys. No backend changes. No handler changes. No test fixture churn.

## Regression check

```
$ go build ./...   # green (only cgo deprecation warnings from sqlite-vec)
$ go vet ./...     # green
$ go test ./...    # green — action, comments, idea, manifest, mcp,
                   #         memory, product, settings, task, watcher, web
```

CI run: `https://github.com/k8nstantin/OpenPraxis/actions/runs/24682198283` —
conclusion `SUCCESS`.

## Visceral rule walk (13 rules vs. diff)

| # | Rule | Verdict |
|---|----|----|
| 1 | Fix source, not scripts | n/a — no scripts touched |
| 2 | Reusable tools | ✓ — `mountSearchInput` is itself the reusable tool |
| 3 | Python for text processing | n/a |
| 4 | Allow curl | n/a |
| 5 | New branch per mod | ✓ — `openpraxis/019dab06-120` |
| 6 | No on-the-fly workarounds | ✓ — new component, not patched inline |
| 7 | OpenLoom tasks, no CronCreate | n/a — UI change |
| 8 | $100 daily budget | n/a |
| 9 | Operational data to SQLite | n/a — no new metrics/telemetry |
| 10 | SQLite WAL + busy_timeout | n/a — no new `db.Open` calls |
| 11 | New branch per task | ✓ — one branch, one PR |
| 12 | Never auto-fire tasks | n/a — no runner changes |
| 13 | Always use worktrees | ✓ — task runs in isolated worktree; review runs in a separate worktree (`019dab06-e41...`) |

## Things I looked for and did **not** find (so, not blocking)

- No attempt to rebuild a jump-by-id global bar inside T4. Correctly deferred
  to M4.
- No leaked legacy `#manifest-search-input` / `#conv-search-input` ids
  (grep across `views/*.js` and `index.html` → no hits).
- No mocked DB / no bypassed hooks / no `--no-verify` / no force-push markers.

## Minor, non-blocking observations

- `views/search.js:17` assigns both `ol-search-input` and `conv-search`
  classes to the `<input>`. `conv-search` is a legacy styling class — harmless
  today, but a small followup could remove it once we audit the CSS. Not a
  blocker; future housekeeping.
- `index.html:103` still says `placeholder="Search memories..."` for the
  global `#search-input`. Correct for T4 (M4 retitles it to
  "Jump to id or marker…"). Left intentionally.

## Verdict

**APPROVE.** The PR implements M3 + M5 faithfully, with clean scope, strong
consistency across the seven tabs, correct debounce + stale-response
semantics, no backend coupling, and green build/vet/test + CI. Nothing rises
to the level of a rejection; the two observations above are cosmetic/followup.
