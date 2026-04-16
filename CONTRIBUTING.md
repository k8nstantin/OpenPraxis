# Contributing to OpenPraxis

Thanks for considering a contribution. This guide covers the dev loop: building,
testing, submitting a pull request, and the conventions the project relies on
(branch-per-task, macOS codesigning, MCP + hooks).

## Prerequisites

- **Go 1.22+** (the module targets a recent Go — see `go.mod` for the exact
  version).
- **Ollama** with the `nomic-embed-text` model for semantic search:
  ```bash
  ollama pull nomic-embed-text
  ```
- **SQLite** — the vendored `mattn/go-sqlite3` driver is cgo-based, so a C
  toolchain is required (Xcode Command Line Tools on macOS, `build-essential`
  on Debian/Ubuntu).
- **git** and **make**.

## Build

```bash
make build          # go mod tidy + go build + codesign (on macOS)
make run            # build and run the server on port 8765
make test           # go test -v ./...
make build-all      # cross-compile darwin-arm64, darwin-amd64, linux-amd64
make clean          # remove built binaries
```

`make build` wraps the plain `go build` with the `-ldflags` needed to stamp
`Version`, `GitCommit`, and `BuildDate` into the binary. If you want to build
directly:

```bash
go build -o openpraxis .
```

### macOS codesign step (required)

Unsigned Go binaries are killed with SIGKILL ("Code Signature Invalid") by
recent macOS kernels when the loader lazily validates code pages. `make build`
handles this automatically with an ad-hoc codesign:

```bash
codesign --force --sign - openpraxis
```

If you build with plain `go build` on macOS, **you must run the `codesign`
command yourself** or the binary will crash as soon as a new code page is
touched at runtime. The same applies to every cross-compiled `openpraxis-*`
darwin artifact produced by `make build-all`.

Linux builds do not need codesigning.

## Test

```bash
make test           # all packages
go test ./internal/task/...
go test -run TestSchedulerCancelsRunningTask ./internal/task
```

Tests write to temporary SQLite databases — no external services are required.
If a test package touches the embedding client it will be skipped unless Ollama
is reachable; other tests are hermetic.

## Branch flow (mandatory)

**Every code modification goes on its own branch and its own pull request.**
Never reuse a branch across unrelated tasks, and never push to `main`.

- Branches created by the autonomous task runner use the pattern
  `openpraxis/<task-id>`.
- Human contributors should use a short, kebab-case topic name,
  e.g. `fix-task-scheduler-deps`, `add-env-example`.

```bash
git checkout main
git pull
git checkout -b <your-branch>
# ... make changes ...
git commit -m "<subject>"
git push -u origin <your-branch>
gh pr create --title "<title>" --body "<summary>"
```

## Commit style

One-line subject in the imperative mood, concise, no trailing period. Match the
recent history:

```
Add .env.example documenting all env vars
Rename Go module to github.com/k8nstantin/OpenPraxis
Fix task scheduler deps + cross-process pause/resume/cancel
```

If the change needs more context, add a blank line and a short body explaining
*why*. Keep the "what" in the diff.

## Pull request process

1. Open a PR against `main` with `gh pr create` (or the GitHub UI).
2. The title should be short (under ~70 characters) and match commit style.
3. Include a body with:
   - **Summary** — 1–3 bullets of what changed.
   - **Test plan** — how to verify the change (commands, screenshots for UI).
4. Run `make test` locally before pushing. A green build is expected.
5. One logical change per PR. Refactors, renames, and behavior changes belong
   in separate PRs whenever possible.
6. PRs from autonomous agent sessions include the run/branch ID in the title
   or body — please preserve those when editing.

## Repo layout

