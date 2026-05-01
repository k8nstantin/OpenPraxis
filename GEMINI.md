# GEMINI.md — OpenPraxis

**Peer-to-peer workflow engine for autonomous coding agents.** One Go binary per peer (MCP stdio + HTTP dashboard + mDNS discovery + Automerge sync). Three-tier DAG (product → manifest → task) drives execution; tasks spawn LLM agent subprocesses in isolated git worktrees off fresh `origin/main`.

→ **Full design doc:** [`docs/workflow-engine.md`](docs/workflow-engine.md) — states, deps, activation model, review loop, SCD principle, and the roadmap. Every PR touching those primitives must check itself against that doc.

## Task System

OpenPraxis has a task entity. **Never use CronCreate or external schedulers.**

Hierarchy: `peer → manifest → task`

- Manifests are specs/plans with linked tasks
- Tasks have states: `waiting`, `scheduled`, `running`, `completed`, `failed`
- Tasks support `depends_on` for sequential chains
- Use OpenPraxis MCP tools for all task/manifest operations

## Build

```bash
make build          # Build binary → ./openpraxis
make run            # Build + run server (port 8765)
make test           # Run all tests
```

The binary is also used as an MCP server via `./openpraxis mcp` (stdio transport).

## Architecture

```
cmd/                    CLI commands (cobra): serve, mcp, version
internal/
  action/               Action recording
  config/               YAML config loader + platform detection
  conversation/         Conversation storage (SQLite)
  embedding/            Ollama embedding client (nomic-embed-text)
  idea/                 Ideas store (many-to-many manifest linking)
  manifest/             Manifest CRUD
  mcp/                  MCP server (stdio + streamable HTTP), all tool handlers
  memory/               Memory store + SQLite-vec semantic search
  node/                 Node orchestrator (wires all stores together)
  peer/                 mDNS peer discovery + Automerge sync
  schedule/             SCD-2 schedules table + cron-driven consumer
  setup/                Agent detection + auto-configuration (Gemini CLI, Claude Code, Cursor, etc.)
  task/                 Task runner + scheduler + prompt builder
  web/                  HTTP dashboard + WebSocket + hook handler
    ui/                 Embedded static files (HTML, CSS, JS)
mobile/                 React Native (Expo) companion app
tools/                  Python utility scripts
```

## Key Files

- `internal/mcp/server.go` — MCP server init, tool registration, session tracking, instructions
- `internal/mcp/tools.go` — All MCP tool handlers (memory, conversation, totalrecall)
- `internal/web/handler.go` — HTTP API routes, hook handler, dashboard data
- `internal/task/runner.go` — Task scheduler, runner, prompt context builder
- `internal/schedule/runner.go` — Cron-driven schedules consumer
- `internal/node/node.go` — Node init, wires stores + DB
- `internal/setup/agents.go` — Agent detection + MCP config writer + hooks setup
- `cmd/serve.go` — Server startup, scheduler callbacks, peer registry

## Hooks

OpenPraxis registers Gemini CLI hooks in `~/.gemini/settings.json`:

- **AfterTool**: Records every tool call as an action (equivalent to `PostToolUse`)
- **AfterAgent**: Tracks session activity (equivalent to `Stop`)
- **SessionEnd**: Saves conversation from transcript

All hooks hit `http://127.0.0.1:8765/api/hook` via `curl`.

## MCP Tools

Memory: `memory_store`, `memory_search`, `memory_recall`, `memory_list`, `memory_forget`, `memory_status`, `memory_peers`
Conversations: `conversation_save`, `conversation_search`, `conversation_list`, `conversation_get`
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
  name: "machine-name"
  data_dir: "~/.openpraxis"
server:
  port: 8765
embedding:
  provider: "ollama"
  model: "nomic-embed-text"
```

## Dashboard

`http://localhost:8765` — Activity feed, memories, manifests, tasks, sessions.

`http://localhost:9766` — Portal V2 (React + TanStack Router).
