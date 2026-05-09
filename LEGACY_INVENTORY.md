# Settings Tab Legacy Inventory

Generated as step 1 of T1 execution. No `views/settings.js` exists in the repo;
the canonical spec is `internal/settings/catalog.go` (32 knobs) and the HTTP API
registered in `internal/web/handlers_settings_exec.go`.

## API Surface

| Endpoint | Method | Purpose |
|---|---|---|
| `/api/settings/catalog` | GET | Full knob catalog (32 knobs) |
| `/api/products/{id}/settings` | GET | Explicit entries for product scope |
| `/api/products/{id}/settings` | PUT | Set one or more knobs at product scope |
| `/api/products/{id}/settings/{key}` | DELETE | Reset knob to inherited at product scope |
| `/api/manifests/{id}/settings` | GET | Explicit entries for manifest scope |
| `/api/manifests/{id}/settings` | PUT | Set one or more knobs at manifest scope |
| `/api/manifests/{id}/settings/{key}` | DELETE | Reset knob to inherited at manifest scope |
| `/api/tasks/{id}/settings` | GET | Explicit entries for task scope |
| `/api/tasks/{id}/settings` | PUT | Set one or more knobs at task scope |
| `/api/tasks/{id}/settings/{key}` | DELETE | Reset knob to inherited at task scope |
| `/api/tasks/{id}/settings/resolved` | GET | Full resolved chain for all 32 knobs |

## Knob Catalog (32 knobs)

| Key | Type | Default | Group |
|---|---|---|---|
| `max_parallel` | int | 3 | Execution |
| `max_turns` | int | 50 | Execution |
| `timeout_minutes` | int | 30 | Execution |
| `temperature` | float | 0.2 | Execution |
| `reasoning_effort` | enum | medium | Execution |
| `default_agent` | enum | claude-code | Execution |
| `default_model` | enum | (empty) | Execution |
| `retry_on_failure` | int | 0 | Execution |
| `approval_mode` | enum | auto | Execution |
| `allowed_tools` | multiselect | [list] | Execution |
| `prompt_max_comment_chars` | int | 2000 | Prompt Context |
| `prompt_max_context_pct` | float | 0.4 | Prompt Context |
| `compliance_checks_enabled` | enum | true | Prompt Context |
| `scheduler_tick_seconds` | int | 10 | Scheduler |
| `host_sampler_tick_seconds` | int | 5 | Scheduler |
| `on_restart_behavior` | enum | stop | Scheduler |
| `branch_prefix` | string | openpraxis | Branch/Worktree |
| `branch_strategy` | enum | task | Branch/Worktree |
| `branch_remote` | string | github | Branch/Worktree |
| `worktree_base_dir` | string | .openpraxis-work | Branch/Worktree |
| `frontend_dashboard_v2` | enum | false | Frontend Flags |
| `frontend_dev_mode` | enum | false | Frontend Flags |
| `frontend_dashboard_v2_products` | enum | false | Frontend Flags |
| `frontend_dashboard_v2_manifests` | enum | false | Frontend Flags |
| `frontend_dashboard_v2_tasks` | enum | false | Frontend Flags |
| `frontend_dashboard_v2_memories` | enum | false | Frontend Flags |
| `frontend_dashboard_v2_conversations` | enum | false | Frontend Flags |
| `frontend_dashboard_v2_settings` | enum | false | Frontend Flags |
| `frontend_dashboard_v2_compliance` | enum | false | Frontend Flags |
| `frontend_dashboard_v2_overview` | enum | false | Frontend Flags |
| `comment_attachment_max_mb` | int | 10 | Attachments |
| `comment_attachment_allowed_mimes` | string | image/,text/,... | Attachments |

## Widget Requirements per Type

| Type | Widget |
|---|---|
| int | Gauge slider + number input (range-bound) |
| float | Gauge slider + number input (float step) |
| enum | Select dropdown (enum_values list) |
| multiselect | Comma-separated text input |
| string | Text input |

## Scope Inheritance Chain

`task → manifest → product → system`

First explicit entry at any tier wins. Delete at a tier restores inheritance from the next tier up.

## Parity Checklist (T1 deliverables)

- [ ] Catalog tab: all 32 knobs rendered, grouped by category, read-only, type badge
- [ ] Scope Editor: product/manifest/task scope picker with entity typeahead UUID lookup
- [ ] Scope Editor: all 32 knobs editable with type-aware widgets, inherited indicator
- [ ] Scope Editor: Reset button per knob (DELETE → re-inherit)
- [ ] Resolution Chain: task picker → /settings/resolved → per-knob source badge
- [ ] Cross-tab links: entity pages link to settings scope for same entity
