# Peer audit — Task `019dab05-b95` (M2/T3: HTTP search endpoints)

**Reviewer:** task `019dab05-d1b` (peer audit)
**Manifest:** `019daafb-b5e` — Per-Tab Search
**PR under review:** [#128](https://github.com/k8nstantin/OpenPraxis/pull/128) — `feat(search): M2/T3 — HTTP endpoints for per-tab search`
**Head commit:** `edabe29b5fda473e11c12affd778c5d84be3ea44`
**Verdict:** ✅ **APPROVE** (two non-blocking nits)

---

## What shipped

| File | Role |
|---|---|
| `internal/web/handler.go` | +10 lines: registers 7 new `GET /api/<type>/search` routes before `/{id}` to prevent `gorilla/mux` from binding "search" as an id |
| `internal/web/handlers_search.go` | NEW, 145 lines: `parseSearchParams` helper + 7 thin handlers delegating to `n.{Tasks,Products,Ideas,Actions}.Search` / `n.Manifests.Search` / `n.SearchMemories` / `n.SearchConversations` |
| `internal/web/handlers_search_test.go` | NEW, 208 lines: `httptest`-backed unit tests for tasks/products/ideas/actions handlers (keyword, id-exact, id-prefix, unknown, empty) + `parseSearchParams` |

Zero deletions, zero drive-bys.

---

## Gate results

| Gate | Result | Evidence |
|---|---|---|
| Build | ✅ | `go build ./...` clean (only pre-existing cgo sqlite-vec deprecation warnings) |
| Vet | ✅ | `go vet ./...` clean |
| Tests | ✅ | `go test ./...` — all packages green, incl. `internal/web 1.230s` |
| Branch policy (rule #5/#11) | ✅ | dedicated branch `openpraxis/019dab05-b95`; PR against `main` |
| WAL+busy_timeout (rule #10) | ✅ | test harness opens DB with `_journal_mode=WAL&_busy_timeout=5000`; handlers delegate to existing WAL-backed stores |
| Worktree (rule #13) | ✅ | PR prepared inside `.openpraxis-work/…` worktree |

---

## Acceptance-criteria trace (manifest M2)

| AC | Satisfied by |
|---|---|
| `GET /api/tasks/search` NEW | `handlers_search.go:apiTasksSearch` + route `handler.go:81`; test `TestTasksSearch_KeywordAndID` |
| `GET /api/products/search` NEW | `handlers_search.go:apiProductsSearch` + route `handler.go:82`; test `TestProductsSearch_KeywordAndPrefix` |
| `GET /api/ideas/search` NEW | `handlers_search.go:apiIdeasSearch` + route `handler.go:83`; tests `TestIdeasSearch_Keyword`, `TestIdeasSearch_Unknown` |
| `GET /api/actions/search` NEW | `handlers_search.go:apiActionsSearch` + route `handler.go:84`; test `TestActionsSearch_Keyword` |
| `GET /api/memories/search` alias | `apiMemoriesSearchGET` + route `handler.go:87`; verified live via curl |
| `GET /api/manifests/search` alias | `apiManifestsSearchGET` + route `handler.go:85`; verified live via curl |
| `GET /api/conversations/search` alias | `apiConversationsSearchGET` + route `handler.go:102`; verified live via curl |
| Empty `q` → `[]`, no 4xx | `handlers_search.go` every handler guards `if q == "" { writeJSON(w, []any{}); return }` |
| Limit default 50, overridable | `parseSearchParams` defaults 50, parses `?limit=<int>`; test `TestParseSearchParams_LimitAndAlias` |
| POST endpoints unchanged (MCP/programmatic) | confirmed: `apiSearch` / `apiManifestsSearch` / `apiConversationSearch` POST handlers left in place at `handler.go:86,?,101`; live curl POST regression passes |
| Response shape = native entity list | `writeJSON(w, results)` passes through store return values; `manifests` enriched via existing `enrichManifests` for UI parity |

---

## Live smoke test (port 18765, isolated `serve` against shared DB)

| Call | HTTP | Result |
|---|---|---|
| `GET /api/tasks/search?q=019dab07-2a1` | 200 | 1 hit — id matches the 12-char marker |
| `GET /api/tasks/search?q=jump-by-id` | 200 | 2 keyword hits |
| `GET /api/tasks/search?q=T&limit=2` | 200 | exactly 2 (limit honored) |
| `GET /api/products/search?q=019da5e8-47f` | 200 | 1 marker hit |
| `GET /api/products/search?q=Visual` | 200 | 1 keyword hit |
| `GET /api/manifests/search?q=019daafb-b5e` | 200 | 1 marker hit |
| `GET /api/ideas/search?q=Idea` | 200 | 6 keyword hits |
| `GET /api/actions/search?q=Bash&limit=2` | 200 | 2 hits |
| `GET /api/memories/search?q=search&limit=3` | 200 | 3 hits |
| `GET /api/conversations/search?q=openpraxis` | 200 | 50 hits (default limit) |
| `GET /api/products/{uuid}` | 200 | `/{id}` route **not** shadowed by `/search` sibling |
| `GET /api/tasks/{uuid}` | 200 | `/{id}` route not shadowed |
| `POST /api/memories/search` | 200 | MCP/programmatic path regression-clean |
| `POST /api/manifests/search` | 200 | regression-clean |
| `POST /api/conversations/search` | 200 | regression-clean |

Route-ordering claim from `handler.go` comment ("registered before `/{id}` routes") is accurate and live-verified.

---

## Quality / best-practice notes

- **Handlers are uniform shape.** Parse → empty-guard → delegate → JSON. Easy to audit, easy to extend.
- **No new coupling surface.** Handlers call existing store/node methods; no new package boundary — consistent with manifest decision **A1** (no shared `internal/search/` package in v1).
- **Route-ordering comment** in `handler.go` captures the non-obvious constraint. Future contributors won't accidentally reorder.
- **Test helper `newSearchNode`** cleanly partitions the stores it exercises; the memory/conversation/manifest stores are intentionally left `nil` and the test file documents why. That is disciplined test design, not a skip.

---

## Non-blocking nits

### Nit A — `nil` vs `[]` inconsistency on no-match

For non-empty `q` with zero matches, JSON body differs by endpoint:

- `tasks`, `products`, `ideas`, `actions` → `null` (store returns a nil slice; marshaller emits `null`)
- `manifests` → `[]` (goes through `enrichManifests` which builds a non-nil slice)
- `memories`, `conversations` → `[]` (semantic search layer returns non-nil)

Neither is wrong and frontend JSON decoders handle both, but a single `if results == nil { results = []T{} }` guard — or a generic `nonNil(x)` helper in `handlers_search.go` — would harmonize the contract. Defer to the T4 UI author's preference; flag-only, not a blocker.

### Nit B — GET-alias handlers are not unit-tested

`handlers_search_test.go` explicitly punts on the memories/manifests/conversations GET aliases (comment at lines 17–23) on the reasoning that the POST paths are already covered. That's defensible — the aliases are ~6 lines each and just re-call the same node method — but a one-liner smoke test per alias would close the loop without meaningful cost. Optional; not blocking.

---

## Scope discipline

PR touches exactly the three files the manifest scope (`M2 — Per-tab HTTP endpoint`) calls for: one route-registration edit, one new handler file, one new test file. No drive-by edits to UI, stores, MCP, or unrelated handlers.

---

## Visceral-rule walk

| Rule | Relevant? | Status |
|---|---|---|
| #5 / #11 — new branch per change | yes | ✅ `openpraxis/019dab05-b95` |
| #10 — WAL + busy_timeout | yes (test DSN) | ✅ `_journal_mode=WAL&_busy_timeout=5000` |
| #13 — git worktrees | yes | ✅ operating in worktree |
| #1/#2/#3 — fix root cause, build reusable tools, Python for text | no | — |
| #4 — allow curl | yes (review used curl) | ✅ |
| #6 — no workarounds on-the-fly | yes | ✅ route-order comment documents the real fix, not a hack |
| #7 — use OpenLoom tasks | yes | ✅ review scheduled as OpenLoom task, no Cron |
| #8 — daily budget $100 | N/A | — |
| #9 — operational data persists to SQLite | yes (by extension) | ✅ searches read persisted stores; no in-memory side channel |
| #12 — never auto-fire tasks | yes | ✅ review does not start/cancel other tasks |

All applicable rules clean.

---

## Verdict

**APPROVE.** M2 is complete, the live endpoints are correct, POST compatibility is preserved, test coverage is proportionate, and the PR is scope-clean. Both nits are cosmetic and can be folded into T4 (UI) if the next author cares about stricter JSON uniformity; neither warrants blocking merge.

Closing memory filed at `/project/openpraxis/universal-search/reviews/t3-search-endpoints`.
