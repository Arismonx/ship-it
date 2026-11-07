# Security policy

## Supported versions

Security fixes go to the latest release only. Upgrade to it before reporting, if you can.

## Reporting a vulnerability

Please do not open a public issue. Report it privately through GitHub's private vulnerability reporting: <https://github.com/GregoireF/ship-it/security/advisories/new> (the "Security" tab, then "Report a vulnerability").

Include the version (`ship version`), your platform, the steps to reproduce and the impact you see.

ship-it has a single maintainer. You get a first answer within 7 days, and a fix is aimed for within 90 days, in line with coordinated disclosure: the advisory is published once the fix is released, or after 90 days if we agree on it. You are credited in the release notes and in the advisory unless you ask otherwise.

## What every release carries

- an SBOM (SPDX) for each archive, attached to the release;
- build provenance attestations, and a cosign signature of the checksums file;
- a `govulncheck` run in CI on every change, and CodeQL on every pull request.

How to check them: [install](README.md#install) and [the release workflow](docs/design/workflows.md).

## Scope

ship-it never sends telemetry. AI features are opt-in and an organisation policy can forbid them. Checkpoints are stored on your machine and never pushed by ship-it.

## OpenSSF Scorecard

The [Scorecard](https://scorecard.dev/viewer/?uri=github.com/GregoireF/ship-it) of this repository has a few checks that stay low for reasons that are not hidden problems:

- **Code-Review**: ship-it has one maintainer, so changes go through pull requests and CI but nobody else approves them. It goes up with a second maintainer.
- **Maintained**: the repository is younger than 90 days.
- **SAST**: CodeQL runs on every pull request since 2026-11-07; older commits were checked by `staticcheck`, `govulncheck` and zizmor only. The ratio goes up with each new change.
