# Peer Review — Task 019dab05-7fb (PR #126)

**Reviewer:** task `019dab05-974` (M1/T2-Review)
**Under review:** PR [#126](https://github.com/k8nstantin/OpenPraxis/pull/126) — `feat(search): M1/T2 — Search(q, limit) for task/product/idea/action stores`
**Manifest:** `019daafb-b5e` — Per-Tab Search, M1
**Verdict:** **APPROVE**

## Scope verified

Four new `Search(query, limit)` methods plus unit tests:

| Store | search.go | search_test.go |
|---|---|---|
| `internal/task` | 28 L | 80 L |
| `internal/product` | 37 L | 115 L |
| `internal/idea` | 36 L | 101 L |
| `internal/action` | 27 L | 121 L |

No other files touched; HTTP wiring correctly deferred to T3.

## Contract conformance (manifest M1)

- [x] Single query string input, default `limit=50` when `<=0`.
- [x] `strings.TrimSpace` + empty-check returns `nil, nil` — prevents a bare `%%` from matching every row (the exact trap PR #121 warned about).
- [x] Single LIKE OR-set over `id` + content columns — mirrors manifest.Store.Search shape (PR #121) per manifest architectural constraint "pattern-match the smaller ladder, don't duplicate memory's semantic path."
- [x] Keyword columns correct per type:
  - task: title/description
  - product: title/description/tags
  - idea: title/description/tags
  - action: tool_name/tool_input/tool_response
- [x] `actions.id` is INTEGER PK — handled via `CAST(id AS TEXT) LIKE ?` (annotated in doc comment).
- [x] `deleted_at = ''` filter on task/product/idea; actions has no `deleted_at` column (verified against `internal/action/store.go:47`), so absence is correct.
- [x] Status ordering preserved on product/idea (draft → open → closed → archive → other); task orders by `updated_at DESC`.

## WAL / busy_timeout (visceral rule #10)

Each new `search_test.go` helper opens its test DB with `?_journal_mode=WAL&_busy_timeout=5000` — matches the existing repo convention (`openRepoTestStore`, `openDepsTestStore`, `migrate_test.go`).

- `internal/task/search_test.go` — uses existing `openRepoTestStore`.
- `internal/product/search_test.go:14` — new `openSearchTestStore`, WAL ✅.
- `internal/idea/search_test.go:13` — new `openSearchTestStore`, WAL ✅.
- `internal/action/search_test.go:14` — new `openSearchTestStore`, WAL ✅.

## Local test execution

Branch: `openpraxis/019dab05-7fb-store-search` @ `a65fae9b` (cloned from GitHub).

```
$ go build ./...
(cgo deprecation warnings only — unchanged baseline)

$ go test ./internal/task/ ./internal/product/ ./internal/idea/ ./internal/action/ -count=1
ok  	github.com/k8nstantin/OpenPraxis/internal/task     2.017s
ok  	github.com/k8nstantin/OpenPraxis/internal/product  0.667s
ok  	github.com/k8nstantin/OpenPraxis/internal/idea     0.890s
ok  	github.com/k8nstantin/OpenPraxis/internal/action   1.085s
```

CI: `build + vet + test (linux/amd64)` — **pass** (run 24676422582).

## Test coverage per store

Each package ships five tests: `TestSearch_Keyword`, `TestSearch_IDExact`, `TestSearch_IDPrefix`, `TestSearch_Unknown`, `TestSearch_EmptyQuery`. This matches manifest acceptance criterion #7 ("id-exact, id-prefix, keyword, unknown") plus the empty-query guard.

## Minor observations (non-blocking)

1. **Action id-prefix test is weak.** `TestSearch_IDPrefix` inserts 5 rows and searches for `"1"`, asserting `len(res) > 0`. Since INTEGER PKs start at 1, this always matches `id=1`; it does not actually exercise multi-digit prefix semantics (e.g., id=12 matching `"1"` as prefix vs substring). The current `CAST LIKE '%1%'` is substring, not prefix, and the test asserts substring behaviour is non-empty — which is fine for the current contract, but the test name is slightly aspirational.

2. **No cross-store pagination contract yet.** Each store honours `limit` but there's no ORDER BY consistency across types (task uses `updated_at DESC`; product/idea use status-bucket + `updated_at DESC`; action uses `created_at DESC`). That's fine for M1 — M4 jump-by-id will drive a stable ordering if it matters — but worth noting before T3 wires the HTTP layer.

3. **Id-substring is intentional, not a bug.** `id LIKE '%q%'` (rather than `id LIKE 'q%'`) matches the manifest's "id-exact, id-prefix, id-substring" ladder language. Keep as-is.

None of these block the merge.

## Gates

- Git gate: 1 commit (`a65fae9b`) on branch, CI green.
- Build gate: `go build ./...` clean on reviewer's machine (Apple clang cgo warnings are pre-existing baseline).
- Manifest gate: M1/T2 contract satisfied; no scope creep into T3/T4.

## Recommendation

**Approve and merge.** T3 (HTTP endpoints) can proceed against this branch's surface.
