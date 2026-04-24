# RC/M5 Operator Knobs — Peer Review

**Task:** `019dbaa4-60fd-7b75-9e47-9bca102e1126` (RC/T5)
**Manifest:** `019dba9e-8be` — RC/M5 operator knobs
**PR:** [#213](https://github.com/k8nstantin/OpenPraxis/pull/213) (bundles RC/M2+M3+M4+M5; this review covers commit `60007a8` only)
**Reviewer:** peer-review agent, 2026-04-24
**Verdict:** **APPROVE**

## Scope

RC/M5 surfaces 4 hardcoded runner behaviours as catalog knobs:
`scheduler_tick_seconds`, `on_restart_behavior`, `branch_prefix`,
`worktree_base_dir`. Each inherits via the existing
`task → manifest → product → system` resolver chain.

## Acceptance checks

| # | Spec check | Result |
|---|---|---|
| 1 | Catalog returns +4 knobs | ✅ `catalog_test.go` asserts `want = 19` (pre-M5 was 15, not 14 as the spec text said — see *Notes*). Four expected entries present in `catalog.go`. |
| 2 | `scheduler_tick_seconds` takes effect live | ✅ `Scheduler.resolveTick` reads the system-scope row per iteration; `TestScheduler_ResolveTick_ReadsSystemKnob` covers update + below-floor clamp. |
| 3a | `on_restart_behavior=stop` ⇒ orphan failed with diagnostic | ✅ `TestRunner_RecoverInFlight_StopMarksFailed` asserts `status=failed` + `"serve restart"` in `last_output`. |
| 3b | `on_restart_behavior=restart` ⇒ orphan scheduled, re-fires | ✅ `TestRunner_RecoverInFlight_RestartReschedules` asserts `status=scheduled` with non-empty `next_run_at`. |
| 3c | `on_restart_behavior=fail` ⇒ orphan failed, no auto-recovery hint | ✅ `TestRunner_RecoverInFlight_FailStaysFailed` asserts `status=failed` + `"no auto-recovery"` in `last_output`. |
| 4 | `branch_prefix=qa` ⇒ `git checkout -b qa/<marker>` | ✅ `TestBuildPrompt_BranchPrefixOverride` asserts both the `checkout -b` and `push -u origin` lines, plus non-leak of the default. Template `defaults.go` confirms the wire-up: `{{.BranchPrefix}}/{{.Marker}}`. |
| 5 | `worktree_base_dir` honoured (relative + absolute) | ✅ `TestResolveWorkspacePath` covers empty / relative / absolute shapes. `prepareTaskWorkspace` takes `baseDir` as a parameter so the test harness and production share a single code path. |
| 6 | `go test ./internal/settings/... ./internal/task/...` green | ✅ Both packages pass locally. Full `go test ./...` is also green. |

## Manifest spec → code trace

| Spec file target | Wired? |
|---|---|
| `internal/settings/catalog.go` — 4 KnobDefs | ✅ lines 74–77 (post-diff) |
| `internal/settings/catalog_test.go` — count bump | ✅ 15 → 19 |
| `internal/task/scheduler.go` — `Start` reads knob per tick | ✅ `resolveTick` + `time.After(dur)` replace the static ticker |
| `internal/task/runner.go` — new `RecoverInFlight` at serve init | ✅ classifies orphans via `stop` / `restart` / `fail` |
| `internal/task/runner.go` — `buildPromptData` populates `BranchPrefix` | ✅ plus `Marker` field split |
| `internal/task/runner.go` — worktree path honours knob | ✅ `prepareTaskWorkspace(taskID, baseDir)` |
| `cmd/serve.go` — `RecoverInFlight` before scheduler | ✅ runs before `scheduler.Start()`, falls back to `CleanupOrphaned` on error |

## Dashboard render check

`internal/web/ui/components/knobs.js` renders knobs dynamically from
`/api/settings/catalog` — no hardcoded list. Int slider, enum dropdown,
string text widgets are already in the component, so the 4 new knobs
surface in every Execution Controls pane (product, manifest, task)
without UI changes. Not exercised against live serve per the review
instructions (RC/T3R + RC/T4 incident: must not restart serve).

## Resolver-bypass fallback — design note

The `settings.Resolver` walks `task → manifest → product → catalog-default`
and never consults `ScopeSystem` rows (confirmed at
`internal/settings/resolver.go:150`). The PR works around this by reading
the system-scope row directly from the store when it needs the operator's
override:

- `Scheduler.resolveTick` → `readSystemIntKnob` (direct `Store.Get`).
- `Runner.recoverOneOrphan` → calls resolver, then if `Source==ScopeSystem`
  (i.e. resolver fell through to catalog default) it re-reads the system
  row directly.

This is defensible: the resolver-extension alternative would be a wider
change, and the scope of "operator knobs that tune infra" is exactly
where a system-scope override is appropriate. Documented in the commit
message and in-code comments. No concern.

One consequence worth flagging: `scheduler_tick_seconds` can only be
meaningfully set at system scope (the scheduler is a singleton), which
the code enforces by bypassing the resolver for that knob. The other
three (`on_restart_behavior`, `branch_prefix`, `worktree_base_dir`)
still honour product/manifest/task overrides through the normal resolver
path — only the *system-scope fallback* bypasses.

## Findings (non-blocking)

1. **Spec arithmetic drift** — the manifest text says "was 14, +4 from
   this PR = 18", but pre-M5 catalog count was already 15 (the
   `compliance_checks_enabled` flag landed between spec authoring and
   implementation). The `+4` delta is correct; only the absolute numbers
   in the spec are stale. Code has the right count (19). No action
   needed.

2. **`time.After` in scheduler loop** — the ticker replacement uses
   `time.After(dur)` on every iteration instead of a reusable
   `time.Timer`. Each unused channel is GC'd, so at 10s ticks this is
   negligible. Acceptable as-is.

3. **`RecoverInFlight` + `CleanupOrphaned` co-existence** — `cmd/serve.go`
   calls `CleanupOrphaned` only on `RecoverInFlight` error, good. The
   blanket `CleanupOrphaned` that previously ran unconditionally is now
   a defensive fallback, matching the spec.

## Verdict

**APPROVE.** Every acceptance check maps to a passing test or a concrete
code path. The resolver-bypass is the correct local choice given the
existing resolver's shape. No blocking issues; two minor notes above
are informational.

## Follow-ups filed

None.
