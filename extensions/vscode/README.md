# ship-it for VS Code

Status: v0.1 (phase 5). Design: [docs/design/vscode-extension.md](../../docs/design/vscode-extension.md).

## What it does
- **Status bar**: "Ship N" with the files not shipped; a click ships.
- **Ship** (`Ctrl+Alt+S`, `Cmd+Alt+S` on macOS, or the rocket of the Source Control view): shows the plan (title, branch, files), one confirmation, then commit, checks, push and pull request. Secrets in the changes stop it before anything happens.
- **Undo**: reverts the last operation; closing a pull request asks for confirmation.
- **Agents**: registers `ship mcp` with VS Code (`mcpServerDefinitionProviders`), so Copilot agent mode can deliver work, always as draft pull requests.

## How it works
No logic of ship-it lives here. The extension starts `ship mcp --attended` (the MCP server, in its human-interface mode) and calls the same tools as agents. The result types in `src/generated/` are generated from the server's `outputSchema` by `go run ./tools/gen`: never edit them.

The binary is the `shipIt.path` setting, else the one bundled in the platform VSIX (`bin/`), else `ship` from the `PATH`.

## Develop
```sh
npm ci
npm run typecheck
npm run build
SHIP_BIN=/path/to/ship npm test   # without SHIP_BIN, the contract test is skipped
```

Layout:

```
extensions/vscode/
  package.json        manifest: commands, keybinding, SCM menu, status bar, mcpServerDefinitionProviders
  src/
    extension.ts      activation, status bar, commands
    mcpClient.ts      JSON-RPC client of `ship mcp` (stdio, 2026-07-28 metadata)
    binary.ts         finds the ship binary
    generated/        result types generated from the outputSchema (do not edit)
    test/             node:test unit tests and the contract test with the real binary
  bin/                platform binary injected at packaging time (one VSIX per target)
```

Not yet: plan review webview with drag and drop, undo timeline view, walkthrough, Language Model API.
