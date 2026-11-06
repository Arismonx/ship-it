# Vision

## Problem

Versioning is a tax. A developer who delivers a feature must also:

1. create and name a branch;
2. split the work into readable commits and write their messages according to the team's convention;
3. sync with the base (rebase, conflicts);
4. push, open a PR, describe it, add labels, pick reviewers, link an issue;
5. watch CI and reviews, fix, push again;
6. clean up merged branches;
7. publish a version (semver, changelog, tag, release).

None of this is business value. For a junior developer or a designer, every step is a chance to be afraid of "breaking git". For a senior developer, it is minutes lost ten times a day. For an AI agent, it is where the code it produced turns into an unreadable history.

## Promise

> **You code. You type `ship` (or click ⛵). It is delivered, cleanly, and everything can be undone.**

## Product principles

1. **Zero git knowledge required.** No command and no message may assume the user knows what a rebase is.
2. **Everything can be undone.** Every ship-it action is recorded and reversible (checkpoints and oplog). Fear is the first obstacle.
3. **The human reviews, they do not write.** The human steps in rarely and effortlessly: approve a plan with one key, review a PR, approve a release. They never do the plumbing.
4. **Deterministic first, AI second.** The tool works entirely without AI. AI improves the messages and the split; it never gates how the tool works.
5. **Sovereign by default.** Not a single byte of code leaves the machine for an AI without explicit opt-in. An organisation can forbid it.
6. **An error is not a dead end.** Every known error comes with its resolution, automatic when it is safe, guided otherwise ([erreurs.md](../reference/error-playbook.md)).
7. **One binary, no installation chores.** No runtime, no admin rights, no `gh`, no Node. Only `git` is required.
8. **Respect existing conventions.** Conventional Commits, gitmoji, semver, Keep a Changelog: we follow the standards, we do not invent new ones.
9. **Built for agents as much as for humans.** Every action is exposed as JSON and through MCP, idempotently and with no hidden prompt.

## Personas

| Persona | Main need | What ship-it does for them |
|---|---|---|
| **Junior / designer** | Not be afraid, break nothing | VS Code: one button, a readable plan, an undo, errors explained in plain language |
| **Busy senior** | Speed, control, scriptability | `ship -y`, stacks, flags, JSON output, no chatter |
| **AI agent** (Claude Code, Cursor, Codex…) | Deliver what it produced, cleanly | MCP tools `ship_plan`, `ship_deliver`, `ship_status`, `ship_undo`, with the agent writing the plan itself |
| **Lead / agency** | Consistency across clients and projects | Locked organisation policy, client releases and changelogs |
| **IT department / public administration** | Compliance, sovereignty | AI can be disabled centrally, signed binary, SBOM, Apache-2.0, installation without admin rights |

## Non-goals (v1)

- Replace git or reinvent its model (as jj or Sapling do). ship-it **drives** git.
- Support GitLab, Bitbucket or Gitea in v1: only GitHub is targeted. The architecture still keeps a `Forge` interface.
- Be a full git GUI (GitButler, Fork and lazygit already do that).
- Do AI code review (Copilot, Graphite/Cursor and CodeRabbit already do that).

## Where ship-it stands apart

In short: **end to end** (from WIP to release), **resilient to errors**, **sovereign and governable**, and **multi-surface**, with the same core for the human in the CLI, the human in VS Code and the agent through MCP.
