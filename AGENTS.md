# AGENTS.md

Instructions for AI coding agents working on **ship-it** (format: [agents.md](https://agents.md/)). The closest `AGENTS.md` to the file you edit wins; see also `internal/core/AGENTS.md` and `extensions/vscode/AGENTS.md`.

## Project overview
ship-it lets developers code while it handles git and GitHub: plan → branch → conventional commits → pull request or native stacked pull requests, with undo for almost every operation (local checkpoints). One Go binary (`ship`) exposes a CLI and an MCP server; the VS Code extension talks to `ship mcp --attended` (there is no JSON-RPC server, see ADR-0010).

Read first: `ARCHITECTURE.md` (layers and import rules) and `docs/adr/` (decisions).

## Build and test
```sh
go build ./...
go test ./...            # unit, golden and end-to-end tests
go test -race ./...
go vet ./...
```
Developer tasks are Go programs: `go run ./tools/<task>` (no Make, no Taskfile).

## Architecture rules (enforced by internal/archtest)
- `internal/core/**` is pure: no `os`, `os/exec`, `net`, no third-party imports.
- `internal/app` orchestrates core + ports and never prompts the user.
- `internal/adapters/<x>` implements one port and never imports another adapter.
- Charm (Bubble Tea v2) is allowed **only** in `internal/surface/cli`.
- Core returns message **keys**, never user-facing strings (`internal/i18n` renders them).
- Any dependency outside the standard library and `golang.org/x` requires an ADR.

## Code style
- Match surrounding code; comments in English; UI English by default (French catalogue available).
- Every file starts with:
  ```go
  // SPDX-License-Identifier: Apache-2.0
  // Copyright 2026 Grégoire Favreau
  ```
  (checked by `addlicense --check` in CI).
- Never invoke a shell from Go: use `exec.Command` with explicit arguments.

## Testing
- A situation of the error playbook (`docs/reference/error-playbook.md`) or a fixed bug gets a test that reproduces it; behaviour across git and GitHub goes in an end-to-end test in `cmd/ship/*_test.go` (real git, a bare remote, a fake GitHub HTTP server, `fakeGitHub` in `cmd/ship/publish_test.go`). There are no testscript `.txtar` scenarios.
- Generated artefacts (`schemas/`, extension types) must be regenerated, not edited by hand.

## Commits and pull requests
Format (see `docs/reference/conventions.md`): `✨ feat(scope): subject`, with the gitmoji first, a Conventional Commits type, a subject of at most 50 characters and a header of at most 72 (`ship check title` validates it). Types: feat, fix, docs, style, refactor, perf, test, build, ci, chore, security, revert, wip. Sign off commits (DCO). Never force-push `main`. A user-visible change adds a fragment in `changelog.d/` (see its README), never an edit of `CHANGELOG.md`.

## Security
Never commit secrets; never add telemetry; AI features stay opt-in. Report vulnerabilities as described in `SECURITY.md`.
