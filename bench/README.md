# bench

Micro-benchmarks live next to the code (`testing.B.Loop`). This folder holds the **macro** and **agent** benchmarks.

| Folder (planned) | Measures |
|---|---|
| `repos/` | Synthetic repositories (1k / 10k / 100k files, large diffs) for `ship --dry-run`, checkpoints, status |
| `agents/` | Same task (ship a PR, undo it) through ship-it MCP vs Pare vs plain Bash: tool calls, measured tokens, duration, success |

Methodology and results are published in the README. See `docs/design/quality-and-tests.md`.
