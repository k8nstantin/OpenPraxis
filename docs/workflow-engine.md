# OpenPraxis — Workflow Engine for Autonomous Coding Agents

Status: canonical design doc · Last updated 2026-05-06 · v0.6 architecture update

This doc exists to stop the architectural drift. Over time OpenPraxis grew from "a memory layer for coding agents" into a full DAG-based workflow engine specialized for autonomous coding agents. That's now the truth. Every future PR that touches the primitives listed here should check itself against this doc before merging.

## What OpenPraxis is

**A workflow engine for autonomous coding agents.** Similar in shape to Airflow / Prefect / Temporal / Argo / Dagster, but specialized for a different substrate:

- **Tasks execute agent subprocess spawns** (Claude Code, Cursor, Codex), not code functions.
- **Runs isolated per-task in git worktrees** off fresh `origin/main` so concurrent tasks don't collide.
- **Tracks turns + lines per run** via the append-only `execution_log` (cost tracking redesigned in v0.6).
- **Has a first-class review/rejection loop** — rerunning tasks with feedback is a primitive, not an afterthought.
- **Carries a visceral-rules enforcement layer + independent watcher audit** — non-negotiable compliance checks agents cannot override.
- **Laptop-first**: one Go binary, SQLite, no external dependencies beyond Ollama for embeddings. Runs on a dev machine.
- **Universal DAG dispatcher (v0.6)**: any entity type (product/manifest/task/skill) can be scheduled. Agent receives entity UUID, reads the DAG itself via the HTTP API, assembles context, and executes.

It is NOT a general-purpose workflow engine. It is NOT a build system. It is NOT just an MCP server.

## v0.6 Architecture changes

### Universal DAG dispatcher

Prior to v0.6, only `task`-type entities could be scheduled. The v0.6 dispatcher generalizes this: **any entity** (product/manifest/task/skill/idea) can have a `schedules` row, and the dispatcher fires an agent session scoped to that entity.

The dispatch flow:

```
schedules table → cron tick → pick due entity
    → agent receives entity UUID + execution skill
    → agent calls GET /api/entities/:id + /api/relationships?dst_id=:id&kind=owns
    → agent walks DAG from entity up to root (task → manifest → product)
    → assembles context: product prompt → manifest prompt → entity prompt
    → executes with full hierarchical context
```

The agent itself is responsible for reading the DAG — it is not pre-assembled by the runner. This keeps the runner dumb and the agent context-aware.

### Execution Skill

The **Execution Skill** (`019dfecc`) is an entity in OpenPraxis that stores the canonical agent operating procedure as a `prompt` comment. At boot, the runner reads this skill and injects it into every agent invocation as the manifest spec.

The execution skill teaches agents:
1. How to read the DAG (entity UUID → walk parents → collect prompts + comments)
2. How to post `**Agent's Decision:**` comments as they work
3. How to post the final report as a `prompt` comment on the target entity
4. How to use the closing protocol (`execution_review` comment)

Storing the protocol in OpenPraxis itself means it's versioned (SCD-2), auditable, and updateable without code changes.

### prompt/comment type system

v0.6 collapses the previous 9-type comment taxonomy to 2 primary types:

| Type | Purpose |
|------|---------|
| `prompt` | **Active instructions** — the latest `prompt` comment on an entity is what the next agent run executes. Append-only; history is preserved. |
| `comment` | **Context** — findings, decisions, blockers, prior attempts. Fed to the next run as situational awareness. |

