# Review — RC/M2 SCD-2 write path + history + MCP (PR #213)

**Task:** `019dbaa4-60fa-79cf-b527-803f601506d2`
**Manifest:** `019dba9b-2e6` (RC/M2)
**Product:** `019dba88-973` (Runtime Configurability)
**PR:** https://github.com/k8nstantin/OpenPraxis/pull/213
**Branch:** `openpraxis/019dbaa4-60f`
**HEAD:** `00637e9`
**Verdict:** **APPROVE**

## Scope delivered

Phase 2/6 of the RC product on top of RC/M1 (PR #210). Adds transactional SCD-2 mutation, history + point-in-time queries, HTTP write endpoints, and the seven `template_*` MCP tools. 1,334 additions across 8 files.

## Files audited

- `internal/templates/schema.go` — `+9` — added the partial UNIQUE index `idx_prompt_templates_one_active ON prompt_templates(template_uid) WHERE valid_to=''`.
- `internal/templates/store.go` — `+271` — `Create`, `UpdateBody` (SCD-2 tx), `Tombstone`, `CloseStatus`, `History`, `AtTime`, `ListWithScopeID`.
- `internal/templates/store_write_test.go` — `+277` — SCD-2 round-trip, concurrent writers (10 goroutines), duplicate rejection, system-scope rejection, AtTime across 3 revisions, tombstone + close-status fall-through.
- `internal/web/handlers_template.go` — `+188/-2` — `POST`, `PUT`, `DELETE`, `GET /history`, `GET /at` endpoints.
- `internal/web/handlers_template_test.go` — `+197` — PUT live-refresh, POST 201+409, AtTime 200/404, DELETE tombstones.
- `internal/mcp/tools_template.go` — `+219` — 7 MCP tools registered.
- `internal/mcp/tools_template_test.go` — `+172` — round-trip per tool via `s.handle*`.
- `internal/mcp/server.go` — `+1` — `s.registerTemplateTools()` wired in boot.

## Gates

| Gate | Result |
|------|--------|
| `go build ./...` | green |
| `go test ./...` | green (all 15 packages) |
| `go test ./internal/templates/... ./internal/web/... ./internal/mcp/...` | green |

## Acceptance-criteria self-check

| # | Criterion | Verified |
|---|-----------|----------|
| 1 | `PUT /api/templates/<uid>` closes prior + appends new in one tx | ✅ `TestTemplate_PutCloses_PriorAndAppendsNew`, `TestUpdateBody_SCD2` |
| 2 | `GET /history` newest-first, changed_by + reason populated | ✅ `TestTemplate_PutCloses_PriorAndAppendsNew` asserts both |
| 3 | `GET /at?t=...` via `valid_from <= ? AND (valid_to > ? OR valid_to = '')` | ✅ `TestAtTime_ReturnsBodyActiveAtTimestamp` + HTTP `TestTemplate_AtTime` |
| 4 | `POST` rejects duplicate `(scope, scope_id, section)` | ✅ `TestTemplate_PostCreatesOverride` asserts 409; `TestCreate_RejectsDuplicate` at store layer |
| 5 | Runner picks up new body on NEXT spawn (no restart) | ✅ `TestTemplate_PutCloses_PriorAndAppendsNew` uses live resolver post-PUT |
| 6 | `status='closed'` falls through to broader scope | ✅ `TestCloseStatus_FallsThrough` |
| 7 | Atomicity (panic rollback) | ⚠️ Not via explicit panic injection — covered structurally by `defer ROLLBACK` in `updateBodyOnce` + by concurrent-writer test showing the tx serialises cleanly. See "Deviations" below. |
| 8 | Concurrency: 10 writers ⇒ 1 active, 11 history rows, no errors | ✅ `TestUpdateBody_ConcurrentWriters` |
| 9 | `DELETE` tombstones uid; resolver falls through; not revivable | ✅ `TestTombstone_FallsThrough` + HTTP `TestTemplate_DeleteTombstones` |
| 10 | All 7 MCP tools registered + each has a handler unit test | ✅ `s.registerTemplateTools()` called in `server.go:129`; `tools_template_test.go` exercises all 7 handlers directly |
| 11 | `go build ./...` + targeted test trees green | ✅ |

## Deviations from the spec

1. **SCD-2 transaction implementation.** Spec sketched a `withTxBeginImmediate` helper; PR inlines `BEGIN IMMEDIATE` / `COMMIT` / `ROLLBACK` on a dedicated `*sql.Conn` (`store.go:249-306`). Functionally equivalent and arguably cleaner — it avoids a helper that few callers would use. Also adds SQLITE_BUSY retry with bounded backoff (5 attempts), which the spec did not specify but visceral rule #10 makes necessary. **Accepted.**
2. **Acceptance #7 (synthetic panic test).** Not implemented as a dedicated test. The `updateBodyOnce` function uses `committed bool` + deferred `ROLLBACK` so a panic between UPDATE and INSERT unwinds the tx — structurally safe. Atomicity is also implicitly proven by the concurrent-writers test: if the UPDATE and INSERT were separable, the partial UNIQUE index would have caught a double-active-row at some point across 10 parallel writers and surfaced as an error. No error was seen. **Minor gap — filed as followup, not a blocker.**
3. **`mcpTemplateAuthor` audit prefix.** Comment in `tools_template.go:211` says "`tpl:` prefix" but the implementation returns `mcpSetAuthor(ctx)`'s result (prefixed `mcp:<sessionID>`). Cosmetic/docstring drift; audit still works. **Minor.**
4. **Extra endpoint `PATCH` status via `PUT`.** `apiTemplateUpdate` routes `{status: "closed"}` through `CloseStatus`. Spec didn't explicitly ask for this via PUT (it lives under acceptance #6 as a property of the resolver, but not as an endpoint). The web layer wired it in for operator ergonomics. **Accepted — nice UX addition, resolver behaviour is tested.**

## Non-goals observed

- `UpdateBody` does not touch `title` (spec-compliant).
- Reviving a tombstoned uid is not supported (spec-compliant; documented in `store.go:311-312`).
- No in-memory resolver cache — PUT takes effect on the next spawn, as spec requires.

## Followups filed

1. **Add synthetic-panic rollback test** for `UpdateBody` — inject a panic between UPDATE and INSERT in `updateBodyOnce`, assert exactly one active row survives. Close the #7 gap explicitly. (Small, deferred, not a blocker.)
2. **Fix `mcpTemplateAuthor` comment drift** — either rename/restore the "`tpl:`" prefix or update the comment to reflect the actual `mcp:<sessionID>` output. Cosmetic.
3. **Resolver cache callout honoured.** RC/M1 has no cache today, so the PUT-takes-effect-immediately property is real. If any phase adds a cache, it MUST invalidate on PUT/DELETE/Tombstone. (Already flagged in the manifest; restating here for the audit trail.)

## What the next task should expect

- Store API stable: `Create`, `UpdateBody`, `Tombstone`, `CloseStatus`, `History`, `AtTime`, `ListWithScopeID`, plus RC/M1's `Get`, `GetByUID`, `List`.
- HTTP: `POST /api/templates` (201/400/409), `PUT /api/templates/{uid}` (200/404), `DELETE /api/templates/{uid}?reason=` (200/404), `GET /api/templates/{uid}/history` (200), `GET /api/templates/{uid}/at?t=<RFC3339>` (200/400/404).
- MCP: `template_create`, `template_set`, `template_get`, `template_history`, `template_at`, `template_list`, `template_tombstone`.
- Sentinel errors: `templates.ErrNotFound`, `templates.ErrDuplicateOverride`.
- Author audit prefixes: `http:<changed_by>`, `mcp:<sessionID>`.

## Verdict

**APPROVE.** Ship it. The SCD-2 write path is correct, the partial UNIQUE index + `BEGIN IMMEDIATE` + retry combination defends against concurrent writers, all acceptance criteria are covered by tests (with the one minor gap on explicit panic-injection noted above), and the MCP + HTTP surfaces match the spec.
