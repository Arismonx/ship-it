# tools

Developer tasks written in Go (decision R3: no Make, no Taskfile). Run with `go run ./tools/<task>` from the module root. `internal/archtest` treats `tools/` like `cmd/`: it may import anything, nothing may import it, except `tools/internal/*`, shared by the tasks (Go's `internal` rule keeps it inside `tools/`).

| Task | Purpose | State |
|---|---|---|
| `gen` | Write the generated files: `schemas/*.schema.json` and `docs/reference/settings.md` (from `internal/core/config`), the extension's tool types, `build/ship-it-action/`, and `THIRD_PARTY_NOTICES.txt` (licences of the modules linked into `ship`, from `go list -deps` and the module cache; a mode of `gen` rather than a task of its own, so that one command and one test cover every generated file). Its test fails when a generated file is out of date | ✅ |
| `check` | Every verification of a pull request, as CI runs it: gofmt, vet, staticcheck, SPDX headers (addlicense), tests (`-race`), short fuzzing of every target (`-fuzztime=10s`), govulncheck. `-workflows` adds actionlint and zizmor on `.github/`; `-bench` adds the performance budgets (`bench`) | ✅ |
| `bench` | Run the benchmarks of `tools/bench/budgets.json` (`go test -bench`, median of `-count` runs) and compare them with their budgets: a ceiling on the time per operation, with headroom for CI machines, and a ceiling on the git processes per command (`git/op`, machine-independent). Prints a table; exit 1 when a budget is exceeded or a benchmark did not run. `-run <regexp>` selects benchmarks. Run by `check -bench` and by the `bench` job of `ci.yml` (docs/design/performance.md) | ✅ |
| `outdated` | Compare the tool versions pinned in `tools/internal/pins` (zizmor, actionlint, GoReleaser, cosign: what Dependabot cannot update) with their latest releases (GitHub releases API, PyPI JSON API; `GH_TOKEN` used if set). 7-day cooldown; `-format=markdown` for the tracking issue; exit 1 when something is behind. Run weekly by `.github/workflows/outdated.yml` | ✅ |
| `gen` (next) | `extensions/vscode/src/generated/` from `internal/schema` | planned |
| `docs` | Generate reference docs and the error playbook from `testdata/script/` | planned |
| `cover` | Merge unit and integration coverage (`go tool covdata merge`) | planned |
| `brand` | Rasterise `assets/brand/*.svg` to PNG (Marketplace icon, social preview) | planned |

Third-party developer tools (linters, govulncheck, addlicense…) are pinned with the `tool` directive in `go.mod` and run with `go tool <name>`.

## Workflow linters (`-workflows`)

`go run ./tools/check -workflows` lints `.github/` like `.github/workflows/zizmor.yml` does (docs/design/workflows.md §5). No Go module dependency is added:

- **actionlint** is built with `go run github.com/rhysd/actionlint/cmd/actionlint@<version>` (verified against sum.golang.org; `go.mod` is untouched). It also runs shellcheck on `run:` scripts when shellcheck is on the `PATH`.
- **zizmor** (Rust) runs through [uv](https://docs.astral.sh/uv/): `uvx zizmor@<version> --persona=pedantic .`. Install uv first. With `GH_TOKEN` set (for example `GH_TOKEN=$(gh auth token)`), zizmor also runs its online audits (known vulnerable actions, impostor commits).

The versions come from `tools/internal/pins` (single source of truth, see docs/design/workflows.md §5); `go run ./tools/outdated` tells when to bump them.

Validate the release configuration with `go run github.com/goreleaser/goreleaser/v2@<version> check` and try it with `… release --snapshot --clean --skip=sbom,sign` (output in `dist/`, ignored by git). The snapshot also builds the image `ghcr.io/gregoiref/ship` (`dockers_v2`): that needs a running Docker with buildx and QEMU (`docker run --privileged --rm tonistiigi/binfmt --install all`); without them add `--skip=docker` (`--skip=sbom,sign,docker`).
