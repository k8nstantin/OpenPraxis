# Security Policy

## Supported versions

OpenPraxis is pre-1.0 software under active development. Security fixes are
applied to the `main` branch and included in the next tagged release. Older
tagged releases do not receive backports.

| Version  | Supported |
|----------|-----------|
| `main`   | yes       |
| tagged releases | only the most recent |

## Reporting a vulnerability

Please **do not open a public GitHub issue** for security vulnerabilities.

Email disclosures to: **constantin@dedomena.io**

Include:

- A description of the vulnerability and its impact.
- Steps to reproduce (proof-of-concept welcome, but not required).
- The affected OpenPraxis version (`./openpraxis version`) and environment.
- Your contact information and whether you would like public credit after the
  fix ships.

You should receive an acknowledgement within **72 hours**. A fix timeline
depends on severity — expect a patch within **30 days** for high-severity
issues, longer for lower-severity or complex ones. You will be kept informed
as the fix is developed.

### Coordinated disclosure

Please give the maintainer a reasonable window to ship a fix before publishing
details. Once a patch is released, a security advisory will be published on
the GitHub repository and credit given to the reporter (unless anonymity is
requested).

## Scope

In scope:

- The `openpraxis` binary (`serve`, `mcp`, and any other subcommands).
- The HTTP dashboard and WebSocket endpoints served on port `8765`.
- The MCP server (stdio and HTTP transports) and its tool handlers.
- The Claude Code hooks installed by `internal/setup/agents.go`.
- The SQLite data store under `~/.openpraxis/data/`.

Out of scope:

- Vulnerabilities in upstream dependencies — please report those to the
  respective projects. If an upstream issue materially affects OpenPraxis,
  feel free to forward the advisory so we can track a bump.
- Social engineering of maintainers or contributors.
- Denial-of-service via resource exhaustion on a single local node
  (OpenPraxis is designed as a single-user local tool).

## Out-of-band hardening tips for operators

- OpenPraxis stores conversation transcripts, tool outputs, and API keys in
  the local SQLite DB. Treat `~/.openpraxis/data/` as sensitive.
- The dashboard binds to `127.0.0.1:8765` by default. Do not expose it to
  untrusted networks without an authenticating reverse proxy.
- MCP stdio transport inherits the parent process environment — keep provider
  API keys out of shell history and committed config.
