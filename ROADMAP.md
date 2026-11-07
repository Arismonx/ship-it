# Roadmap

Where ship-it stands and what comes next, by version. There are no dates: an item is ticked when it is done and tested. The detailed history of the work, phase by phase, is in the [work log](docs/design/roadmap-log.md).

The file uses the format of [`ship roadmap sync`](docs/how-to/plan-with-a-roadmap.md), so ship-it can keep its own milestones and issues in line with it.

## 0.1: deliver, check and undo
- [x] One command from work in progress to a pull request: plan, review screen, branch, commit, push, and a pull request that follows the repository's conventions and template
- [x] The project's checks before the push (Node, Go, Rust, Python, PHP, Ruby, JVM, .NET), on demand with `ship check gates`, and before each commit when you ask for it
- [x] A secret scan of the changes and of the unpushed commits
- [x] Undo of almost every operation, local or on GitHub, with byte-exact checkpoints ([the exceptions](docs/explanation/safety-net.md#what-cannot-be-undone))
- [x] Stacked pull requests on GitHub's native stacks, with a chain of pull requests on GitHub Enterprise Server (tested against a fake GitHub so far; real end-to-end runs are planned for 0.2)
- [x] An MCP server for coding agents, whose pull requests are always drafts, declared to Claude Code, VS Code, Cursor or Codex with one command
- [x] A VS Code extension on the same server (in the repository; not on the Marketplace yet)
- [x] Layered settings and an organisation policy; labels and `ROADMAP.md` kept in line with GitHub
- [x] Builds for Linux, macOS and Windows with signed checksums and an SBOM; install scripts, Homebrew, Scoop and a Docker image
- [x] First release published: [v0.1.0](https://github.com/GregoireF/ship-it/releases/tag/v0.1.0)

## 0.2: next
- [ ] Better names: branch names and commit subjects from a finer reading of the diff, or from the host agent (MCP or skill), and a factual summary of the diff in commit and pull request bodies
- [ ] Atomic commits: the work split by intent, down to the hunk, adjustable on the review screen
- [ ] A time machine: `ship watch` takes checkpoints as files change, `ship rewind` goes back
- [ ] Conflicts seen early: a warning before publishing when an open pull request touches the same lines
- [ ] `ship preview`: the pull request as reviewers will see it, a factual risk score, reviewers suggested from CODEOWNERS and the history
- [ ] Agents: a policy per agent, an audit journal (`ship log --agent`), and an Agent Skill installed by `ship skill install`
- [ ] Help when things go wrong: `ship explain <code>`, `ship bug` (a report you read before sending, without telemetry), `ship doctor --deep`
- [ ] `ship uninstall`: removes ship's data from a repository, its user files and its integrations
- [ ] Tests: nightly end-to-end runs against a real GitHub sandbox repository, and the tutorials run in CI

## 0.3
- [ ] `ship login`: sign in with the GitHub device flow and keep the token in the system keychain
- [ ] Offline mode: when GitHub is down, finish the local work and publish once it is back
- [ ] A guided path for a secret already pushed (revoke it first)
- [ ] Custom checks, and a `format` or `lint:fix` clean-up before the commit
- [ ] Shell completions and man pages
- [ ] VS Code extension: a review view, an undo timeline, and packages on the Marketplace and Open VSX

## Later
- [ ] Opt-in AI providers for people (Ollama, Mistral, Anthropic, OpenAI-compatible), with a scan for personal data before anything is sent
- [ ] GitLab and Gitea
- [ ] `ship redo`
- [ ] `ship release`: version, changelog, tag and GitHub release
- [ ] `ship daemon`: checkpoints and draft pull requests in the background
- [ ] An organisation policy read from a private repository; rulesets, merge settings and issue types in `ship sync`
- [ ] Windows signing, macOS notarisation, WinGet, deb and rpm packages, reproducible builds
