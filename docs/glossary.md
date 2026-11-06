# Glossary

The words you meet in ship-it's messages and documentation, in one or two sentences each.

**Branch**: A separate line of work in git. You work on a branch so that `main` stays clean until your work is reviewed. ship creates one for you when you are on `main`.

**Base branch** (or default branch): The branch your work goes into once accepted, usually `main`. ship reads it from GitHub.

**Commit**: A saved step of your work in git, with a message that says what changed. ship writes the message for you, in the [Conventional Commits](https://www.conventionalcommits.org/) format.

**Conventional Commits**: A standard format for commit messages: `type(scope): subject`, for example `feat(billing): add vat`. Tools read it to build changelogs and version numbers.

**Gitmoji**: An emoji that shows the kind of change at a glance, such as ✨ for a feature or 🐛 for a fix. ship puts it where your repository's history puts it.

**Pull request** (PR): A request to merge your branch into the base branch. Teammates review it on GitHub before it is merged.

**Draft pull request**: A pull request marked as not ready for review yet. ship opens drafts when nobody reviewed the plan in a terminal (`--yes`, an AI agent).

**Push**: Sending your commits from your computer to GitHub.

**Merge**: Adding the commits of one branch into another. On GitHub, merging a pull request puts your work into the base branch.

**Rebase**: Replaying your commits on top of the latest base branch, as if you had started from it. ship does it before publishing when the base has moved.

**Conflict**: Two changes to the same lines that git cannot combine alone. A human decides which version to keep.

**Plan**: What ship would do with your work: the title, the branch, the files. `ship plan` shows it without changing anything.

**Checkpoint**: A full snapshot of your files that ship takes before changing anything. It stays on your computer, inside `.git`.

**Undo**: `ship undo` reverts the last thing ship did (a commit, a publication, a configuration change) and keeps your files.

**Gate**: A check of your project that ship runs before pushing: lint, type check, tests. A failing gate keeps your work on your computer.

**Secret**: A password, token or key in your code. ship refuses to commit or push one.

**Label**: A tag on a pull request or issue, such as `kind/feature` or `size/S`, used to sort and filter them.

**Issue**: A task, bug or idea tracked on GitHub. A pull request can close one when merged (`Closes #42`).

**Milestone**: A group of issues with an optional due date, for example a phase of a roadmap.

**Ruleset**: Rules a repository imposes on GitHub, for example a required branch name pattern. ship reads them before pushing.

**MCP**: The Model Context Protocol: how AI coding agents (Claude Code, VS Code agents…) call tools. `ship mcp` makes ship one of those tools.

**Exit code**: The number a command returns when it ends: `0` means success. ship's codes are listed in the [command reference](reference/cli.md).
