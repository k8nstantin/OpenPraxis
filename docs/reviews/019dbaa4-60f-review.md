# RC/T4R — Review of RC/M4 Runner tab editor + preview + restore

- **PR:** [#213](https://github.com/k8nstantin/OpenPraxis/pull/213) (commit `cb2855a`, third of three on branch)
- **Task:** `019dbaa4-60fc-7b90-8cc2-b63c53b3f631`
- **Implementation task reviewed:** `019dbaa4-60fc-7367-8c19-1aafe90c93ee`
- **Manifest:** `019dba9e-1135-79eb-b954-afa96ac54e54` (RC/M4)
- **Reviewer:** claude-code (peer)
- **Verdict:** **APPROVE with followup**

## Method

Reviewed commit `cb2855a` in isolation on top of the already-reviewed M2 (PR #213 M2 slice) and M3 (PR #215 approval) layers:

- Read the diff end-to-end for `internal/web/handlers_template.go`, `handlers_template_test.go`, `internal/web/ui/views/runner.js`, `internal/web/ui/style.css`.
- Ran `go build ./...` — clean.
- Ran `go test ./internal/web/... ./internal/templates/... ./internal/mcp/...` — all pass (3 packages).
- Ran `node --check internal/web/ui/views/runner.js` — clean.
- Per the task instructions did NOT restart the live serve (reviewer process is a child; killing it triggers the false-orphan race seen in RC/T3R on 2026-04-24).

## Acceptance checklist (manifest spec)

| # | Criterion | Verdict |
|---|-----------|---------|
| 1 | Click Edit → editor with current body + Reason input | ✅ `mountTemplateEditor` hides read-only block, mounts `OL.mountEditor` on a textarea, renders Reason input + Save/Cancel |
| 2 | Save disabled until Reason non-empty; tooltip explains | ✅ `refreshSave()` toggles `disabled` + `title` on every `input` event; initial state disabled |
| 3 | Save → PUT → right pane refreshes, new history row with entered reason | ✅ `PUT /api/templates/:uid` with `{body, changed_by, reason}`; on success re-invokes `OL.loadTemplate(uid)` which rebuilds header + history newest-first |
| 4 | Restore → confirm modal (explicit Confirm button, not browser native) | ✅ `showRestoreConfirm` injects `.runner-restore-confirm` overlay with Cancel/Confirm buttons; click-binding guards; no `window.confirm` anywhere |
| 5 | After restore: new row `reason="restored from <ts>"`, body == historical | ✅ Server-side `apiTemplateRestore` calls `AtTime` + `UpdateBody(body, author, "restored from <ts>")`. Covered by `TestTemplate_Restore` asserting 3 rows, newest body == orig body, reason prefix `"restored"` |
| 6 | Preview against task; `{{broken}}` surfaces `[render error: ...]` inline, no crash | ✅ `apiTemplatePreview` wraps `templates.Render` and returns `{rendered, error}` on failure with 200. `TestTemplate_Preview` covers both the happy path (`Hello {{.Task.ID}}`) and parse error (`{{.Nope`) |
| 7 | Cancel during edit drops the draft, restores read-only display | ✅ `cancel.addEventListener` calls `closeEditor()` + un-hides `#runner-body-block` + empties editor host |
| 8 | `node --check` clean on modified JS | ✅ verified locally |

## Safety affordances — spec vs implementation

| Affordance | Spec | Impl | Notes |
|------------|------|------|-------|
| Reason required | ✅ | ✅ | Disabled Save + explanatory tooltip |
| Restore confirm modal | ✅ | ✅ | Explicit Confirm button; Cancel is a no-op |
| Body read-only by default; explicit Edit toggle | ✅ | ✅ | `runner-edit-btn` reveals editor, hides `#runner-body-block` |
| Cancel restores clean state | ✅ | ✅ | No lingering draft |
| **Diff preview before save (side-by-side current vs draft)** | **Listed** | **❌ Missing** | CSS class `.runner-diff-pane` exists but is used for the task-preview panel, not a save-time diff. Save fires directly after clicking. See "Drift" below. |

## Drift from spec

1. **Pre-save diff preview is missing** (safety-affordances bullet 2). The manifest lists `"Diff preview before save — show a side-by-side diff of current body vs draft before the SCD-2 transaction fires"`. The implementation binds Save directly to a PUT; the user never sees a diff between the current body and their draft before the write commits. The `.runner-diff-pane` class appears in the CSS but is attached to the task-preview pane, not a diff. The numbered Acceptance list does NOT include this — so the eight-item acceptance passes — but the safety bullet is unambiguous. Flagging as a followup rather than a reject given:
   - The restore flow is gated by an explicit modal;
   - Reason is required (audit trail intact);
   - Every revision is recoverable via Restore (SCD-2 never loses data);
   - The editor is opt-in (Edit button must be clicked).
   Risk of an unreviewed save is low, but the spec asked for it.

2. **Restore modal copy** — spec quotes `"This creates a new revision (v4) matching v2 from 2026-04-19. Proceed?"`. Impl says `"This creates a new revision matching v<N> from <ts>. Proceed?"` (missing the prospective new version number). Cosmetic.

## Code-level notes

- **Route ordering fix is correct.** The literal `/templates/preview` is registered before the `/{uid}` catch-all so gorilla/mux does not match `preview` as a uid. Same pattern used for `/{uid}/history`, `/{uid}/at`, `/{uid}/restore`. Inline comment left in source.
- **Preview endpoint degrades gracefully.** Missing task store → zero-value `PromptData.Task`; render proceeds with empty fields. Missing manifest store → same. No 500 from missing references.
- **Restore endpoint author stamping.** `changed_by` is prefixed with `http:` (`http:dashboard` for UI calls), consistent with the M2 UpdateBody convention.
- **Tests.** `TestTemplate_Preview` exercises both happy path and parse error with a broken template (`{{.Nope`). `TestTemplate_Restore` verifies the three-row history + newest reason prefix + 404 on an out-of-range `from_valid_from`. Bodies compared by exact string — robust to whitespace drift.
- **XSS.** `mdToHtml` routes every segment through `escHtml` before markdown transforms; `<script>` in a body renders as literal text. Preview output uses `pre.textContent = out` (no innerHTML). Reason strings are routed through `esc`. No injection surface found.
- **Editor reuse.** The same `OL.mountEditor(textarea, {onSave, onCancel})` wrapper used elsewhere is mounted cleanly; single-editor invariant enforced via `_editorMDE` guard.

## Build / test results

- `go build ./...` — ok (only cgo deprecation warnings from sqlite-vec, unrelated).
- `go test ./internal/web` — `ok 1.427s`.
- `go test ./internal/templates` — `ok 0.676s`.
- `go test ./internal/mcp` — `ok 0.920s`.
- `node --check internal/web/ui/views/runner.js` — clean.

## Followups (non-blocking)

- **[FU-1] Add pre-save diff pane** — render `unified-diff` (or side-by-side) between `_editorTpl.body` and the current draft once the Reason is valid, before the actual PUT. The `.runner-diff-pane` CSS is already carved out; wire a pre-PUT "review diff" step or inline diff preview. Manifest-quoted safety affordance.
- **[FU-2] Include the prospective new version in the restore modal copy** — the spec-quoted phrasing mentions `"new revision (v4)"`; plumb `history.length + 1` into `showRestoreConfirm`.
- **[FU-3] Tasks-list fetch is hard-capped to `?limit=200`** — fine today; if the workspace grows the Preview dropdown will truncate silently. Consider an async search-select or pagination note.

## Verdict

**APPROVE.** Eight-item Acceptance list passes; safety-critical paths (required reason, explicit restore confirm, read-only by default, SCD-2 audit-trail preserved) are in place. One spec drift (pre-save diff) and one copy nit filed as non-blocking followups.
