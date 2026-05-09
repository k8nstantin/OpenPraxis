# Trace-Grounded Feedback Loop

*Introduced in v0.9.0. Based on the meta-harness pattern (Lee et al., arXiv:2603.28052).*

Agents running the same task twice used to start completely fresh — no memory of what broke, what was tried, or how many times the same mistake had been made. v0.9 closes that loop.

---

## How It Works

Three phases, each building on the previous:

```
Task runs → prior_context injected → agent adjusts
         → pass rates queryable    → operator sees what's struggling
         → proposer fires          → scaffold improves automatically
```

---

## Phase 1 — Prior Context Injection

Every task prompt now includes a `<prior_context>` block automatically inserted between `<manifest_spec>` and `<task>`. On first run the block is absent (nothing to show). On subsequent runs it contains:

**Prior run digests** — what previous runs produced:
```
Run #1 (abc12345) — completed/max_turns — 51 turns, 84 actions, $0.18, 501s. Branch: openpraxis/xxx. Lines: +1032/-4.
Run #2 (def67890) — completed/success — 38 turns, 37 actions, $0.12, 320s. Branch: openpraxis/xxx. Lines: +47/-3.
```

**Prior agent comments** — execution review comments written by agents in prior runs.

**Budget enforcement** — total prior context is capped at `prompt_max_context_pct` of the model context window. Oldest entries drop first.

### Settings

| Knob | Default | Description |
|------|---------|-------------|
| `prompt_prior_runs_limit` | 5 | Prior run digests to inject. 0 disables. |
| `prompt_prior_comments_limit` | 3 | Prior agent comments to inject. 0 disables. |
| `prompt_build_timeout_seconds` | 5s | DB query deadline. |
| `prompt_max_comment_chars` | 2000 | Max chars per comment. |
| `prompt_max_context_pct` | 0.40 | Max fraction of context window for prior_context. |

All knobs inherit via task → manifest → product → system. Set at any scope:

```
PUT /api/products/<id>/settings
{ "prompt_prior_runs_limit": 10 }
```

<p align="center">
  <img src="screenshots/v0.9/task-settings-prompt-context.png" alt="Prompt Context knobs in the Settings tab" width="100%" />
</p>

---

## Phase 2 — Frontier Queries

Query pass rates for every task in a manifest:

```
GET /api/execution/frontier?manifest_id=<uuid>&window_days=30
```

```json
{
  "manifest_id": "...",
  "window_days": 30,
  "avg_pass_rate": 0.62,
  "best": { "task_id": "...", "pass_rate": 0.90 },
  "tasks": {
    "<task_id>": {
      "total_runs": 8,
      "success_runs": 5,
      "failed_runs": 1,
      "max_turns_runs": 2,
      "avg_turns": 67,
      "avg_cost_usd": 0.31,
      "pass_rate": 0.625
    }
  }
}
```

Tasks with zero runs are absent from the map. `window_days` defaults to the `frontier_window_days` knob (default 30).

Run detail responses now include `cost_per_turn` and `cost_per_action`.

### Setting

| Knob | Default | Description |
|------|---------|-------------|
| `frontier_window_days` | 30 | Rolling window for pass-rate computation. |

---

## Phase 3 — Proposer Loop

**Off by default.** Flip `proposer_enabled=true` to activate.

<p align="center">
  <img src="screenshots/v0.9/settings-catalog.png" alt="Proposer Loop knobs in the Settings catalog" width="100%" />
</p>

### How it works

1. After every task completion, the runner checks trigger conditions against the parent manifest
2. When triggered, a `meta-harness proposer` task is created under the manifest and scheduled immediately
3. The proposer reads the frontier, finds the worst-performing task, and changes **one** prompt template section
4. An evaluator task follows (via `depends_on`), accepts or tombstones the change based on pass-rate delta
5. The cycle repeats on the next trigger

### Enabling

```
PUT /api/products/<id>/settings
{
  "proposer_enabled": "true",
  "proposer_trigger_failure_streak": 3
}
```

Or per-manifest:
```
PUT /api/manifests/<id>/settings
{
  "proposer_enabled": "true",
  "proposer_trigger_failure_streak": 2
}
```

### Trigger conditions

| Condition | Knob | Default |
|-----------|------|---------|
| N consecutive `max_turns` or `deliverable_missing` failures | `proposer_trigger_failure_streak` | 3 |
| Single run exceeds cost | `proposer_trigger_cost_usd` | 0 (disabled) |

`timeout` and `process_error` reset the streak counter but don't trigger the proposer.

### Acceptance threshold

A candidate template change is accepted only if pass rate improves by ≥ `proposer_min_pass_rate_delta` (default 0.05 = 5 pp). Below the threshold the change is tombstoned and the prior template body restored automatically.

### All proposer knobs

| Knob | Default | Description |
|------|---------|-------------|
| `proposer_enabled` | false | Master gate. |
| `proposer_trigger_failure_streak` | 3 | Non-transient failures before auto-fire. |
| `proposer_trigger_cost_usd` | 0.0 | Per-run cost threshold. 0 = disabled. |
| `proposer_min_pass_rate_delta` | 0.05 | Minimum improvement to accept a change. |
| `proposer_max_candidates` | 3 | Max template variants per iteration. |

### What the proposer can change

**Allowed:** `preamble`, `prior_context`, `task`, `instructions`, `closing_protocol` sections via `template_set`; manifest or skill description via `entity_update`.

**Not allowed:** `visceral_rules`, `manifest_spec`, `git_workflow`.

All changes are SCD-2 versioned. Rollback via `template_tombstone` is automatic on rejection.

---

## Typical Workflow

1. Run your tasks normally — prior context works automatically from the second run onward
2. After a few runs, query the frontier to see which tasks are struggling
3. Enable the proposer once you have ≥ 3 runs per task
4. Watch proposer and evaluator tasks appear under your manifest in the dashboard
5. Pass rates improve over time without manual prompt tuning

---

## New Allowed Tools

The following MCP tools are now in the system-default `allowed_tools` — available to all agents:

`mcp__openpraxis__template_set`, `mcp__openpraxis__template_get`, `mcp__openpraxis__template_history`, `mcp__openpraxis__template_tombstone`, `mcp__openpraxis__entity_get`
