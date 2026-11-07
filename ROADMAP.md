# Roadmap

Where ship-it stands and what comes next, by version. There are no dates: an item is ticked when it is done and tested. The detailed history of the work, phase by phase, is in the [work log](docs/design/roadmap-log.md).

The file uses the format of [`ship roadmap sync`](docs/how-to/plan-with-a-roadmap.md): ship-it keeps its own milestones, issues and [roadmap board](https://github.com/users/GregoireF/projects/1) in line with it. Want to help? [CONTRIBUTING.md](CONTRIBUTING.md#where-to-start) says where to start.

## 0.1: deliver, check and undo
- [x] One command from work in progress to a pull request: plan, review screen, branch, commit, push, and a pull request that follows the repository's conventions and template #20
- [x] The project's checks before the push (Node, Go, Rust, Python, PHP, Ruby, JVM, .NET), on demand with `ship check gates`, and before each commit when you ask for it #21
- [x] A secret scan of the changes and of the unpushed commits #22
- [x] Undo of almost every operation, local or on GitHub, with byte-exact checkpoints #23
  [The exceptions](docs/explanation/safety-net.md#what-cannot-be-undone).
- [x] Stacked pull requests on GitHub's native stacks, with a chain of pull requests on GitHub Enterprise Server (tested against a fake GitHub, and by hand on a real repository; automated real runs are planned for 0.2) #24
- [x] An MCP server for coding agents, whose pull requests are always drafts, declared to Claude Code, VS Code, Cursor or Codex with one command #25
- [x] A VS Code extension on the same server (in the repository; not on the Marketplace yet) #26
- [x] Layered settings and an organisation policy; labels and `ROADMAP.md` kept in line with GitHub #27
- [x] Builds for Linux, macOS and Windows with signed checksums and an SBOM; install scripts, Homebrew, Scoop and a Docker image #28
- [x] First release published #29
  [v0.1.0](https://github.com/GregoireF/ship-it/releases/tag/v0.1.0), on 2026-11-07.

## 0.2: next
**Exit:** ship-it names, splits and previews the work better than a person in a hurry would, and its real end-to-end runs pass every night.
- [ ] feat: Better branch names and commit subjects #30
  Names from a finer reading of the diff, or from the host agent, and a factual summary of the diff in commit and pull request bodies. The plan lives in internal/core/plan; its reasons are shown by `ship plan`.
  - [ ] feat: Name a change after the declaration it adds, in TypeScript and Python as in Go #31
  - [ ] feat: A factual summary of the diff in the commit and pull request bodies (files, declarations added or removed) #32
  - [ ] feat: Let the host agent propose the branch and title through the Agent Skill, checked like a human's #33
- [ ] feat: Atomic commits #34
  The work split by intent, down to the hunk, and adjustable on the review screen.
  - [ ] feat: Group the hunks of a change by intent (a rename, a new feature, a test) in internal/core #35
  - [ ] feat: Show and move hunks between commits on the review screen #36
- [ ] feat: A time machine with `ship watch` and `ship rewind` #37
  `ship watch` takes a checkpoint when files change, with light watching (no daemon); `ship rewind` goes back to one, itself undoable.
  - [ ] feat: `ship watch`, a checkpoint after each pause in the edits, kept by the rules of `ship gc` #38
  - [ ] feat: `ship rewind`, a list of the checkpoints with their changed files, and a restore that can be undone #39
- [ ] feat: See conflicts early #40
  A warning before publishing when an open pull request of the repository touches the same lines.
  - [ ] feat: Read the changed lines of the open pull requests through the API and compare them with the branch's #41
  - [ ] feat: The warning in `ship plan`, `ship publish` and `ship status`, with the pull requests concerned #42
- [ ] feat: `ship preview` #43
  The pull request as reviewers will see it, before it exists.
  - [ ] feat: Render the title, the body, the labels and the size the pull request would get #44
  - [ ] feat: A factual risk score: lines, files without tests, generated files, sensitive paths #45
  - [ ] feat: Reviewers suggested from CODEOWNERS and from the history of the changed files #46
- [ ] feat: Agents #47
  - [ ] feat: A policy per agent (claude, codex, copilot, cursor): allowed commands, drafts, paths #48
  - [ ] feat: `ship log --agent`, the journal of what each agent did #49
  - [ ] feat: An Agent Skill installed by `ship skill install` #50
- [ ] feat: Help when things go wrong #51
  - [ ] feat: `ship explain <code>` prints the page of a SHIP-E or SHIP-W code from docs/reference/errors.md, offline #52
  - [ ] feat: `ship bug`, a report built from the run's journal (.git/ship/logs) that you read before sending, without telemetry #53
  - [ ] feat: `ship doctor --deep`, which also checks the token's scopes, the rulesets and the project's checks #54
- [ ] feat: `ship uninstall` #55
  Removes ship's data from a repository (.git/ship), its user files and the client files written by `ship mcp install`, after showing the list.
- [ ] fix: Polish found on the first real runs #56
  Small and self-contained: good first issues.
  - [ ] fix: `ship stack sync` accepts `--yes`, like the other commands that write #57
    `ship stack sync --yes` fails with "flag provided but not defined: -yes", while `ship stack merge`, `ship sync` and `ship roadmap sync` accept it. Add the flag in `stackSync` (internal/surface/cli/stack.go); the sync asks nothing today, so it only needs to be accepted. Add a case to the tests of internal/surface/cli.
  - [ ] fix: `ship version` without "(none, unknown)" when go install built it #58
    Installed with `go install github.com/GregoireF/ship-it/cmd/ship@latest`, `ship version` prints `ship v0.1.0 (none, unknown)`: Go records no commit or date for a module download. Print the version alone when both are unknown (`buildInfo` in cmd/ship/main.go), with a test.
- [ ] test: Real end-to-end runs #59
  - [ ] test: A nightly run of `ship`, `publish`, `undo` and `stack` against the GregoireF/ship-it-sandbox repository #60
  - [ ] test: Run the commands of the tutorials in CI #61

## 0.3
**Exit:** anyone can install ship-it, sign in and use it daily without reading the docs, from the terminal or from VS Code.
- [ ] feat: `ship login` #62
  Sign in with the GitHub device flow and keep the token in the system keychain (a new dependency: an ADR first).
- [ ] feat: Offline mode #63
  When GitHub is down, finish the local work and publish once it is back.
- [ ] feat: A guided path for a secret already pushed #64
  Revoke it first, then clean the history if needed.
- [ ] feat: Custom checks #65
  - [ ] feat: Custom checks declared in .github/ship.json, run with the detected ones #66
  - [ ] feat: A format or lint:fix clean-up before the commit #67
- [ ] feat: Shell completions and man pages #68
  Generated from the commands and their flags, shipped in the release archives and the Homebrew formula.
  - [ ] feat: `ship completion` for bash, zsh, fish and PowerShell #69
  - [ ] feat: Man pages generated by `go run ./tools/gen` #70
- [ ] feat: VS Code extension #71
  - [ ] feat: A review view and an undo timeline #72
  - [ ] build: Packages on the Marketplace and Open VSX #73

## Later
- [ ] feat: Opt-in AI providers for people (Ollama, Mistral, Anthropic, OpenAI-compatible), with a scan for personal data before anything is sent #74
- [ ] feat: GitLab and Gitea #75
- [ ] feat: `ship redo` #76
- [ ] feat: `ship release`, with the version, the changelog, the tag and the GitHub release #77
- [ ] feat: `ship daemon`, with checkpoints and draft pull requests in the background #78
- [ ] feat: An organisation policy read from a private repository; rulesets, merge settings and issue types in `ship sync` #79
- [ ] build: Windows signing, macOS notarisation, WinGet, deb and rpm packages, reproducible builds #80
