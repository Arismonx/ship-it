# Architecture

> Status: proposal of phase 0. The structural choices are in [adr/](../adr/), and the real tree is in [ARCHITECTURE.md](../../ARCHITECTURE.md). One part was dropped: `ship rpc`. The VS Code extension talks to `ship mcp --attended` instead ([ADR-0010](../adr/0010-surfaces.md)), so the diagram and the module table below show what is built.

## Overview

```
            ┌──────────────┐   ┌───────────────────┐   ┌──────────────────┐
 Humans  →  │  CLI / TUI   │   │ VS Code extension │   │ AI agents (MCP)  │
            │  `ship …`    │   │ (TypeScript)      │   │ Claude, Cursor…  │
            └──────┬───────┘   └────────┬──────────┘   └────────┬─────────┘
                   │ direct call        │ MCP stdio             │ MCP stdio
                   │                    │ `ship mcp --attended` │ `ship mcp`
            ┌──────▼────────────────────▼───────────────────────▼─────────┐
            │                  CORE (Go, a single binary)                 │
            │  engine : snapshot → plan → gates → deliver → watch         │
            │  checkpoint · oplog/undo · fix (playbook) · policy · i18n   │
            ├───────────┬───────────┬──────────────┬──────────────────────┤
            │  git      │  forge    │  ai          │  secrets / pii       │
            │ (exec git)│ (GitHub   │ (none│host│   │  local scan          │
            │           │  REST)    │  ollama│…)   │                      │
            └─────┬─────┴─────┬─────┴──────┬───────┴──────────────────────┘
                  │           │            │
              git binary  api.github.com  opt-in provider
                                         ┌─────────────┐
            `ship daemon` (autopilot)  → │  same core  │
                                         └─────────────┘
```

**Golden rule:** every surface is a **thin adapter**. Logic lives only in the core. From the API's point of view, each core operation is a pure function: typed input, typed result, progress events, and no prompt. Prompts belong to the surfaces.

## Modules (`internal/…`)

| Module | Responsibility | Notes |
|---|---|---|
| `git` | Runs the `git` binary (`-z`/porcelain output), parses status, diff and numstat | **Never** goes through a shell; `core.quotepath=false`; inherited from the draft |
| `forge` | `Forge` interface; `github` implementation on stdlib `net/http`: PRs, stacks, labels, reviewers, issues, checks, releases | No `gh`. Pagination, ETag, rate limit |
| `auth` | Token resolution: `SHIP_TOKEN` → `GITHUB_TOKEN`/`GH_TOKEN` → the `gh` token if it is installed → **OAuth device flow** → storage in the OS keychain | Keychain: Windows Credential Manager, Keychain, Secret Service. See ADR-0003 |
| `snapshot` | Captures the state of the work: files, hunks, symbols, categories | Deterministic engine ported from `ship-detect.mjs` |
| `plan` | Produces a `Plan`: commits[], branch(es), PR(s), stack, labels, reviewers, issue | Sources: deterministic, host agent (MCP) or AI provider |
| `split` | Splits the WIP into atomic commits (files, then hunks) and into PRs/a stack | Deterministic by topic; AI refines |
| `gates` | Runs the project's checks (lint, typecheck, test), detected per ecosystem | Detection: package.json, go.mod, Cargo.toml, pyproject, composer.json… |
| `secrets` | Scans added lines: secrets, sensitive files, simple PII (e-mail addresses, IBANs, French phone numbers) | Runs **before** anything is sent to an AI and before any push |
| `deliver` | Branch, commits, sync with the base, push, PR or stack | Idempotent: running it again resumes where it stopped |
| `checkpoint` | Byte-exact snapshots in a private repository `.git/ship/store.git`, one ref per checkpoint | Temporary index (`GIT_INDEX_FILE`) + `write-tree` + `commit-tree`: neither the user's index nor their worktree is touched. ADR-0005 (revisions U1, U2) |
| `oplog` | Append-only log of every operation, with the before/after state of the refs | Source of `ship undo` and `ship log` |
| `fix` | Error playbook: detector → diagnosis → remedy (automatic, guided or manual) | [erreurs.md](../reference/error-playbook.md) |
| `policy` | Layered configuration + locked organisation policy | [cli.md](../reference/cli.md#configuration) |
| `ai` | Providers: `none` (deterministic), `host` (MCP agent), `ollama`, `mistral`, `anthropic`, `openai-compatible` | Disabled by default. JSON outputs validated against a schema. ADR-0004 |
| `release` | Semver from history, changelog, tag, GitHub release | ADR-0013 |
| `i18n` | Message catalogues (fr, en); UI language and commit language configured separately | ADR-0011 |
| `ui` | Prompts, arrow-key menus, spinner, colors, fallback without a TTY | In-house implementation on top of `golang.org/x/term`. ADR-0012 |
| `rpc` | Dropped: the VS Code extension uses `ship mcp --attended` | ADR-0010, revised |
| `mcp` | **Dual-era** stdio MCP server (legacy ≤ 2025-11-25 and modern 2026-07-28) | ADR-0010 |
| `daemon` | Autopilot: periodic checkpoints, automatic draft PRs, watch | Phase 6 |

## Main `ship` flow

```
preflight ──► checkpoint(before) ──► snapshot ──► secrets ──► plan ──► [human review] ──►
gates ──► deliver(branch, commits, sync, push, PR/stack) ──► oplog ──► watch (optional)
```

- **Preflight**: a git repository? a remote? auth? an operation in progress? a merged branch to leave? Every failure goes through `fix`.
- **Checkpoint before**: ship-it itself can be undone.
- **Human review**: the only mandatory touchpoint, and a quick one (Enter = accept). With `-y`, under MCP or in the daemon, the review moves to the PR: it is opened as a **draft** and the human marks it ready.
- **Gates after the plan**: on failure, the plan is kept and `ship` resumes.

## Data model (sketch)

```go
type Plan struct {
    Base      string
    Units     []Unit      // 1 unit = 1 PR; several = stack (order bottom → top)
    Source    PlanSource  // deterministic | host | provider
    Provider  string      // e.g. "mistral-small" (shown in the PR if AI was used)
}
type Unit struct {
    Branch   string
    Commits  []Commit
    PR       PRSpec        // title, body, labels, reviewers, issue, draft
}
type Commit struct {
    Type, Scope, Emoji, Subject, Body string
    Breaking bool
    Paths    []string
    Hunks    []HunkRef     // fine-grained split (phase 3)
}
```

## VS Code protocol (`ship mcp --attended`)

The first proposal was a separate JSON-RPC server, `ship rpc`. It was dropped ([ADR-0010](../adr/0010-surfaces.md)): the extension starts `ship mcp --attended` and calls the same MCP tools as the agents, with a human reviewing each plan. Its TypeScript types are generated from the tools' `outputSchema`s, and a contract test makes its client talk to the real binary. Since the extension carries no logic, the extension and the CLI cannot drift apart.

## Daemon / autopilot

- Worktree watching by polling (system notifications would require a dependency, see ADR-0012), with a debounce.
- One checkpoint every N minutes if something changed.
- Optionally: when a topic looks stable (green gates, inactivity), an automatic **draft** PR. The human stays in control by marking it ready for review.

## Cross-platform

Windows is a first-class platform (the author's machine). The points of attention come from the draft: long paths, CRLF, legacy console without Unicode, native toasts, no bash.
