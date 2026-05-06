# OpenPraxis Code Review — Scheduler / Settings / Entity / Relationships / Execution

Task: 019dfd04-5845-7d7d-a498-e6930e8c9171 (OpenPraxisCodeReview)
Manifest: 019dfd38-9b38-74f1-99a5-dab8ffa68a66 (Code Review — OpenPraxis codebase)
Reviewer: claude-opus-4-7 (agent)

The full review findings are posted as a `description_revision` comment on
product `019dfd3d-a61e-7182-ae48-bea201ba1192` (OpenPraxisCodeImprovements)
so the next agent in the improvements run consumes them as its instruction
set. This file is the on-branch mirror so the PR has reviewable content.

## Scope

Focus areas defined by the manifest:

- Scheduler and DAG dispatcher — `cmd/serve.go`, `internal/schedule/`
- Settings scope resolution — `internal/node/node.go`, `internal/settings/`
- Entity + relationships stores — `internal/entity/`, `internal/relationships/`
- Execution log — `internal/execution/`

## High-severity findings (summary)

1. **H1** — `internal/execution/store.go:649-689`: `rowColumns` and
   `scanRow` omit `session_id`, `tests_run`, `tests_passed`, `tests_failed`,
   so every read-path silently zeroes those fields even though `Insert`
   persists them.
2. **H2** — `internal/schedule/runner.go:209-232`: `Schedule.Timezone` is
   never threaded into `cron.NewParser`. `cron.New(...)` itself is
   constructed without `cron.WithLocation`, so spec evaluation runs in
   `time.Local` regardless of the row's stored `timezone` column.
3. **H3** — `internal/schedule/migrate.go:57,64,68`: `BackfillFromTasks`
   writes `datetime('now')` (`YYYY-MM-DD HH:MM:SS`) while the rest of the
   codebase writes RFC3339Nano. SCD-2 readers compare lexicographically
   on TEXT, so `'2026-05-06 12:00:00'` sorts wrong against
   `'2026-05-06T11:59:00Z'`.
4. **H4** — `cmd/serve.go:273-284` + `internal/task/runner.go:603-619`:
   the global "one agent at a time" guard and the per-product cap both
   do non-atomic check-then-claim. Two simultaneous fires can race past
   either gate.
5. **H5** — `internal/execution/store.go:163-184`: ALTER-column helpers
   silently swallow every error, not just "duplicate column". Failed
   migrations surface later as runtime "no such column" errors far from
   the migration site.

## Medium / Low / Nit findings

See the comment on product 019dfd3d for the full list (M1–M8, L1–L10,
N1–N6) plus per-issue locations and fix suggestions.

## Top 3 priorities (for the improvements run)

1. **H1** — append the four missing columns to `rowColumns` and the scan
   list. Add a regression test that round-trips a non-zero
   `tests_passed` through `LatestByRun`.
2. **H2 + M6** — make `cron.New(cron.WithLocation(time.UTC), ...)` the
   default and thread `row.Timezone` into `buildSchedule` (e.g., via
   `CRON_TZ=` prefix or per-spec wrapping).
3. **H4 + M1** — promote `Runner.Execute` to take `r.mu` for write
   across `IsRunning` + cap check + register-into-`r.running`. Drop the
   redundant guard in `cmd/serve.go` once the runner-side is atomic.

## What's done well

- Relationship walk: depth clamp + slow-query log + Go-side dedup is a
  clean combination, and the CTE replaces the 6+-join legacy hierarchy
  query as advertised.
- Settings resolver: `NormalizeScope` lets `Resolve` and `ResolveAll`
  share lookup logic; `ResolveAll` issues at most three `ListScope`
  queries regardless of catalog size.
- SCD-2 invariants: the unique partial index `idx_entities_uid_current`
  is a real safety net — concurrent `Update` calls fail fast at INSERT
  instead of producing two "current" rows.
- Schema portability discipline: no CHECK / triggers / SQLite-only
  dialect; Go-side validation with test coverage for every rejection
  path. Worth preserving.
- Doc-comments explain trade-offs (`UNION ALL` vs `UNION`, why a Go
  mutex around Create, why not amend) — future maintainers benefit.
