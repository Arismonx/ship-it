# ADR-0017: Lightweight hexagonal architecture, enforced by a test

**Status:** accepted (2026-10-23, user choice)

## Context
Four surfaces (CLI, VS Code RPC, MCP, daemon) share the same logic. The most delicate logic (plan, conventions, error playbook, oplog, policy) must be testable without git, without a network and without a disk, and the adapters must be replaceable: a fake GitHub for tests, GHES later.

## Options
| | Pros | Cons |
|---|---|---|
| **Lightweight hexagonal** (`core` / `ports` / `app` / `adapters` / `surface`) | Pure domain, interchangeable adapters, clear boundaries for AI agents | More packages; a bit of ceremony (interfaces) |
| Flat packages per feature | Simple | Dependencies that drift, I/O leaking into the logic |
| Vertical slices | Cohesion per use case | Duplication across slices (git and GitHub used everywhere) |

## Decision
Lightweight hexagonal architecture described in [ARCHITECTURE.md](../../ARCHITECTURE.md). Four guardrails:
1. `ARCHITECTURE.md` at the root (map and import rules);
2. a `doc.go` per package (role, allowed imports, reference ADR);
3. an `AGENTS.md` per area (root, `internal/core`, `extensions/vscode`);
4. an **architecture test** (`internal/archtest`) that makes CI fail on a violation. It is implemented in sprint 1 with `go list -json` (stdlib, without `golang.org/x/tools`).

## Consequences
- `cmd/ship` is the only composition root.
- The core returns i18n keys, never text; no prompt outside `surface`.
- Pinned toolchain: `go 1.27` + `toolchain go1.27.4` (latest patch: `crypto/x509` security fix, V15; then `net/http` GO-2026-6859, 2026-11-12).
