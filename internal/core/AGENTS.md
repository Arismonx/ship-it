# AGENTS.md: internal/core

This is the **pure domain**. Rules here are stricter than at the root:

- No I/O of any kind: no `os`, `os/exec`, `net/*`, `io/fs` access to the real disk, no time.Now() (take a clock value as input).
- No third-party imports. Standard library and other `core` packages only.
- No user-facing strings: return message keys + parameters (`internal/i18n` renders them).
- Functions are deterministic: same input, same output. This is what makes golden, fuzz and property tests possible.
- Every new behaviour comes with a table test; parsers also get a fuzz target.
