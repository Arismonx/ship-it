# ADR-0004: Opt-in AI, three plan sources

**Status:** accepted

## Decision
The plan (messages, split, PR summary) comes from one of these sources:
1. **Deterministic**: always available, it is the default;
2. **Host agent**: under MCP, the calling agent (Claude Code, Cursor…) writes the plan. ship-it gives it the snapshot and validates the JSON it returns. No key is needed and ship-it sends nothing itself;
3. **VS Code model** (`vscode-lm`): in the extension, through VS Code's Language Model API, with explicit consent (see [extension-vscode](../design/vscode-extension.md));
4. **Configured provider**: `ollama`, `mistral`, `anthropic` or `openai-compatible`, **only after explicit opt-in** (by the user, or by a policy that allows it).

## Guardrails
- Secrets/PII scan before anything is sent. A sensitive file is never sent.
- JSON output validated against a schema. If it is invalid, the deterministic plan is kept.
- `allowedProviders` in the organisation policy.
- "AI-assisted" disclosure in the PR (see [conventions.md](../reference/conventions.md)).

## Consequences
- The quality of the deterministic plan is **critical**. It is the product for most users.
- Each provider's default models are set in the config, never hard-coded (they change fast).
