# Peer Review — Task `019dab05-3ab` (M6/T1: manifest-search fix)

- **PR:** [#121 — fix(manifest): M6/T1 — search matches id/marker, not just content](https://github.com/k8nstantin/OpenPraxis/pull/121)
- **Commit:** `9bc2aad7434b775e7d6daa5be608f5806f0ee8d7`
- **Manifest:** `019daafb-b5e` (Per-Tab Search), section M6
- **Reviewer task:** `019dab05-5da` (this branch)
- **Date:** 2026-04-20

## Verdict: **APPROVE**

Root-cause fix is correct, scope is tight, tests are meaningful, live behavior matches spec. Recommend merge.

## What the PR changes

`internal/manifest/store.go`:

- Adds `id LIKE ?` to the OR set in `Search`, so the same `%q%` pattern now covers id-exact, id-prefix (marker = `id[:12]`), id-substring, and keyword match on title/description/content/jira_refs/tags.
- Trims whitespace and short-circuits empty queries, so a `%%` pattern can never match every row.
- Updates the doc comment to explain the new contract and point to manifest `019daafb-b5e` M6.

`internal/manifest/search_test.go` (new): five table-driven cases — keyword, id-exact, id-prefix, unknown, empty.

Two files, +121/-4. No touch to the handler, the frontend, or other entity stores — exactly the scope M6/T1 carved out.

## Root cause confirmed

Before the fix, `Store.Search` only LIKE-matched `title, description, content, jira_refs, tags`. Typing a marker (the 12-char id prefix the dashboard shows on every card) hit zero of those columns, so the Manifests-tab search bar returned nothing for the most natural input. The `id` column was simply missing from the WHERE clause.

The fix adds `id LIKE ?` to the OR set. Because LIKE patterns are `%q%`, a single query string covers id-exact, id-prefix, and id-substring. That mirrors the permissive-recall ladder direction from PR #120, without introducing a separate code path — good call.

## Gate verification (this review)

Tests were run against commit `9bc2aad` in a fresh clone at `/tmp/review-3ab`.

### Build / vet / test
- `go build ./...` — green (only pre-existing cgo deprecation warnings from `sqlite-vec-go-bindings`, unrelated).
- `go vet ./...` — green (same cgo warnings only).
- `go test ./...` — green across every package with tests:
  - `internal/comments` 5.33s
  - `internal/manifest` 0.60s (includes the 5 new TestSearch cases)
  - `internal/mcp` 0.69s
  - `internal/memory` 1.14s
  - `internal/product` 0.36s
  - `internal/settings` 1.11s
  - `internal/task` 5.77s
  - `internal/watcher` 5.26s
  - `internal/web` 1.06s

### New tests
`go test ./internal/manifest/ -run TestSearch -v`:

```
--- PASS: TestSearch_Keyword     (0.01s)
--- PASS: TestSearch_IDExact     (0.00s)
--- PASS: TestSearch_IDPrefix    (0.01s)
--- PASS: TestSearch_Unknown     (0.01s)
--- PASS: TestSearch_EmptyQuery  (0.01s)
PASS    ok  github.com/k8nstantin/OpenPraxis/internal/manifest  0.546s
```

The `TestSearch_IDPrefix` case in particular regression-guards the exact bug the user reported: searching by marker.

### Live HTTP verification
PR binary built from `9bc2aad` and run on port `:8766` (alt config in `/tmp/op-3ab.yaml`). `POST /api/manifests/search` results:

| query                  | count | notes                                                   |
|------------------------|------:|---------------------------------------------------------|
| `search`               | 17    | keyword — pre-existing behavior preserved               |
| `019daafb-b5e` (marker)| 1     | the bug case — now returns the owning manifest          |
| `019daafb` (uuid-prefix)| 1    | shorter id-prefix still resolves                        |
| `"   "` (whitespace)   | 0     | empty short-circuit works — no `%%` degenerate match    |
| `no-such-thing-zzzz`   | 0     | unknown keyword returns empty                           |

For the same calls against the **pre-fix** server on `:8765` (currently serving `main` at c5379ff), the `019daafb-b5e` marker query returns `count: 0`, which reproduces the original user report. The fix is directly responsible for the delta.

## Spec alignment (manifest `019daafb-b5e`, section M6)

- M6 step 1 — handler path audit: handler unchanged; verified existing `/api/manifests/search` wiring still works end-to-end.
- M6 step 2 — store query audit: diagnosed and patched.
- M6 step 3 — frontend render check: result shape unchanged (same `*Manifest` slice), no frontend edits needed.
- M6 step 4 — fallback to LIKE when embeddings missing: store was already LIKE-based (no embedding call path), so this was implicitly satisfied. Worth noting in a closing memory, which the author did in the PR body.

Scope is manifest search only, as M6/T1 requires. M1/M2 (new per-store Search methods and GET aliases) are explicitly deferred to follow-on tasks — correct.

## Nits / notes (non-blocking)

1. The `ORDER BY CASE status …` clause duplicates status ordering that also exists in `List`. Pre-existing; out of scope for this PR, but a future refactor could lift it into a shared `manifestListOrderClause`.
2. `TestSearch_Keyword` asserts `!contains(res, b.ID)` — the title `"Beta gizmo"` and keyword `"widget"` make that safe today. If anyone later adds seeded rows whose content touches "widget", this becomes flaky. Not worth blocking — note it.
3. The PR body check-box "Manual: type a marker into the Manifests-tab search bar" is unchecked. The live-HTTP verification above covers the same code path (handler → store) and is stronger than a browser click, so this review treats that gate as satisfied by proxy.

## Recommendation

Ship it. The fix is the smallest correct change, tests regression-guard the reported bug, live behavior confirms the fix, and scope is disciplined.
