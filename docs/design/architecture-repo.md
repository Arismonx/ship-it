# Architecture of the ship-it repository

> Proposal of phase 0. Facts: ✅ primary source · 🟡 secondary · ❓ to verify.
>
> What was built differs in places, and [ARCHITECTURE.md](../../ARCHITECTURE.md) describes the real tree. testscript was not adopted: there is no `testdata/script/` and no `.txtar` scenario, and behaviour across git and GitHub is tested end to end in `cmd/ship/*_test.go` (real git, a bare remote, a fake GitHub HTTP server). `ship rpc` was dropped: the VS Code extension uses `ship mcp --attended` ([ADR-0010](../adr/0010-surfaces.md)). There is no Taskfile: developer tasks are `go run ./tools/<task>`. The `bench/` comparison with Pare and Bash is not done.

## 1. Research behind the proposal

| Fact | Status | Source |
|---|---|---|
| Official Go docs: commands in `cmd/`, logic in `internal/` ("keep packages in internal as much as possible"); for a server or tool project, all unexported code in `internal/` | ✅ | [go.dev/doc/modules/layout](https://go.dev/doc/modules/layout) |
| Go 1.24: **`tool`** directive in `go.mod` (`go get -tool`, `go tool <name>`) that replaces `tools.go`; developer tools versioned with the module | 🟡 proposal and blogs | [proposal 48429](https://go.googlesource.com/proposal/+/master/design/48429-go-tool-modules.md), [rednafi](https://rednafi.com/go/tool-directive/) |
| **testscript** (`rogpeppe/go-internal`): CLI tests as txtar scripts, extracted from the harness that tests the `go` command itself; golden file updates; coverage | 🟡 | [rednafi](https://rednafi.com/go/testscript-cli/), [go-internal](https://redirect.github.com/rogpeppe/go-internal) |
| **GoReleaser**: SPDX SBOM through Syft by default, cosign signing (keyless possible), Homebrew (now as a **Cask**; ship-it ships a formula instead, see [workflows](workflows.md)), Scoop, WinGet, deb/rpm; macOS signing and notarization; v2.18 announced | 🟡 | [goreleaser.com](https://www.goreleaser.com/getting-started/install/oss/), [sign](https://goreleaser.com/customization/sign/) |
| VS Code: **platform-specific** extensions through `vsce publish --target …`; targets `win32-x64`, `win32-arm64`, `linux-x64`, `linux-arm64`, `linux-armhf`, `alpine-x64`, `alpine-arm64`, `darwin-x64`, `darwin-arm64`, `web`; a package without a target serves as the fallback | ✅ | [VS Code publishing](https://code.visualstudio.com/api/working-with-extensions/publishing-extension) |
| The Claude Code extension does exactly that: one VSIX per platform, with the native binary bundled | 🟡 | [nixpkgs](https://code.ornl.gov/nix/nixpkgs/-/commit/652adc674a292f632d4370d90c3148bafdf96341) |
| **AGENTS.md**: an open format of instructions for agents, contributed to the Agentic AI Foundation (Linux Foundation) on Dec. 9, 2025 together with MCP; adopted by Codex, Cursor, Copilot, VS Code, Gemini CLI… (60,000+ projects according to OpenAI) | 🟡 | [openai.com](https://openai.com/index/agentic-ai-foundation/) |
| Taskfile: cross-platform (Windows included), YAML; Make is painful on Windows | 🟡 | [marmelab 2026](https://marmelab.com/blog/2026/03/12/taskfile-alternative-makefile.html) |

## 2. Proposed tree

```
ship-it/
├── AGENTS.md                  # instructions for AI agents (AAIF standard), dogfooding
├── README.md · ROADMAP.md · LICENSE (Apache-2.0) · SECURITY.md · CHANGELOG.md
├── go.mod                     # a single Go module; developer tools through the `tool` directive
├── cmd/
│   └── ship/                  # minimal main.go: wires the surfaces
├── internal/
│   ├── engine/                # the core: snapshot → plan → gates → deliver (no terminal I/O)
│   ├── git/                   # exec git, -z parsing
│   ├── forge/  forge/github/  # Forge interface + stdlib REST client
│   ├── auth/ checkpoint/ oplog/ fix/ policy/ secrets/ split/ release/ roadmap/ i18n/
│   ├── schema/                # ★ Go types = single source of the schemas (see §3)
│   └── surface/
│       ├── cli/               # Bubble Tea v2 (the only place that imports Charm)
│       ├── rpc/               # dropped: the extension uses `ship mcp --attended`
│       ├── mcp/               # dual-era MCP server
│       └── daemon/
├── extensions/
│   └── vscode/                # TypeScript, no business logic; types generated from internal/schema
├── schemas/                   # GENERATED JSON Schemas (config, --json outputs, MCP tools), published
├── testdata/
│   └── script/*.txtar         # not done: end-to-end tests live in cmd/ship/*_test.go
├── bench/                     # tokens/calls benchmark: ship-it vs Pare vs Bash (measured, reproducible)
├── docs/                      # design (this folder) + user guides
├── .github/
│   ├── ship.json              # ★ dogfooding: ship-it configures its own repository
│   └── workflows/             # CI: actions pinned by SHA, minimal permissions, attestations
├── .goreleaser.yaml
└── Taskfile.yml               # or equivalent, see question R3
```

## 3. Proposed innovations

1. **Single schema ("single source of truth").** The Go types of `internal/schema` generate:
   - the JSON Schema of `.github/ship.json` (VS Code completion);
   - the `outputSchema` and `inputSchema` of each MCP tool;
   - the schema of the CLI's `--json` output;
   - the TypeScript types of the VS Code extension.

   A CI test fails if a generated artefact is out of date. The surfaces therefore cannot drift apart.
2. **The error playbook is executable.** Each row of the [error playbook](../reference/error-playbook.md) matches a `testdata/script/<situation>.txtar` scenario that builds a broken repository, runs `ship fix` and checks the result. The error docs are **generated from the scenarios**, so they never lie. *Not done as proposed*: the situations ship handles are tested in Go, end to end in `cmd/ship`, and the [error reference](../reference/errors.md) is generated from the code, not from scenarios.
3. **Full dogfooding.** ship-it is delivered by ship-it: commits, PRs, stacks, releases and `ROADMAP.md → GitHub`. The repository is its own first customer and its own demo.
4. **Fake GitHub for tests** (done). A local HTTP server (stdlib `httptest`) replays the GitHub API. The e2e tests run offline and in CI, and they also simulate incidents (5xx, rate limit) to test the offline mode. Real tests against a sandbox repository run nightly.
5. **Public token benchmark** (*planned*, not done). `bench/` would measure calls and tokens for the same task (deliver a PR, undo) with ship-it MCP, Pare and plain Bash. The result is published in the README, with the method.
6. **Dual-purpose AGENTS.md.** It guides the agents that contribute to ship-it, and it serves as the **template** for what `ship mcp install` will write in users' repositories.
7. **Platform-specific VS Code extension.** One VSIX per target bundles the signed `ship` binary, so there is nothing to install besides the extension. It is published on the Marketplace and Open VSX (❓ `--target` support on the Open VSX side to verify).

## 4. Pros and cons of the major choices

| Choice | Pros | Cons |
|---|---|---|
| **One repository** (Go + TS extension) | Single schema, a single PR for an end-to-end feature, simpler dogfooding | Two toolchains (Go + Node) for contributors; longer CI |
| Two repositories | Separate toolchains | Schema drift, versions to keep in sync |
| **One Go module** | Simple, `go install …/cmd/ship@latest` | A future public SDK will force a split (the Go docs plan for it) |
| testscript (not adopted) | Readable scenarios, proven by the Go team | Test dependency (not in the binary) |
| GoReleaser | Covers signing, SBOM, Homebrew, Scoop, WinGet, deb/rpm | YAML configuration, some features in the Pro version (❓ which ones) |
