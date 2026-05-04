# CLAUDE.md — OpenPraxis

**Peer-to-peer workflow engine for autonomous coding agents.** One Go binary per peer (MCP stdio + HTTP dashboard + mDNS discovery + Automerge sync). Three-tier DAG (product → manifest → task) drives execution; tasks spawn LLM agent subprocesses in isolated git worktrees off fresh `origin/main`.

→ **Full design doc:** [`docs/workflow-engine.md`](docs/workflow-engine.md) — states, deps, activation model, review loop, SCD principle, and the roadmap. Every PR touching those primitives must check itself against that doc.

## Session Init — Total Recall Protocol

**Every session MUST execute this before any work:**

1. Call `visceral_rules` — load mandatory operating rules
2. Call `visceral_confirm` with the rule count — acknowledge rules
3. Call `memory_search` with current project/task context — load relevant prior knowledge
4. Call `manifest_list` — know what active specs exist

Or call `totalrecall` which does all of the above in one shot.

**These are not optional.** Skipping them results in amnesia flagging on the dashboard.

## Task System

OpenPraxis has a task entity. **Never use CronCreate or external schedulers.**

Hierarchy: `peer → manifest → task`

- Manifests are specs/plans with linked tasks
- Tasks have states: `waiting`, `scheduled`, `running`, `completed`, `failed`
- Tasks support `depends_on` for sequential chains
- Use OpenPraxis MCP tools for all task/manifest operations

## Build

```bash
make build          # Build binary → ./openpraxis (always use this — builds React frontend first)
make run            # Build + run server (port 9966)
make test           # Run all tests
```

**Never use bare `go build` — it skips the React frontend build and embeds a stub dist.**

The binary is also used as an MCP server via `./openpraxis mcp` (stdio transport).

## Architecture

```
cmd/                    CLI commands (cobra): serve, mcp, version
internal/
  action/               Action recording + visceral/delusion compliance checks
  config/               YAML config loader + platform detection
  conversation/         Conversation storage (SQLite)
  embedding/            Ollama embedding client (nomic-embed-text)
  idea/                 Ideas store (many-to-many manifest linking)
  manifest/             Manifest CRUD + marker tracking
  mcp/                  MCP server (stdio + streamable HTTP), all tool handlers
  memory/               Memory store + SQLite-vec semantic search
  node/                 Node orchestrator (wires all stores together)
  peer/                 mDNS peer discovery + Automerge sync
  schedule/             SCD-2 schedules table + cron-driven consumer
  setup/                Agent detection + auto-configuration (Claude Code, Cursor, etc.)
  task/                 Task runner + scheduler + prompt builder
  web/                  HTTP dashboard + WebSocket + hook handler
    ui/dashboard-v2/    React dashboard (Vite + React 19 + Tailwind v4 + TanStack Router)
mobile/                 React Native (Expo) companion app
tools/                  Python utility scripts
```

## Key Files

- `internal/mcp/server.go` — MCP server init, tool registration, session tracking, instructions
- `internal/mcp/tools.go` — All MCP tool handlers (memory, conversation, marker, totalrecall)
- `internal/mcp/visceral.go` — Visceral rule tools + manifest/idea/link tools
- `internal/web/handler.go` — HTTP API routes, hook handler, embedded React dashboard
- `internal/task/runner.go` — Task scheduler, runner, prompt context builder
- `internal/schedule/runner.go` — Cron-driven schedules consumer
- `internal/node/node.go` — Node init, wires stores + DB
- `internal/setup/agents.go` — Agent detection + MCP config writer + hooks setup
- `cmd/serve.go` — Server startup, scheduler callbacks, peer registry

## Hooks

OpenPraxis registers Claude Code hooks in `~/.claude/settings.json`:

- **PostToolUse** (`*`): Records every tool call as an action, checks visceral compliance + manifest delusion
- **PreToolUse** (`mcp__openpraxis__*`): Enforces visceral rules are loaded before any OpenPraxis MCP tool call
- **UserPromptSubmit**: Tracks session activity
- **Stop**: Checks visceral compliance, flags amnesia if rules weren't confirmed, saves conversation
- **SessionEnd**: Saves conversation from transcript

All hooks hit `http://127.0.0.1:9966/api/hook`.

## MCP Tools (55)

Memory: `memory_store`, `memory_search`, `memory_recall`, `memory_list`, `memory_forget`, `memory_status`, `memory_peers`
Conversations: `conversation_save`, `conversation_search`, `conversation_list`, `conversation_get`
Markers: `marker_flag`, `marker_list`, `marker_done`
Visceral: `visceral_rules`, `visceral_confirm`, `visceral_set`, `visceral_remove`
Products: `product_create`, `product_get`, `product_list`, `product_update`, `product_delete`, `product_dep_add`, `product_dep_remove`, `product_dep_list`
Manifests: `manifest_create`, `manifest_get`, `manifest_list`, `manifest_update`, `manifest_delete`, `manifest_search`, `manifest_dep_add`, `manifest_dep_remove`, `manifest_dep_list`
Tasks: `task_create`, `task_list`, `task_get`, `task_start`, `task_cancel`, `task_link_manifest`, `task_unlink_manifest`
Ideas: `idea_add`, `idea_list`, `idea_update`
Links: `link_idea_manifest`, `unlink_idea_manifest`
Meta: `totalrecall`

## Config

Default config: `~/.openpraxis/config.yaml`

```yaml
node:
  uuid: "auto-generated"
  display_name: "Your Name"
server:
  host: 127.0.0.1
  port: 9966
sync:
  host: 0.0.0.0
  port: 6699
embedding:
  ollama_url: http://localhost:11434
  model: nomic-embed-text
```

## Dashboard

`http://localhost:9966` — React dashboard (Portal V2): Overview, Tasks, Products, Manifests, Audit, Activity, Settings.
`http://localhost:9966/api/*` — REST API
`http://localhost:9966/mcp` — MCP HTTP transport (agent configs point here)
`http://localhost:9966/ws` — WebSocket broadcast hub
