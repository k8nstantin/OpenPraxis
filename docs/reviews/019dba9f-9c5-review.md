# Review — RC/M1 backend read substrate (task 019dba9f-9c5, PR #210)

**Reviewer:** RC/T1R (task 019dbaa4-60f9-7265-b2bf-5cbeecc72be4)
**Manifest:** 019dba89-45d — Runner Controls backend read substrate
**PR:** https://github.com/k8nstantin/OpenPraxis/pull/210
**Verdict:** APPROVE

## Summary

Phase 1 of the Runner Controls product lands cleanly. The `prompt_templates`
table, its two indexes, the 7-row system seed, the task→manifest→product→
agent→system resolver, the `text/template` render path, and the two read-only
HTTP endpoints are all present, match the manifest, and produce byte-identical
runtime output relative to the pre-refactor hardcoded `buildPrompt()`.

## Acceptance criteria — per-bullet verification

1. **Schema + indexes** ✓
   `sqlite3 ~/.openpraxis/data/memories.db ".schema prompt_templates"` shows
   all SCD-2 columns (`template_uid`, `valid_from`, `valid_to`, `changed_by`,
   `reason`) plus the standard audit pair. Both indexes present:
   - `idx_prompt_templates_current` — partial on `valid_to = ''`
   - `idx_prompt_templates_history` — on `(template_uid, valid_from DESC)`

2. **Seed 7 rows, idempotent** ✓
   `SELECT COUNT(*) FROM prompt_templates WHERE scope='system' AND valid_to=''` → 7.
   `TestSeed_Idempotent` exercises the re-seed path and asserts count stays at 7.
   Note: the seed gates idempotency with `COUNT(*) WHERE scope='system'` rather
   than the separate marker row the manifest prose suggested. The code comment
   on `Seed()` explicitly calls this out (acceptance #1 requires
   `COUNT(*)==7` on a fresh DB, so a marker row would inflate the count).
   Acceptable deviation.

3. **`GET /api/templates?scope=system`** ✓
   Exercised against a fresh data dir:
   ```
   curl /api/templates?scope=system | jq length  → 7
   ```
   Sections returned: `closing_protocol, git_workflow, instructions,
   manifest_spec, preamble, task, visceral_rules`. All have `scope=system`,
   `scope_id=""`, `valid_to=""`, `deleted_at=""`, `status=open`,
   `changed_by=system-seed`.

4. **`GET /api/templates/{uid}`** ✓
   Valid uid → 200 with the full row including body.
   Unknown uid → 404 (`"template not found"`), via the `sql.ErrNoRows` branch
   in `apiTemplateGet`.

5. **Byte-identical render** ✓
   `TestRunner_BuildPrompt_ByteIdentical` in `internal/task/runner_prompt_snapshot_test.go`
   double-asserts: (a) vs an inlined `legacyBuildPrompt` oracle that is a
   verbatim copy of the pre-refactor writer; (b) vs `testdata/runner_prompt_snapshot.txt`
   so any silent edit to either side is caught. `TestRunner_BuildPrompt_VisceralRulesEmpty`
   additionally covers the skip-when-empty branch (the visceral block must
   disappear entirely, not render as an empty wrapper).

6. **`runner_exec_review_test`** ✓
   `go test ./internal/task/...` green (0 failures, 5.4s).

7. **`go test ./internal/templates/... ./internal/task/...`** ✓
   Both green locally. Templates package tests cover seed row count,
   idempotency, Get/GetByUID, resolver fall-through to system, task-scope
   override winning over system, and the `printf "%q"` round-trip that
   `<task title=%q>` depends on.

8. **`go build ./...`** ✓ Clean (only the unrelated sqlite-vec cgo
   deprecation warnings).

9. **Resolver order** ✓
   `internal/templates/resolver.go:39–92` walks task → manifest → product →
   agent → system in exactly that order, with each tier guarded by a
   non-empty-id / non-nil-adapter check. `sql.ErrNoRows` is folded into
   `found=false` so missing rows fall through cleanly; only other DB errors
   surface. The agent tier is reserved but not wired from `node.go:241`
   (AgentLookup is nil) — deliberate per the manifest (lands in RC/M6).

10. **WAL + busy_timeout=5000** ✓
    `prompt_templates` lives in the shared index DB opened by
    `memory.NewIndex` at `internal/memory/index.go:44`:
    ```go
    sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
    ```
    followed by explicit `PRAGMA journal_mode=WAL` + `PRAGMA busy_timeout=5000`
    on the live connection. `PRAGMA journal_mode` on a fresh file I seeded
    returned `wal`. (busy_timeout is per-connection so the sqlite3 CLI shows
    `0`, which is expected — not a regression.)

## Code review notes

- **Package boundary is clean.** `internal/templates` imports nothing from
  `internal/task`, `internal/manifest`, or `internal/product`. The view
  structs (`TaskView`, `ManifestView`, `ProductView`) + the `ScopeLookup` and
  `AgentLookup` interfaces let the node layer wire the adapters without
  creating a cycle. Good call — this keeps RC/M2..M6 layered on top without
  refactors.

- **Fallback to package defaults is belt-and-suspenders.** `buildPrompt`
  calls `templates.Render(defaults[section], data)` when the resolver returns
  `""`, which means even if the seed were dropped or the resolver failed
  quietly the runner would still produce legacy output. Deliberate and
  appropriate for a substrate-only phase.

- **Idempotency approach is sound.** Using `COUNT(*) WHERE scope='system'`
  as the gate means a seed re-run after an operator manually deletes a seed
  row would re-insert only if *all* system rows are gone — not per-section
  top-up. That matches the "seed only on first boot" semantics the manifest
  asks for and avoids the edge case where a deliberately-closed system row
  gets resurrected.

## Minor observations (not blockers)

- **Scope lookup runs per section.** `Resolver.Resolve` calls
  `r.scope.ManifestAndProductForTask(ctx, taskID)` on every invocation, and
  `buildPrompt` invokes Resolve 7× per task spawn. That's 7 extra `task.Get` +
  up to 7 `manifest.Get` per spawn. Both hit cached stores so it is not a
  real hotspot, but a follow-up could hoist the scope resolution once per
  `buildPrompt` call and pass `(manifestID, productID)` to each `lookup`.
  Not worth a fix right now; flagging for RC/M2 or later.

- **`Seed` takes `*Store` but reaches straight into `store.DB()`** for the
  bootstrap insert. Acceptable one-off; tightening the API surface is a
  candidate for RC/M2 when the transactional write path lands anyway.

- **`Sections` is exported as a mutable package-level slice.** A future
  caller could `append` to it. Low risk — flagging for the eventual RC/M2
  reshape.

## Gates self-check

- git gate — PR #210 exists with a meaningful commit message; review branch
  for this doc is `openpraxis/019dbaa4-60f`, split from `main`.
- build gate — `go build ./...` clean.
- manifest gate — every one of the 7 acceptance bullets verified above; no
  scope creep into RC/M2 write path.

## Followups to file

None that block the merge. Deferred polish:
1. Hoist scope resolution out of `Resolver.Resolve` once RC/M2 lands and the
   call frequency grows.
2. Consider an unexported `sections` + an accessor, or a slice copy in
   `SystemDefaults`, to harden the API.

## Verdict

**APPROVE.** The substrate is correct, well-tested, and runtime-identical to
the code it replaces. RC/M2 can layer the transactional write path on this
foundation without needing to revisit anything here.
