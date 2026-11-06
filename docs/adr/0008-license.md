# ADR-0008: License

**Status:** revised on 2026-11-06: Apache-2.0 replaces EUPL-1.2 (user choice). The original decision is kept below for the record.

## Revision of 2026-11-06: Apache-2.0

The goal of ship-it moved towards wide adoption by developers and companies. An outside review noted that corporate legal teams know the EUPL less than the permissive licences, which slows adoption of a tool meant for everyone. The author owns all the code, so the licence could change before the first public release.

- Apache-2.0 is permissive, OSI approved, widely known by legal teams, and it carries an explicit patent grant, which MIT does not.
- It stays on the list of licences authorised for French public administrations (CRPA D.323-2-1), and OSI approval keeps the free signing route (SignPath) open.
- It does not protect against a closed fork, which the EUPL did; this is accepted for a developer tool.

Consequences: `LICENSE` holds the official Apache License 2.0 text from apache.org; every SPDX header reads `Apache-2.0` and `addlicense --check` enforces it; packaging metadata (GoReleaser, the Homebrew formula, Scoop, the VS Code extension, the image labels) says Apache-2.0.

## Original decision (2026-10-22): EUPL-1.2

## Context
- MIT, Apache-2.0 and the EUPL are authorized for French public administrations (CRPA D.323-2-1; EUPL added by decree 2021-1559).
- The EUPL is the reference license of the European public sector: copyleft extended to network use, compatible with the GPL, AGPL, LGPL, MPL…
- SignPath (free signing) requires an OSI license **without a commercial dual license**.

## Pros / cons
- ✅ Protects against a proprietary or closed SaaS takeover; legitimacy with European public administrations; available in 23 official EU languages (❓ to confirm).
- ⚠️ Some corporate lawyers are less familiar with the EUPL than with MIT or Apache. This is not a blocker for a **tool** that is used without being embedded in one's product.
- ⚠️ Prevents a future commercial dual license if we want to keep SignPath free.

## Consequences
`LICENSE` = official text; SPDX headers `EUPL-1.2`; dependency compatibility check.
