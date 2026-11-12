# Changelog

All notable changes are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- `ship log --agent [<name>]`: what AI agents did, or one of them. Every operation made through `ship mcp` records the agent, named by `--client` (which `ship mcp install` now writes) or by the agent's handshake (#49).
- `ship preview` ends with a review risk (low, medium, high) and the facts behind it: the size, source changed without tests, sensitive paths (`ship.pullRequest.sensitivePaths`), removals (#45).
- Pull requests get an `area/<scope>` label from their commit scope when the repository has it (`✨ feat(cli): …` gets `area/cli`), in `ship publish` and `ship label` (#127).
- `ship preview`: the pull request `ship publish` would open (title, body, labels, base, warnings), rehearsed by the publication itself, without sending anything (#44).
- Conflicts seen early: before pushing, `ship publish` warns when an open pull request into the same base changes the same or adjacent lines (`SHIP-W0619`), and `ship status` lists them (`ship.publish.overlaps`, on by default) (#40).
- The pull request body sums up the diff: the files added, changed, renamed and removed, and the declarations added and removed (`ship.pullRequest.sections.changes`, on by default) (#32).
- `ship roadmap sync` keeps what people write in an issue below its `Tracked in ROADMAP.md` line: the file owns the title and the summary, the issue can hold the details for contributors.

### Fixed

- `ship version` prints the version alone when `go install` built it, instead of `(none, unknown)` (#107, thanks @painbaba).

### Security

- Built with Go 1.27.4, which fixes GO-2026-6859 in `net/http`. The flaw is an HTTP/1 server crash and ship runs no server, but `govulncheck` finds the affected functions on its call paths, so the release is rebuilt with the fixed standard library.

## [0.1.1] - 2026-11-07

Fixes found by running ship-it on real repositories, and a safer secret fingerprint. Released from the `release/0.1` branch: the additions above wait for 0.2.

### Fixed

- `ship log` and `ship status` showed raw message keys after a stack, a stack sync or merge, a roadmap sync, a fix or a tidy; `ship status` said `ship undo` refuses a publication, though it closes it once you confirm, and showed "unknown (offline)" for the default branch (#17).
- `ship stack` put a test file and the file it tests in two layers, so the bottom pull request did not build; a test file now stays with its subject (#18).
- `ship roadmap sync` failed on a milestone without a due date (#19), and on an issue that GitHub's "Auto-add sub-issues to project" workflow had already put on the board (#81).

### Security

- The fingerprint of a secret finding, which `ship.secrets.allow` lists, is now a PBKDF2-HMAC-SHA256 of the value (600 000 iterations, salted with the rule id) instead of a plain SHA-256: a fingerprint committed in `.github/ship.json` no longer lets anyone find a weak secret back by brute force. The fingerprints of 0.1 are still accepted during 0.x; ship prints the new one to replace them.

## [0.1.0] - 2026-11-07

First release.

### Delivering work

- `ship`: plan, review screen, checkpoint, branch, commit, checks, push and pull request in one command. `ship plan` shows the plan and changes nothing; `--local` stops after the commit; `ship publish` pushes later.
- `--only` delivers part of the work; above `ship.pullRequest.maxLines` the plan suggests a split by area.
- `--why` and `--tested` go into the commit body, a `Tested:` trailer and the pull request.
- `--dry-run` on every command that writes, with the exit code the real run would have.

### Pull requests

- From a fork: ship pushes to your fork and opens the pull request on the parent repository.
- The body is built from the commit messages and the repository's pull request template; the checks that ran and the linked issue are listed.
- `kind/*` and `size/*` labels when the repository has them.
- AI assistance is disclosed the way the repository asks.
- Drafts when nobody reviewed the plan in a terminal, and always for AI agents.

### Conventions

- Conventional Commits titles with gitmoji, short Conventional Branch names, tracker tickets (`ACME-123`) carried into titles.
- The repository's own rules come first: commitlint, GitHub rulesets (a branch ship created is renamed to fit), the gitmoji habit of its history.
- `ship check title`, `ship check branch`, `ship check signoff`, `ship types`, `ship branch`.

### Checks before the push

- The project's own checks for Node, Go, Rust, Python, PHP, Ruby, the JVM and .NET, cheapest first.
- A secret scan of the work and of every commit a push would send, with custom rules, disabled rules and accepted fingerprints.
- `ship check gates` runs the checks now, without committing or pushing; a repository can also ask for them before every commit.

### Safety net

- Byte-exact checkpoints in a private store inside `.git`, an append-only operation log, `ship undo` for local and GitHub effects (with consent), `ship log`, `ship checkpoint`, `ship gc`.
- `ship tidy` after a merge, `ship status` with the pull request's checks and reviews and the next step, `ship doctor` and `ship fix`.

### Configuration

- Almost every behaviour is a setting: rebase, merge or leave a branch before publishing, sign-off and commit signing, the remote and the base, the start-of-run tidy, drafts, label prefixes and sizes, the sections of the pull request body, reviewers, assignee and milestone, commit lengths, types, emojis and scopes, branch names, secret rules, and the interface language, colours and review screen.
- No configuration needed. JSONC files with a JSON Schema; presets (`ship-it:recommended`, `strict`, `agency`, or a file pinned in another repository), per-path overrides, contexts per client, `SHIP_*` variables, `--set`, and an organisation policy.
- `ship config get|set|unset|explain`, `ship init`.

### Stacked pull requests

- `ship stack` splits the work into layers by area, or the layers you give, and opens one pull request per layer in a native GitHub stack (a classic chain of pull requests where the server has no stacks).
- `ship` on the top layer adds a layer; `ship stack status`, `ship stack sync` after a merge (local rebase by default, so signed commits stay signed), and `ship stack merge` through GitHub's merge API, only after you confirm.
- A review screen for the layers, like the one of `ship`: reorder them, move files between them, edit their titles and branches.
- Each layer pull request gets the labels and the linked issue a single one gets, and every layer passes the GitHub rulesets before anything is pushed. `ship undo` after `ship stack sync` also puts GitHub back, with your consent.

### GitHub planning

- `ship roadmap sync`: `ROADMAP.md` becomes milestones, issues and sub-issues, and an optional GitHub Project; undoable.
- `ship sync` keeps the repository's labels in line.

### AI agents

- `ship mcp`: an MCP server whose tools plan, deliver, publish, stack, undo and read the settings, with the same checks as a human. Agents open drafts, and never undo a publication or merge.
- `ship mcp install --client claude|vscode|cursor|codex` writes the project file of each client; `--print` shows it.

### Distribution

- Binaries for Linux, macOS and Windows (amd64, arm64) with signed checksums, SBOMs and build provenance; a container image `ghcr.io/gregoiref/ship`.
- Install with one command (a script for Linux and macOS, one for Windows, both checking the checksums), Homebrew (`brew install gregoiref/tap/ship`), Scoop or `go install`.
- Licensed under the Apache License 2.0.
- English and French interface.

[Unreleased]: https://github.com/GregoireF/ship-it/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/GregoireF/ship-it/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/GregoireF/ship-it/releases/tag/v0.1.0
