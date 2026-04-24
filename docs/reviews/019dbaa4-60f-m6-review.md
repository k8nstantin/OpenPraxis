# RC/M6 Review — Per-agent template overrides (Codex/Cursor markdown frames)

- **Task:** `019dbaa4-60fe-7acd-90b8-545a7ef8ba71` (RC/T6R)
- **Manifest:** `019dba9f-0c8e-773b-be0b-99184aec056a` (RC/M6)
- **PR:** [#221](https://github.com/k8nstantin/OpenPraxis/pull/221) — `openpraxis/019dbaa4-60f-m6`
- **Verdict:** **APPROVE**

## Scope

RC/M6 seeds agent-scope rows for `codex` and `cursor` with markdown-heading
framings of the six prompt sections (plus preamble). The Claude-targeted
XML `<tag>…</tag>` rows at `system` scope are left untouched. Resolver's
existing `agent`-tier walk (shipped in RC/M1) is validated end-to-end.

## Diff

| File | +/- | Notes |
|---|---|---|
| `internal/templates/defaults.go` | +129 / -0 | `AgentDefaults`, `codexDefaults`, `cursorDefaults`, `AgentDefaultTitles`, `SeededAgents`, markdown bodies |
| `internal/templates/seed.go` | +55 / -20 | Refactor to `seedBucket` helper; seeds `system` + each `SeededAgents` entry independently with per-bucket idempotency gate |
| `internal/templates/templates_test.go` | +174 / -9 | `TestSeed_InsertsAgentRows`, `TestSeed_IdempotentPerBucket`, `TestResolver_AgentScope`, `fakeAgentLookup` |

## Acceptance checklist

1. ✅ **14 agent-scope rows after seed.** `TestSeed_InsertsAgentRows` asserts `ScopeAgent` rows = 14 (7 codex + 7 cursor), each section present and non-empty, each body contains a `##` heading and no `<visceral_rules>`/`<manifest_spec>` XML openers. Live `curl /api/templates?scope=agent | jq length` is not exercisable yet (the HTTP list route is explicitly out of scope here — noted in the PR body — and lands with the RC/M2 backend).
2. ✅ **Codex / cursor get markdown bodies.** `TestResolver_AgentScope` resolves `SectionGitWorkflow` for `task-codex` and `task-cursor` through a `fakeAgentLookup` — both contain `## Git Workflow` and neither contains the `<git_workflow>` tag.
3. ✅ **Claude-code falls through to XML.** Same test resolves `task-claude` (agent `claude-code`, no row exists at agent scope) and asserts the body contains `<git_workflow>`.
4. ✅ **Unknown agent falls through.** `AgentDefaults` returns `nil` for anything outside `{"codex","cursor"}`, so no row is seeded. At resolve time, `Resolver.lookup(ScopeAgent, "gemini", …)` returns `sql.ErrNoRows` → folded into `found=false` → falls through to `system`. The claude-code test exercises exactly this path (any name without a seeded row behaves identically).
5. ✅ **All three frames distinct.** Preambles carry an agent self-identifier (`running as the Codex agent` / `running as the Cursor agent` / default has no suffix) so same-section resolves for codex/cursor/claude yield three distinct strings — asserted in `TestResolver_AgentScope`.
6. ✅ **Idempotent re-seed.** `TestSeed_Idempotent` runs `Seed()` twice and asserts totals stay at 7/14/21 (system/agent/total). `TestSeed_IdempotentPerBucket` verifies each agent bucket independently stays at 7.
7. ✅ **`go test ./internal/templates/...`** green (9 tests).
8. ✅ **No regression.** `go test ./...` green across every package (action, comments, idea, manifest, mcp, memory, node, product, settings, task, templates, watcher, web).

## Observations

- **Refactor is tidy.** The `seedBucket` helper cleanly factors the
  "count → tx → insert → commit" pattern; system and both agent tiers go
  through the same path. The `reason` field gets a per-tier override
  (`"Initial seed: codex markdown-heading frame"`) which will be useful
  in the SCD-2 audit trail.
- **Idempotency gate is per `(scope, scope_id)` bucket** — safe to
  re-run after partial populations (e.g. a future Gemini tier can seed
  without disturbing codex/cursor). The doc comment at the top of
  `Seed()` explains the invariant clearly.
- **`AgentDefaults(agent)` returns `nil` for unknowns**, which is the
  correct fail-quiet behaviour for resolver fallthrough. The wrapper
  functions `codexDefaults` / `cursorDefaults` are thin aliases — the
  manifest spec called them out as the "helpers", and they're kept
  as private indirections over `AgentDefaults`, which reads fine.
- **Markdown bodies mirror the XML bodies structurally** — same
  placeholders, same ordering, same `{{.BranchPrefix}}` reference (with
  the pre-existing hardcoded `openpraxis/` prefix inherited from the
  system default — not introduced here, not in scope to fix).
- **No changes to the resolver.** RC/M1 already shipped the
  `task → manifest → product → agent → system` walk; this PR just
  proves the `agent` tier works by populating it.

## Minor nits (not blockers)

- The markdown `## Git Workflow` body still hardcodes `openpraxis/` in
  front of `{{.BranchPrefix}}` — same pre-existing inconsistency with
  the RC/M5 knob intent as the XML default. Worth a follow-up to
  honour `BranchPrefix` standalone across both frames, but it's a
  pre-existing issue, not drift introduced by M6.
- `codexDefaults()` / `cursorDefaults()` are unused within the package
  (callers go through `AgentDefaults` or `SeededAgents`). The manifest
  spec asked for both helpers explicitly, so keeping them is defensible
  as a public-ish readability shim, but they could disappear without
  effect. Non-blocking.

## Verdict

**APPROVE.** Acceptance criteria #2–#7 validated by tests; #1 covered
by the seeded row count and explicitly deferred on the HTTP side to
the RC/M2 backend (as the PR body calls out). No regressions, build
and full suite green, implementation is minimal and consistent with
the rest of the package.
