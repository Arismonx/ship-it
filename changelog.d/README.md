# Changelog fragments

A pull request that changes what users see adds one file here instead of editing [CHANGELOG.md](../CHANGELOG.md), so that pull requests never conflict on it. Name it after your branch, for example `changelog.d/feat-ship-bug.md`:

```markdown
### Added

- `ship bug`: a bug report built from a run's journal (#53).
```

- Sections are those of [Keep a Changelog](https://keepachangelog.com/en/1.1.0/): Added, Changed, Deprecated, Removed, Fixed, Security.
- One `- ` line per change, written for users, ending with the issue or pull request number. Thank an outside contributor as the other entries do: `(#107, thanks @painbaba)`.
- `go run ./tools/releasenotes -pending` shows what the next release will say; a test rejects a fragment it cannot read.
- At release, `go run ./tools/releasenotes -release <version> <date>` moves the entries into CHANGELOG.md and removes the fragments.

Changes that users do not see (tests, CI, refactoring) need no fragment. The `changelog` check of every pull request asks for one when the title is `feat`, `fix`, `perf` or `security`; for such a change that users still do not see, a maintainer adds the `changelog/skip` label.
