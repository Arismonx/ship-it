# ADR-0007: `✨ feat(scope): subject` format

**Status:** accepted (user choice)

## Context
- Conventional Commits 1.0.0: the message **must start with the type** ✅.
- gitmoji: `<intention> [scope?][:?] <message>`, unicode or shortcode ✅.
- With a squash merge, the PR title becomes the commit on the base.

## Options
| Format | Pros | Cons |
|---|---|---|
| **`✨ feat(ui): subject`** | Maximum readability in PR lists and in the history | **Not strict CC**: commitlint and release-please do not parse it as is |
| `feat(ui): ✨ subject` | Valid CC and visual | Emoji less visible |
| No emoji | Sobriety | Less readable |

## Decision
`✨ feat(ui): subject` by default, a **unicode** emoji from the official gitmoji table, setting `format.emoji`: `leading` | `after-type` | `none`.

## Consequences
- `ship release` uses its own parser, which accepts the optional leading emoji.
- Teams that depend on commitlint or release-please switch to `after-type` or `none`. `ship init` detects these tools and offers it.
- ❓ Publish a compatible commitlint preset.
