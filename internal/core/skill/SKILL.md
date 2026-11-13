---
name: ship-it
description: Deliver work in a git repository with ship-it - conventional commits, branches, draft pull requests and undo. Use it whenever the task is to commit, push, open or update a pull request, split work into a stack, or undo a delivery, in a repository where the ship command or the ship_* MCP tools are available.
---

<!-- ship-it skill, written by `ship skill install`; `ship skill install` updates it. -->

# Delivering work with ship-it

ship-it turns finished work into a conventional commit on its own branch and a pull request, with a checkpoint before every change so that it can be undone. Let it do the git and GitHub part: do not run `git commit`, `git push` or `gh pr create` yourself.

## Use the MCP tools when you have them

If the `ship_*` tools are available, use them; otherwise use the `ship` command with the same steps (below).

1. `ship_status`: where the work stands and the one next action.
2. `ship_plan`: what a delivery would do (title, branch, files, secrets found). Read it before going on. If it reports a secret, stop and tell the human.
3. `ship_deliver`: one conventional commit on a dedicated branch, with a checkpoint first. Pass `title` only to improve the planned one; it is checked against the project's conventions. Pass `why` and `tested` when you know them.
4. `ship_publish`: runs the project's checks, pushes and opens a **draft** pull request, or updates the open one. A red check stops it: fix the cause, then publish again.
5. `ship_undo`: reverts the last local operation, keeping the files. Undoing a publication is left to a human.

Large work: `ship_plan` says so (`size`); `ship_stack` splits it into one draft pull request per layer. Problems: `ship_doctor` lists them, each with the command that fixes it.

## Without MCP: the ship command

| Step | Command |
|---|---|
| Where it stands | `ship status` |
| Plan, changing nothing | `ship plan` |
| Commit only | `ship --local --yes` |
| Commit and open the pull request | `ship --yes` |
| Publish what is committed | `ship publish` |
| Preview the pull request | `ship preview` |
| Undo the last operation | `ship undo --yes` |
| Check a title | `ship check title -- "<title>"` |

Add `--json` to read the result, and `--dry-run` to see what would happen without changing anything.

## Rules

- Never push to the default branch, never force-push, never rewrite pushed history: ship-it works on branches and pull requests.
- Never edit or delete `.git/ship`: it holds the checkpoints that make undo possible.
- Titles follow the repository's conventions (`ship check title` says whether one does); never bypass a red check, a ruleset or a policy, and never use `--skip-gates` on your own.
- Merging pull requests, undoing a publication and uninstalling ship-it are a human's decisions: propose them, do not do them.
- When ship-it refuses something, read its message: it names the error code (`SHIP-E…`) and the command that fixes it.
