# OpenLoom

**Shared memory layer for coding agents.** Every AI coding session on your machine shares context, learns from past work, and enforces your rules — automatically.

OpenLoom sits between you and your coding agents (Claude Code, Cursor, Copilot, etc.), providing persistent memory, task orchestration, compliance monitoring, and peer-to-peer synchronization across all sessions.

## Why

Coding agents are stateless. Every session starts from zero — no memory of decisions, no awareness of parallel sessions, no enforcement of your rules. You repeat yourself constantly.

OpenLoom fixes this:
- **Memory persists.** Decisions, patterns, bugs, and constraints survive across sessions and agents.
- **Rules are enforced.** Visceral rules are non-negotiable operating constraints that every agent session must acknowledge — violations are flagged automatically.
- **Tasks chain and execute.** Define work as manifests (specs) with linked tasks that run sequentially or in parallel, with dependency chains and autonomous agent execution.
- **Sessions are visible.** Every tool call, every conversation, every cost — captured and searchable from a single dashboard.
- **Peers sync.** Multiple machines discover each other via mDNS and synchronize memories, conversations, and manifests in real-time.

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
              +-----+-----------+-----+
              |       OpenLoom        |
              |                       |
              |  Memory    Tasks      |
              |  Rules     Sessions   |
              |  Manifests Watcher    |
              |  Search    Actions    |
              |                       |
              +-----------+-----------+
                    |           |
              SQLite+vec    Dashboard
              (memories.db) (localhost:8765)
                    |
              Peer Sync (mDNS + Automerge)
                    |
              Other Machines
```

OpenLoom is a single Go binary that runs as:
1. **MCP server** (stdio) — spawned by Claude Code/Cursor as a subprocess, exposing 40+ tools
2. **HTTP server** — dashboard UI, REST API, WebSocket events, hook handler
3. **Peer node** — mDNS discovery + Automerge CRDT sync with other OpenLoom instances

## Key Concepts

### Memories
Semantic, path-organized knowledge stored with embeddings for vector search. Memories have scopes (personal, project, team), types (insight, decision, pattern, bug), and are searchable by natural language queries.

```
/project/openloom/bugs/macos-codesign-required
/project/openloom/audit/task-e33-slog-postmortem
/personal/gryphon-data-lake/testing/preferences
```

### Visceral Rules
Non-negotiable operating constraints set by the user. Every agent session must call `visceral_rules` then `visceral_confirm` before doing any work. Violations are flagged as **amnesia** on the dashboard. Examples:

- "Never fix scripts on the fly — fix the original source"
- "Every new code modification is a new branch"
- "Daily budget = $100"
- "SQLite must always use WAL mode + busy_timeout=5000"

### Manifests
Development specs — detailed markdown documents describing a feature, module, or refactor. Manifests have versions, statuses (draft, open, closed, archive), and linked tasks. Think of them as living design docs that agents execute against.

### Tasks
Scheduled work units linked to manifests. Tasks support:
- **Chaining** — `depends_on` field creates sequential execution chains. Task B waits until Task A completes before starting.
- **Autonomous execution** — the task runner spawns a Claude Code subprocess with the manifest spec, visceral rules, and relevant memories as context.
- **Watcher audit** — an independent server-side auditor checks every completed task for git commits, build success, and manifest compliance. Agents cannot self-grade.

### Products
Top-level organizational hierarchy: **Product > Manifest > Task**. Products aggregate cost, turns, and task status from all their manifests. Visualized as an interactive Cytoscape.js DAG with dependency edges.

### Conversations
Every agent session's tool calls and interactions are captured as conversations — searchable by semantic similarity and browsable by date, agent, and project.

### Watcher
Independent server-side task execution auditor. Runs three gates on every completed task:
1. **Git Gate** — did the agent create commits on the task branch?
2. **Build Gate** — does the code compile?
3. **Manifest Gate** — were the deliverables addressed?

If any gate fails, the task is downgraded from "completed" to "failed" — the agent has no say in the verdict.

## Stats

| Metric | Count |
|--------|-------|
| Go source files | 78 |
| Go lines of code | ~18,000 |
| Dashboard (app.js) | ~5,500 lines |
| MCP tools | 40+ |
| SQLite tables | 31 |
| Mobile app screens | 12 (React Native/Expo) |

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

Open `http://localhost:8765` — you'll see:
- **Overview** — running tasks, total cost, turns, top tasks by cost
- **Activity** — real-time feed of all agent actions across sessions
- **Memories** — semantic search, path browser, memory tree
- **Conversations** — all captured sessions, searchable
- **Manifests** — development specs with linked tasks
- **Tasks** — task status, dependency chains, run history, cost tracking
- **Products** — interactive DAG visualization (Cytoscape.js)
- **Compliance** — amnesia violations, delusions, watcher audits

## MCP Tools (40+)

| Category | Tools |
|----------|-------|
| **Memory** | `memory_store`, `memory_search`, `memory_recall`, `memory_list`, `memory_forget`, `memory_status`, `memory_peers` |
| **Conversations** | `conversation_save`, `conversation_search`, `conversation_list`, `conversation_get` |
| **Markers** | `marker_flag`, `marker_list`, `marker_done` |
| **Visceral** | `visceral_rules`, `visceral_confirm`, `visceral_set`, `visceral_remove` |
| **Manifests** | `manifest_create`, `manifest_get`, `manifest_list`, `manifest_update`, `manifest_delete`, `manifest_search` |
| **Products** | `product_create`, `product_get`, `product_list`, `product_update`, `product_delete` |
| **Tasks** | `task_create`, `task_list`, `task_get`, `task_start`, `task_pause`, `task_resume`, `task_cancel`, `task_link_manifest`, `task_unlink_manifest` |
| **Ideas** | `idea_add`, `idea_list`, `idea_update`, `link_idea_manifest`, `unlink_idea_manifest` |

## Database Schema

SQLite with WAL mode. Single file at `~/.openloom/data/memories.db`.

**Core tables:** `memories`, `conversations`, `manifests`, `tasks`, `task_runs`, `products`, `ideas`, `actions`, `sessions`

**Compliance tables:** `amnesia` (rule violations), `delusions` (manifest deviations), `visceral_confirmations`, `watcher_audits`

**Vector search:** `vec_memories`, `vec_conversations` (sqlite-vec extension, 768-dim cosine similarity)

**Join tables:** `task_manifests`, `idea_manifest_links`, `task_runtime_state`

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
  manifest/                 Manifest CRUD + delusion detection
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

## Mobile App

React Native (Expo) companion app — acts as a full OpenLoom peer from your phone. Create manifests, schedule tasks, monitor execution, browse memories. Syncs to laptop peers over local network.

```bash
cd mobile
npm install
npx expo start
```

## License

Private. Not yet open source.
