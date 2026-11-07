# GitHub Actions workflows of the ship-it repository

> Decision W1 (2026-10-23): the **four families** are adopted. They concern **only the ship-it repository**: ship-it does not offer CI to its users' projects.
> Common rules: actions **pinned by full SHA** + a version comment; minimal `permissions` per job; `concurrency` to cancel obsolete runs; workflows audited by zizmor; secrets restricted to environments.

## State (2026-10-23)

Written, **never run on GitHub** (the repository is not there yet): to watch on the first push.
- [`ci.yml`](../../.github/workflows/ci.yml): Linux, Windows and macOS matrix. It calls `go run ./tools/check -fuzztime=10s -race=…`, so that a developer machine runs **exactly** the same checks as CI. These checks are: gofmt, vet, staticcheck, SPDX headers ([`GregoireF/addlicense`](https://github.com/GregoireF/addlicense) v1.0.1 as a `go tool`, dogfooding), tests, a short fuzz of each target (discovered by `go test -list`), govulncheck. `-race` is enabled on Linux and macOS (cgo).
- [`pr-title.yml`](../../.github/workflows/pr-title.yml): the PR title goes through `ship check title`, as an environment variable and never interpolated into the script (untrusted input). Dependabot PRs are exempt.
- [`dependabot.yml`](../../.github/dependabot.yml): gomod, github-actions and npm (extension), once a week, grouped, with a 7-day cooldown (see §5).
- Actions **pinned by SHA**, resolved on 2026-10-23 from the latest releases: `actions/checkout` v7.0.1, `actions/setup-go` v7.0.0, inputs checked in their `action.yml`. `persist-credentials: false` on checkout (its default is `true`).
- Deliberate deviation: **staticcheck** (already a `go tool`) rather than golangci-lint, which avoids one more tool.
- Added on 2026-10-23 (§5, in English): `zizmor.yml` (zizmor + actionlint), `codeql.yml`, `scorecard.yml`, `dependency-review.yml`, `release.yml` (GoReleaser).
- To do: merged coverage, benchstat, nightly. testscript was dropped (end-to-end tests in `cmd/ship` instead); DCO is checked by `ship check signoff`.

## 1. Quality CI (`ci.yml`, on every PR, blocking)
**windows-latest / macos-latest / ubuntu-latest** matrix:
- `go build`, `go vet`, golangci-lint, **addlicense `--check`** (Apache-2.0);
- unit tests + **`-race`**, golden, **short fuzz** per parser;
- ~~testscript~~ (not adopted): the end-to-end tests of `cmd/ship` cover the error playbook;
- **coverage**: `-coverprofile` + instrumented binary (`go build -cover`, `GOCOVERDIR`), merged with `go tool covdata merge` across the 3 OSes, nothing may drop;
- **benchstat**: comparison with the base, a significant regression = failure (❓ shared-runner noise to keep under control);
- **govulncheck**;
- single schema: the generated artefacts (JSON Schema, TS types) must be up to date;
- VS Code extension: lint + tests.

## 2. Release and distribution (`release.yml`, on a `v*` tag)
1. GoReleaser: win/linux/darwin × amd64/arm64 binaries, `-trimpath -ldflags "-s -w -X …"`, archives, checksums, **SPDX SBOM**, keyless **cosign** signing, **attestations** (`actions/attest`).
2. `repository_dispatch` (the GregoireF/addlicense model) or dedicated jobs:
   - **npm** (npmjs.com): main package + one package per platform;
   - **WinGet**: `wingetcreate update … --submit`;
   - **Homebrew** and **Scoop**: a formula (`Formula/ship.rb`, prebuilt binaries for macOS and Linux, written by `scripts/homebrew-formula.sh` and pushed by the release job) and a Scoop manifest (`bucket/ship.json`, pushed by GoReleaser) land in `GregoireF/homebrew-tap` (§5, "The tap deploy key"; written, not run yet);
   - **Install scripts**: `scripts/install.sh` (Linux, macOS) and `scripts/install.ps1` (Windows) download a release archive and `checksums.txt`, check the SHA-256 and the origin of the release: the cosign signature of `checksums.txt` when cosign is installed, otherwise `gh attestation verify` of the archive (signer workflow `release.yml`, source ref of the tag) when the GitHub CLI is logged in (§5, "Verifying a release"); `--require-signature` / `-RequireSignature` fails when neither is available. They install to `~/.local/bin` or `%LOCALAPPDATA%\Programs\ship` without admin rights. `install.sh` runs from a `main` called on its last line, so a download cut short runs nothing; both find the latest version through the `/releases/latest` redirect (no API rate limit); the flag variables `SHIP_NO_COSIGN` and `SHIP_REQUIRE_SIGNATURE` count only when they are `1` or `true`; `install.ps1` renames a running `ship.exe` (for example `ship mcp`) to `ship.exe.old` before copying the new one, then deletes it when nothing runs it any more;
   - **VS Code**: one VSIX per target, `vsce publish --target …` **and** `ovsx publish --target …` (always explicit, V4);
   - **Docker**: Alpine + git image (non-root 65532, `build/docker/Dockerfile`), published by GoReleaser on `ghcr.io/gregoiref/ship` (linux/amd64 and linux/arm64), signed (cosign) and attested (§5).
3. Windows signing through SignPath as soon as the project is eligible (public and already published).

## 3. Nightly (`nightly.yml`, scheduled)
- **Real E2E** against the sandbox repository (stacks, rulesets, issue types on a personal repository (V9), draft PRs, compensating actions).
- **Mutation testing** (Gremlins or go-mutesting), score tracked.
- **Drift detection**: schemas of the stacks API and of the MCP spec, state of `githubstatus.com`.
- Long fuzzing.

## 4. Hygiene and dogfooding
- **PR title** validated by ship-it's own parser (we eat our own format).
- OpenSSF Scorecard, zizmor, Dependabot (+ auto-merge of security updates).
- `wiki/` → GitHub Wiki (`wiki-sync`) if D1 settles on that format.
- **`ROADMAP.md` → GitHub** through `ship roadmap sync` as soon as the feature exists.
- **Automatic PNG export of the brand assets** (256 px Marketplace icon, 1280×640 social preview) on every change to `assets/brand/` (rsvg-convert), PNG decision.
- DCO: check of the `Signed-off-by` (N14).

## 5. Workflows and supply-chain security (2026-10-23)

Written in English. Follows [zizmor](https://docs.zizmor.sh)'s audits. Nothing here has run on GitHub yet: the repository is not there.

### Rules every workflow follows
- **`permissions: {}` at the top**, then the least privilege per job, each non-obvious permission explained by a comment (zizmor: `excessive-permissions`, `undocumented-permissions`).
- **Actions pinned by full commit SHA** with a `# vX.Y.Z` comment, resolved from the tag through the GitHub API, annotated tags dereferenced (zizmor: `unpinned-uses`, `impostor-commit`, `ref-version-mismatch`). Dependabot updates them.
- **`persist-credentials: false`** on every checkout: no job pushes with git (zizmor: `artipacked`).
- **No `${{ }}` inside a `run:` script**: expressions go through `env:` (zizmor: `template-injection`). `ci.yml` had two, now fixed.
- **No `pull_request_target`, no `workflow_run`**: no workflow runs untrusted code with a privileged token (zizmor: `dangerous-triggers`).
- **`concurrency`** on every workflow (stale runs cancelled; a release is never cancelled) and **`timeout-minutes`** on every job.
- **No cache in the release workflow**: `setup-go` with `cache: false`, so that a cache written by a pull request can never reach a release (zizmor: `cache-poisoning`).
- **Code scanning gate**: code scanning, dependency review and the upload of SARIF reports need a public repository or a GitHub Code Security / Advanced Security licence on a private one. The jobs that need it run only when the repository variable **`CODE_SCANNING`** is `true`, so a private repository without the licence gets no red runs. Set it when the repository goes public.
- Locally: `go run ./tools/check -workflows` runs actionlint and zizmor (pedantic), see [tools/README.md](../../tools/README.md).

### Workflows

| Workflow | Trigger | Permissions (job level) | Why |
|---|---|---|---|
| `ci.yml` | `pull_request`, push to `main` | `contents: read`; `coverage` also `id-token: write` | Quality gate (§1), same steps as `go run ./tools/check`. The `coverage` job sends the test coverage to Codecov with OIDC (no token), from this repository's branches only; it never fails the pull request. |
| `pr-title.yml` | `pull_request` (opened, edited, synchronize, reopened) | `contents: read` | PR title checked by our own action [`actions/check-title`](../../actions/check-title) built from this commit (`version: source`, §6); the title goes through `env:`. |
| `zizmor.yml` | `pull_request`, push to `main` | `zizmor`: `contents: read`, `actions: read`. `zizmor-sarif` (if `CODE_SCANNING`): same + `security-events: write`. `actionlint`: `contents: read` | Workflows hold tokens: an injection or an over-broad permission is a supply-chain hole. `zizmor` fails the PR on any finding (annotations); `zizmor-sarif` sends findings to the Security tab (that mode never fails, by design of zizmor-action); `actionlint` checks syntax, expressions and `run:` scripts (shellcheck). |
| `codeql.yml` | `pull_request`, push to `main`, weekly | `contents: read`, `actions: read`, `security-events: write`; only if `CODE_SCANNING` | Data-flow analysis that staticcheck and govulncheck do not do. Languages: `go` (manual build, `go build ./...`), `javascript-typescript` (extension), `actions` (workflows). Query suite `security-extended`. |
| `scorecard.yml` | push to `main`, weekly, manual | `contents: read`, `issues: read`, `pull-requests: read`, `checks: read`, `security-events: write`, `id-token: write`; only if `CODE_SCANNING` | OpenSSF Scorecard: measures pinning, token permissions, branch protection, signed releases… Results published to api.scorecard.dev (badge) only if the variable `SCORECARD_PUBLISH` is `true` (public repository). The job only uses the actions Scorecard allows when publishing. |
| `dependency-review.yml` | `pull_request` | `contents: read`; only if `CODE_SCANNING` | Fails a PR that adds a dependency with a known vulnerability of moderate severity or worse (Go, npm, actions). govulncheck stays the reference for Go (reachability). |
| `release.yml` | push of a `v*` tag | `check`: `contents: read`. `release` (environment **`release`**, owner approval): `contents: write`, `id-token: write`, `attestations: write`, `packages: write` | `check` re-runs `tools/check -race`; `release` waits for the owner's approval, then runs GoReleaser ([`.goreleaser.yaml`](../../.goreleaser.yaml)): `ship` for linux, darwin and windows × amd64 and arm64 (`CGO_ENABLED=0`, `-trimpath`, commit date, version injected into `main.version`, `main.commit`, `main.date`), tar.gz (zip on Windows), `checksums.txt`, one SPDX SBOM per archive (syft, CRA engagement E5), **keyless cosign signature** of `checksums.txt` (bundle `checksums.txt.sigstore.json`), a release published by GoReleaser once every file is attached; then `actions/attest-build-provenance` attests every file of `checksums.txt` (CRA engagement E6; the binaries themselves are not code-signed yet); the image `ghcr.io/gregoiref/ship` (linux/amd64 + arm64: QEMU from a digest-pinned binfmt image, buildx v0.38.0, both without cache; login to ghcr.io with the job token) is pushed, signed by cosign and attested (`push-to-registry`, digest read from `dist/artifacts.json`); GoReleaser pushes the Scoop manifest to `GregoireF/homebrew-tap` over SSH with the deploy key `HOMEBREW_TAP_SSH_KEY`, then the step `homebrew formula` writes `Formula/ship.rb` with `scripts/homebrew-formula.sh` from `dist/checksums.txt` and pushes it with the same key, trusting only GitHub's published ed25519 host key (neither for a pre-release). See "Verifying a release" and "The tap deploy key". |
| `outdated.yml` | weekly, manual | `check`: `contents: read`. `issue`: `issues: write` only | `go run ./tools/outdated` compares the pinned tools (below) with their latest releases; when one is behind (and out of its 7-day cooldown), one issue titled "Pinned tools are behind their latest release" is opened or updated, and closed once all are up to date. The repository code runs in the read-only job; the job that writes issues runs no repository code. |
| `dco.yml` | `pull_request` | `contents: read` | `ship check signoff` (built from the pull request, read-only token) on `base.sha..head.sha`, full history fetched (§7). |
| `fuzz.yml` | weekly, manual | `targets`, `fuzz`: `contents: read`. `issues`: `issues: write` only | Long fuzzing of every target, one job each (§8). |
| `triage.yml` | `pull_request`, `issues` (opened), hourly, manual | `label`: `contents: read`, `pull-requests: write`, `issues: write`. `welcome`: `issues: write`, `pull-requests: write` | `ship label` and the first-time contributor welcome, without `pull_request_target` (§9). |
| `stale.yml` | daily, manual | `issues: write`, `pull-requests: write` | `actions/stale` and `dessant/lock-threads`, conservative (§9). |
| `labels.yml` | push to `main` changing `.github/ship.json` or the workflow, manual | `contents: read`, `issues: write` | `ship sync --yes`: GitHub labels as code (§9). |

### Dependabot
`gomod` (root), `github-actions` (root) and `npm` (`extensions/vscode`), weekly. **Cooldown of 7 days** for version updates (zizmor: `dependabot-cooldown`; security updates are not delayed). Minor and patch updates are **grouped** in one PR per ecosystem (npm: one group for development and one for production dependencies); a major update keeps its own PR.

### Versions that Dependabot does not update
zizmor, actionlint, GoReleaser, cosign and buildx are downloaded by a workflow or by `tools/check`, so Dependabot cannot see them. Their single source of truth is [`tools/internal/pins`](../../tools/internal/pins/pins.go): `tools/check` uses it directly, and every YAML use carries a marker right after the version (`version: 1.31.1 # pin: zizmor`) that the package test compares with `pins.go`. `go run ./tools/outdated` (and the weekly `outdated.yml`) reports what is behind. syft follows the `anchore/sbom-action` release, which Dependabot updates.

To bump one: change `pins.go` and every `# pin: <tool>` marker (the test lists any you missed), then run `go run ./tools/check -workflows`.

### Verifying a release
Every release carries `checksums.txt`, its cosign bundle `checksums.txt.sigstore.json`, the archives and one `.sbom.json` per archive. Each archive holds `ship`, `LICENSE` (Apache-2.0), `README.md` and **`THIRD_PARTY_NOTICES.txt`**: the licence of the Go standard library and of every module linked into the binary on any release platform (MIT, BSD-3-Clause, which require their notice in a binary distribution). `go run ./tools/gen` writes it from `go list -deps ./cmd/ship` (GOOS/GOARCH of the release matrix, `CGO_ENABLED=0`) and the LICENSE, COPYING, NOTICE and PATENTS files of each module in the module cache; its test fails when a dependency bump left it stale. The image `ghcr.io/gregoiref/ship` has both files in `/usr/share/licenses/ship/`, where Alpine packages keep their licences (abuild's `default_doc` moves `usr/share/licenses` into the `-doc` subpackage).

```sh
# 1. checksums.txt was signed by the release workflow of GregoireF/ship-it, for that tag
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity "https://github.com/GregoireF/ship-it/.github/workflows/release.yml@refs/tags/vX.Y.Z" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
# 2. the archive matches checksums.txt
sha256sum --ignore-missing -c checksums.txt
# or, with the GitHub CLI: the archive was built by this repository's workflow (SLSA provenance)
gh attestation verify ship_X.Y.Z_linux_amd64.tar.gz -R GregoireF/ship-it
```

### The `release` environment (to create by hand)
The release job runs in the environment `release`. GitHub does not create it from the workflow: create it before the first tag, in **Settings → Environments → New environment** → `release`, then:
- **Required reviewers**: the owner (GregoireF). Only one approval is needed. Leave "Prevent self-review" off: the owner pushes the tag and approves the release.
- **Deployment branches and tags**: "Selected branches and tags", add the tag rule `v*`.
- One environment secret, `HOMEBREW_TAP_SSH_KEY` (next section). Everything else goes through the `GITHUB_TOKEN` and OIDC.

Availability: on GitHub Free, Pro and Team, required reviewers only exist on **public** repositories; ship-it is public, so the approval applies.

### The tap deploy key
The release job pushes the Homebrew formula (`Formula/ship.rb`) and GoReleaser the Scoop manifest (`bucket/ship.json`) to the public repository `GregoireF/homebrew-tap`. The job's `GITHUB_TOKEN` only reaches `ship-it`, so it pushes there over SSH with a deploy key: a key pair whose public half is a deploy key of `homebrew-tap` with write access, and whose private half is the environment secret `HOMEBREW_TAP_SSH_KEY` of `ship-it`'s `release` environment. A deploy key reaches one repository and nothing else, and it does not expire with a personal token. To rotate it:

```sh
ssh-keygen -t ed25519 -N "" -C "ship-it release" -f tap_key
gh repo deploy-key add tap_key.pub -R GregoireF/homebrew-tap --allow-write -t "ship-it release"
gh secret set HOMEBREW_TAP_SSH_KEY --env release -R GregoireF/ship-it < tap_key
rm tap_key tap_key.pub
```

Then delete the old deploy key in the tap's settings. The tap needs a `main` branch (one commit, a README, is enough); The first release creates `Formula/` and `bucket/`. Scoop reads the manifests of a bucket from its `bucket/` folder, so one repository serves both: `brew install gregoiref/tap/ship`, and `scoop bucket add gregoiref https://github.com/GregoireF/homebrew-tap` then `scoop install ship`. A pre-release (`v1.2.0-rc.1`) does not update them (`skip_upload: auto` for Scoop, a check of the tag for the formula).

The release stays a draft while GoReleaser uploads, then GoReleaser publishes it; the human gate is the approval in the `release` environment, before the job runs.

Why a formula and not a cask. The macOS binaries are not signed nor notarized (no Apple Developer ID). A cask would have to remove their quarantine attribute, the workaround GoReleaser documents for unsigned casks ([homebrew_casks](https://goreleaser.com/customization/publish/homebrew_casks/)), and Homebrew maintainers advise against it ([Homebrew discussion 7050](https://github.com/orgs/Homebrew/discussions/7050)). Homebrew only quarantines cask downloads ([`cask/download.rb`](https://github.com/Homebrew/brew/blob/main/Library/Homebrew/cask/download.rb)), so a formula needs no workaround; it also installs on Linux, carries the licence and runs `ship version` as its test. GoReleaser's formula generator, `brews`, is deprecated in favour of `homebrew_casks` ([deprecations](https://goreleaser.com/deprecations/#brews)). It still works in v2.19.0 and deprecated options are only removed in a major version, but `goreleaser check` exits with an error on it ("configuration is valid, but uses deprecated properties"). So the formula comes from `scripts/homebrew-formula.sh`, which reads the four macOS and Linux archives from `checksums.txt`; it does not depend on GoReleaser keeping that feature. Check its output locally with `sh scripts/homebrew-formula.sh 0.0.0-SNAPSHOT-none dist/checksums.txt` after a snapshot (`HOMEBREW_TAP_SSH_KEY` must be set to any value, the Scoop section reads it).
### Going public: checklist
1. Repository variable `CODE_SCANNING` = `true` (Settings → Secrets and variables → Actions → Variables): turns on CodeQL, the zizmor SARIF upload, Scorecard and dependency review.
2. Repository variable `SCORECARD_PUBLISH` = `true`, then add the badge to `README.md`: `[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/GregoireF/ship-it/badge)](https://scorecard.dev/viewer/?uri=github.com/GregoireF/ship-it)`.
3. Environment `release` with the owner as required reviewer and the `v*` tag rule (above), and its secret `HOMEBREW_TAP_SSH_KEY`, the private half of a write deploy key of `GregoireF/homebrew-tap` ("The tap deploy key", above). Without it the release job fails when GoReleaser pushes the Scoop manifest. After the first release, check that `Formula/ship.rb` and `bucket/ship.json` landed in the tap, then try `brew install gregoiref/tap/ship` (on macOS and on Linux; `brew test ship` runs the formula's test), `scoop install ship` and both install scripts.
4. Rulesets on `main`: pull request required, required checks (`check (ubuntu-latest)`, `check (windows-latest)`, `check (macos-latest)`, `extension (…)`, `title`, `zizmor`, `actionlint`, `dependency review`), block force pushes and deletions; a tag ruleset restricting the creation of `v*` tags to the owner; code scanning merge protection, so that zizmor and CodeQL alerts block a merge.
5. Settings → Code security: dependency graph, Dependabot alerts and security updates, secret scanning and push protection, private vulnerability reporting (`SECURITY.md`).
6. After the first release pushed `ghcr.io/gregoiref/ship`: on the package page (github.com/users/GregoireF/packages/container/ship) check that it is linked to the repository (the `org.opencontainers.image.source` label does it; otherwise **Package settings → Connect repository**), then **Change visibility → Public** (a package pushed from a private repository starts private; a public package cannot go back to private). Keep **Manage Actions access** to this repository only.
7. Check the first run of every workflow (none has run yet), and `gh attestation verify` on the first release, for an archive and for the image (`gh attestation verify oci://ghcr.io/gregoiref/ship:X.Y.Z -R GregoireF/ship-it`).
8. Turn on **immutable releases** (repository settings, generally available since 2025-10-28): once published, a release's assets and tag can no longer change, and GitHub adds a release attestation. It suits our flow, which publishes a draft only once every asset is attached.
9. **Rulesets, tested against the real API by hand** (the tests only use recorded answers): create a public test repository (for example `GregoireF/ship-it-sandbox`), add a branch ruleset on `~ALL` in **Active** mode with a **branch name** restriction (regex, for example `^(feat|fix|docs|chore)/`) and a **commit message** restriction (regex on the first line, for example `^\S+ (feat|fix|docs|chore)(\([a-z0-9./_-]+\))?!?: `; start with `(?m)` if it must span lines, and keep `Signed-off-by` trailers in mind). Then, in a clone: `ship` on a change whose branch would break the name rule, and `ship publish`: ship must read the rules (`GET /repos/{owner}/{repo}/rules/branches/{branch}`) and refuse or rename before pushing, with the remedy; then a compliant change must publish. Repeat with the ruleset in **Evaluate** mode (ship must not block) and with a token that cannot read rulesets (ship must carry on). Whether metadata restrictions are available on a personal repository on your plan was not confirmed here: the test repository answers it.
10. **GitHub App for `dependabot-gen.yml`** (so that the checks run on the regenerated commit, §10): Settings (of the account) → Developer settings → GitHub Apps → **New GitHub App**: name it (for example `ship-it-dependabot-gen`), any homepage URL (the repository), **Webhook: untick Active**, **Repository permissions: Contents → Read and write** and nothing else (Metadata read-only is added by GitHub and cannot be removed), **Where can this GitHub App be installed: Only on this account**. Create it, note its **Client ID** (not the App ID: `actions/create-github-app-token` v3 deprecates `app-id` for `client-id`), **Generate a private key** (a `.pem` file). **Install App** → this account → **Only select repositories** → `ship-it`. Then in the repository: Settings → Secrets and variables → **Dependabot** → New repository secret `APP_CLIENT_ID` (the client ID) and `APP_PRIVATE_KEY` (the whole `.pem`, header and footer lines included); delete the `.pem` from the disk. They must be **Dependabot** secrets: a Dependabot-triggered run reads no Actions or environment secret. Without them, the workflow keeps pushing with the job token and prints a notice.

### Not done yet
GoReleaser publishers for WinGet and npm, deb and rpm packages, Windows code signing, macOS notarization, nightly, coverage of the VS Code extension, benchstat.

Known limit: on a pull request from a fork, GitHub downgrades `security-events: write` to read, so the SARIF uploads (`zizmor-sarif`, `codeql`) fail there; the blocking `zizmor` and `actionlint` jobs still run.

## 6. Reusable action: `actions/check-title`
[`actions/check-title`](../../actions/check-title/README.md) is a composite action that other repositories can use to check their pull request titles with ship-it's conventions:

```yaml
- uses: GregoireF/ship-it/actions/check-title@<full commit SHA> # vX.Y.Z
  with:
    title: ${{ github.event.pull_request.title }}
    version: X.Y.Z   # a ship release; "source" builds ship from the action's commit
```

- **Release mode** (`version: X.Y.Z`): downloads the archive of the runner's OS and architecture from the GitHub release and checks its SHA-256 against `checksums.txt`; the version must match `X.Y.Z[-pre]` before it is used in a URL.
- **Source mode** (`version: source`): installs Go (`actions/setup-go`, no cache) from the action's `go.mod` and builds `./cmd/ship` from the action's own checkout. ship-it's own `pr-title.yml` uses it this way, so the parser under review checks its own pull request.
- The title only reaches `ship` through an environment variable (no template injection); the binary path goes through step outputs, not `GITHUB_ENV`.
- Location: a subdirectory of the repository, used as `{owner}/{repo}/{path}@{ref}` (workflow syntax reference). Locally, `pr-title.yml` uses `./actions/check-title` after checkout; zizmor's `self-repository` audit suggests the newer `$/actions/check-title` form, but actionlint v1.7.12 rejects it, so that finding is ignored inline.
- **`verify-attestation: "true"`** (optional, default `"false"` until it is tested on the first public release): after the checksum, `gh attestation verify <archive> --repo GregoireF/ship-it --signer-workflow GregoireF/ship-it/.github/workflows/release.yml`, with the job's token. The checksum alone only proves the download is intact; the attestation proves ship-it's release workflow built it.

### Marketplace: `GregoireF/ship-it-action`
The Marketplace lists only an `action.yml` at the root of a public repository, one action per repository. `go run ./tools/gen` therefore writes `build/ship-it-action/` (its test fails when the copy is stale): `action.yml` generated from `actions/check-title/action.yml` without the `source` mode (the blocks between `# marketplace:omit-begin` and `# marketplace:omit-end`), a README, and the Apache-2.0 `LICENSE`. Branding: Feather icon `check-circle`, colour `blue` (metadata syntax: Feather v4.28.0 icons; colours white, black, yellow, blue, green, orange, red, purple, gray-dark). Nothing has been created on GitHub. The owner's steps, once ship-it has a public release:
1. Create the **public** repository `GregoireF/ship-it-action` (empty, no template), with 2FA on the account.
2. Copy the content of `build/ship-it-action/` at the root, commit with sign-off, push to `main`.
3. Repository settings: turn on **immutable releases**; a ruleset on `main` (pull request required, no force push).
4. Open `action.yml` on GitHub → **Draft a release** → tick **Publish this Action to the GitHub Marketplace** (accept the Marketplace Developer Agreement the first time), fix any metadata warning until "Everything looks good!", choose a primary category (the list is shown in that form; not checked from here).
5. Tag `v1.0.0`, title `v1.0.0`, publish. Then point the major tag at it: `git tag -f v1 v1.0.0 && git push -f origin v1`. Only the major tag moves; no release is ever made on `v1`, so immutable releases (which freeze the tags of releases) do not block it.
6. Each new version: regenerate in ship-it (`go run ./tools/gen`), copy, commit, release `vX.Y.Z`, move `v1`. A breaking change of the inputs makes `v2`.

Known limit: the name `ship-it check title` must be unique on the Marketplace; GitHub checks it at step 4.

## 7. DCO (`dco.yml`)
Every commit of a pull request must carry a `Signed-off-by` trailer naming its author or committer (Developer Certificate of Origin 1.1, decision N14). The check is ship-it's own `ship check signoff` (docs/reference/cli.md), built from the pull request with a read-only token; the range is `base.sha..head.sha` with the whole history fetched (`fetch-depth: 0`). Same rule as the DCO GitHub App (name and email of the author or committer, without case; merges and GitHub App bots exempt), but only the trailer block counts, as for git. The fix it prints: `git rebase --signoff <base> && git push --force-with-lease`.

## 8. Weekly long fuzzing (`fuzz.yml`)
- `targets` lists every Fuzz function with `go run ./tools/fuzz -list` (`go test -list '^Fuzz' ./...`, shared with `tools/check` through `tools/internal/fuzztargets`): 14 targets on 2026-10-24.
- `fuzz`: one matrix job per target, `go run ./tools/fuzz -run -package P -name FuzzX -time 10m` (`-timeout` set above the fuzz time), `fail-fast: false`, `max-parallel: 6` (GitHub Free allows 20 concurrent jobs; a matrix at most 256 jobs; a job at most 6 hours). On failure, the artifact `fuzz-<target>` (30 days) holds `crashers/` (the failing inputs go wrote under `testdata/fuzz/<target>/`), `corpus/` (the generated corpus from the build cache, at most 2,000 files), `report.md` and `title.txt`.
- `issues` (only `issues: write`, no checkout, no repository code) downloads the reports and opens one issue per failing target, titled `fuzz: <target> fails in <dir>`, or comments on it when it is already open. The report gives the reproducer path and command (`go test -run='^FuzzX$/^<file>$' ./<dir>`) and the end of the output.
- Cost: 14 targets × about 12 minutes ≈ 170 Linux minutes a week. Free on a public repository (standard runners); about 700 of the 2,000 monthly minutes of GitHub Free on a private one. `workflow_dispatch` takes a shorter `fuzztime` to try it.
- Not done: keeping the corpus from one week to the next (it would need a cache, to be weighed against cache poisoning).

## 9. Triage
- **Labels without `pull_request_target`.** zizmor's `dangerous-triggers` audit flags `pull_request_target`, `workflow_run` and `issue_comment` and warns that "not running the pull request's code" is not enough to make them safe. `triage.yml` therefore uses only `pull_request`, `issues`, `schedule` and `workflow_dispatch`. Its `label` job runs `ship label --pr N` (docs/reference/cli.md): ship's own `kind/` and `size/` logic, computed from the pull request's commits and files through the API, lockfiles and `linguist-generated` files excluded. ship is built from the base branch, never from the pull request. A fork's `pull_request` event gets a read-only token, so: same-repository pull requests are labelled at once, every open pull request (forks included) by the hourly run. Permissions: `contents: read`, `pull-requests: write` (labels on a pull request need it; `issues: write` alone gets a 403), `issues: write`.
- **Welcome.** `actions/first-interaction` (v3.1.0) documents `pull_request` and `issues` triggers; on `pull_request` it cannot comment on a fork's pull request. So `gh` does it: on `issues: opened` when `author_association` is `FIRST_TIMER` or `FIRST_TIME_CONTRIBUTOR`, and in the hourly run for open pull requests with that association that do not have the `<!-- ship-it:welcome -->` comment yet.
- **Stale and lock** (`stale.yml`, daily): `actions/stale` v11.0.0 marks issues and pull requests `lifecycle/stale` after 90 days without activity and closes issues 30 days later (reason "not planned"); pull requests are never closed by a bot; `lifecycle/frozen`, `lifecycle/active`, assigned items, milestones and drafts are exempt. `dessant/lock-threads` v6.0.2 locks issues and pull requests closed for 365 days, except `lifecycle/frozen`.
- **Labels as code** (`labels.yml`): on a push to `main` that changes `.github/ship.json` or the workflow itself (so a new repository gets its labels without a `.github/ship.json`), `ship sync --yes` creates, renames and recolours the labels it declares (it never deletes one), with the job token (`issues: write`) read from `GH_TOKEN`.

## 10. Generated files on Dependabot pull requests (`dependabot-gen.yml`)
A dependency bump changes `THIRD_PARTY_NOTICES.txt` (and possibly other outputs of `go run ./tools/gen`), and the staleness test of `tools/gen` would fail the pull request. `dependabot-gen.yml` regenerates and commits the result to the Dependabot branch.

- **Trigger: `pull_request`, no `pull_request_target`** (zizmor `dangerous-triggers`). GitHub treats a Dependabot-triggered `pull_request` run like a fork's (read-only token, Dependabot secrets only) and documents that "you can use the `permissions` key in your workflow to increase the access for the token". A manual re-run keeps the original run's privileges.
- **Only Dependabot's own pull requests**: `github.event.pull_request.user.login == 'dependabot[bot]'` (who opened it; zizmor's `bot-conditions` warns that `github.actor` can be spoofed) and a head branch in this repository.
- **Two jobs.** `generate` (`contents: read`) checks out the branch's own commit and runs `go run ./tools/gen`: that compiles the bumped dependencies, so it never holds a write token; it hands over the files it changed, with their list (`changed.txt`), as an artifact (1 day). `commit` (`contents: write`) never checks out the pull request (CodeQL `actions/untrusted-checkout`) and runs no repository code: it checks that the branch has not moved, refuses any path outside the `ALLOWED` list (every output of `tools/gen`; `TestDependabotWorkflowAllowsEveryOutput` keeps it in step) and any file that is not a regular file, then makes the commit through GitHub's Git Data API: one blob per file, a tree on the head commit's tree, a commit whose parent is the head commit, and a fast-forward of the branch (`force: false`, so it fails if the branch moved). The commit has the title `📦️ build(deps): regenerate the generated files` (it passes `ship check title`) and a sign-off by its author, the App's bot user (or `github-actions[bot]`); the DCO check exempts bot authors anyway. A commit made through the API with the App's token is signed by GitHub.
- **`[dependabot skip]` in the commit message**: by default Dependabot stops rebasing a pull request once others push to it; this marker lets it keep force-pushing (the workflow then regenerates).
- **The push token.** "Events triggered by the `GITHUB_TOKEN` will not create a new workflow run", so a commit made with the job token would be left without checks. The `commit` job therefore mints an installation token of a dedicated GitHub App with `actions/create-github-app-token` v3.3.0 (`client-id` and `private-key` from the Dependabot secrets `APP_CLIENT_ID` and `APP_PRIVATE_KEY`; without `owner` it is scoped to this repository; `permission-contents: write` and nothing else; revoked at the end of the job by the action). Its commit starts the checks. Setting it up: checklist item 10 (done for ship-it on 2026-11-07). Without the secrets, it falls back to the job token (`contents: write`) and prints a notice: close and reopen the pull request before merging (`@dependabot rebase` would not help: it replaces the commit and the workflow pushes a new one). zizmor's auditor-only `secrets-outside-env` is ignored inline on these lines: a Dependabot-triggered run cannot read environment secrets.
