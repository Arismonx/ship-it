<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/brand/one-key/banner-dark.svg">
  <img alt="ship-it: the developer codes, ship-it handles git and GitHub" src="assets/brand/one-key/banner-light.svg" width="760">
</picture>

**Your coding agent's pull requests, with your team's rules: conventions, checks, secret scan, drafts only, and undo.**

It serves people as much as agents. One command, `ship`, creates the branch, the commit and the pull request the way your repository expects. Your checks and a secret scan run before the push, `ship check gates` runs the checks whenever you want, and a repository can ask for them before each commit too. `ship undo` takes it all back.

<img alt="A terminal: ship plan proposes a title and a branch for a new Go file, ship --local shows the review screen and commits on feat/add-vat, ship undo takes the commit and the branch back, and the file is uncommitted again" src="assets/demo/demo.gif" width="760">

0.1, early: works on Linux, macOS and Windows, with GitHub. What comes next is in the [roadmap](ROADMAP.md).

[![CI](https://github.com/GregoireF/ship-it/actions/workflows/ci.yml/badge.svg)](https://github.com/GregoireF/ship-it/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/GregoireF/ship-it?sort=semver)](https://github.com/GregoireF/ship-it/releases)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/GregoireF/ship-it/badge)](https://scorecard.dev/viewer/?uri=github.com/GregoireF/ship-it)
[![Go Report Card](https://goreportcard.com/badge/github.com/GregoireF/ship-it)](https://goreportcard.com/report/github.com/GregoireF/ship-it)
[![Go Reference](https://pkg.go.dev/badge/github.com/GregoireF/ship-it.svg)](https://pkg.go.dev/github.com/GregoireF/ship-it)
<br>
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)
[![Go 1.27](https://img.shields.io/badge/go-1.27-00ADD8?logo=go&logoColor=white)](go.mod)
[![Platforms](https://img.shields.io/badge/platforms-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey)](#install)

[Install](#install) · [Quick start](#quick-start) · [Documentation](docs/README.md) · [Compared to GitHub](docs/explanation/compared-to-github.md) · [Roadmap](ROADMAP.md)

</div>

---

## Why I built it

I kept seeing junior developers around me lose confidence with git. A rebase gone wrong, a secret pushed by mistake, a pull request with an empty description: each time, the trouble was not their code but everything around it.

Experienced developers lose time there too. Naming a branch, wording a commit, filling in the template, remembering what this repository expects: none of it is hard, but it comes back ten times a day, and every team and every client does it differently. Histories become hard to read and pull requests uneven.

ship-it takes that part. You write the code. It prepares the branch, the commit and the pull request the way your repository expects, checks your work before anything leaves your machine, and keeps a way back for every step. You see the plan first and can change any of it.

## In 30 seconds

<table>
<tr><th>Without ship-it</th><th>With ship-it</th></tr>
<tr><td>

```sh
git switch -c feat/add-vat-rate
git add -A
git commit -m "feat(cart): add vat rate"
go vet ./... && go test ./...
git push -u origin feat/add-vat-rate
gh pr create --fill
# …then fill in the template by hand
```

</td><td>

```sh
ship
```

One command does all of this, and shows you the plan first. Changed your mind? `ship undo`.

</td></tr>
</table>

Before anything happens, you see the plan and can change any part of it:

```text
⛵ ship · main → feat/add-vat-rate

  Title    ✨ feat(cart): add vat rate
           ✓ valid · 26/72
  Branch   feat/add-vat-rate
           ✓ valid
  Why?     optional
  Tested?  optional

› Files    ▾ 3/3 selected
           [x] internal/cart/vat.go
           [x] internal/cart/vat_test.go
           [x] docs/vat.md

Enter ship · Ctrl+L commit only · Tab field · Space toggle · ? help · Esc cancel
```

<sub>The review screen, as ship's own tests render it.</sub>

## With AI agents

Coding agents commit and open pull requests too, often without your team's rules. Through its MCP server, ship gives them the same plan, checks and secret scan as you, with stricter limits: through the MCP tools, their pull requests are always drafts, and they cannot undo a publication, merge or change settings. An agent that also has a shell can still run ship or git directly, so keep those commands behind your agent's permission prompts and your GitHub rulesets. One command declares ship to Claude Code, VS Code, Cursor or Codex. See [using ship-it with AI agents](docs/how-to/use-with-ai-agents.md).

## Safe by default

It never pushes to your main branch. It never overwrites a teammate's work. It asks before changing anything on GitHub. And `ship undo` takes back what it did.

The details are in [how a delivery works](docs/explanation/how-a-delivery-works.md), and almost every behaviour can be changed in the [settings](docs/reference/settings.md).

## Stacked pull requests

A large change reviews badly. `ship stack` splits your work into layers, one pull request each, every layer built on the one below, and groups them in a native GitHub stack. Reviewers read small pieces in order, and once the bottom one merges, `ship stack sync` moves the others onto it. If you already cut your branches by hand, GitHub's own `gh stack` does the job well; `ship stack` starts earlier, from your work in progress, and one `ship undo` removes the whole stack. See [how to stack pull requests](docs/how-to/stack-pull-requests.md).

## Install

On Linux or macOS, this script puts the latest release in `~/.local/bin`, without sudo:

```sh
curl -fsSL https://raw.githubusercontent.com/GregoireF/ship-it/main/scripts/install.sh | sh
```

On Windows, in PowerShell, this one puts it in `%LOCALAPPDATA%\Programs\ship`:

```powershell
irm https://raw.githubusercontent.com/GregoireF/ship-it/main/scripts/install.ps1 | iex
```

Both scripts check the archive against the release's SHA-256 checksums and stop on a mismatch. If cosign is installed, they also check that the checksums were signed by this repository's release workflow; without cosign, a logged-in GitHub CLI checks the archive's build provenance instead. Add `--require-signature` (`-RequireSignature` on Windows) to stop when neither is there. They tell you how to add the folder to your PATH if it is not there yet. Read them first if you prefer: [install.sh](scripts/install.sh), [install.ps1](scripts/install.ps1).

With Homebrew (macOS or Linux):

```sh
brew install gregoiref/tap/ship
```

With Scoop (Windows):

```powershell
scoop bucket add gregoiref https://github.com/GregoireF/homebrew-tap
scoop install ship
```

With Go 1.27:

```sh
go install github.com/GregoireF/ship-it/cmd/ship@latest
```

Or download the archive for your system from the [latest release](https://github.com/GregoireF/ship-it/releases/latest).

You need git 2.43 or later. If you already use the GitHub CLI or push over HTTPS, ship finds your GitHub token by itself; `ship doctor` tells you if anything is missing. [GitHub access](docs/reference/github-access.md) lists which token it uses and the permissions it needs.

## Quick start

In any git repository with some changes:

```sh
ship plan      # what ship would do, changes nothing
ship           # review, then commit, checks, push and pull request
ship status    # where you stand, and the next step
ship undo      # changed your mind
```

The commands that change something also take `--dry-run`, to see what would happen first.

## Find your path

| You are… | Start here |
|---|---|
| **New to git and GitHub** | [Your first pull request](docs/tutorials/first-pull-request.md), then [undo a mistake](docs/tutorials/undo-a-mistake.md). Unknown words: the [glossary](docs/glossary.md). |
| **An experienced developer** | [How ship-it compares to GitHub's own tools](docs/explanation/compared-to-github.md), [how a delivery works](docs/explanation/how-a-delivery-works.md), the [command reference](docs/reference/cli.md). |
| **Leading a team or an agency** | [Configure ship-it for your team](docs/how-to/configure-your-team.md), [client repositories](docs/how-to/client-repositories.md), the [settings](docs/reference/settings.md). |
| **Working with AI agents** | [Use ship-it with AI coding agents](docs/how-to/use-with-ai-agents.md): MCP server, safeguards. |
| **Contributing** | [CONTRIBUTING.md](CONTRIBUTING.md), the [architecture](ARCHITECTURE.md) and its [decisions](docs/adr/). |

When something goes wrong, every message carries a code: look it up in the [error reference](docs/reference/errors.md).

## Community

Questions: [SUPPORT.md](SUPPORT.md). Please follow the [code of conduct](CODE_OF_CONDUCT.md). Report security issues as described in [SECURITY.md](SECURITY.md), never in a public issue.

## License

[Apache License 2.0](LICENSE). Use ship on any code, open or proprietary, change it and redistribute it; keep the licence and copyright notices with it. Third-party notices: [THIRD_PARTY_NOTICES.txt](THIRD_PARTY_NOTICES.txt).
