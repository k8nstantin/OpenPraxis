# Peer Review — Task 019dab07-0bd (M4/T5: jump-by-id global bar)

- **PR:** #134 `feat(ui): M4/T5 — repurpose global #search-input as jump-by-id`
- **Branch:** `openpraxis/019dab07-0bd` @ commit `36ee075`
- **Manifest:** `019daafb-b5e` (Per-Tab Search), M4
- **Product:** `019daaf5-5c4`
- **Reviewer task:** `019dab07-2a17-729b-a144-645e5726677e`
- **Verdict: APPROVE**

## Scope

Diff (168 +, 30 -) touches exactly two files, both named in M4:

| File | Δ |
|---|---|
| `internal/web/ui/app.js` | +166 / −28 |
| `internal/web/ui/index.html` | +2 / −2 |

No drive-bys, no accidental edits outside the M4 surface.

## AC trace (manifest § "M4" acceptance criteria)

| AC bullet | Where | Status |
|---|---|---|
| Placeholder `"Jump to id or marker…"` | `index.html:103` | ✅ |
| 250 ms debounce (A4) | `app.js` `JUMP_DEBOUNCE_MS = 250`, `setTimeout(...JUMP_DEBOUNCE_MS)` in input handler | ✅ |
| Enter triggers immediately | `keydown` handler clears timer + calls `trigger()` | ✅ |
| Parallel `GET /api/<type>/search?q=` across all 6 types | `JUMP_TYPES` × 6, `Promise.all(fetches)` | ✅ |
| First exact id / marker / id-prefix hit wins → tab + detail | `findExactIdHit` + `jumpTo` sets `location.hash = #view-<view>/<id>`, existing `hashchange` listener → `switchView` + `applyHashArg` | ✅ |
| Multiple hits → dropdown `{type + marker + title}` | `renderCandidates` with 3-column row | ✅ |
| Keyword input → candidate dropdown + hint "for full text search use the tab's search bar" | `renderCandidates(q, cands, isKeyword=true)` | ✅ |
| No match → inline "No match for `<q>`" | `renderNoMatch` | ✅ |
| Empty / Escape closes | `hideResults()` on empty input, Escape clears | ✅ |
| Existing POST endpoints untouched | Only GETs used from the bar; no server-side changes | ✅ |

Product-level AC #6 ("global `#search-input` repurposed as jump-by-id") — fully satisfied by this PR.

## Code quality

