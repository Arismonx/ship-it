# VS Code extension

> Decisions of 2026-10-23. Architecture reminder: the extension contains **no business logic**. It drives the binary through **`ship mcp --attended`** (revision of ADR-0010: a single protocol, the agents' one), and its TypeScript types are generated from the `outputSchema`s (`go run ./tools/gen`).

## State of v0.1 (2026-10-23)
- ✅ "Ship N" status bar item, **Ship** command (shortcut **Ctrl+Alt+S / Cmd+Alt+S**, user choice, and a rocket in the Source Control view): plan, one modal confirmation, delivery, gates, push, PR. Secrets stop everything before that.
- ✅ **Undo**: modal confirmation before closing a PR.
- ✅ Registration of `ship mcp` for VS Code's agents (`mcpServerDefinitionProviders`, `McpStdioServerDefinition`, a stable API since **VS Code 1.101**: verified in `@types/vscode`, missing in 1.100); `engines.vscode` set to `^1.101.0`.
- ✅ Tooling: npm, esbuild, `tsc --noEmit` (TypeScript 7), `node:test`; contract test with the real binary; Linux and Windows CI job.
- ⚠️ **Shortcut and AltGr**: on Windows, Ctrl+Alt is equivalent to AltGr. On some keyboards, the shortcut conflicts with typing (for example AltGr+S = "ś" in Polish). No default VS Code binding was found on Ctrl+Alt+S ([reference](https://code.visualstudio.com/docs/reference/default-keybindings)), but that is an absence of results, to double-check in the keyboard shortcuts editor.
- Coming: plan webview (editable title and branch, drag and drop), activity bar view and undo timeline, walkthrough, Language Model API, platform-specific VSIX with a bundled binary.
- Provisional `publisher`: `GregoireF` (to be confirmed on the Marketplace and Open VSX).

## Entry points (all three)
1. **Status bar**: a "Ship" button with the number of modified files, and a keyboard shortcut (❓ to choose without collision).
2. **Dedicated view** in the activity bar (24 px monochrome icon): work in progress, PRs and stacks, undo timeline.
3. **Source Control panel**: a ship-it action next to the native commit.

## Plan to approve: rich webview
The title, the branch and the PR are editable. Files are grouped by commit, with **drag and drop to re-split**. The stack is previewed. **Enter** accepts the plan. Accessibility: full keyboard navigation, ARIA roles, VS Code themes honored, high contrast included.

## v1 features
| Feature | CLI equivalent |
|---|---|
| **Visual undo / timeline**: a strip of checkpoints and operations, one-click restore with a diff | `ship undo`, `ship log`, `ship restore` |
| **PR / CI / review status**: badge, VS Code notifications, link to the failing CI log excerpt | `ship status`, `ship inbox`, `ship watch` |
| **Guided error resolution**: the playbook presented as notifications with action buttons | `ship fix` |
| **Onboarding walkthrough** (native walkthroughs API): login, first ship, undo | tutorials |

## AI: neutral + MCP + Language Model API (opt-in)
- **MCP**: the extension registers `ship mcp` with VS Code through `contributes.mcpServerDefinitionProviders` + `vscode.lm.registerMcpServerDefinitionProvider`, with a **`McpStdioServerDefinition`** (command, args, cwd, env) ✅ [VS Code MCP docs](https://code.visualstudio.com/api/extension-guides/ai/mcp). VS Code's agents (Copilot agent mode, etc.) can then call ship-it.
- **Language Model API** (opt-in, in line with ADR-0004): `vscode.lm.selectChatModels` to have the plan written by a model already available in VS Code. Constraints ✅ [VS Code LM docs](https://code.visualstudio.com/api/extension-guides/ai/language-model):
  - user consent through an authentication dialog, to be called **from a user action**;
  - a model list that **may be empty**, to be handled;
  - quotas and rate limits visible to the user;
  - **no dependency on Copilot in the manifest** (an explicit recommendation for extensions whose features are not all AI);
  - no use of the API in integration tests.
- This third source (`vscode-lm`) goes through the **same secrets/PII scan and the same organisation policy** as the other providers; an organisation can forbid it.

## Packaging
- One VSIX per platform, with the signed binary bundled; published on the Marketplace and Open VSX with an explicit `--target`.
- Marketplace icon as a **PNG** (vsce refuses SVG), generated in CI.
- Compatible with Remote-WSL, SSH and Dev Containers (the Linux binary runs on the remote side; ❓ to test).
