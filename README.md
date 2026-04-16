# OpenLoom

**Spec-driven development platform for autonomous coding agents.** Define products, write specs, and let independent agent sessions build them — with persistent memory, compliance enforcement, and independent execution auditing.

OpenLoom is the operating system between you and your coding agents. It manages the full lifecycle: ideas become specs, specs become tasks, tasks execute autonomously, a watcher audits the output, and everything persists across sessions, agents, and machines.

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
              |         OpenLoom         |
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

OpenLoom is a single Go binary that runs as:
1. **MCP server** (stdio) — spawned by coding agents as a subprocess, exposing 40+ tools
2. **HTTP server** — dashboard UI, REST API, WebSocket events, hook handler
3. **Task runner** — spawns autonomous agent sessions from scheduled tasks
4. **Watcher** — independent server-side auditor that agents cannot override
5. **Peer node** — mDNS discovery + Automerge CRDT sync with other OpenLoom instances

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
/project/openloom/bugs/macos-codesign-required
/project/openloom/audit/task-e33-slog-postmortem
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
./openloom serve    # Start dashboard + MCP + sync (port 8765)
```

### Connect to Claude Code

Add to your project's `.mcp.json`:

```json
{
  "mcpServers": {
    "openloom": {
      "command": "/path/to/openloom",
      "args": ["mcp"]
    }
  }
}
```

Claude Code will spawn OpenLoom as a subprocess. On first session, the agent receives instructions to call `visceral_rules` + `visceral_confirm` before any work.

### Dashboard

Open `http://localhost:8765`.

## Stats

| Metric | Count |
|--------|-------|
| Go source files | 78+ |
| Go lines of code | ~18,000 |
| Dashboard JS | ~6,000 lines (modular: api.js, tree.js, lifecycle.js, 16 view modules) |
| MCP tools | 40+ |
| REST API endpoints | 70+ |
| SQLite tables | 31 |
| Dashboard tabs | 16 |

## Database

SQLite with WAL mode. Single file at `~/.openloom/data/memories.db`.

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

OpenLoom registers Claude Code hooks in `~/.claude/settings.json`:

| Hook | Trigger | What it does |
|------|---------|-------------|
| PostToolUse (`*`) | Every tool call | Records action, checks visceral compliance + manifest delusion |
| PreToolUse (`mcp__openloom__*`) | OpenLoom tool calls | Enforces visceral rules are loaded first |
| UserPromptSubmit | User sends message | Tracks session activity |
| Stop | Session ends | Checks compliance, flags amnesia, saves conversation |
| SessionEnd | Session cleanup | Saves conversation from transcript |

All hooks hit `http://127.0.0.1:8765/api/hook`.

## Peer Sync

OpenLoom instances discover each other via mDNS on the local network and synchronize using Automerge CRDTs. Each node has a stable UUID v7 identity and MAC fingerprint.

```
Laptop (OpenLoom) <--mDNS--> Desktop (OpenLoom) <--mDNS--> Server (OpenLoom)
```

Memories, conversations, manifests, and visceral rules sync automatically. Each peer maintains its own SQLite database — sync is eventually consistent.

## Configuration

Default config at `~/.openloom/config.yaml`:

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
  data_dir: ~/.openloom/data
```

## Mobile App

React Native (Expo) companion app — acts as a full OpenLoom peer from your phone. Create manifests, schedule tasks, monitor execution, browse memories. Syncs to laptop peers over local network.

```bash
cd mobile
npm install
npx expo start
```

## License

Private. Not yet open source.
