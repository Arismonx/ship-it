# ADR-0010: One core, several surfaces

**Status:** accepted

## Decision
| Surface | Transport | Audience |
|---|---|---|
| CLI / TUI | direct call | seniors, scripts, CI |
| VS Code extension (TypeScript) | ~~`ship rpc`~~ → **`ship mcp --attended`** (revision of 2026-10-23, user choice) | juniors, designers, everyone |
| MCP server | `ship mcp`: stdio, **dual-era** (legacy spec ≤ 2025-11-25 and modern 2026-07-28) | AI agents |
| Daemon | same process, loop | autopilot |

## Rules
- Zero business logic in the surfaces.
- MCP tools are **idempotent**, with no prompt.
- **Every PR created by an agent (MCP) or by the daemon is a draft** (user choice): marking it "ready for review" is the human gesture. Anything that would require another confirmation returns exit code `2` ("human action required").
## Revision of 2026-10-23: the extension speaks MCP
- A single protocol and a single tested server: the VS Code extension calls the **same tools** as the agents. Its TypeScript types are generated from the `outputSchema`s (`go run ./tools/gen`), and a **contract test** makes its client talk to the real binary, in CI. `ship rpc` will not exist.
- The **`--attended`** mode, which only the extension starts, marks a human interface: there, the human has reviewed the plan. PRs therefore follow the settings instead of being forced into draft, and `ship_undo` accepts `allowRemote` after a confirmation in VS Code. Agents get `ship mcp` without this mode, which the extension also registers with VS Code: the guardrails below apply to them.

## Implemented (2026-10-23): `ship mcp`
- **Dual-era over stdio** ([`internal/surface/mcp`](../../internal/surface/mcp/server.go)), based on the spec reread the same day (V38):
  - a request with `_meta["io.modelcontextprotocol/protocolVersion"]` is served **statelessly** in 2026-07-28; `clientCapabilities` is required (`-32602` otherwise); an unknown version returns `-32022` with `supported`/`requested`; every result carries `resultType: "complete"` and `_meta.serverInfo`; `server/discover` gives versions, capabilities and instructions;
  - an `initialize` (2025-11-25 or 2025-06-18) opens a **legacy** session; without either, the error names the versions served;
  - one JSON line per message on stdout, nothing else; stops when stdin closes.
- **Tools** (choice of 2026-10-23): `ship_status`, `ship_plan`, `ship_log`, `ship_doctor` read-only; `ship_deliver` (local commit, the agent's title and branch **accepted but validated** against our conventions; branch with the normal prefix); `ship_publish` (**always a draft**); `ship_undo` (local only: a remote undo returns an error that leaves it to the human). A **deterministic** list, `ttlMs`/`cacheScope`, annotations (`readOnlyHint`, `destructiveHint`…).
- **`outputSchema`s generated** from the Go types ([`internal/schema`](../../internal/schema/reflect.go)): a single source for MCP and `--json`. The spec requires `structuredContent` to conform to it: a test validates every tool result against its schema.
- Business errors (`SHIP-E…`: secret, red gate, invalid title, consent required) are **tool execution errors** (`isError: true`, translated message and details), which the model can correct; an unknown tool is a protocol error.
- A plan written by the agent is recorded in the oplog (`ai_assistant: "host agent"`): the PR carries "AI-assisted (host agent)" (docs/reference/conventions.md), even when published from the CLI.
- **`ship mcp install`** (Claude Code first): adds `{"type":"stdio","command":"ship","args":["mcp"]}` under `mcpServers.ship-it` in the project's `.mcp.json`, **keeping the other servers and the key order**, after a preview and confirmation; undoable with `ship undo`. Claude Code then asks the user to approve the server.

- 2026-07-28 spec read (V2): the server answers `initialize` (legacy clients) **and** implements `server/discover` (mandatory, the compatibility probe over STDIO); version and capabilities read from `_meta` on every request; `resultType: "complete"`; `tools/list` ordered deterministically with `ttlMs`/`cacheScope`; schemas in JSON Schema 2020-12; logs on stderr (Logging is deprecated); no dependency on Roots or Sampling (deprecated).
