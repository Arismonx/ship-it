# ADR-0013: Built-in releases

**Status:** proposed (user instruction: "aim for agency level at best")

## Proposal
`ship release`:
1. reads the commits since the last semver tag (emoji-tolerant parser, ADR-0007);
2. computes the bump (`!`/`BREAKING CHANGE` → major, `feat` → minor, otherwise patch). **In 0.x** (a proposal based on the semver.org FAQ): breaking → minor, feat → minor, fix → patch; moving to 1.0.0 is an explicit human decision;
3. writes `CHANGELOG.md` (Keep a Changelog) and, optionally, non-technical **client notes**;
4. opens a **release PR** (reviewed by a human), whose merge triggers the tag and the GitHub release.

## For agencies
- Client notes in the client's language, without jargon.
- ❓ Monorepos (per-package versions), calendar versions, projects without semver (brochure sites): to settle.

## Alternative
Delegate to release-please or changesets. **Ruled out** (decision H4): **`ship release` replaces changesets**, eventually on the author's repositories too. When `.changeset/` is detected, `ship init` offers a **migration** (reading the pending changesets, converting them into changelog entries, removing the config) as plan → diff → apply, and never runs both systems in parallel.