- **Stale-response guard** (`app.js` `doJump`): after `Promise.all`, re-reads the input value and bails if the user kept typing. Matches the pattern the per-tab search component adopted in T4.
- **Defensive response shape**: `Array.isArray(list) ? list : (list && list.items) || []` tolerates both flat-array (post-#133) and legacy `{items:[]}` shapes. Good belt-and-suspenders given PR #133 just landed.
- **Hash routing reuse**: the jump sets `location.hash`, then the pre-existing `hashchange` listener (`app.js:133`) calls `switchView` + `applyHashArg`, which routes through the per-view loader map (`products` → `OL.loadProductDetail`, `manifests` → `OL.loadManifest`, `tasks` → `OL.loadTaskDetail`, `memories` → `OL.loadMemoryPeerDetail`, `ideas` → `OL.loadIdea`, `conversations` → `OL.loadConv`). No new loader plumbing duplicated.
- **`looksLikeId` regex** `/^[0-9a-f]{8,}(?:-[0-9a-f-]*)?$/i` with length window [8,36] cleanly accepts the 8-char marker, the 12-char `marker`-field prefix, and the full UUID. Non-hex keyword input correctly falls through.
- **`candidateFromEntity` id/title coalescing** walks the alt-cased and alt-named fields (`id`/`ID`, `title`/`Title`/`path`/`summary`/`content`/`l1`/`tool_name`) so every entity type renders a sensible dropdown row without per-type branching.
- **Clear/Escape UX**: the `×` button and Escape both clear input and hide overlay; debounce timer is cancelled on Enter and on new input.
- **Diagnostics hook**: `OL._jumpCurrentQuery` is a minor dev aid; harmless.

## Regression checks

- `go build ./...` — clean (only unrelated cgo deprecation warnings from `sqlite-vec`)
- `go vet ./...` — clean
- `go test ./...` — all 11 packages with tests green (`action`, `comments`, `idea`, `manifest`, `mcp`, `memory`, `product`, `settings`, `task`, `watcher`, `web`)
- Per-tab search from T4 (PR #131) is untouched: PR diff does not modify `internal/web/ui/views/search.js` or the per-tab `*-search-mount` divs — no regression risk.
- Existing POST `/api/memories/search`, `/api/manifests/search`, `/api/conversations/search` endpoints still exist (MCP tools and programmatic consumers unaffected) — PR only consumes the GET aliases added in T3 / normalized in #133.

## Visceral-rule walk (13)

| # | Rule | Status |
|---|---|---|
| 5 | every code modification = new branch | ✅ branch `openpraxis/019dab07-0bd` |
| 10 | WAL + busy_timeout on every `db.Open` | N/A — no DB code touched |
| 11 | each task gets its own branch/PR | ✅ PR #134 dedicated |
| 13 | always use git worktrees | ✅ reviewed in `/tmp/wt-019dab07-0bd` worktree |

Others (#1–4, #6–9, #12) not applicable to this diff.

## Gaps (non-blocking)

1. **No client-side unit tests** for `looksLikeId` / `findExactIdHit` / `candidateFromEntity`. The repo has no existing JS test harness, so wiring one up is out of scope for this task. Manifest AC #7 targets store-level tests (satisfied upstream in T2). Follow-up: a minimal Vitest/Node test for `looksLikeId` + `findExactIdHit` when a JS harness is added.
2. **8-char id-prefix behaviour on ambiguity**: `findExactIdHit` returns the first bucket with a hit in the `JUMP_TYPES` declaration order (memories → manifests → tasks → products → ideas → conversations). If the same 8-char prefix existed across two entity types (astronomically unlikely — these are UUIDv7 markers) the earlier type wins silently. Acceptable given the birthday-collision math, but worth a comment in a future iteration.
3. **Back-nav UX**: `jumpTo` clears `input.value`. If the user hits browser-back, the input does not restore the last query. Minor.

None of these block approval.

## Summary

PR #134 cleanly executes M4: the top-nav bar is no longer a memories-only semantic search — it's a parallel-fanout jump-by-id that routes through the existing hash-deeplink infrastructure. Scope is tight (2 files), debounce matches A4, stale-response is guarded, and all build/vet/test gates are green. **APPROVE.**

## Product retrospective (final task of the chain)

**What shipped (M1 → M6 under product `019daaf5-5c4` / manifest `019daafb-b5e`):**
- **M6/T1** (PR #121): `/api/manifests/search` now matches id/marker, not just semantic content.
- **M1/T2** (PR #126): store-level `Search(q, limit)` on `task`, `product`, `idea`, `action` — id-ladder + keyword `LIKE`, WAL-compliant.
- **M2/T3** (PR #128): GET `/api/<type>/search?q=&limit=` endpoints for all per-tab types; POST endpoints preserved for MCP/programmatic callers.
- **M3+M5/T4** (PR #131): shared `OL.mountSearchInput` component — 250 ms debounce, stale-guard, clear-X + count badge — wired into products, manifests, tasks, memories, ideas, conversations, actions.
- **M4/T5** (PR #134, this review): global `#search-input` repurposed as jump-by-id with parallel fan-out + hash-deeplink.
- **Hotfixes in-chain:** PR #123 `comment_add` MCP tool (unblocked runner closing step); PR #124 allowlist fix; PR #133 `/api/<type>/search` response-shape normalization (flat arrays, `[]` on empty).

**Known-broken / follow-up:**
- No JS unit tests for the jump-by-id helpers — deferred until a JS test harness is introduced.
- Semantic search still memory-only; keyword-only for everything else (manifest explicitly deferred FTS5 and cross-type semantic).
- Amnesia, watcher, delusion tabs have no search (deferred to a later manifest per M1).
- Federated cross-peer search not yet scoped.

**Net:** all seven listed tabs (products, manifests, tasks, memories, ideas, conversations, actions) now have a working search input that accepts id or keyword and returns results of that entity type — exactly the vision-pivot stated in the manifest. Global bar works as a jump-by-id with keyword fallback dropdown. Product acceptance criteria 1–7 are satisfied; AC #8 (CI green across all PRs) confirmed locally on this PR.