Legacy types (`agent_note`, `user_note`, `decision`, `link`, etc.) are transparently remapped to `comment` at both the MCP and HTTP boundaries (PR #344).

Special subtypes retained: `execution_review`, `review_approval`, `review_rejection`, `watcher_finding`.

### Unified entity model

All entity types — `product`, `manifest`, `task`, `skill`, `idea` — now live in a single `entities` table (SCD-2). Every change creates a new row; time travel is native. The `relationships` table is the single source for all edges.

```
entities (SCD-2)
  ├── type: product | manifest | task | skill | idea
  └── Every change → new row (valid_from / valid_to)

relationships
  ├── kind: owns | depends_on | links_to
  └── Connects any entity to any other entity

execution_log (append-only)
  ├── event: started | sample | completed | failed
  └── Full metrics: turns, tokens, CPU, RSS, lines, commits

schedules (SCD-2)
  └── Drives when any entity fires
```

### Concurrency guard

One agent at a time system-wide. PID-based zombie detection: dead processes are auto-closed; live ones block dispatch. This prevents overlapping runs from the same schedule.

## The three-tier DAG (canonical)

```
Peer (node) → Product → Manifest → Task
                ↕          ↕          ↕
             deps       deps       deps
```

- **Product** is a top-level organizational entity. Many-to-many deps on other products. Arbitrary depth. Deps are organizational AND execution-gating per the session 2026-04-19 decision.
- **Manifest** is a milestone/spec within a product. Many-to-many deps on other manifests, cross-product allowed. Arbitrary depth. Deps gate task execution within the manifest.
- **Task** is the atomic unit of execution. Single-parent `depends_on` today (many-to-many is idea `019da59d-a63` when the need arises). Deps gate execution.

Cycle detection is enforced at every tier via DFS on insert. Cycles across tiers (product → manifest → task) are impossible by construction — different edge tables.

Relevant files:
- `internal/product/dependencies.go` (#89)
- `internal/manifest/dependencies.go` (#76)
- `internal/task/repository.go` `SetDependency` (#87)

## Entity state machine

Entities have a `status` field. For task-type entities, the canonical states (locked in #73, extended in #93) are:

| Status | Meaning |
|---|---|
| `pending` | Created, no dep, no schedule. Never auto-fires; needs manual start or a scheduled time. |
| `waiting` | Has an unmet dep (task-level, manifest-level, or product-level). Auto-fires when blocker clears. |
| `scheduled` | Armed; scheduler picks up at `next_run_at`. |
| `running` | Agent subprocess live. |
| `paused` | SIGSTOP'd, resumable. |
| `completed` | Agent exited successfully. |
| `failed` | Agent errored, hit cost cap, timed out, or watcher downgraded from completed. Terminal. |
| `cancelled` | Operator cancelled. Terminal. |

v0.6: status is stored in the `entities` table (SCD-2). Every status transition creates a new entity row. `store.UpdateEntityStatus` enforces valid transitions.

Notable exceptions to "terminal is terminal":
- `completed → failed` (watcher audit downgrade — the original exception)
- `completed → scheduled` (review-rejection re-run per #93)

## The activation model

**"Dependency means execution"** — per operator directive, session 2026-04-19.

### Seeding at create time

`task.Store.Create` consults three gates in order (innermost first):

1. **Task-level** `depends_on` — if parent not `completed`, seed `waiting` with `block_reason="task <marker> not completed (is <parentStatus>)"`.
2. **Manifest-level** `ManifestReadinessChecker.IsSatisfied` — if unsatisfied, seed `waiting` with `block_reason="manifest not satisfied — blocked by: <markers>"`.
3. **Product-level** `ProductReadinessChecker.IsSatisfied` — if unsatisfied, seed `waiting` with `block_reason="product not satisfied — blocked by: <markers>"`.

Closer blocker wins in `block_reason`. Outer checks re-run at activation time as inner blockers clear.

### Propagation walker

When a manifest or product transitions from non-terminal (`draft`/`open`) to terminal (`closed`/`archive`):

1. `ListDependents(closedEntity)` — in-edges.
2. For each dependent, re-evaluate `IsSatisfied`.
3. If newly satisfied, `FlipManifestBlockedTasks` (or `FlipProductBlockedTasks`) runs a scoped `UPDATE tasks SET status='scheduled'` filtered by the correct `block_reason` prefix.
4. Enqueue the dependent in the BFS frontier for transitive closures (visited set guards cycles).

Only tasks with the matching `block_reason` prefix are flipped — task-level blocks stay blocked by their own resolution path.

### Dep-removal rehab (Option B)

When an operator removes a dep (`RemoveDep(A, B)`):

1. Re-evaluate `IsSatisfied(A)`.
2. If newly satisfied, flip blocked tasks `waiting → pending` (NOT `scheduled` — operator must manually arm to prevent accidental budget burn).

Implemented at the manifest tier in #79, product tier in #92.

## Review loop (#93)

A completed task can be rejected by a reviewer (human or agent) and sent back for another pass:

- **Rejection**: `RejectCompletedTask(taskID, reason, reviewer)` writes a `review_rejection` comment against the task + flips status `completed → scheduled` + clears `block_reason`. One-shot atomic-enough pair (not a transaction; worst case a comment exists without the status flip and can be re-rejected).
- **Approval**: `ApproveCompletedTask(taskID, reviewer)` writes a `review_approval` comment. Does NOT change status — approval is a signal consumed by manifest-closure warnings.
- **Derived state**: `TaskReviewStatus` walks the comment stream newest-first. Latest comment wins — if rejection is newer than approval, `NeedsRework=true`.

Comments-as-artifacts means the review history IS the audit trail; no separate review table.

Cascading review at manifest + product tiers is idea `019da5b9-685` → issue #95 (not shipped; operator decides later).

## SCD Type 2 principle ("nothing is deleted")

Per session directive 2026-04-19: the system never loses prior state. Every slowly-changing value becomes a row in an append-only history table.

Current SCD adoption:

- **Task `depends_on`** — `task_dependency` table (#87). Each SetDependency closes the prior active row (stamps `valid_to`) and inserts a fresh one.
- **Dep edges (manifest, product)** — created_at + created_by on join-table rows. History is implicit (a dropped edge leaves no trace yet; #94 soft-delete + audit queue is the fix).
- **Review comments** — inherently append-only via the comments table.
- **Task runs** — `task_runs` is append-only (each execution is a new row).

Queued SCD work (idea `019da5e4-601`):

- Manifest status + content + title
- Product status + description
- Task title + schedule
- Soft-delete for comments (#94)

## Peer-to-peer architecture (not client-server)

OpenPraxis is **peer-to-peer by design**. Every node is a full instance: same binary, same DB, same MCP surface. Nodes discover each other via mDNS and sync memories / manifests / conversations over an Automerge CRDT. There is no central coordinator.

Consequences for the workflow model:

- **Execution is local.** A peer's tasks run on that peer's worktree, not on some remote worker pool. Your laptop's OpenPraxis is both the planner and the executor.
- **Shared state is shared-by-sync, not shared-by-server.** When two peers both care about the same product, both hold copies of its manifests + tasks + comments + dep edges. Changes propagate via CRDT merge, not a central database.
- **Identity is per-peer.** Every peer has its own UUID (`peer.Registry`). Entities carry `source_node` so the graph remembers which peer originated what, even after sync.

This is why we can't adopt Temporal — its architecture assumes a central server cluster. Our model assumes the opposite.

### Future: submit-for-execution

A long-standing intent of the P2P design is the **submit-for-execution** pattern: an operator plans a product (with its manifests + tasks) on their laptop, then submits the whole package to a peer configured as an **executor node** (e.g. a beefy workstation, a team's shared runner, a cloud instance). The executor runs the tasks — possibly in parallel across multiple peers — and sync streams the results back.

This is NOT a central coordinator; it's a specialization of the peer role. The executor peer is still a peer, just one that opted into accepting submitted work. Implementation intentionally deferred; see roadmap item "submit-for-execution" below.

## What we deliberately deviate from workflow-engine norms on

This is not a general-purpose workflow engine. We specifically do NOT:

- **Execute arbitrary code as tasks.** Tasks spawn LLM agent subprocesses. This is non-negotiable.
- **Run workflows in a separate server cluster.** We are laptop-first AND peer-to-peer. Single Go binary + SQLite + Automerge sync. Adding Temporal-style central infrastructure breaks both stories.
- **Implement strong replay semantics.** A task's "output" is side effects in git, not a deterministic return value. Replay is git-checkout, not workflow-replay.

We DO specialize in:

- **Agent-aware cost tracking** (#61 self-calibrating pricing)
- **Git worktree isolation per run** (#66)
- **Watcher-as-compliance-tier** (#62 — independent auditor agents can't override)
- **Visceral rules** (mandatory operating constraints on every session)
- **Review-as-primitive** (#93)
- **Comments-as-audit-trail** for every entity (#60/#63/#65)
- **Memory layer for agent context continuity** (orthogonal to workflow concerns but foundational)

## Roadmap — named, not accidental

Things workflow engines typically have that we don't yet:

### Borrowed from Temporal

These are lessons worth stealing without adopting Temporal itself:

1. **Comprehensive checkpointing.** Today `task_runtime_state` holds partial state — a crash mid-run can leave the runner with dangling state. Make checkpointing exhaustive so any task is resumable from its last known tick across process restart. Not atomic like Temporal, but best-effort is better than none.

2. **Explicit retry-with-backoff primitive.** Today `retry_on_failure` is a count. Add `retry_delay` + `retry_backoff_multiplier` knobs so transient failures (network, rate limit, flaky test) can retry with exponential backoff instead of firing back-to-back.

3. **Workflow versioning.** When a manifest's content changes mid-task, the running task sees the new content but was planned against the old one. Add: each task pins to the manifest version it was created against; explicit re-plan button forces a new task with the current version. Prevents "the spec changed under me" surprises in long-running chains.

### Our own roadmap

4. **Dynamic DAG expansion.** An agent running task M2-T4 discovers a new sub-task is needed mid-execution; calls `task_create` as a child of itself. Today this works but the relationship is just `depends_on`; there's no "this task spawned that one" semantics. Formalize.

5. **Event triggers.** Today: scheduled + manual + dep-completion. Add: webhook triggers (GitHub PR merged → fire task), file-change triggers, schedule-with-cron.

6. **Backfill / rerun-from-point.** Idea: "re-run everything under product X from task Y onward." Useful when a bug is found in an earlier step and downstream work needs re-execution.

7. **Parallelism caps per tier.** Today `max_parallel` is per-product. Add per-manifest and per-task-family caps.

8. **Subworkflow composition.** Today a task declares a dep; it doesn't "call" another task as a function returning a value. Add: explicit parent-child with return value capture.

9. **Observability — timeline view + critical path analysis.** Dashboard has per-task view and aggregated cost. Add: Gantt-style timeline across a product, critical-path highlighting, cost rollup per chain.

10. **Submit-for-execution across peers.** A peer can package a product + its manifests + tasks + dep edges + visceral rules into a submission bundle and hand it off to another peer configured as an executor node. The executor runs the tasks (possibly dispatching further across its own peer mesh) and syncs results back via Automerge. Keeps the local-first + P2P story intact while letting operators burst work to beefier hardware. Designs to lock before implementing: authentication/authorization between peers, cost-budget passing, who owns the git branches the executor creates, review-loop routing when reviewer is on a different peer than the executor.

## Contract for future PRs

Every PR that touches these primitives MUST:

- Update this doc with new invariants or deviations.
- Add a test that asserts the new behavior (state-machine tests, propagation tests, SCD append tests).
- Check whether the change creates a new `block_reason` format or new status or new audit-worthy event. If yes: use the shared modules (see refactor queue below), don't invent a new format.

## Refactor queue (forcing functions against drift)

These are code-level centralizations that will prevent the class of bugs this session exposed (#97 block_reason format divergence, three near-identical dep walkers, ad-hoc status transitions at the manifest/product tiers):

- **Shared `block_reason` module.** One enum of block categories + canonical format producers. Three codepaths currently write this — centralize.
- **Shared `DepGraph` interface + walker.** Cycle detection, `IsSatisfied`, activation BFS via one generic implementation + per-tier adapters. Currently three near-identical codebases.
- **Extend `validTransitions` enforcement to manifests + products.** Or declare their statuses free-form. Pick one and document.

Each is its own issue + PR.

## References

- #73 — task status taxonomy + transitions
- #76, #89 — dep data layers (manifest, product)
- #77, #92 — task seeding via readiness checks
- #78, #92 — close propagation
- #79, #92 — dep-removal rehab
- #87 — task-dep cycle + SCD
- #93 — review/rejection loop
- #97 — block_reason format normalization (the drift this doc exists to prevent)
- idea `019da5e4-601` — SCD everywhere
- idea `019da59d-a63` — many-to-many task deps
- idea `019da5b9-685` — visual DAG designer
- idea `019da5e8-47f` — Visual DAG Designer Product entity
