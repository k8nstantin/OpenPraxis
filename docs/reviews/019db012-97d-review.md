# Peer review — task 019db012-97d (edit parity: task_update MCP + manifest UI click-to-edit + task title dashboard edit)

**Reviewer:** agent (openpraxis/019db012-da8)
**Target work task:** 019db012-97d
**Manifest:** edit parity — task_update MCP tool + fix manifest UI click-to-edit
**PR under review:** [#144](https://github.com/k8nstantin/OpenPraxis/pull/144) — head `openpraxis/019db012-97d` @ `50aa7c4`
**Verdict:** **APPROVE**

> v1 of this review (earlier commit on `openpraxis/019db012-da8`) rejected because the work task run produced no artifacts. The work task was rerun and PR #144 was filed; this v2 supersedes the prior REJECT.

## Required review checklist

1. **Quality of coding — PASS.** Handler uses map-presence (`a["title"]`) to distinguish "not sent" from "empty string", matching the `*string` signature of `task.Store.Update` (`internal/task/repository.go:217`); UI save paths now dedupe blur+Enter double-fires via a `committed` guard and surface HTTP failures via `console.error` instead of silently reloading stale state.
2. **Best practices — PASS.** One commit, one PR, scope-matched diff, test file ships with the new handler, marker resolution reused from `Tasks.Get`, no new db.Open, no schema change.
3. **Product + manifest + task compliance — PASS.** AC1 (M1), AC2 (M2), AC3 (M3), AC4 (no product edit regression — product path not touched), AC5 (`go test ./...` green) all verified.
4. **AC trace**

    | AC | Expected | Evidence |
    | --- | --- | --- |
    | AC1 `task_update` MCP (title / description / both / marker resolution / empty reject / not-found) | `internal/mcp/tools_task.go` + test | `internal/mcp/tools_task.go:55-62` (registration), `:356-398` (handler); tests `internal/mcp/tools_task_test.go` 7 cases: `TestTaskUpdate_TitleOnly`, `_DescriptionOnly`, `_BothFields`, `_MarkerResolved`, `_RejectsNoFields`, `_NonExistent`, `_MissingID` — all pass in `go test ./internal/mcp` |
    | AC2 manifest click-to-edit title/description/content persists across refresh | `internal/web/ui/views/manifests.js` | Title: `manifests.js:602-645`; description: `:648-685`; content save: `:734-753`. All three paths now (a) guard with `committed` flag against blur-after-save double-PUT, (b) log non-2xx response bodies, (c) use an explicit `cancel` branch for Escape. PUT body shape matches `apiManifestUpdate` at `internal/web/handlers_manifest.go:124`. |
    | AC3 task detail click-to-edit title | `internal/web/ui/views/tasks.js` + style | Span added at `tasks.js:354` (`#task-edit-title.task-editable`); handler `:563-606` mirrors manifest pattern; PATCH `/api/tasks/:id` at `internal/web/handlers_task.go` (existing endpoint, line ~220) unchanged. Instructions/description edit path untouched (no regression). |
    | AC4 no product edit regression | product path untouched | `gh pr view 144 --json files` shows no product files touched; product edit views unchanged. |
    | AC5 `go test ./...` green | CI | see §6 |

5. **Scope discipline — PASS.** PR touches exactly 5 files, all within manifest scope:
    - `internal/mcp/tools_task.go` (M1)
    - `internal/mcp/tools_task_test.go` (M1 tests)
    - `internal/web/ui/views/manifests.js` (M2)
    - `internal/web/ui/views/tasks.js` (M3)
    - `internal/web/ui/style.css` (+1 line for `.task-editable` hover — supporting M3)

    No drive-bys.

6. **Regression check** — last 10 lines of `go test ./...` on pr-144:

    ```
    ok  	github.com/k8nstantin/OpenPraxis/internal/action	0.477s
    ok  	github.com/k8nstantin/OpenPraxis/internal/comments	5.693s
    ok  	github.com/k8nstantin/OpenPraxis/internal/idea	0.804s
    ok  	github.com/k8nstantin/OpenPraxis/internal/manifest	0.912s
    ok  	github.com/k8nstantin/OpenPraxis/internal/mcp	0.704s
    ok  	github.com/k8nstantin/OpenPraxis/internal/memory	1.128s
    ok  	github.com/k8nstantin/OpenPraxis/internal/product	0.812s
    ok  	github.com/k8nstantin/OpenPraxis/internal/settings	1.150s
    ok  	github.com/k8nstantin/OpenPraxis/internal/task	2.300s
    ok  	github.com/k8nstantin/OpenPraxis/internal/watcher	2.313s
    ok  	github.com/k8nstantin/OpenPraxis/internal/web	1.499s
    ```

7. **Visceral rule compliance**

    | # | Rule | Status |
    | --- | --- | --- |
    | 1 | Fix source, not scripts | PASS |
    | 2 | Reusable tools | PASS — handler reuses `Tasks.Get` + `Tasks.Update`, UI commit helper mirrors manifest pattern |
    | 3 | Python only for text processing | N/A |
    | 4 | Allow curl | N/A |
    | 5 | New branch per modification | PASS — branch `openpraxis/019db012-97d` carries the single commit `50aa7c4` |
    | 6 | No workarounds | PASS |
    | 7 | OpenLoom tasks for scheduling | N/A |
    | 8 | Daily budget | N/A |
    | 9 | Persist ops data to SQLite | N/A |
    | 10 | WAL + busy_timeout=5000 on every db.Open | PASS — no new `db.Open` call introduced |
    | 11 | New branch per task / own PR | PASS — PR #144 |
    | 12 | No auto-fire | PASS — no task_start / scheduler hooks touched |
    | 13 | Always use git worktrees | PASS (work task ran in its own worktree `.openpraxis-work/019db012-97d1-…`) |

8. **Concrete rejection** — N/A (approve).
9. **Watcher findings read** — target task 019db012-97d has one `watcher_finding` comment from the prior (artifact-less) run: "Manifest gate failed — Deliverables missing: internal/mcp/tools_task.go, internal/task/repository.go:217, internal/mcp/tools_task_test.go, internal/web/ui/views/manifests.js, internal/web/ui/views/tasks.js". That finding no longer applies to PR #144: all listed paths (except `internal/task/repository.go:217`, which is an existing referenced call site, not a deliverable to modify) are present in the diff. No open watcher FAIL findings block approval.

## Verdict

**APPROVE.** Ship PR #144.
