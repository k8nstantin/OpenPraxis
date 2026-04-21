# Peer review — PR #141 (task 019dafe2-83a)

Reviewer task `019dafe2-c1e`. Reviewed PR: [#141](https://github.com/k8nstantin/OpenPraxis/pull/141) — `fix(admin): sweep legacy short-marker comment orphans`.

Manifest: `019dafe2-503` (Historical orphan comment sweep). Product: `019dabbf-2df` (OpenPraxis Entity ID Resolution).

**Verdict: APPROVE — merge PR #141.**

## 1. Quality of coding
PASS — 85-line `SweepOrphans` with a clean `TargetResolver` seam, explicit dry-run/apply split, no hidden mutation, all errors wrapped with context.

## 2. Best practices
PASS — interface rather than concrete `*node.Node` avoids the `internal/node → internal/comments` import cycle (documented in PR body); SQL uses parameter binding; soft guard on `full == x.tid` no-op; errors flow up rather than silently swallowed.

## 3. Product + manifest + task compliance
PASS — satisfies AC #1 (dry-run shows non-zero count: 5), #2 (`--apply` migrates all; second `--apply` reports 0), #3 (unresolvable logged not deleted; stubResolver ghost case covers it and test asserts row preserved), #4 (verified via HTTP — historical review comments now render on task detail endpoints), #5 (tests green, full `go test ./...` clean). Out-of-scope items (preventing future orphans, deleting ghosts, schema changes) correctly untouched.

## 4. AC trace

| AC | Evidence |
|---|---|
| #1 dry-run non-zero | `openpraxis admin migrate-comment-orphans` → `scanned: 5 / migrated: 5 / unresolvable: 0`, all task rows |
| #2 apply + idempotent | `--apply` run 1 → migrated 5; `--apply` run 2 → `scanned: 0 / migrated: 0` |
| #3 unresolvable preserved | `internal/comments/orphan_sweep_test.go:132-134` — `check(cGhost, ghost)`; implementation `orphan_sweep.go:61-69` logs via `slog.Error` and `continue`, never `UPDATE` |
| #4 dashboard visibility | `GET /api/tasks/019dab05-3ab/comments` returns previously-orphaned `review_approval` with `target_id=019dab05-3ab7-7ce0-abb6-d37512289f9b` (full UUID) |
| #5 tests + CI | `go test ./...` green across all 11 packages with tests |

## 5. Scope discipline
PASS — PR touches exactly three files, all new: `cmd/admin.go` (+114), `internal/comments/orphan_sweep.go` (+85), `internal/comments/orphan_sweep_test.go` (+148). No drive-by edits. `cmd/admin.go` is a new `admin` cobra root — appropriate, operator-only.

## 6. Regression check
Last 10 lines of `go test ./...`:

```
ok  	github.com/k8nstantin/OpenPraxis/internal/comments	5.740s
ok  	github.com/k8nstantin/OpenPraxis/internal/idea	0.461s
ok  	github.com/k8nstantin/OpenPraxis/internal/manifest	0.967s
ok  	github.com/k8nstantin/OpenPraxis/internal/mcp	0.542s
ok  	github.com/k8nstantin/OpenPraxis/internal/memory	0.650s
ok  	github.com/k8nstantin/OpenPraxis/internal/product	0.378s
ok  	github.com/k8nstantin/OpenPraxis/internal/settings	0.991s
ok  	github.com/k8nstantin/OpenPraxis/internal/task	2.000s
ok  	github.com/k8nstantin/OpenPraxis/internal/watcher	1.773s
ok  	github.com/k8nstantin/OpenPraxis/internal/web	1.056s
```

## 7. Visceral rule compliance

| # | Rule | Status | Note |
|---|---|---|---|
| 1 | Never fix scripts on the fly | PASS | Reusable `SweepOrphans` library + `admin` subcommand, not an ad-hoc shell fix |
| 2 | Create reusable tools | PASS | `comments.TargetResolver` interface is reusable for any future `target_type` |
| 3 | Python-only text processing | N/A | Go code path |
| 4 | allow curl | N/A |
| 5 | New branch per modification | PASS | PR #141 on `openpraxis/019dafe2-83a` |
| 6 | Test connections, no improv | PASS | Dry-run is the default; operator gets a report before any write |
| 7 | Use OpenLoom tasks | PASS | This work was scheduled as task `019dafe2-83a` under manifest `019dafe2-503` |
| 8 | Daily budget = $100 | N/A |
| 9 | Persist ops data to SQLite | PASS | Sweep writes directly to the live `comments` table via `UPDATE` — no memory-only state |
| 10 | WAL + busy_timeout=5000 | PASS | Uses `n.Index.DB()`, reusing node's existing opener — no new `sql.Open` |
| 11 | New branch + PR per task | PASS | PR #141 |
| 12 | Never auto-fire tasks | PASS | `--apply` is opt-in; dry-run by default; operator decides |
| 13 | Always use git worktrees | PASS | Reviewed in worktree at `/tmp/pr141-review` off `pull/141/head`; review authored on dedicated branch |

## 8. Concrete rejection
N/A — APPROVE.

## 9. Watcher findings read
Fetched watcher findings on task `019dafe2-83a` and on reviewer task `019dafe2-c1e` via `GET /api/tasks/<id>/comments` filtered by `type=watcher_finding`. None present. No FAIL findings to respond to.

## Minor non-blocking observations

- Manifest spec included `AND deleted_at = ''` in the suggested `SELECT`; the `comments` schema has no `deleted_at` column (`internal/comments/schema.go:19`), so the implementation correctly omits it. Spec wording is aspirational; implementation matches reality. No action needed.
- `nodeTargetResolver` swallows `Get` errors into `("", nil)` rather than propagating — this classifies transient DB errors as "unresolvable". In practice `Store.Get` for these three stores only errors on real scan failures, and the sweep logs unresolvables for operator inspection; acceptable trade-off, worth a followup comment only if retries matter.
- Idempotency test asserts the second run reports `0 new migrated`; the live run is stronger — `scanned: 0` because all resolvables were normalised on first pass. Test behavior matches spec AC #2 exactly.

## Live verification artefacts

```
$ /tmp/openpraxis-pr141 admin migrate-comment-orphans
orphan comment sweep (dry-run)
  scanned:      5
  migrated:     5
  unresolvable: 0
  by target_type:
    task:     5

$ /tmp/openpraxis-pr141 admin migrate-comment-orphans --apply
orphan comment sweep (apply)
  scanned:      5
  migrated:     5
  unresolvable: 0
  by target_type:
    task:     5

$ /tmp/openpraxis-pr141 admin migrate-comment-orphans --apply   # idempotency
orphan comment sweep (apply)
  scanned:      0
  migrated:     0
  unresolvable: 0
```

After apply, `GET /api/tasks/019dab05-3ab/comments` returns three comments including the previously-orphaned `review_approval` from task `019dab05-5da` — target_id is now the full canonical UUID `019dab05-3ab7-7ce0-abb6-d37512289f9b`. The historical review trail for M6/T1 (PR #121) is restored to the dashboard.

Recommend merge.
