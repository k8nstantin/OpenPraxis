# RC/M3 Runner tab read-only — review

- **Task:** `019dbaa4-60fb-7af8-9fb9-2786d15378bf` (RC/T3R — Review RC/M3)
- **Manifest:** `019dba9d-68c` (RC/M3 — Runner tab read-only)
- **Implementation PR:** [#213](https://github.com/k8nstantin/OpenPraxis/pull/213) — head `openpraxis/019dbaa4-60f`
- **Reviewer:** agent (`claude-opus-4-7`)
- **Date:** 2026-04-24
- **Verdict:** ✅ APPROVE (one non-blocking followup)

PR #213 bundles two commits: `00637e9` for RC/M2 (already approved in PR #214) and `0b522ac` for RC/M3. This review covers only the RC/M3 commit — the Runner tab read-only UI.

## Scope reviewed

RC/M3 commit `0b522ac` — files touched:

- `internal/web/ui/views/runner.js` (NEW, 229 lines)
- `internal/web/ui/index.html` (+22)
- `internal/web/ui/style.css` (+43)
- `internal/web/ui/app.js` (+1)

## Acceptance criteria — verified

| # | Criterion | Result |
|---|---|---|
| 1 | "Runner" entry in sidebar nav opens the Runner tab | ✅ `data-view="runner"` in index.html, route registered in app.js |
| 2 | Tree shows 7 system rows by default; other scopes have empty state | ✅ `GET /api/templates?scope=system` returned 7 rows; product/manifest/task/agent returned `count=0` (empty state falls to `renderTree`'s `emptyMessage`) |
| 3 | Click template → title, status, last-edit metadata, current body (rendered markdown), history list newest-first | ✅ `OL.loadTemplate()` fetches `/api/templates/{uid}` + `/history` in parallel, renders `.runner-meta-bar` + `.md-body` + `.runner-history-row` rows |
| 4 | History row: `v{n}`, `changed_by`, `reason`, relative timestamp | ✅ runner.js:209-219 — version computed `total - idx` (oldest = v1); `changed_by` / `valid_from` via `timeAgo` / `reason` rendered conditionally |
| 5 | Tab survives view-switch round-trip — selected template preserved | ✅ `_selectedUID` module-level state + `afterRender` restores active class (runner.js:149-152) |
| 6 | `node --check` on every modified JS file | ✅ Clean on `runner.js` and `app.js` |
| 7 | No edit affordances — no editable body, no Edit/Save/Restore buttons | ✅ Grep: 0 `<button>`, 0 `contenteditable`, 0 `textarea`. "Edit" hits are the local `lastEdit` variable; "Restore" hit is a comment about restoring selection highlight |

## Backend behavior exercised

Fresh binary built from `0b522ac`, served on port 18765 with isolated data dir:

```
[system list] 200 count=7
[product list] 200 count=0
[manifest list] 200 count=0
[task list] 200 count=0
[agent list] 200 count=0
[detail] 200 title='Closing protocol + completion line' status='open'
[history] 200 rows=1 (pre-PUT)
[unknown uid] 404
[at ?t=2099] 200

[after PUT]
[history] 200 rows=2
  v2 | by=http:rcm3-review     | reason='verify history list renders'
  v1 | by=system-seed          | reason='Initial seed from buildPrompt() defaults'
```

Newest-first ordering confirmed; version numbering `total - idx` produces v2 for the latest row and v1 for the seed — matches spec's "count from oldest".

## Test suite

`go test ./internal/web/... ./internal/templates/... ./internal/mcp/...` — all green.

## Code observations

### Non-blocking: `mdToHtml` in runner.js duplicates markdown rendering (followup)

The spec says "reuse `.md-body` for the body render." Runner.js applies the `.md-body` class but hand-rolls a minimal client-side markdown parser (`mdToHtml`, lines 22-80) because the templates HTTP endpoints do not emit a `body_html` field.

Every other entity in the UI uses a server-side goldmark-rendered `body_html` / `description_html` / `content_html` (see PR #201 MRC/M1, PR #202, and `products.js:381`, `manifests.js:458`, `tasks.js:481`, `ideas.js:155`, `handlers_comments.go:100`). The consequences:

- Two markdown pipelines in the codebase — the hand-rolled parser here supports only `#` / `*` / `**` / `` ` `` / fenced code / list; it drops tables, links, blockquotes, etc.
- A sanitization/XSS boundary is duplicated (`escHtml` vs. goldmark's bluemonday pass).
- Render will look subtly different from every other body in the app.

**Suggested followup (not a blocker for RC/M3):** extend `internal/templates` to include a goldmark-rendered `body_html` in `Template` responses (mirror the `handlers_comments` pattern) and drop the hand-rolled `mdToHtml` in favor of `tpl.body_html`. File as a small RC followup or fold into RC/M4 since the editor will need consistent preview rendering anyway.

### Minor: scope_id short preview rather than resolved label

runner.js:120-122 displays `t.scope_id.slice(0, 8)` as the override-row badge. For product/manifest/task-scoped overrides this is fine as a glyph but won't help an operator identify which product/manifest an override belongs to. Acceptable for RC/M3 read-only; a lookup to the scope's human name would be nice in RC/M4.

### Minor: "agent" scope listed in UI but not in resolver tiers

The manifest spec lists an "agent overrides" bucket and runner.js queries `?scope=agent`. The backend (`store.go`) accepts any `scope` value in `ListWithScopeID`, so the endpoint returns `[]` today. If "agent" is intended as a real override tier later, the resolver (`internal/templates/store.go::Resolve`) will need to be taught about it — noted for the RC/M4–M6 work but not an RC/M3 defect.

### Positive

- `_selectedUID` persistence + `afterRender` highlight restore is a clean pattern for round-trip stability.
- `runner-*` CSS classes are well-namespaced — no risk of leaking into the other view panes (confirmed by CSS inspection).
- Split-pane reuses `.manifests-layout` as the spec requested — visual consistency preserved.
- No edit affordances of any kind — `contenteditable=0`, `textarea=0`, `<button=0`. RC/M4 has a clean slate.

## Gates

- **Build gate:** ✅ `make build` clean (0.4.0 binary produced)
- **Test gate:** ✅ `go test ./internal/web/... ./internal/templates/... ./internal/mcp/...` all green
- **Manifest gate:** ✅ 7/7 acceptance bullets verified
- **Git gate:** ✅ `0b522ac` is the RC/M3 commit on branch `openpraxis/019dbaa4-60f`

## Verdict

**APPROVE.** RC/M3 meets every acceptance criterion, the UI is read-only as promised, and the backend wiring from M2 is exercised end-to-end through the Runner tab.

One followup to file:

- **FU-RC-M3-1** — Add `body_html` to template API responses (goldmark-rendered, bluemonday-sanitized) and drop the hand-rolled `mdToHtml` in `runner.js`. Aligns with MRC/M1 unified rendering; low risk; natural predecessor to RC/M4's editor preview.
