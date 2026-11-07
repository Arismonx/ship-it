# Roadmap work log

> The detailed log of the work, phase by phase, as it was planned during design. The public roadmap is [ROADMAP.md](../../ROADMAP.md); this page keeps the history and the exit criteria of each phase for maintainers.

## Phase 0: Design
- [x] Reading of the nuxt-core draft
- [x] Sourced research (log)
- [x] Vision, market, architecture, CLI, conventions, errors, compliance, automations, frictions
- [x] ADRs 0001 to 0017
- [x] Public documentation in English (research notes stay in French)
- [x] Settle the open questions that block phase 1
- [x] Official EUPL-1.2 text in `LICENSE` (SPDX list)
- [ ] Public GitHub repository `GregoireF/ship-it`

**Exit:** no open question marked "blocks P1" is left.

## Phase 1: Foundations (parity with the draft, in Go)
- [x] Go skeleton: `cmd/ship`, `internal/*` (hexagonal architecture, ADR-0017)
- [x] Architecture test (`internal/archtest`) that enforces the import rules
- [x] `core/convention`: titles, 13 types + gitmoji, short Conventional Branch 1.1.0 branches, issue references, semver (table tests + fuzz)
- [x] `core/fault`: stable `SHIP-E####` error codes
- [x] `obs`: JSONL journal, black box, redaction, run IDs, OTel severity
- [x] Journal wired into every run ([logs](../reference/logs.md)): `.git/ship/logs/<run_id>.jsonl` (user state folder outside the repository), black box written on failure or panic and named in the error message, `-q`/`-d`/`--trace`, instrumented steps (git commands, GitHub requests, delivery, publication, undo, tidy, fix, gates) with `step`, `duration_ms`, `op_id`, `repo_hash`; private data removed (paths, e-mails, identifiers, tokens; fuzzed); MCP `traceparent` carried into `trace_id`/`span_id`; retention of 20 runs / 10 MB by `ship gc` and on every run; `ship trace`
- [x] Configurable retention and black box (`ship.logs.*`, LOG2); `repo_hash` as an HMAC with a per-install salt; `-d`/`-q` reserved (test)
- [ ] `ship bug` (reviewed report, no telemetry), generated pages for the `SHIP-E####` codes
- [x] `platform` (paths), `i18n` (en/fr, key parity tested)
- [x] CLI: `version`, `types`, `check title`, `check branch`, `branch`
- [x] `ports.Git` + `gitexec` adapter (status porcelain v2, diff, worktree reads), tested on a real repository
- [x] `core/snapshot` (unified diff parsing, categories) + `core/plan` (generic deterministic planner, translated reasons)
- [x] `ship plan [--issue N] [--agent …] [--json]`: shows the plan without changing anything
- [x] staticcheck and govulncheck through the `tool` directive of `go.mod`; tool versions pinned, outdated ones reported
- [x] `ship doctor` (git ≥ 2.43, identity, repository, operation in progress, detached HEAD, remote, mirror, Windows longpaths) with an exact remedy
- [x] `ship`: plan → human review (Enter, or `--yes`) → checkpoint → dedicated branch if on the trunk → commit; exit code 2 without a terminal
- [x] `internal/testkit`: fake Git and fake CheckpointStore shared by the tests
- [x] Multi-OS CI written (`ci.yml`, `pr-title.yml`, Dependabot; actions pinned by SHA), the same checks locally through `go run ./tools/check`; ⚠️ never run on GitHub while the repository is not there
- [x] Supply-chain workflows written: zizmor + actionlint, CodeQL, OpenSSF Scorecard, dependency review, DCO through `ship check signoff`, weekly fuzzing of every target, triage (`ship label`), stale, label sync, outdated tools tracked in one issue, reusable check-title action
- [x] `git` layer (exec, `-z`, status, diff and numstat parsing) + tests on temporary repositories
- [x] Stdlib GitHub REST client: repository, PR (creation, lookup of the open PR), typed errors (token, 404, rate limit, 422 with details)
- [x] Automatic `kind/*` and `size/*` labels (only those that exist, generated files excluded), `Link` header pagination; colors by meaning, as Kubernetes
- [x] `linguist-generated` files read through `git check-attr`: excluded from `size/*` and weightless in the plan
- [ ] GitHub client: reviewers (beyond CODEOWNERS, after `.github/ship.json`), issues, ETag
- [x] Auth from existing tokens: `SHIP_TOKEN`, gh's variables, `gh auth token`, git credential helpers, never a prompt
- [ ] Auth, part 2: `ship login` (device flow, OAuth App), keychain, credential helper, HTTPS ↔ SSH conversion, detection of missing scopes and SSO
- [x] `ship` publishes: push + PR (draft with `--yes`, issue read from the branch, docs/reference/conventions.md body), `ship publish`, `--local`; end-to-end test with a real push
- [x] Pull request body from the repository's template: filled rather than overwritten, AI help disclosed the repository's way
- [x] Commits follow the repository: its gitmoji habit, tracker tickets carried into titles, `--why` and `--tested` (commit body, `Tested:` trailer, PR sections)
- [x] Bubble Tea v2 TUI (CLI surface only) + `--plain` fallback: editable review screen for `ship` (title, branch, why, tested, file selection), accessible mode `--plain` / `SHIP_ACCESSIBLE=1`
- [x] Layered config (defaults < user < `.github/ship.json`), strict parser with precise errors, `ship config`, generated JSON Schema (`go run ./tools/gen`)
- [x] Config layers, part 2: JSONC (comments, trailing commas), `overrides`, `contexts`, `extends` and presets (`ship-it:recommended`, strict, agency), `SHIP_*` variables, `--set`
- [x] `ship config get|set|unset|explain`: edits that keep comments, validated before writing, atomic, undoable
- [x] Organisation policy through a system file (imposed keys, explicit refusal with exit code 5, `ship config` and `ship doctor`)
- [ ] Policy read from a private `<org>/ship-policy` repository
- [ ] Deterministic plan (port of ship-detect, generic analyzers)
- [x] Node gates before the push (lint, typecheck, test; vitest limited to the affected tests; package manager inferred), `--skip-gates` written in the PR, real test with npm
- [x] Gates of the other ecosystems (Go, Rust, Python, PHP, Ruby, JVM, .NET)
- [ ] Custom gates, `format` / `lint:fix` clean-up
- [x] Secret scan (rules ported from the draft, placeholders and code ignored, masked value), refused before review (exit code 3), `ship.secrets.ignorePaths`
- [x] Scan of the unpushed local commits before publishing
- [ ] PII scan before anything is sent to an AI
- [x] `ship status` (next action, PR, ahead and behind, `--offline`) and `ship doctor` with a GitHub check
- [x] CI checks and reviews of the pull request in `ship status`
- [x] i18n guardrail: every key used in the code must exist in the catalogue (test)
- [x] `--dry-run` on every command that writes (`ship`, `publish`, `undo`, `tidy`, `fix`, `sync`, `init`, `config set|unset`, `mcp install`, `label`) and `dryRun` on the MCP tools that write; nothing written, proven by tests
- [x] `✨ type(scope): subject` format, `--json` mode, stable exit codes
- [x] `ship check signoff` (DCO, git's trailer rules) and `ship label --pr`
- [x] ship-it release workflow written: GoReleaser binaries, checksums signed with cosign, SBOM, attestations, approval in an environment (first run for v0.1.0, 2026-11-07)
- [ ] First ship-it release published (unsigned on Windows at first)

**Exit:** a junior developer ships a clean PR from Windows **without gh or Node**, and the draft's full journey is reproduced.

## Phase 2: Safety net
- [x] Byte-exact checkpoints in an isolated private repository (`.git/ship/store.git`), `ship checkpoint [list|restore]`
- [x] Checkpoint retention and `ship gc` (last 20 + 1 per day over 30 days + those an undo is waiting for)
- [x] Append-only oplog + `ship undo` of local deliveries (refs and index only, compare-and-swap guardrails, end-to-end test)
- [x] `ship tidy` (also at the start of `ship`): merged PR → back on the up-to-date base with the work in progress, all or nothing; merged branches deleted, undoable with `ship undo`
- [x] Undo of remote effects (opened PR → closed with a comment, remote branch deleted under a lease), with explicit consent; `ship log`
- [ ] `ship redo`
- [x] `ship fix`: playbook v1 (noreply identity, longpaths, mirror, files not to track, merged branches), a single confirmation, undoable
- [ ] Full playbook (catalogue tested scenario by scenario; testscript was dropped, the scenarios are end-to-end tests in `cmd/ship`)
- [ ] "Secret pushed: revoke first" path, case renames
- [ ] **Offline mode / GitHub incident**: local work finished, delivery queued and replayed (moved up from phase 7, user priority)
- [x] **PR size guardrail**: warning beyond `ship.pullRequest.maxLines` (400), split proposed by area, `ship --only` to deliver part of the work
- [x] Branch behind the base: rebase before publication, push under a lease, conflict → published as is with a warning
- [x] Benchmarks and performance budgets: `go run ./tools/bench` checks time ceilings and git processes per command in CI ([performance.md](performance.md))

**Exit:** every ship-it operation can be undone; 100 % of the playbook entries have an integration test; a ship during a GitHub outage loses nothing and completes when the service is back.

## Phase 3: Split & opt-in AI
- [ ] Split into atomic commits (files → hunks), isolation of refactors
- [x] PR size guardrail (see phase 2)
- [ ] Opt-in AI providers (ollama, mistral, anthropic, openai-compatible), schema validation
- [x] "AI-assisted" disclosure in the PR (the repository's way, host agent included)

**Exit:** a multi-topic diff gives coherent commits and PRs without AI; with AI, they are better written, and nothing is sent without opt-in.

## Phase 4: Stacks
- [x] Stack creation through the native REST API, `ship stack`, `ship stack sync`
- [x] "PR chain" fallback for GHES

**Exit:** a stack of 3 PRs created, reviewed, merged from the bottom up and cleaned up, using only `ship`.

## Phase 5: Surfaces
- [x] VS Code extension v0.1 on `ship mcp --attended` (status bar, Ship, Undo, MCP for VS Code's agents), generated types, TS/Go contract test, CI
- [ ] Extension: plan webview, undo timeline, walkthrough, LM API; platform-specific VSIX (Marketplace + Open VSX)
- [x] Dual-era `ship mcp` (stateless 2026-07-28 + legacy `initialize`), generated and checked `outputSchema`s, the agent's plan validated and recorded as "AI-assisted (host agent)"
- [x] MCP config tools for agents: `ship_config_get`, `ship_config_explain` (11 tools in all, with the stack tools)
- [x] `ship mcp install` for Claude Code (`.mcp.json`, a merge that keeps what is there)
- [x] `ship mcp install --client` for VS Code (`.vscode/mcp.json`), Cursor (`.cursor/mcp.json`) and Codex (`.codex/config.toml`, appended only), `--print` for any client
- [ ] `ship watch`, `ship inbox`, CI log excerpt, `ship ci rerun --failed`
- [ ] Shell completions and man pages
- [ ] CLI accessibility: `--plain` output checked by golden files, `NO_COLOR`, screen readers

**Exit:** the same ship through the CLI, the VS Code button and Claude Code (MCP) produces an identical result.

## Phase 6: GitHub governance & planning
- [x] `ship init` (no overwrite, `area/*` from the scopes) and `ship sync` plan/apply for labels (GitHub labels renamed, read-only if `managedBy`)
- [ ] `ship init --level solo|team|agency|public-sector`; `ship sync` of the merge settings, assignment and Project
- [x] Follow the project's GitHub rulesets before pushing (compliant rename of a branch ship-it created, refusals that quote the rule) and its commitlint (titles adapted, every message validated by the project's commitlint)
- [ ] `ship sync` of rulesets (evaluate mode first)
- [ ] Issue Types/Fields + label fallback, issue forms
- [ ] Auto-assignment (author, linked issue → Project), native Teams, suggested CODEOWNERS
- [x] `ship roadmap sync` (ROADMAP.md → milestones, issues, native sub-issues; numbers and boxes written back; undoable)
- [x] Roadmap: GitHub Project fields and views, linked issue → "In progress" (status, date and phase fields; the Roadmap view itself is set up in GitHub)
- [ ] Locked organisation policy from a policy repository (the system file is done, see phase 1)

**Exit:** a new client repository configured at the `agency` level in less than 2 minutes, with the diff reviewed before applying.

## Phase 7: Releases & autopilot
- [ ] `ship release`: semver, changelog, client notes, release PR, tag, GitHub release
- [ ] `ship daemon`: periodic checkpoints, draft PRs, offline queue

## Phase 8: Distribution & compliance
- [ ] Windows signing (SignPath once eligible), macOS notarization (❓)
- [ ] WinGet (portable, `--scope user`), deb/rpm
- [x] Homebrew formula (pushed by the release workflow) and Scoop manifest (pushed by GoReleaser) to `GregoireF/homebrew-tap`, install scripts without admin rights (`scripts/install.sh`, `scripts/install.ps1`: SHA-256 check, cosign check when installed; not run against a real release yet)
- [x] Docker image `ghcr.io/gregoiref/ship` (Alpine + git, non-root, OCI labels, linux/amd64 + arm64) built by GoReleaser `dockers_v2` from `build/docker/Dockerfile`, base pinned by digest
- [x] Docker image built, signed (cosign) and attested by the release workflow (GHCR login, buildx + QEMU; first pushed for v0.1.0, 2026-11-07)
- [x] Community health files: CONTRIBUTING, CODE_OF_CONDUCT (Contributor Covenant 3.0), SUPPORT, CODEOWNERS, issue forms, pull request template filled by `ship`
- [x] Contributor dev container (`.devcontainer/`, Codespaces)
- [x] `SECURITY.md` and an SBOM per archive and per image
- [ ] `THIRD_PARTY_NOTICES` for the bundled dependencies
- [ ] Reproducible builds verified, CRA preparation
