# OpenPraxis

[![CI](https://github.com/k8nstantin/OpenPraxis/actions/workflows/ci.yml/badge.svg)](https://github.com/k8nstantin/OpenPraxis/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/k8nstantin/OpenPraxis)](https://github.com/k8nstantin/OpenPraxis/blob/main/go.mod)
[![Go Report Card](https://goreportcard.com/badge/github.com/k8nstantin/OpenPraxis)](https://goreportcard.com/report/github.com/k8nstantin/OpenPraxis)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Release](https://img.shields.io/github/v/release/k8nstantin/OpenPraxis?include_prereleases&sort=semver)](https://github.com/k8nstantin/OpenPraxis/releases)

**Spec-driven development platform for autonomous coding agents.** Define products, write specs, and let independent agent sessions build them — with persistent memory, compliance enforcement, and independent execution auditing.

OpenPraxis is the operating system between you and your coding agents. It manages the full lifecycle: ideas become specs, specs become tasks, tasks execute autonomously, a watcher audits the output, and everything persists across sessions, agents, and machines.

## Table of contents

- [See it in action](#see-it-in-action)
  - [Dashboard — cost today vs. budget, tasks ranked by spend](#dashboard--cost-today-vs-budget-tasks-ranked-by-spend)
  - [Live tool output — watch the agent work, turn by turn](#live-tool-output--watch-the-agent-work-turn-by-turn)
  - [Products → Manifests → Tasks — every cost and every turn rolls up](#products--manifests--tasks--every-cost-and-every-turn-rolls-up)
  - [Visualize the plan — interactive DAG, status-colored](#visualize-the-plan--interactive-dag-status-colored)
  - [Every conversation, every tool call, every visceral-rule ack — searchable forever](#every-conversation-every-tool-call-every-visceral-rule-ack--searchable-forever)
  - [Independent evaluation — three gates an agent can't override](#independent-evaluation--three-gates-an-agent-cant-override)
  - [Hierarchical execution controls — budgets, turns, and agent knobs that inherit](#hierarchical-execution-controls--budgets-turns-and-agent-knobs-that-inherit)
- [How It Works](#how-it-works)
  - [The Workflow](#the-workflow)
- [Architecture](#architecture)
- [Key Concepts](#key-concepts)
  - [Products](#products) · [Manifests (Specs)](#manifests-specs) · [Tasks (Autonomous Execution)](#tasks-autonomous-execution) · [Watcher (Independent Auditor)](#watcher-independent-auditor)
  - [Ideas](#ideas) · [Memories](#memories) · [Visceral Rules](#visceral-rules) · [Conversations](#conversations) · [Chat](#chat)
- [Dashboard (16 Tabs)](#dashboard-16-tabs)
- [MCP Tools (44+)](#mcp-tools-44)
- [Quick Start](#quick-start)
  - [Prerequisites](#prerequisites) · [Build and Run](#build-and-run) · [Connect to Claude Code](#connect-to-claude-code) · [Dashboard](#dashboard)
- [Stats](#stats)
- [Database](#database)
- [Project Structure](#project-structure)
- [Hooks](#hooks)
- [Peer Sync](#peer-sync)
- [Configuration](#configuration)
- [Mobile App](#mobile-app)
- [License](#license)

**Deeper references:**
- [Execution Controls — all 12 knobs in detail](docs/execution-controls.md)

## See it in action

Most agent tools hand you a black box: you push "run", hope for the best, and find out what happened when the monthly bill lands. OpenPraxis inverts that. **Every turn, every tool call, every commit, every dollar — captured as it happens, aggregated upward, and surfaced on a single dashboard.** Cost is a first-class metric on the task, the manifest, the product, and the day. Activity is a first-class record: every prompt, every tool input, every tool result, every acknowledgement of a visceral rule. Independent evaluation is non-negotiable: the watcher audits every completed task and has the final say on "done."

The result is cost control by construction — you set a daily budget, you see spend accrue against it live, and runaway sessions can't hide behind a finished status.

### Dashboard — cost today vs. budget, tasks ranked by spend

<p align="center">
  <img src="docs/images/overview.png" alt="OpenPraxis dashboard overview — running task with live elapsed time, daily cost vs budget, tasks-today ranked by cost" width="100%" />
</p>

One glance tells you the entire cost story of the day:

- **Running Tasks** — live card with agent, action count, elapsed time, pause/stop/emergency-stop buttons. A runaway task is one click away from dead.
- **Cost Today ($16.24 / $100)** — current spend against the daily budget you set as a visceral rule. Crosses into red the moment you exceed it.
- **Turns Today (438)** — total agent turns billed today across every task.
- **Tasks (147) · Memories (134) · Nodes (1) · Uptime** — the state of the platform at a glance.
- **Productivity (99 A)** — score derived from lines-of-code changed per dollar per task, letter-graded.
- **Tasks Today — By Cost** — every task that ran today, sorted by cost. Columns: marker, title, branch, turns, cost, status. No ranking favourite, no scrolling through 200 tasks to find the expensive one.

### Live tool output — watch the agent work, turn by turn

<p align="center">
  <img src="docs/images/tasks-live-output.png" alt="Task detail with live bash, read, and edit tool calls streaming as the agent runs" width="100%" />
</p>

Open any running task and every tool call streams in as the agent makes it:

- **Bash** — full command, working directory, stdout, stderr, exit code.
- **Read / Edit / Write** — file path, line ranges, diffs.
- **Web fetch, Grep, Glob** — full query and result.
- **Turn counter** — each agent turn is numbered and costed individually, so you can see exactly where the session got expensive.
- **Pause (SIGSTOP) / Stop / Emergency Stop All** — freeze or kill at any point; no waiting for the agent to decide.

Every action row is stored and searchable forever. "What did the agent actually edit on Thursday?" is answerable.

### Products → Manifests → Tasks — every cost and every turn rolls up

<table>
  <tr>
    <td width="50%"><img src="docs/images/products-detail.png" alt="Product detail showing three linked manifests with aggregate cost, tasks, turns, and status" /></td>
    <td width="50%"><img src="docs/images/manifests-detail.png" alt="Manifest detail with four executed tasks inline, each showing status, cost, turns, and runs" /></td>
  </tr>
  <tr>
    <td align="center"><sub><b>Products</b> group manifests under one initiative. Header shows <b>Manifests / Tasks / Turns / Cost</b> aggregated across every child — spend per initiative, not guesswork.</sub></td>
    <td align="center"><sub><b>Manifests</b> are markdown specs. Every executed task is linked inline with its status, cost, turns, run count, and branch — trace any line in the spec to the task that implemented it.</sub></td>
  </tr>
</table>

Hierarchy: **Product → Manifest → Task → Run → Action**. Costs and turns aggregate at every level. Drill from "my product cost me $12" → "this manifest accounted for $8" → "this one task is $6" → "this one agent turn burned $3" → "here's the exact prompt and tool call."

### Visualize the plan — interactive DAG, status-colored

<p align="center">
  <img src="docs/images/product-dag-openpraxis.png" alt="Product DAG — product at top, manifests as blue-edged nodes, task chains below with green/red status colors" width="100%" />
</p>

Cytoscape.js renders every product as an interactive directed acyclic graph:

- **Purple product node** at the top.
- **Manifest nodes** linked by **blue manifest-dep edges** — the build order.
- **Task nodes** under each manifest linked by **yellow task-dep edges** — the execution chain.
- **Status colors** — green done, gray pending, red failed, amber in flight.
- **Zoom, pan, click to drill.** Every node is reachable by URL (`#view-products/<id>/dag`) so diagrams are shareable.

### Every conversation, every tool call, every visceral-rule ack — searchable forever

<p align="center">
  <img src="docs/images/conversations-detail.png" alt="Conversation detail — an agent session's visceral rule acknowledgement at session start, plus tool calls" width="100%" />
</p>

Every agent session is captured as a conversation:

- **Every turn** — prompt, response, tool call, tool result, thinking block.
- **Visceral-rule acknowledgements** — an agent that skips the `visceral_rules` → `visceral_confirm` handshake is flagged as **amnesia** on the dashboard (look at the badge in the nav — 10000 violations shown here).
- **Semantic search** — 768-dim embeddings via Ollama. "Find the session where we fixed the sqlite busy_timeout bug" is a similarity query, not a filename search.
- **Per-peer, per-session grouping** — every conversation is tagged with the peer that ran it, so multi-machine teams get one unified history.

### Independent evaluation — three gates an agent can't override

<p align="center">
  <img src="docs/images/watcher-audit-history.png" alt="Watcher audit history — 53 total audits, 24 passed, 29 failed, per-task verdict with git, build, and manifest checks" width="100%" />
</p>

The watcher is a **separate process** outside every agent session. After a task finishes, it runs three gates:

1. **Git gate** — did the agent produce commits on the task branch? (Zero commits = instant fail. No "I did the work, I just didn't commit it" excuses.)
2. **Build gate** — does the code compile? (`go build ./...` here; configurable per language.)
3. **Manifest gate** — were the deliverables addressed? (Parses the manifest markdown into a checklist, scores how many items were actually touched in the diff.)

The screenshot shows 53 total audits, 24 passed, 29 failed, 45 % pass rate — and the **Nice-to-have: Polish** manifest's tasks all green on all three gates. A task marked "completed" by the agent runner gets **downgraded to failed** if any gate fails. Nothing an agent can say overrides the watcher.

The three gates together enforce what an outside reviewer would check: code exists, code builds, code addresses the spec. Agents don't self-grade. **The watcher does.**

### Hierarchical execution controls — budgets, turns, and agent knobs that inherit

<table>
  <tr>
    <td width="33%"><img src="docs/images/exec-controls-product.png" alt="Product detail — Execution Controls panel with 12 knobs at product scope (max_parallel, max_turns, temperature, daily_budget_usd, etc.), soft-cap warning shown on daily_budget_usd" /></td>
    <td width="33%"><img src="docs/images/exec-controls-manifest.png" alt="Manifest detail — same 12 knobs at manifest scope, showing which inherit from product vs. locally overridden" /></td>
    <td width="33%"><img src="docs/images/exec-controls-task.png" alt="Task detail — same 12 knobs at task scope, narrowest override point with inheritance provenance" /></td>
  </tr>
  <tr>
    <td align="center"><sub><b>Product</b> — set the wide default once</sub></td>
    <td align="center"><sub><b>Manifest</b> — override for a feature area</sub></td>
    <td align="center"><sub><b>Task</b> — override for a single run</sub></td>
  </tr>
</table>

Twelve execution knobs — `max_parallel`, `max_turns`, `max_cost_usd`, `daily_budget_usd`, `timeout_minutes`, `temperature`, `reasoning_effort`, `default_agent`, `default_model`, `retry_on_failure`, `approval_mode`, `allowed_tools` — configurable at **four scopes with inheritance**: **task → manifest → product → system**. Set a product-wide budget once; every task under it inherits. Override one manifest's `max_turns` for a hard job; the product default still covers the rest.

- **Inline provenance** — every knob shows where the value came from ("inherited from product X") so you know which scope to edit to change it everywhere vs. here.
- **Reset to inherited** — one click drops a local override, reverts to the scope above.
- **Soft-cap warnings** — push `daily_budget_usd` above the visceral rule's hard cap and the UI warns before save; the runner clamps at runtime regardless.
- **Debounced save** — slide the value, it saves 300 ms after you stop; no "Apply" button to forget.
- **Agent-readable** — the MCP `settings_resolve` tool returns the effective value walking the inheritance chain, so agents query the budget instead of guessing.

Storage: a single `settings` table with `(scope_type, scope_id, key)` primary key. The resolver walks task → linked manifests → product → system on every read, clamped by visceral rules. Nine HTTP endpoints (`/api/settings/...`) mirror the four MCP tools for dashboard use.

📖 **[Full reference: all 12 knobs, types, ranges, and enforcement](docs/execution-controls.md)** — what each knob does, how the runner uses it, and which scope typically owns it.

## How It Works

```
You have an idea
     |
     v
  [Idea] -----> [Manifest] -----> [Task] -----> [Agent Session]
  capture it     write the spec    schedule it    autonomous execution
     |                |                |                |
     v                v                v                v
  [Product]      [Dependencies]   [Dependency      [Watcher]
  group specs    chain manifests   chain tasks]     audit: git, build,
  track cost     set build order   sequential       manifest compliance
                                   execution        agent can't self-grade
```

### The Workflow

1. **Capture ideas** — save feature requests, bugs, improvements with priority and tags
2. **Write manifests** — detailed markdown specs with deliverables, architecture, requirements
3. **Organize into products** — group manifests under products. Set manifest dependencies for build order
4. **Create tasks** — break manifests into executable tasks with dependency chains
5. **Execute autonomously** — the task runner spawns independent agent sessions (Claude Code, Cursor) with the manifest spec, visceral rules, and relevant memories as context
6. **Audit independently** — the watcher (server-side, outside the agent) checks every completed task: did the agent commit code? does it compile? were deliverables addressed?
7. **Enforce rules** — visceral rules are non-negotiable constraints every agent must acknowledge. Violations are flagged automatically as amnesia
8. **Share across sessions** — every decision, conversation, and tool call persists. New sessions inherit context from previous ones. Multiple machines sync via peer-to-peer

## Architecture

```
                          You
                           |
              +-----------+-----------+
              |           |           |
         Claude Code    Cursor     Copilot
              |           |           |
              +-----+-----+-----+----+
                    |           |
               stdio MCP    hooks
                    |           |
              +-----+-----------+--------+
              |         OpenPraxis         |
              |                          |
              |  Products  > Manifests   |
              |  Manifests > Tasks       |
              |  Tasks     > Execution   |
              |  Watcher   > Audit       |
              |  Memory    > Persistence |
              |  Rules     > Compliance  |
              |  Ideas     > Pipeline    |
              |  Chat      > Dashboard   |
              |                          |
              +-----+----------+---------+
                    |          |
              SQLite+vec    Dashboard
              (memories.db) (localhost:8765)
                    |
              Peer Sync (mDNS + Automerge)
                    |
              Other Machines
```

OpenPraxis is a single Go binary that runs as:
1. **MCP server** (stdio) — spawned by coding agents as a subprocess, exposing 40+ tools
2. **HTTP server** — dashboard UI, REST API, WebSocket events, hook handler
3. **Task runner** — spawns autonomous agent sessions from scheduled tasks
4. **Watcher** — independent server-side auditor that agents cannot override
5. **Peer node** — mDNS discovery + Automerge CRDT sync with other OpenPraxis instances

## Key Concepts

### Products
Top-level organizational hierarchy. A product groups related manifests into a single initiative with aggregated cost, turns, and task status. Visualized as an interactive DAG (Cytoscape.js) showing the manifest dependency chain with tasks below each manifest.

**Hierarchy:** Product > Manifest > Task > Run

### Manifests (Specs)
Development specs — detailed markdown documents describing a feature, module, or refactor. Manifests have:
- **Versions** — every update bumps the version
- **Statuses** — draft, open, closed, archive
- **Dependencies** — manifests chain to each other (`depends_on`). The scheduler blocks tasks from a manifest until its dependency manifests are closed
- **Linked tasks** — many-to-many relationship
- **Linked ideas** — trace which idea spawned which spec
- **Deliverables** — extracted from markdown checklists, verified by the watcher

### Tasks (Autonomous Execution)
Scheduled work units linked to manifests. The execution engine:
1. Picks up due tasks from the scheduler
2. Spawns an independent agent session (Claude Code subprocess)
3. Injects: manifest spec, visceral rules, relevant memories, allowed tools, git workflow instructions
4. Captures all output, tool calls, and costs in real-time
5. Supports: dependency chains (`depends_on`), pause/resume (SIGSTOP/SIGCONT), max turns, recurring schedules
6. On completion: watcher audit runs independently

### Watcher (Independent Auditor)
Server-side execution auditor. Runs **outside** the agent session — the agent has no visibility or control. Three gates on every completed task:

1. **Git Gate** — did the agent create commits on the task branch? (Zero commits = instant fail)
2. **Build Gate** — does the code compile? (`go build ./...`)
3. **Manifest Gate** — were the deliverables addressed? (Scores deliverables found in diff)

If any gate fails, the task is downgraded from "completed" to "failed" — the agent has no say in the verdict.

### Ideas
Capture product ideas, feature requests, bugs, and improvements with priority levels (low/medium/high/critical) and tags. Ideas link to manifests — trace which idea became which spec, and track from concept to execution.

### Memories
Semantic, path-organized knowledge stored with embeddings for vector search. Memories persist across sessions and agents — decisions, patterns, bugs, and constraints survive.

```
/project/openpraxis/bugs/macos-codesign-required
/project/openpraxis/audit/task-e33-slog-postmortem
/personal/gryphon-data-lake/testing/preferences
```

Memories have scopes (personal, project, team), types (insight, decision, pattern, bug, context, reference), and tiered responses (L0 one-liner, L1 summary, L2 full content).

### Visceral Rules
Non-negotiable operating constraints set by the user. Every agent session must call `visceral_rules` then `visceral_confirm` before doing any work. Violations are flagged as **amnesia** on the dashboard. Examples:

- "Never fix scripts on the fly — fix the original source"
- "Every new code modification is a new branch"
- "Daily budget = $100"
- "SQLite must always use WAL mode + busy_timeout=5000"
- "Never start or fire a task unless the user explicitly says to start it"

### Conversations
Every agent session's tool calls and interactions are captured — searchable by semantic similarity and browsable by date, agent, and project. Full audit trail of what every agent did.

### Chat
Multi-provider AI chat built into the dashboard. Supports Anthropic (Claude), Google (Gemini), OpenAI (GPT-4), and Ollama (local models) with streaming, tool use, vision, and extended thinking.

## Dashboard (16 Tabs)

| Tab | What it shows |
|-----|---------------|
| **Overview** | Running tasks, total cost, turns, session count, peer list |
| **Memories** | Semantic search, path browser, memory tree, tiered content |
| **Conversations** | All captured sessions, searchable by similarity |
| **Actions** | Every tool call, searchable by tool name/input |
| **Activity** | Real-time feed across all sessions and peers |
| **Visceral** | Rules list, pattern detection, confirmation tracking |
| **Compliance** | Amnesia (rule violations), delusions (factual errors) |
| **Ideas** | Capture, prioritize, link to manifests |
| **Products** | Interactive DAG (Cytoscape.js), cost aggregates, manifest chains |
| **Manifests** | Specs with dependencies, linked tasks/ideas, version history |
| **Tasks** | Status, dependency chains, run history, real-time output, pause/resume |
| **Cost** | Daily/weekly trends, agent breakdown, spend forecasting |
| **Watcher** | Audit results (git/build/manifest gates), pass/fail rates |
| **Recall** | Soft-deleted items, restorable |
| **Chat** | Multi-provider AI chat with model switching and thinking toggle |
| **Settings** | Profile, agent integrations, chat provider config |

## MCP Tools (40+)

| Category | Tools |
|----------|-------|
| **Memory** | `memory_store`, `memory_search`, `memory_recall`, `memory_list`, `memory_forget`, `memory_status`, `memory_peers` |
| **Conversations** | `conversation_save`, `conversation_search`, `conversation_list`, `conversation_get` |
| **Visceral** | `visceral_rules`, `visceral_confirm`, `visceral_set`, `visceral_remove` |
| **Manifests** | `manifest_create`, `manifest_get`, `manifest_list`, `manifest_update`, `manifest_delete`, `manifest_search` |
| **Products** | `product_create`, `product_get`, `product_list`, `product_update`, `product_delete` |
| **Tasks** | `task_create`, `task_list`, `task_get`, `task_start`, `task_pause`, `task_resume`, `task_cancel`, `task_link_manifest`, `task_unlink_manifest` |
| **Ideas** | `idea_add`, `idea_list`, `idea_update`, `link_idea_manifest`, `unlink_idea_manifest` |
| **Markers** | `marker_flag`, `marker_list`, `marker_done` |
| **Settings** | `settings_catalog`, `settings_get`, `settings_set`, `settings_resolve` (task/manifest/product/system scoped knobs with inheritance) |

## Quick Start

### Prerequisites
- Go 1.22+
- [Ollama](https://ollama.ai) with `nomic-embed-text` model (for semantic search)

```bash
# Install Ollama and pull the embedding model
ollama pull nomic-embed-text
```

### Build and Run

```bash
make build          # Build binary (includes macOS codesign)
./openpraxis serve    # Start dashboard + MCP + sync (port 8765)
```

### Connect to Claude Code

Add to your project's `.mcp.json`:

```json
{
  "mcpServers": {
    "openpraxis": {
      "command": "/path/to/openpraxis",
      "args": ["mcp"]
    }
  }
}
```

Claude Code will spawn OpenPraxis as a subprocess. On first session, the agent receives instructions to call `visceral_rules` + `visceral_confirm` before any work.

### Dashboard

Open `http://localhost:8765`.

## Stats

| Metric | Count |
|--------|-------|
| Go source files | 78+ |
| Go lines of code | ~18,000 |
| Dashboard JS | ~6,000 lines (modular: api.js, tree.js, lifecycle.js, 16 view modules) |
| MCP tools | 44+ |
| REST API endpoints | 79+ |
| SQLite tables | 32 |
| Dashboard tabs | 16 |

## Database

SQLite with WAL mode. Single file at `~/.openpraxis/data/memories.db`.

**Core:** `memories`, `conversations`, `manifests`, `tasks`, `task_runs`, `products`, `ideas`, `actions`, `sessions`

**Compliance:** `amnesia`, `delusions`, `visceral_confirmations`, `watcher_audits`, `rule_patterns`

**Vector search:** `vec_memories`, `vec_conversations` (sqlite-vec extension, 768-dim cosine similarity)

**Join tables:** `task_manifests`, `idea_manifest_links`, `task_runtime_state`

## Project Structure

```
cmd/                        CLI commands (cobra): serve, mcp, version, chatbridge
internal/
  action/                   Action recording + visceral/delusion compliance
  chat/                     Multi-provider AI chat (Anthropic, Google, OpenAI, Ollama)
  config/                   YAML config loader + platform detection
  conversation/             Conversation storage + vector search
  embedding/                Ollama embedding client (nomic-embed-text, 768-dim)
  idea/                     Ideas store with manifest linking
  manifest/                 Manifest CRUD + dependency chains + delusion detection
  marker/                   Flag/notification system between peers
  mcp/                      MCP server (stdio + streamable HTTP), 40+ tool handlers
  memory/                   Memory store + tiered responses (l0/l1/l2) + vector search
  node/                     Node orchestrator — wires all stores together
  peer/                     mDNS peer discovery + Automerge CRDT sync
  product/                  Product hierarchy (product > manifest > task)
  setup/                    Agent detection + auto-configuration
  task/                     Task runner, scheduler, repository, metrics, runtime state
  watcher/                  Independent task execution auditor (3 gates)
  web/                      HTTP handlers, WebSocket hub, embedded dashboard
    ui/                     Static frontend (HTML, CSS, vanilla JS, Cytoscape.js)
mobile/                     React Native (Expo) companion app
tools/                      Python utility scripts
```

## Hooks

OpenPraxis registers Claude Code hooks in `~/.claude/settings.json`:

| Hook | Trigger | What it does |
|------|---------|-------------|
| PostToolUse (`*`) | Every tool call | Records action, checks visceral compliance + manifest delusion |
| PreToolUse (`mcp__openpraxis__*`) | OpenPraxis tool calls | Enforces visceral rules are loaded first |
| UserPromptSubmit | User sends message | Tracks session activity |
| Stop | Session ends | Checks compliance, flags amnesia, saves conversation |
| SessionEnd | Session cleanup | Saves conversation from transcript |

All hooks hit `http://127.0.0.1:8765/api/hook`.

## Peer Sync

OpenPraxis instances discover each other via mDNS on the local network and synchronize using Automerge CRDTs. Each node has a stable UUID v7 identity and MAC fingerprint.

```
Laptop (OpenPraxis) <--mDNS--> Desktop (OpenPraxis) <--mDNS--> Server (OpenPraxis)
```

Memories, conversations, manifests, and visceral rules sync automatically. Each peer maintains its own SQLite database — sync is eventually consistent.

## Configuration

Default config at `~/.openpraxis/config.yaml`:

```yaml
node:
  uuid: "auto-generated-uuidv7"
  display_name: "Your Name"
  email: "you@example.com"
  avatar: "\U0001F989"
server:
  host: 127.0.0.1
  port: 8765
  open_browser: true
sync:
  host: 0.0.0.0
  port: 8766
embedding:
  ollama_url: http://localhost:11434
  model: nomic-embed-text
  dimension: 768
storage:
  data_dir: ~/.openpraxis/data
```

## Mobile App

React Native (Expo) companion app — acts as a full OpenPraxis peer from your phone. Create manifests, schedule tasks, monitor execution, browse memories. Syncs to laptop peers over local network.

```bash
cd mobile
npm install
npx expo start
```

## License

Apache License 2.0. See [LICENSE](LICENSE).
