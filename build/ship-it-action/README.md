# ship-it check title

Check pull request titles against [ship-it](https://github.com/GregoireF/ship-it)'s commit conventions: gitmoji first, a Conventional Commits type, a subject of at most 50 characters, a header of at most 72. With squash merges, the title becomes the commit header.

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
      - uses: GregoireF/ship-it-action@<full commit SHA> # vX.Y.Z
        with:
          title: ${{ github.event.pull_request.title }}
          version: X.Y.Z
```

Pin the action by full commit SHA. The major tag (`v1`) follows the latest compatible release; every `vX.Y.Z` release is immutable.

## Inputs

| Input | Required | Meaning |
|---|---|---|
| `title` | yes | The title to check. It reaches `ship` through an environment variable, never through the script text, so a hostile title cannot inject commands. |
| `version` | yes | A ship release (`1.2.0` or `v1.2.0`): the archive for the runner's OS and architecture is downloaded from the GitHub release of GregoireF/ship-it and its SHA-256 checked against the release's `checksums.txt`. |
| `verify-attestation` | no | `"true"` also runs `gh attestation verify` on the archive (built by ship-it's release workflow), with the job's token. Default `"false"`. |

Linux, macOS and Windows runners, x64 and ARM64. No permission is needed.

## Source and license

This repository is generated from [`actions/check-title`](https://github.com/GregoireF/ship-it/tree/main/actions/check-title) of GregoireF/ship-it: issues and changes go there. Licensed under the Apache License 2.0 (see LICENSE).
