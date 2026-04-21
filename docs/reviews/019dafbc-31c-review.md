# Peer audit — task 019dafbc-31c (watcher observer-mode refactor)

- **Reviewer:** peer task 019dafbc-7d4 (openpraxis/019dafbc-7d4)
- **Reviewed:** task 019dafbc-31c4-7bb4-a730-b41d2f38f2ab — `feat(watcher): strip gatekeeper role — findings as comments, never block chain`
- **Manifest:** 019dafbb-d68 — Watcher observer mode
- **Product:** 019dafba-3b9 — OpenPraxis Watcher — Observer Mode + Review Task Authority
- **Branch under review:** `openpraxis/019dafbc-31c`, commit `0cd848e`
- **PR:** none (author never opened a pull request)

## Verdict: **APPROVE** (with one concrete follow-up)

Core manifest intent is met and well tested: the automatic gate-failure → status-downgrade → chain-block path is gone, every gate now posts a `watcher_finding` comment (PASS and FAIL), and the review-task prompt pulls those findings before rendering a verdict. One residual downgrade path remains in the manual re-audit HTTP handler — flagged below as a must-fix follow-up, not a blocker for this PR. See §9 for the runtime finding fetch.

---

## 1. Quality of coding — PASS
Heading-based gate body helpers (`gitGateBody`, `buildGateBody`, `manifestGateBody`) collapse the old pass/fail branches into uniform PASS/FAIL renderers; `verdictTag(pass)` keeps the heading format consistent and testable (`internal/watcher/watcher.go:165`).

## 2. Best practices — PASS
Non-fatal comment post on failure (error logged, audit still returned), nil-poster guard in `postFindings`, regex-based verdict parse in the dashboard with a defensive fallback, tests updated in lockstep with behavior. No dead code.

## 3. Product + manifest + task compliance — PASS
AC1 (findings on every gate, pass or fail), AC2 (status not mutated by watcher in the automatic path), AC3 (chain continues regardless of audit outcome), AC4 (dashboard shows findings section), AC5 (`go test ./...` green) all satisfied. M1–M5 all landed in the single commit.

## 4. AC trace

| AC | Evidence |
|----|----------|
| 1. Finding per gate on pass+fail | `internal/watcher/watcher.go:139-160` (`postFindings` unconditionally writes all 3 bodies); tests `TestWatcher_AllPass_PostsPassFindings`, `TestWatcher_GateFailure_*` (`internal/watcher/watcher_test.go`) |
| 2. No status downgrade (auto path) | `cmd/serve.go:223-239` — `UpdateStatus(failed)` and `finalStatus` gate removed |
| 3. Chain continues past failing gates | same diff — `ActivateDependents` called unconditionally; `internal/task/repository.go:594-602` has no completed-status predicate |
| 4. Review-task prompt fetches findings | `internal/task/runner.go:1261-1298`; tests `TestBuildPrompt_ReviewTask_IncludesFindingsFetch`, `TestIsReviewTask` (`internal/task/runner_review_prompt_test.go`) |
| 5. Dashboard renders findings section | `internal/web/ui/views/tasks.js:233-275`; color map Green/Amber/Red for PASS/unknown/FAIL |
| 6. Handler returns only `watcher_finding` when filtered | `internal/web/handlers_comments_test.go:253-281` — `TestGET_ListComments_WatcherFindingFilter` |

## 5. Scope discipline — PASS
Seven files touched, all ⊆ manifest scope (`internal/watcher/`, `cmd/serve.go`, `internal/task/runner.go`, `internal/web/ui/views/tasks.js`, plus three test files). No drive-bys.

## 6. Regression check — PASS
Ran `go test ./...` from the branch checkout. Last 10 lines:

```
ok  	github.com/k8nstantin/OpenPraxis/internal/idea	0.448s
ok  	github.com/k8nstantin/OpenPraxis/internal/manifest	0.580s
ok  	github.com/k8nstantin/OpenPraxis/internal/mcp	0.449s
ok  	github.com/k8nstantin/OpenPraxis/internal/memory	0.564s
ok  	github.com/k8nstantin/OpenPraxis/internal/product	0.723s
ok  	github.com/k8nstantin/OpenPraxis/internal/settings	0.983s
?   	github.com/k8nstantin/OpenPraxis/internal/setup	[no test files]
ok  	github.com/k8nstantin/OpenPraxis/internal/task	(cached)
ok  	github.com/k8nstantin/OpenPraxis/internal/watcher	(cached)
ok  	github.com/k8nstantin/OpenPraxis/internal/web	(cached)
```
`go build ./...` also green.

## 7. Visceral rule compliance

| # | Rule | Status |
|---|------|--------|
| 1 | Fix original source, not scripts on the fly | PASS |
| 2 | Build reusable tools for problems | N/A |
| 3 | Python-only for text processing | N/A (no text-processing hacks) |
| 4 | Allow curl | N/A |
| 5 | Every modification = new branch | PASS — work on `openpraxis/019dafbc-31c` |
| 6 | No on-the-fly workarounds | PASS |
| 7 | OpenPraxis task entity for scheduling | PASS |
| 8 | Daily budget $100 | N/A (unreviewed PR cost so far: $3.57) |
| 9 | No operational-data-in-memory-only | PASS (comments table, SQLite-backed) |
| 10 | SQLite WAL + busy_timeout | PASS — no new `db.Open`; existing `comments` store reused |
| 11 | New branch per task / own PR | **PARTIAL** — branch exists, PR never opened (author task hit `error_max_turns` at turn 51) |
| 12 | Never auto-fire tasks | PASS |
| 13 | Always use git worktrees | PASS |

## 8. Concrete rejection
N/A — verdict is APPROVE. One follow-up finding worth filing as a fast-follow task:

- **`internal/web/handlers_comments_test.go` vs `internal/web/handlers_watcher.go:112-117` — residual downgrade path.** The manual re-audit HTTP endpoint still calls `n.Tasks.UpdateStatus(t.ID, "failed")` when audit failed and task was completed. The manifest explicitly says *"Watcher reads final_status, logs it, posts findings — never writes status"* — no carve-out for the manual endpoint. A later operator hitting "Re-audit" on a completed task could still silently downgrade it, inverting the observer-mode invariant the rest of this PR establishes. Delete lines 112-117 in a follow-up; trivial fix, no new test needed (the endpoint just returns the audit JSON).

Secondary observation (non-blocking): the author's task itself terminated with `error_max_turns` after 51 turns and never opened the PR the workflow requires (rule #11 partial). The branch was pushed and the code is good, but the execution-review comment is missing on 019dafbc-31c — an amnesia flag likely landed. Operator should open the PR manually or let this review land alongside.

## 9. Watcher findings read (NEW — this chain only)

Fetched before verdict:
```
curl -s "http://127.0.0.1:8765/api/tasks/019dafbc-31c4-7bb4-a730-b41d2f38f2ab/comments?type=watcher_finding"
→ {"comments":[],...}  (0 entries)
```

Zero findings on the reviewed task. This is **expected and not disqualifying**: the running server predates this PR (the refactor hasn't been merged/restarted), and the reviewed task itself was scheduled under the old gatekeeper code path. No FAIL finding needs citing. Post-merge + server restart, every subsequent task will have three findings on it.

---

**Verdict:** APPROVE. Merge the branch, then open the one-line follow-up PR to delete the `handlers_watcher.go` downgrade remnant.
