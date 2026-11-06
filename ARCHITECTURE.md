# Architecture

> A map of the repository for humans and agents. It describes **where each thing lives and who may import what**. The *why* behind the choices is in [docs/adr/](docs/adr/); the functional detail is in [docs/design/architecture.md](docs/design/architecture.md).
> These rules are **enforced by a test** ([internal/archtest](internal/archtest/arch_test.go), via `go list -json`): a violation makes `go test ./...` fail.
> Legend: ✅ = implemented and tested. A package without the mark is a placeholder (a `doc.go` that says what it will hold) or not written yet.

## Overview: lightweight hexagonal

```
                ┌──────────── surface ────────────┐
   humans ────► │   cli (Charm)   mcp   (daemon)  │ ◄─── VS Code / AI agents
                └───────────────┬─────────────────┘
                                │ calls use cases
                        ┌───────▼────────┐
                        │      app       │  orchestration: deliver, undo, status,
                        │  (use cases)   │  sync, init, release, doctor, fix…
                        └───┬────────┬───┘
           pure decisions   │        │ effects through interfaces
                    ┌───────▼──┐  ┌──▼──────┐
                    │   core   │  │  ports  │  Git, Forge, CheckpointStore, OpLog,
                    │ (domain) │  │(contracts)│ Credentials, AI, Notifier, Clock…
                    └──────────┘  └──▲──────┘
                                     │ implement
                        ┌────────────┴────────────┐
                        │        adapters         │  gitexec, github (REST), keychain,
                        │   (the outside world)   │  notify, ai, checkpointstore, fsconfig
                        └─────────────────────────┘
   cmd/ship = composition root: builds the adapters, injects them into app, starts the surface.
```

**Guiding idea:** `core` decides, `adapters` act, `app` connects them. All the delicate logic (plan, conventions, error playbook, policy, oplog) is **pure** and is tested without git, without a network and without a disk.

## Tree

```
cmd/ship/                 composition root (the `ship` binary)
internal/
  core/                   DOMAIN: pure, no I/O
    fault/                typed errors: stable SHIP-E#### code + i18n key + parameters
    convention/           ✅ `✨ type(scope)!: subject` titles, types and gitmoji, Conventional Branch 1.1.0 branches, issue references, semver
    snapshot/             ✅ model of the work in progress: unified diff parsing, categories, generated files
    plan/                 ✅ generic deterministic planner (type, scope, subject, branch, reasons)
    split/                ✅ proposed split by area (size guardrail), --only selection; coming: atomic commits
    stack/                ✅ layers of a stack by area, chains from the oplog, sync order after a merge, merge target (ADR-0006)
    gates/                ✅ detection of the project's checks (Node, Go, Rust, Python, PHP, Ruby, JVM, .NET)
    secrets/              ✅ secret detection in added lines (rules from the draft); coming: PII
    policy/               placeholder: the layers and the organisation policy live in `config` today
    oplog/                ✅ operations, before/after views, undo decision (compensating steps)
    retention/            ✅ which checkpoints `ship gc` may delete
    forge/                ✅ GitHub repository named by a remote URL, API root (github.com, ghe.com, GHES)
    pullrequest/          ✅ PR body (docs/reference/conventions.md), title, draft rule
    situation/            ✅ next action suggested by `ship status`
    config/               ✅ layered settings and organisation policy (single source: the Settings struct → strict parser + JSON Schema)
    commitlint/           ✅ titles adapted to the project's own commitlint configuration
    rules/                ✅ GitHub ruleset rules that apply to a branch, checked before a push
    review/               ✅ checks and review decision of a pull request, for `ship status`
    mcpconfig/            ✅ adds ship-it to an agent's project MCP file without disturbing the rest
    glob/                 ✅ path patterns of the configuration
    settings/             ✅ label taxonomy and `ship sync` plan (rename, create, update; never delete)
    fix/                  ✅ ship doctor diagnosis (facts → findings → remedies); full playbook to come
    release/              placeholder for `ship release` (planned): semver computation, changelog, customer notes
    roadmap/              ✅ parsing of ROADMAP.md and plan against the GitHub state (milestones, issues, Project)
  ports/                  CONTRACTS: interfaces that core/app expect from the world (✅ Git, CheckpointStore, OperationLog,
                          Forge, Credentials)
  app/                    USE CASES: orchestrate core + ports (✅ CheckTitle, CheckBranch, SuggestBranch, ProposePlan,
                          Doctor, DoctorWithGitHub, DeliverLocally, Publish, Status, Undo, checkpoints, CollectGarbage)
  adapters/               OUTSIDE WORLD: one implementation per port
    gitexec/              ✅ runs the git binary (porcelain v2 -z), never through a shell
    github/               ✅ stdlib REST client (repository, PR, typed errors, stacks, asynchronous merge); coming: ETag, SSO
    projectexec/          ✅ runs the project's commands (gates) without a shell, keeps the tail of the output
    ghauth/               ✅ finds an existing token (env, gh, git credential helpers) without ever prompting
    checkpointstore/      ✅ byte-exact checkpoints in an isolated private repository, one ref each (ADR-0005)
    oplogstore/           ✅ oplog in .git/ship/oplog/, one JSON entry per operation (atomic writes)
    roadmapstore/         ✅ what `ship roadmap sync` remembers, in .git/ship/roadmap.json
    keychain/             placeholder: OS keychain (Windows CredRead/Write, macOS security, Linux secret-tool)
    notify/               placeholder: native notifications (WinRT, osascript, notify-send)
    ai/                   placeholder: opt-in providers (ollama, mistral, anthropic, openai-compatible)
    fsconfig/             ✅ user and repository config files (created without overwriting); coming: system policy
    ghstatus/             placeholder: githubstatus.com state (offline mode)
  surface/                USER INTERFACES: "driving" adapters
    cli/                  commands, Bubble Tea v2 TUI (the only package allowed to import Charm)
    (rpc/)                dropped: the VS Code extension speaks MCP (`ship mcp --attended`, ADR-0010 revised)
    mcp/                  ✅ dual-era MCP server (stateless 2026-07-28, legacy 2025-11-25/2025-06-18), 11 tools
    daemon/               placeholder: autopilot (polling, checkpoints, offline queue)
  schema/                 ✅ JSON Schema of the Go types (MCP outputSchema) + validator for the generated subset (TS types for the extension come from `tools/gen`)
  i18n/                   ✅ message catalogues (en, fr), rendering of the keys produced by core/app
  obs/                    ✅ JSONL journal (slog), in-memory black box, redacted Secret, run IDs, OTel severity
  platform/               ✅ standard paths (config, logs) per OS; coming: WSL, Windows console, CI detection
  testkit/                ✅ shared fakes for the ports (Git, Forge, stacks, projects, tracker, CheckpointStore, OperationLog, config, runner); tests only
  archtest/               test of the import rules in this document
tools/                    developer tasks in Go (`go run ./tools/<task>`): generation, docs, release
schemas/                  GENERATED JSON Schemas (do not edit by hand)
testdata/
  golden/                 expected outputs (review screen, `ship mcp install` files)
bench/                    planned macro and agent benchmarks (the budgets checked today run through `go run ./tools/bench`)
extensions/vscode/        VS Code extension (TypeScript, no business logic)
docs/                     design (vision, research, ADRs…) then user guides
assets/brand/             visual identity (SVG)
build/docker/             Dockerfile of the ghcr.io/gregoiref/ship image (built by GoReleaser)
.devcontainer/            contributor dev container (Codespaces)
```

