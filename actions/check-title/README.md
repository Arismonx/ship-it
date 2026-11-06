# ship-it check title

A composite GitHub Action that checks a pull request title with ship-it's own parser (`ship check title`): gitmoji first, a Conventional Commits type, a subject of at most 50 characters and a header of at most 72 (docs/reference/conventions.md). With squash merges, the title becomes the commit header.

```yaml
name: pr-title
on:
  pull_request:
    types: [opened, edited, synchronize, reopened]
permissions: {}
jobs:
  title:
    runs-on: ubuntu-latest
    timeout-minutes: 10
    permissions:
      contents: read
    steps:
      - uses: GregoireF/ship-it/actions/check-title@<full commit SHA> # vX.Y.Z
        with:
          title: ${{ github.event.pull_request.title }}
          version: X.Y.Z
```

Pin the action by full commit SHA, like any third-party action.

## Inputs

| Input | Required | Meaning |
|---|---|---|
| `title` | yes | The title to check. It reaches `ship` through an environment variable, never through the script text, so a hostile title cannot inject commands. |
| `version` | yes | A ship release (`1.2.0` or `v1.2.0`): the archive for the runner's OS and architecture is downloaded from the GitHub release and its SHA-256 checked against the release's `checksums.txt`. `source` builds `ship` with Go from the action's own commit instead (ship-it uses this on its own pull requests). |
| `verify-attestation` | no | `"true"` also runs `gh attestation verify` on the downloaded archive (`--repo GregoireF/ship-it --signer-workflow GregoireF/ship-it/.github/workflows/release.yml`), with the job's token. Default `"false"` until it is tested on the first public release. |

Runs on Linux, macOS and Windows runners, x64 and ARM64. No token and no permission are needed. The `source` mode installs Go with `actions/setup-go` (no cache).

To check that the release itself is genuine, see "Verifying a release" in [docs/design/workflows.md](../../docs/design/workflows.md) (cosign bundle and build provenance).

The GitHub Marketplace lists one action per repository, at its root: the same action, without the `source` mode, is generated into `build/ship-it-action/` by `go run ./tools/gen` for the repository `GregoireF/ship-it-action` (docs/design/workflows.md §6). Edit this file, never the copy.