```
cmd/                CLI commands (cobra): serve, mcp, version
internal/
  action/           Action recording + visceral/delusion compliance
  chat/             Multi-provider AI chat (Anthropic/Google/OpenAI/Ollama)
  config/           YAML config loader + env overrides
  conversation/     Conversation storage + vector search
  embedding/        Ollama embedding client
  idea/             Ideas store + manifest linking
  manifest/         Manifest CRUD + dependency chains
  marker/           Flag/notification system between peers
  mcp/              MCP server (stdio + streamable HTTP), tool handlers
  memory/           Memory store + tiered responses + vector search
  node/             Node orchestrator — wires stores together
  peer/             mDNS discovery + Automerge CRDT sync
  product/          Product hierarchy (product > manifest > task)
  setup/            Agent detection + auto-configuration
  task/             Task runner, scheduler, repository
  watcher/          Independent task execution auditor
  web/              HTTP handlers, WebSocket hub, embedded dashboard
    ui/             Static frontend (HTML, CSS, JS)
mobile/             React Native (Expo) companion app
tools/              Python utility scripts
```

See `CLAUDE.md` for a denser architectural summary.

## MCP server

OpenPraxis is itself an [MCP](https://modelcontextprotocol.io) server. Any
coding agent that speaks MCP (Claude Code, Cursor, Copilot) spawns the binary
over stdio as a subprocess:

```bash
./openpraxis mcp       # stdio transport — not meant to be run interactively
./openpraxis serve     # HTTP dashboard + MCP HTTP transport on :8765
```

### Wiring the MCP server into Claude Code

Add an `.mcp.json` at the repo root (or in your own project):

```json
{
  "mcpServers": {
    "openpraxis": {
      "command": "/absolute/path/to/openpraxis",
      "args": ["mcp"]
    }
  }
}
```

The handlers live in `internal/mcp/tools.go` and `internal/mcp/visceral.go`.
Tool registration happens in `internal/mcp/server.go`.

### Adding a new MCP tool

1. Add the handler in the appropriate file under `internal/mcp/`.
2. Register it in `server.go` next to the existing tools.
3. Document it in `README.md` (the "MCP Tools" table) and `CLAUDE.md`.
4. Add a test — `internal/mcp/` has examples exercising handlers against an
   in-memory node.

## Hooks

OpenPraxis installs Claude Code hooks at `~/.claude/settings.json` via
`internal/setup/agents.go`. They POST to the running dashboard at
`http://127.0.0.1:8765/api/hook` — the handler is in
`internal/web/handler.go`.

| Hook | Trigger | Purpose |
|------|---------|---------|
| `PreToolUse` (`mcp__openpraxis__*`) | Before any OpenPraxis MCP call | Enforces that `visceral_rules` has been loaded |
| `PostToolUse` (`*`) | Every tool call | Records the action, checks visceral + manifest compliance |
| `UserPromptSubmit` | User sends a message | Tracks session activity |
| `Stop` | Session stops | Flags amnesia if rules weren't confirmed, saves the conversation |
| `SessionEnd` | Session cleanup | Persists the transcript |

If you change hook behavior, update both `internal/setup/agents.go` (writer)
and `internal/web/handler.go` (receiver). The full hook payload contract lives
alongside the handler.

## Visceral rules (operational constraints)

Every agent session must call `visceral_rules` then `visceral_confirm` before
doing any work. A few rules that materially affect contributions:

- **Every code modification is a new branch.** See *Branch flow* above.
- **SQLite must use WAL mode with `busy_timeout=5000`.** Multiple processes
  share the same DB file. Never open a DB without these pragmas.
- **Operational data must persist to SQLite.** Anything the dashboard shows
  (metrics, costs, run history, task output) has to survive a restart — don't
  hold it in memory only.
- **Never start or fire a task without explicit user approval.** After
  scheduling a task, ask before firing.

These rules are the canonical list — check `visceral_rules` at runtime if you
want the authoritative set.

## Reporting bugs / requesting features

Please file GitHub Issues for bugs and feature requests. For a bug, include:

- OpenPraxis version (`./openpraxis version`).
- OS and architecture.
- Steps to reproduce, expected vs. actual behavior.
- Relevant output from `~/.openpraxis/data/` logs or the dashboard.

Security issues: please email the maintainer rather than opening a public
issue.

## License

By contributing you agree that your changes are licensed under the Apache
License 2.0, matching the rest of the project.
