# AGENTS.md: extensions/vscode

- TypeScript extension with **no business logic**: it starts `ship mcp --attended` and renders what the binary returns (ADR-0010, revised: no separate `ship rpc`).
- Types under `src/generated/` come from the server's outputSchema (`go run ./tools/gen`); never edit them by hand.
- Tooling: npm, esbuild (bundle), `tsc --noEmit` (typecheck), `node:test` (tests). No UI framework: webviews use HTML/CSS with the VS Code theme variables.
- `engines.vscode` and `@types/vscode` stay at 1.101.0, the first release with the stable MCP provider API: do not raise them without a reason.
- No dependency on GitHub Copilot in the manifest; the Language Model API is used only after explicit user consent.
- Accessibility: keyboard navigation, VS Code theme tokens (high contrast included), ARIA roles in webviews.
- Marketplace assets are PNG generated in CI (vsce rejects SVG icons).
- Publish one VSIX per platform; always pass `--target` to both `vsce` and `ovsx`.

See `docs/design/vscode-extension.md`.
