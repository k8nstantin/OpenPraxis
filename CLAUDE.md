# CLAUDE.md — OpenPraxis

Shared memory layer for coding agents. Go binary providing stdio MCP server, HTTP dashboard, peer discovery, and task scheduling.

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
make build          # Build binary → ./openpraxis
make run            # Build + run server (port 8765)
make test           # Run all tests
```

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
  marker/               Flag/notification system between peers
  mcp/                  MCP server (stdio + streamable HTTP), all tool handlers
  memory/               Memory store + SQLite-vec semantic search
  node/                 Node orchestrator (wires all stores together)
  peer/                 mDNS peer discovery + Automerge sync
  setup/                Agent detection + auto-configuration (Claude Code, Cursor, etc.)
  task/                 Task runner + scheduler + prompt builder
  web/                  HTTP dashboard + WebSocket + hook handler
    ui/                 Embedded static files (HTML, CSS, JS)
mobile/                 React Native (Expo) companion app
tools/                  Python utility scripts
```

## Key Files

- `internal/mcp/server.go` — MCP server init, tool registration, session tracking, instructions
- `internal/mcp/tools.go` — All MCP tool handlers (memory, conversation, marker, totalrecall)
- `internal/mcp/visceral.go` — Visceral rule tools + manifest/idea/link tools
- `internal/web/handler.go` — HTTP API routes, hook handler, dashboard data
- `internal/task/runner.go` — Task scheduler, runner, prompt context builder
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

All hooks hit `http://127.0.0.1:8765/api/hook`.

## MCP Tools (37)

Memory: `memory_store`, `memory_search`, `memory_recall`, `memory_list`, `memory_forget`, `memory_status`, `memory_peers`
Conversations: `conversation_save`, `conversation_search`, `conversation_list`, `conversation_get`
Markers: `marker_flag`, `marker_list`, `marker_done`
Visceral: `visceral_rules`, `visceral_confirm`, `visceral_set`, `visceral_remove`
Manifests: `manifest_create`, `manifest_get`, `manifest_list`, `manifest_update`, `manifest_delete`, `manifest_search`
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

`http://localhost:8765` — Activity feed, memories, manifests, tasks, sessions, visceral compliance, amnesia flags.
