# Peer review — task 019db012-97d (edit parity: task_update MCP + manifest UI click-to-edit + task title dashboard edit)

**Reviewer:** agent (openpraxis/019db012-da8)
**Target work task:** 019db012-97d
**Manifest:** edit parity — task_update MCP tool + fix manifest UI click-to-edit
**Verdict:** **REJECT — work not performed**

## Blocker

No PR, no remote branch, and no commits exist for task 019db012-97d. The review cannot proceed because there is nothing to review.

Evidence:

- `gh pr list --state all --repo k8nstantin/OpenPraxis` → no PR with head `openpraxis/019db012-97d` and none with title referencing `task_update` / `edit parity`.
- `git ls-remote github refs/heads/*` → no remote branch `openpraxis/019db012-97d`.
- `git log --all --grep="task_update\|edit parity\|click-to-edit"` → no commits.
- Local branch `openpraxis/019db012-97d` exists but has zero commits ahead of `main` (`git log main..openpraxis/019db012-97d` is empty).
- `git log --since=2026-04-20` on all refs shows no M1/M2/M3 work landed under this marker.

Per visceral rule #6 (don't improvise — report the real state) and #1 (never fix on the fly), I am not fabricating a review for work that does not exist.

## Required review checklist

1. **Quality of coding** — **FAIL**. No code to assess.
2. **Best practices** — **FAIL**. No diff to audit.
3. **Product + manifest + task compliance** — **FAIL**. AC1/AC2/AC3 all unverifiable (no `task_update` tool registered upstream of this review task; no manifest click-to-edit fix; no task-title edit affordance).
4. **AC trace**

    | AC | Expected | Status |
    | --- | --- | --- |
    | AC1 `task_update` MCP tool (title/description/both, marker resolution, empty-field reject) | `internal/mcp/tools_task.go` + test | **Missing** — tool not registered; no test file |
    | AC2 manifest detail click-to-edit title/description/content persists across refresh | `internal/web/ui/views/manifests.js` repair | **Missing** — no change landed |
    | AC3 task detail click-to-edit title | `internal/web/ui/views/tasks.js` + style | **Missing** — no change landed |
    | AC4 no regression to product edit UX | manual check post-change | N/A (no change) |
    | AC5 `go test ./...` green | CI | green **on main** (see §6) but does not credit this task |

5. **Scope discipline** — N/A. No PR files to intersect with manifest scope.
6. **Regression check** — last lines of `go test ./...` on `main` (reviewer sanity run):

    ```
    ok  	github.com/k8nstantin/OpenPraxis/internal/manifest	0.814s
    ok  	github.com/k8nstantin/OpenPraxis/internal/mcp	0.569s
    ok  	github.com/k8nstantin/OpenPraxis/internal/memory	0.241s
    ok  	github.com/k8nstantin/OpenPraxis/internal/product	0.685s
    ok  	github.com/k8nstantin/OpenPraxis/internal/settings	0.492s
    ok  	github.com/k8nstantin/OpenPraxis/internal/task	2.055s
    ok  	github.com/k8nstantin/OpenPraxis/internal/watcher	(cached)
    ok  	github.com/k8nstantin/OpenPraxis/internal/web	0.837s
    ```

    Baseline is green; no work-task delta exists to regress it.

7. **Visceral rule compliance**

    | # | Rule | Status |
    | --- | --- | --- |
    | 1 | Fix source, not scripts on the fly | N/A (no changes) |
    | 2 | Reusable tools | N/A |
    | 3 | Python only for text processing | N/A |
    | 4 | Allow curl | N/A |
    | 5 | New branch per modification | **FAIL** — no branch/PR produced for the work |
    | 6 | No workarounds; test connections first | PASS (reviewer honored: reported missing state instead of fabricating) |
    | 7 | Use OpenLoom tasks, not cron | N/A |
    | 8 | Daily budget $100 | N/A |
    | 9 | Persist ops data to SQLite | N/A |
    | 10 | WAL + busy_timeout=5000 on every db.Open | N/A (no new db.Open) |
    | 11 | New branch per task / own PR | **FAIL** — work task produced no PR |
    | 12 | No auto-fire | N/A |
    | 13 | Always use git worktrees | N/A (cannot verify without artifacts) |

8. **Concrete rejection** — rejection cause: absence of artifacts. No commit SHA, no file:line to cite. Rerun task 019db012-97d so the work branch `openpraxis/019db012-97d` carries commits and a PR is filed; this review will then be redone against the PR.
9. **Watcher findings read** — no watcher_finding comments fetched against this task because no PR / target exists. Revisit after the work lands.

## Verdict

**REJECT.** Rerun the work task so a branch + PR exists; re-invoke the review.
