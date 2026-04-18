# Hierarchical Execution Controls

Autonomous coding agents don't fail uniformly. A polish task needs 30 turns and $0.50; a migration needs three hours and $30. A read-only audit must never call `Edit`; a feature build lives or dies by it. A single global "max turns" slider can't encode any of that — so OpenPraxis doesn't offer one.

Instead: **twelve execution knobs**, each configurable at **four scopes** (`task → manifest → product → system`), inherited top-down, resolved fresh at execution time. Set a budget once on the product and every task under it inherits. Crank `max_turns` for one stubborn refactor without touching any sibling. Narrow `allowed_tools` on an audit manifest and every task it spawns is read-only by construction — not by the agent's goodwill.

This document is the full reference: the inheritance model, the save/clamp/cap semantics, every knob with what it does, how the runner enforces it, its type and bounds, and the scope it typically lives at. The underlying tables, HTTP endpoints, and MCP tools are at the bottom so agents and dashboards can read these values programmatically.

## Table of contents

- [Inheritance model](#inheritance-model)
- [Save semantics](#save-semantics)
- [Visceral-rule caps](#visceral-rule-caps)
- [The twelve knobs](#the-twelve-knobs)
  - [1. `max_parallel`](#1-max_parallel)
  - [2. `max_turns`](#2-max_turns)
  - [3. `max_cost_usd`](#3-max_cost_usd)
  - [4. `daily_budget_usd`](#4-daily_budget_usd)
  - [5. `timeout_minutes`](#5-timeout_minutes)
  - [6. `temperature`](#6-temperature)
  - [7. `reasoning_effort`](#7-reasoning_effort)
  - [8. `default_agent`](#8-default_agent)
  - [9. `default_model`](#9-default_model)
  - [10. `retry_on_failure`](#10-retry_on_failure)
  - [11. `approval_mode`](#11-approval_mode)
  - [12. `allowed_tools`](#12-allowed_tools)
- [Reading values programmatically](#reading-values-programmatically)
  - [From a coding agent (MCP)](#from-a-coding-agent-mcp)
  - [From the dashboard (HTTP)](#from-the-dashboard-http)
  - [Database](#database)
- [Why this matters](#why-this-matters)

## Inheritance model

```
                       narrowest override point
                                    |
                                    v
  task  ─────────────►  manifest  ─────────────►  product  ─────────────►  system
  single run             feature area              initiative              built-in default
```

Every read walks **task → manifest → product → system** and returns the first explicit value found. If nothing is set anywhere, the built-in system default applies. The resolver is called fresh on every execution — changing `daily_budget_usd` at the product level affects the next task that runs, no restart required.

- **Set once, inherit everywhere.** A product-wide `daily_budget_usd = 50` covers every manifest and task under it. Nothing else needs to be touched.
- **Override narrowly when you need to.** A single risky migration task can carry its own `timeout_minutes = 180`; every other task under the manifest still uses the default.
- **Reset to inherited.** One click on the knob's reset icon drops the local override and reverts to whatever the next scope up says.
- **Inline provenance.** Each knob displays *"inherited from product X"* (or manifest / system) so you always know which scope to edit to change the value everywhere vs. just here.

## Save semantics

- **Writes are scoped.** Setting a value on a manifest writes a row to `settings` keyed by `(scope_type='manifest', scope_id=<manifest_id>, key)`. It does not affect sibling manifests.
- **Debounced.** UI slider changes save 300 ms after you stop; no Apply button to forget.
- **Soft slider bounds vs. hard visceral caps.** Slider min/max in the catalog are UI hints — pushing past them surfaces a warning but does not block. Visceral-rule caps (see below) hard-block the write.
- **Agent-readable.** The MCP `settings_resolve` tool walks the chain and returns the effective value, so agents can query *"what budget do I have?"* instead of being told.

## Visceral-rule caps

One knob currently has a hard cap enforced by a visceral rule:

| Knob | Cap | Source |
|------|------|--------|
| `daily_budget_usd` | ≤ $100 | Visceral rule #8 (`daily budget = $100`) |

Writes above the cap — at any scope, via HTTP or MCP — are rejected with a hard error. The runner also clamps at read-time as a defense in depth: even if a value somehow got into the DB above cap, the effective value seen by the task runner is still clamped.

---

## The twelve knobs

Knobs are listed in catalog order. Each section covers: what the knob does, how the task runner uses it, the type/default/bounds from `internal/settings/catalog.go`, and which scope typically owns it.

### 1. `max_parallel`

**What it does.** Ceiling on concurrent task executions. Ten tasks scheduled to run at the same moment with `max_parallel = 3` means three start, seven wait.

**Enforcement.** Read by the task scheduler at dispatch time. Before launching a new agent session, the runner calls `resolver.Resolve(scope, "max_parallel")` and counts currently-running tasks under the same effective scope.

**Type / default / range.** Integer, default `3`, slider 1–100.

**Where to set it.** Usually at **product** or **system** — you want a platform-wide ceiling that keeps your laptop/box from melting. Manifest or task-level overrides are useful for batch reprocessing jobs that want higher concurrency.

### 2. `max_turns`

**What it does.** Hard ceiling on agent turns (prompt → response cycles) within a single task run. When the agent hits `max_turns`, the session terminates with `terminal_reason = "max_turns"` and the watcher treats the task as completed-with-warning.

**Enforcement.** Passed as `--max-turns` (or equivalent) to the agent subprocess at spawn.

**Type / default / range.** Integer, default `50`, slider 10–10000.

**Where to set it.** **Manifest** is the natural home — a polish manifest might need 30 turns, a large refactor 500. Task-level override for outliers.

### 3. `max_cost_usd`

**What it does.** Hard ceiling on the per-task-run dollar spend. If the running cost crosses this value mid-session, the runner signals the agent to stop.

**Enforcement.** Per-turn cost is recorded in real time. The runner checks `accumulated_cost >= max_cost_usd` and triggers SIGTERM if so.

**Type / default / range.** Float, default `$10.00`, slider $0.01–$1000. Unit: USD.

**Where to set it.** **Manifest** for feature-area policy, **task** for individual high-cost jobs.

### 4. `daily_budget_usd`

**What it does.** Rolling 24-hour spend ceiling for the effective scope. When today's accumulated spend reaches the budget, new task launches are blocked (existing ones keep running to completion).

**Enforcement.** On dispatch, runner sums today's `task_runs.total_cost_usd` for tasks under the scope and compares to the budget. Blocks with a clear error if exceeded. Dashboard's "Cost Today / $X" gauge turns red at 100 %.

**Type / default / range.** Float, default `$100.00`, slider $1–$10000. Unit: USD. **Hard-capped at $100 by visceral rule #8.**

**Where to set it.** **Product** — one budget per initiative is the whole point. System default covers anything outside a product.

### 5. `timeout_minutes`

**What it does.** Max wall-clock time for a single task run. Regardless of turns or cost, if the session runs longer than this, it gets terminated.

**Enforcement.** `context.WithTimeout` wrapped around the agent subprocess. SIGTERM at timeout; SIGKILL on a grace-period follow-up if the process doesn't exit.

**Type / default / range.** Integer, default `30`, slider 1–1440. Unit: minutes.

**Where to set it.** **Task** for one-off long jobs, **manifest** for a whole feature area with known characteristic duration.

### 6. `temperature`

**What it does.** LLM sampling temperature. 0 = deterministic, 2 = maximum creativity. Directly passed to the model API.

**Enforcement.** Included in the spawn config for the agent subprocess, which forwards it as the `temperature` parameter on API calls.

**Type / default / range.** Float, default `0.2`, slider 0–2, step 0.05.

**Where to set it.** **Product** for initiative-wide style (analytical vs. exploratory), **manifest** for specific work types (code review = 0, ideation = 0.7).

### 7. `reasoning_effort`

**What it does.** Thinking-budget hint for reasoning-capable models (Claude extended thinking, o-series models). Higher values let the model think longer before answering.

**Enforcement.** Forwarded to the agent which passes it as `thinking.budget` / `reasoning.effort` on API calls. Silently ignored by models that don't support reasoning.

**Type / default / range.** Enum. Values: `minimal`, `low`, `medium`, `high`. Default `medium`.

**Where to set it.** **Manifest** — a "debug this flaky test" manifest might want `high`; routine codegen `low`.

### 8. `default_agent`

**What it does.** Which CLI agent runtime executes tasks under this scope.

**Enforcement.** The task runner looks up the agent name, finds its configured command/args in `internal/setup/`, and spawns accordingly. If the task itself specifies an agent, that wins; otherwise this default applies.

**Type / default / range.** Enum. Values: `claude-code`, `codex`, `cursor`, `windsurf`. Default `claude-code`.

**Where to set it.** **Product** if the whole initiative uses one agent; **task** for one-off experimentation with a different runtime.

### 9. `default_model`

**What it does.** Model ID override. Agent-specific — `claude-code` accepts `claude-opus-4-7`, `claude-sonnet-4-6`, etc.; `codex` accepts its own IDs.

**Enforcement.** Forwarded to the agent CLI as `--model <id>`. Empty string means "agent default".

**Type / default / range.** String, default `""` (empty = let the agent decide).

**Where to set it.** **Task** when a specific run needs Opus-level reasoning; otherwise leave blank and let the agent pick.

### 10. `retry_on_failure`

**What it does.** Auto-retry count on task failure. `0` = never retry, `3` = retry up to 3 times before marking failed.

**Enforcement.** Task runner checks the exit code and watcher verdict; if failed and retries remaining, it re-queues the task with a fresh run row.

**Type / default / range.** Integer, default `0`, slider 0–10.

**Where to set it.** **Manifest** for known-flaky areas (integration tests); keep `0` at the product level to avoid wasted spend.

### 11. `approval_mode`

**What it does.** Approval policy for agent-proposed actions (applies mainly to Codex, ignored by agents without approval gating).

**Enforcement.** Forwarded to the agent. `auto` skips prompts, `manual` blocks on every action, `on-failure` only prompts when an action errors.

**Type / default / range.** Enum. Values: `auto`, `manual`, `on-failure`. Default `auto`.

**Where to set it.** **Product** or **manifest** — approval policy is usually a workflow decision, not a per-task one.

### 12. `allowed_tools`

**What it does.** Tool allowlist for the agent session. Only tools in this list are available for the agent to call.

**Enforcement.** Forwarded to the agent as the tool allowlist at spawn. Tools outside the list are hidden from the agent entirely — it can't call them.

**Type / default / range.** Multiselect (array of strings). Default: `["Bash", "Read", "Write", "Edit", "Glob", "Grep"]`.

**Where to set it.** **Manifest** — a "read-only audit" manifest narrows the list to `["Read", "Glob", "Grep"]`; a standard feature manifest inherits the default; a deploy-ready task might add `["WebFetch"]` and nothing else.

---

## Reading values programmatically

### From a coding agent (MCP)

```
settings_resolve(scope_type="task", scope_id="<task_id>", key="daily_budget_usd")
  → { "value": 50, "source": { "scope_type": "product", "scope_id": "<pid>" } }

settings_resolve(scope_type="task", scope_id="<task_id>")   # all knobs at once
  → { "max_parallel": {...}, "max_turns": {...}, ... }

settings_catalog()   # knob definitions (types, ranges, defaults)
settings_get(scope_type, scope_id, key)   # explicit entries at one scope
settings_set(scope_type, scope_id, key, value)   # write at one scope
```

### From the dashboard (HTTP)

```
GET  /api/settings/catalog
GET  /api/settings/:scope_type/:scope_id              # explicit entries at this scope
GET  /api/settings/:scope_type/:scope_id/resolved     # fully walked values
GET  /api/settings/:scope_type/:scope_id/:key         # single resolved value
PUT  /api/settings/:scope_type/:scope_id/:key         # write
DELETE /api/settings/:scope_type/:scope_id/:key       # remove an explicit override
```

### Database

Single table, WAL-mode SQLite, keyed by `(scope_type, scope_id, key)`:

```sql
CREATE TABLE settings (
  scope_type TEXT NOT NULL CHECK (scope_type IN ('system','product','manifest','task')),
  scope_id   TEXT NOT NULL,
  key        TEXT NOT NULL,
  value      TEXT NOT NULL,   -- JSON-encoded
  updated_at INTEGER NOT NULL,
  updated_by TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (scope_type, scope_id, key)
) WITHOUT ROWID;
```

`updated_by` records who wrote the value (`http:<session>`, `mcp:<session>`, `migration:<name>`) for audit.

---

## Why this matters

Most agent platforms give you one global slider: a single "max turns" that applies to every task you ever run. Reality doesn't work that way — polish jobs need 30 turns, refactors need 500, audits need zero writes. OpenPraxis lets policy live at the level it naturally belongs to: budget at the initiative, turn ceiling at the feature, timeout on the one migration that actually needs three hours.

Combined with the watcher's independent audit and visceral-rule enforcement, you get a platform where *agents cannot exceed what you've authorised*, at any layer — and you don't have to repeat yourself to make that true.
