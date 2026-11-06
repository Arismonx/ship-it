# Plan your project with ROADMAP.md

Write your plan as a markdown file; ship keeps GitHub's milestones and issues in line with it.

## 1. Write the roadmap

```markdown
## Phase 1: Foundations
**Due:** 2026-12-15
- [ ] feat: command-line skeleton
  Binaries for Linux, macOS and Windows.
  - [ ] git layer
- [x] Architecture decisions
**Exit:** a junior ships a clean pull request.
```

- Each `##` heading with a checklist is a **milestone**; `**Due:**` sets its date, `**Exit:**` its description.
- Each item is an **issue**; indented text is its description; indented items are **sub-issues**.
- A `feat:` or `fix:` prefix adds the matching `kind/` label, when the repository has it.

## 2. Sync

```sh
ship roadmap sync
```

ship shows the plan (milestones and issues to create or update, boxes to tick) and applies it when you press Enter. Then it writes the issue numbers into the file (`- [ ] git layer #13`): deliver that change with `ship` like any other.

## 3. Keep it in sync

- Closing an issue on GitHub (a merged pull request does it) ticks its box at the next sync; ticking a box closes the issue.
- Removing an item closes its issue as *not planned*; ship never deletes anything.
- `ship undo` reverts a sync, after you confirm, because it changes GitHub.

The full rules are in [the roadmap design](../design/github-automation.md#6-planning-roadmapmd--github).
