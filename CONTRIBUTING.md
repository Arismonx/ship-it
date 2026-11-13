# Contributing to ship-it

Thanks for your interest! ship-it is young: open an issue before a large change, so that we agree on the approach before you write the code.

Everyone taking part follows the [code of conduct](CODE_OF_CONDUCT.md). Questions go to [Discussions](https://github.com/GregoireF/ship-it/discussions) (see [SUPPORT.md](SUPPORT.md)); vulnerabilities never go in a public issue (see [SECURITY.md](SECURITY.md)).

## Where to start
The [roadmap board](https://github.com/users/GregoireF/projects/1) shows every planned issue by version, kept in line with [ROADMAP.md](ROADMAP.md) by `ship roadmap sync`.

- [Good first issues](https://github.com/GregoireF/ship-it/issues?q=is%3Aopen+label%3A%22good+first+issue%22) are small and self-contained: each names the files to change and how to test it.
- [Help wanted](https://github.com/GregoireF/ship-it/issues?q=is%3Aopen+label%3A%22help+wanted%22) are larger pieces where a hand is welcome.
- Comment `/take` on an issue nobody is assigned to: within the hour it is assigned to you, so that two people do not do the same work. `/release` gives it back. Ask there if anything is unclear.
- An idea that is not on the board yet goes to [Discussions › Ideas](https://github.com/GregoireF/ship-it/discussions/categories/ideas) first.

## Before you start
- Read [ARCHITECTURE.md](ARCHITECTURE.md) (layers and import rules, enforced by `internal/archtest`) and the ADRs in [docs/adr/](docs/adr/) that touch your change.
- [ROADMAP.md](ROADMAP.md) lists what is planned; open an issue to discuss what is not decided yet.
- AI agents also read [AGENTS.md](AGENTS.md).

## Set up

### Dev container (recommended)
The repository has a dev container (`.devcontainer/`): Go from `go.mod`, Node.js 22 (as the extension CI job), git, and the recommended VS Code extensions. After creation it runs `go mod download` and `npm ci` in `extensions/vscode`.

- **GitHub Codespaces**: "Code" → "Codespaces" → "Create codespace on main".
- **Locally**: VS Code with the [Dev Containers](https://code.visualstudio.com/docs/devcontainers/containers) extension and Docker, then "Dev Containers: Reopen in Container".

### On your machine
Go (the version of `go.mod`; with the default `GOTOOLCHAIN=auto`, any Go 1.21+ downloads it), git 2.43 or later, and Node.js 22 for the VS Code extension. Windows, macOS and Linux are all supported: CI runs on the three.

## Check your change
```sh
go run ./tools/check                  # what a pull request must pass: gofmt, vet, staticcheck, license headers, tests, govulncheck
go run ./tools/check -race            # with the race detector (needs cgo)
go run ./tools/check -fuzztime=10s    # also fuzz every target briefly
```
`tools/check` is the same program CI runs, so a green run on your machine means a green CI.

The VS Code extension (`extensions/vscode`):
```sh
go build -o extensions/vscode/out/ship ./cmd/ship   # the binary the contract test talks to
cd extensions/vscode
npm ci && npm run typecheck && npm run build
SHIP_BIN="$PWD/out/ship" npm test
```

Generated files (`schemas/`) are regenerated with `go run ./tools/gen`, never edited by hand; a test fails when they are out of date.

The release configuration: `go run github.com/goreleaser/goreleaser/v2@v2.19.0 check`, then `… release --snapshot --clean --skip=sbom` to try it (output in `dist/`). The snapshot also builds the Docker image ([build/docker/Dockerfile](build/docker/Dockerfile)), which needs Docker with buildx and QEMU for arm64; add `--skip=docker` without them.

## Code
- Every source file starts with the SPDX header (`Apache-2.0`, checked by `addlicense`); match the surrounding code.
- New behaviour → table tests; new parser → fuzz target; new error situation → a test that reproduces it, end-to-end in `cmd/ship` when git or GitHub is involved.
- No dependency outside the standard library and `golang.org/x` without an ADR ([ADR-0012](docs/adr/0012-dependency-budget.md)).
- Never call a shell from Go: `exec.Command` with explicit arguments.

## Commits
1. Format `✨ feat(scope): subject`: gitmoji, then a Conventional Commits type, subject ≤ 50 characters, header ≤ 72 ([docs/reference/conventions.md](docs/reference/conventions.md)). `ship types` lists the types and their gitmoji. Check a header with:
   ```sh
   go run ./cmd/ship check title -- "📝 docs(readme): explain the dev container"
   ```
   Simplest of all: ship your change with ship-it itself (`go run ./cmd/ship`).
2. Explain **why** in the commit body: ship-it builds the pull request summary from it.
3. **Sign off every commit** with `git commit -s`. The `Signed-off-by:` line certifies the [Developer Certificate of Origin 1.1](https://developercertificate.org/): you wrote the change, or have the right to submit it under the project licence. Only a human can sign off; an AI agent never adds it for you.

## Pull requests
- The [pull request template](.github/pull_request_template.md) asks how **you** tested the change: report what you did and saw, not CI output. `ship` fills the summary, the issue and the AI usage section; you answer the rest.
- A change users will see gets a changelog entry: a small file in [changelog.d/](changelog.d/README.md), never an edit of CHANGELOG.md, so that pull requests do not conflict. The `changelog` check asks for it on `feat`, `fix`, `perf` and `security` pull requests.
- Tick exactly one box of the "Authorship" section: the `authorship` check fails until you do, and passes as soon as you edit the description.
- Say which AI tools you used, if any, in the "AI usage" section. You must be able to explain every line you submit.
- AI assistance is welcome, but the person who opens a pull request is its author: read and run the changes yourself before you push, and answer the review in your own words. A pull request that no human has read or run is not ready for review yet.
- CI must be green. It runs on the systems your change needs: Linux always, Windows for Go code, macOS for code that talks to the system (`go run ./tools/changes -base origin/main` tells which).
- Every change to `main` goes through a pull request, squash-merged, the maintainer's own included: the branch rules allow no direct push and no bypass.
- Labels sort the work: `kind/` (what it is), `area/` (the part of ship-it), `priority/`, `size/` (set by a workflow from the diff) and `triage/`. You do not need to set them; a maintainer does.
- Review comments may use [Conventional Comments](https://conventionalcomments.org/) (`suggestion (non-blocking): …`), which `ship` will sort by whether they block.

## Propose an ADR
Architectural changes, new dependencies and changes to the conventions need an Architecture Decision Record:
1. Copy the structure of an existing ADR in `docs/adr/`: context → options (pros and cons) → decision → consequences.
2. Use the next free number (`docs/adr/NNNN-short-title.md`) with status `proposed`, and add it to the table of [docs/adr/README.md](docs/adr/README.md).
3. Open a pull request with the ADR before the code. Its status changes to `accepted` once the decision is agreed in review.

## Licence
By contributing you agree that your contributions are licensed under the [Apache License 2.0](LICENSE).
