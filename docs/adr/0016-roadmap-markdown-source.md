# ADR-0016: ROADMAP.md as the source of truth

**Status:** accepted (user choice)

## Decision
`ROADMAP.md` (phases → milestones, items → issues, sub-items → sub-issues) is synchronized **to** GitHub by `ship roadmap sync` (plan → diff → apply). The `#n` numbers are written back into the markdown. The status (checked box) follows the issue's state read-only: an issue closed on GitHub checks the box at the next sync.

## Consequences
- Writing a roadmap stays as quick as writing a file.
- A rename in the GitHub UI will be overwritten at the next sync: this is documented.
- The format and the rules were frozen on 2026-11-01 and are implemented by `ship roadmap sync`: see [github-automation.md §6](../design/github-automation.md#6-planning-roadmapmd--github). Revision of the same day: a box ticked or unticked in the file closes or reopens its issue when the local memory of the last sync shows the file changed, not GitHub; without that memory GitHub wins.
- Revision of 2026-11-07 (#87): an issue's body has two owners. Above the invisible marker that ends the generated part, the file wins and each sync rewrites it; below it, the text people write on GitHub (context, where to start, what done means, open questions for contributors) is kept as it is, and a change there alone never makes an update. The roadmap stays short to read while each issue carries the details.