## Import rules (enforced by `internal/archtest`)

| Package | May import | Must **never** import |
|---|---|---|
| `core/...` | stdlib **without I/O**, other `core/...` | `os`, `os/exec`, `net/...`, `ports`, `app`, `adapters`, `surface`, any third-party dependency |
| `ports` | `core/...`, `context`, `io`, `time` | `app`, `adapters`, `surface` |
| `app` | `core/...`, `ports`, `i18n` (keys only) | `adapters`, `surface`, `os/exec`, `net/http` |
| `adapters/<x>` | `ports`, `core/...`, `platform`, stdlib, `golang.org/x/...` | `app`, `surface`, **another adapter** |
| `surface/<x>` | `app`, `ports` (types), `schema`, `i18n`, `platform`, `obs` | `adapters` (except through `cmd/ship`), `core` (faults go through the `app.Fault` alias) |
| `surface/cli` | same + **Charm v2** (`charm.land/...`, `github.com/charmbracelet/...`) | - |
| `schema`, `i18n` | stdlib | everything else |
| `obs` | stdlib, `platform` | `core`, `app`, `surface` |
| `platform` | stdlib, `golang.org/x/sys` | `core`, `app`, `surface` |
| `testkit` | everything | must only be imported from `_test.go` files |
| `cmd/ship` | everything (composition root) | nothing else imports it |

Cross-cutting rules:
- **Charm only in `surface/cli`** (ADR-0012).
- **No prompt outside `surface`**: `app` and `core` return decisions or typed requests, and the surface is what asks the human (or opens a draft PR under MCP and the daemon).
- **No user-facing string in `core`**: it returns **keys** and parameters, which `i18n` renders in the chosen language.
- **Any dependency outside the stdlib and `golang.org/x`** requires an ADR (ADR-0012).

## Typical flow: `ship`

1. `surface/cli` reads the flags, then calls `app.Deliver(ctx, req)`.
2. `app`: preflight through the ports (`Git`, `Forge`, `Credentials`), then a checkpoint (`CheckpointStore`), then `core/snapshot` → `core/secrets` → `core/plan`.
3. `app` returns a **review request** for the plan to the surface; the surface presents it (the TUI, or a draft PR under MCP).
4. `app` executes, as a persisted state machine (`OpLog`): gates → commits → sync → push → PR or stack.
5. Each step writes a view to the oplog: an interruption resumes, and an undo compensates.

## Where to add…

| I want… | I go to… |
|---|---|
| a new command | a use case in `app/`, then expose it in `surface/cli` (and `mcp` if relevant) |
| a new handled error | `core/fix` (detector + remedy) + a test that reproduces the situation (end-to-end in `cmd/ship` when it involves git or GitHub) |
| a new AI provider | `adapters/ai/<name>` implementing `ports.AI` |
| a new forge (GHES, other) | `adapters/<forge>` implementing `ports.Forge` |
| a config field | `schema` (DTO) + `core/policy` + regeneration (`go run ./tools/gen`) |
