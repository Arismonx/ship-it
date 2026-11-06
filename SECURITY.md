# Security policy

## Supported versions

Security fixes go to the latest release only. Upgrade to it before reporting, if you can.

## Reporting a vulnerability

Please do not open a public issue. Use GitHub's private vulnerability reporting on this repository: the "Security" tab, then "Report a vulnerability".

Include the version (`ship version`), your platform, the steps to reproduce and the impact you see.

ship-it has a single maintainer. You get a first answer within 7 days, and a fix is aimed for within 90 days, in line with coordinated disclosure. You are credited in the release notes unless you ask otherwise.

## What every release carries

- an SBOM (SPDX) attached to the release;
- build provenance attestations and cosign signatures;
- a `govulncheck` run in CI on every change.

## Scope

ship-it never sends telemetry. AI features are opt-in and an organisation policy can forbid them. Checkpoints are stored on your machine and never pushed by ship-it.
