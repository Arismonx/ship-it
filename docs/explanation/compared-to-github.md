# ship-it compared with git, GitHub and the GitHub CLI

This page compares ship-it with the tools you probably use today, task by task: what ship adds, what it costs you, and when another tool is the better choice.

ship-it 0.1 is an early release, tested in CI on Linux, macOS and Windows (see the [roadmap](../../ROADMAP.md)). Features marked *planned* do not exist yet.

The four ways of working compared here:

- **git + web**: the `git` command line, plus the GitHub website for pull requests, issues and settings.
- **gh**: the [GitHub CLI](https://cli.github.com/manual/), which brings pull requests, issues and checks to the terminal. It works next to `git`; it does not commit for you.
- **Desktop/IDE**: [GitHub Desktop](https://docs.github.com/en/desktop) and the Source Control view of [VS Code](https://code.visualstudio.com/docs/sourcecontrol/overview), with the [GitHub Pull Requests extension](https://code.visualstudio.com/docs/sourcecontrol/github) and Copilot where it applies.
- **ship**: the `ship` command (and its [MCP server and VS Code extension](../how-to/use-with-ai-agents.md)). ship runs `git` and the GitHub API for you. It replaces neither.

New words are explained in the [glossary](../glossary.md).

## Start work on a branch

| With git + web | With gh | With Desktop/IDE | With ship |
|---|---|---|---|
| `git switch -c <name>`: you pick the name. | [`gh issue develop --checkout`](https://cli.github.com/manual/gh_issue_develop) creates a branch linked to an issue and checks it out. | Desktop: **Current Branch → New Branch**, you type a name ([docs](https://docs.github.com/en/desktop/making-changes-in-a-branch/managing-branches-in-github-desktop)). VS Code's PR extension has **Start Working on Issue**, which creates a branch ([docs](https://code.visualstudio.com/docs/sourcecontrol/github)). | You can code on `main`. When you run `ship`, it creates a short [Conventional Branch](https://conventionalbranch.org/) name such as `feat/42-add-login-page`. `ship branch` only suggests one. |

With ship, you do not have to think of a branch before you start. The name follows the commit title, and an issue number in it links the pull request later.

The branch exists only when you ship, though. If your team names branches after a ticket first, `gh issue develop` or the VS Code extension fit that habit better. ship does keep Jira or Linear keys (`ACME-123`) found in a branch name ([conventions](../reference/conventions.md#branches)).

## Write a commit message

| With git + web | With gh | With Desktop/IDE | With ship |
|---|---|---|---|
| `git commit -m "…"`: you write every word. | gh does not commit. | VS Code: the sparkle icon in the commit box generates a message with AI ([docs](https://code.visualstudio.com/docs/sourcecontrol/staging-commits)). Desktop: a Copilot button generates a summary and description ([GitHub changelog](https://github.blog/changelog/2025-06-24-github-desktop-3-5-github-copilot-commit-message-generation-now-generally-available)). | A deterministic plan proposes `✨ feat(billing): add vat`, with the reasons. You edit it on the review screen. Your `--why` becomes the body, `--tested` a `Tested:` trailer. |

ship writes a message in a format your project accepts (Conventional Commits, gitmoji where your history puts it, your commitlint rules), with no AI and no network by default. It never invents the reason for a change: that part comes from you.

A deterministic planner reads file categories, not your intent. It proposes the type and scope; you often correct the subject. Copilot's message can describe the code change better in words, but we found no sign in its documentation that it follows your project's commitlint.

## Check the work before pushing

| With git + web | With gh | With Desktop/IDE | With ship |
|---|---|---|---|
| You run lint and tests yourself, or set up git hooks. GitHub's CI runs after the push. | No local checks; [`gh pr checks`](https://cli.github.com/manual/gh_pr_checks) shows CI after the push. | No built-in local checks. | Before the push, ship runs the checks the project declares, cheapest first, for Node, Go, Rust, Python, PHP, Ruby, JVM and .NET ([gates](../reference/gates.md)). It scans the work and every unpushed commit for secrets. |

GitHub also blocks secrets, on its side, at push time. [Push protection](https://docs.github.com/en/code-security/secret-scanning/introduction/about-push-protection) "blocks pushes that contain secrets before they reach your repository". Two levels exist:

- **User push protection** is on by default, on GitHub.com only, and stops you from pushing secrets to **public** repositories.
- **Repository push protection** is off by default. An administrator turns it on, and it requires GitHub Secret Protection.

By default, anyone with write access can bypass it by giving a reason, and the bypass creates an alert.

With ship, red checks and secrets stop the work on your computer. The commit stays local and undoable, and nothing reaches GitHub. It works on private repositories without Secret Protection. The pull request lists the checks that really ran.

Local checks take time on every push, up to 15 minutes per gate. A missing tool stops the push. ship's secret rules are its own, so they are not the same as GitHub's partner patterns. Keep push protection on as well, since ship's scan does not replace it.

## Open a pull request

| With git + web | With gh | With Desktop/IDE | With ship |
|---|---|---|---|
| Push, then open the PR on the website. The repository's template appears in the body automatically ([docs](https://docs.github.com/en/communities/using-templates-to-encourage-useful-issues-and-pull-requests/creating-a-pull-request-template-for-your-repository)). Copilot can write a summary in the description field ([docs](https://docs.github.com/en/copilot/using-github-copilot/creating-a-pull-request-summary-with-github-copilot)). | [`gh pr create`](https://cli.github.com/manual/gh_pr_create) with `--draft`, `--label`, `--reviewer`, `--milestone`, `--template`, `--fill` (title and body from commits). | Desktop shows a diff preview, then opens the browser to create the PR on GitHub ([docs](https://docs.github.com/en/desktop/working-with-your-remote-repository-on-github-or-github-enterprise/creating-an-issue-or-pull-request-from-github-desktop)). VS Code's PR extension creates it in the editor ([docs](https://code.visualstudio.com/docs/sourcecontrol/github)). | `ship` pushes and opens the PR in the same run. It fills the repository's template where it only holds guidance, adds `Closes #42` from the branch, the checks that ran, and existing `kind/*` and `size/*` labels, and requests the reviewers, assignee and milestone the settings name. `--yes` and AI agents open a **draft**. From a fork, the PR opens on the parent. |

GitHub's closing keywords (`Closes`, `Fixes`, `Resolves`…) only work when the PR targets the **default branch** ([docs](https://docs.github.com/en/issues/tracking-your-work-with-issues/using-issues/linking-a-pull-request-to-an-issue)). ship targets the default branch unless `ship.pullRequest.base` names another; then it links the issue with `Refs`. From a fork, `gh pr create` asks where to push and which repository to target; ship sees the fork, pushes to it and targets the parent's default branch.

ship pushes and opens the pull request in the same run, with a body that holds only what ship knows: no placeholders, no empty sections, no tool signature. Checklists in your template stay unticked, for you to tick.

Reviewers, teams, assignee and milestone come from the settings (`ship.pullRequest.reviewers`…), the same for every pull request; for a one-off choice, use `gh pr edit --add-reviewer` or the website. Copilot's summary describes the diff; ship's body describes only what you told it, so a one-line commit gives a short body.

## Keep the branch up to date

| With git + web | With gh | With Desktop/IDE | With ship |
|---|---|---|---|
| `git fetch` then `git rebase origin/main` and `git push --force-with-lease`. The PR page has **Update branch** (merge) and **Update with rebase** ([docs](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/proposing-changes-to-your-work-with-pull-requests/keeping-your-pull-request-in-sync-with-the-base-branch)). | [`gh pr update-branch`](https://cli.github.com/manual/gh_pr_update-branch) merges the base in; `--rebase` rebases instead. | Desktop: merge, rebase or squash another branch into the current one, then push or force push ([docs](https://docs.github.com/en/desktop/working-with-your-remote-repository-on-github-or-github-enterprise/syncing-your-branch-in-github-desktop)). | Before publishing, ship takes a checkpoint, rebases on the moved base and pushes under a lease (or merges the base in, with `ship.publish.update`). It does not rebase if someone else pushed commits you do not have. On a conflict, it cancels the rebase and publishes as is, with a warning. |

ship keeps the branch up to date without being asked, with a snapshot first and a lease that never overwrites someone else's push. It does not resolve conflicts: you still do that, with git or your editor.

## Follow team conventions

| With git + web | With gh | With Desktop/IDE | With ship |
|---|---|---|---|
| commitlint in a git hook or in CI. [Rulesets](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/about-rulesets) can also "control commit metadata, such as commit messages and author email addresses", and GitHub refuses the push. | No convention checks. | No convention checks. | ship reads the project's commitlint and runs it on every message before committing. It reads the active rulesets before the push: it renames a branch it created to fit a required pattern, and checks message, e-mail and signature rules first. |

With ship, a broken rule fails on your computer with an explanation, not as a rejected push or a red CI. A team's settings live in `.github/ship.json`, with presets and an organisation policy ([configuration](../reference/configuration.md)).

ship never rewrites your history. If an older commit breaks a ruleset, it names the commit and tells you to fix it with `git commit --amend` or `git rebase -i`. The organisation policy governs ship only: anyone can still use git directly. To enforce a rule for everyone, use GitHub rulesets.

## Undo a mistake

| With git + web | With gh | With Desktop/IDE | With ship |
|---|---|---|---|
| [`git reflog`](https://git-scm.com/docs/git-reflog) records where branch tips were. `git reset` moves a branch back. [`git revert`](https://git-scm.com/docs/git-revert) records new commits that undo old ones. On the website, you close a PR and delete its branch. | [`gh pr close --delete-branch --comment`](https://cli.github.com/manual/gh_pr_close) closes a PR and deletes the local and remote branch. | Desktop: **Undo** for a commit you have not pushed yet ([docs](https://docs.github.com/en/desktop/managing-commits/undoing-a-commit-in-github-desktop)). VS Code: **Commit → Undo Last Commit**, also for unpushed commits ([docs](https://code.visualstudio.com/docs/sourcecontrol/staging-commits)). | `ship undo` reverts the last ship operation: a commit (back to uncommitted changes), a publication (PR closed with a comment, remote branch deleted if nobody else pushed, after you confirm), a tidy, a fix, a configuration edit or a roadmap sync. |

ship undo is one command for local and remote effects, with checks that refuse to undo if the repository or your files changed since. ship also takes byte-exact checkpoints you can restore ([the safety net](safety-net.md)).

It only knows what ship did, though. A `git reset --hard` you typed yourself still needs the reflog. There is no `ship redo` yet (*planned*). Undoing a pushed commit that others already pulled is still a `git revert` job.

## Know where you stand

| With git + web | With gh | With Desktop/IDE | With ship |
|---|---|---|---|
| `git status`, then the PR page for checks and reviews. | [`gh pr status`](https://cli.github.com/manual/gh_pr_status) summarises your PRs with "CI checks, reviews". [`gh pr checks --watch`](https://cli.github.com/manual/gh_pr_checks) waits for CI. | Branch and changes in the UI. VS Code's PR extension lists pull requests ([docs](https://code.visualstudio.com/docs/sourcecontrol/github)). | `ship status`: branch, ahead and behind, upstream, the open PR with its checks and reviews, what `ship undo` would do, and **one** next action (fix CI, address review, mark ready, merge). |

`ship status` prints one suggested command for the next step. It covers only the current branch: `gh pr status` is better to see all your PRs at once, and `gh pr checks --watch` waits for CI, which ship does not do.

## Clean up after merge

| With git + web | With gh | With Desktop/IDE | With ship |
|---|---|---|---|
| The repository setting deletes head branches automatically after merge, **on GitHub** ([docs](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/managing-the-automatic-deletion-of-branches)). Locally: `git switch main`, `git pull`, `git branch -d`. | [`gh pr merge --delete-branch`](https://cli.github.com/manual/gh_pr_merge) deletes the local and remote branch. | Desktop: **Branch → Delete…**, which cannot be undone ([docs](https://docs.github.com/en/desktop/making-changes-in-a-branch/managing-branches-in-github-desktop)). | `ship tidy` (also run at the start of `ship`): back on the updated base with your work in progress, merged branches deleted. `ship undo` brings them back. |

ship handles the local side of the clean-up, and you can undo it. Keep GitHub's auto-delete setting on for the remote side. ship tidies only after the merge; the merge itself happens on GitHub or with gh.

## Plan work

| With git + web | With gh | With Desktop/IDE | With ship |
|---|---|---|---|
| Milestones, issues, and [sub-issues](https://docs.github.com/en/issues/tracking-your-work-with-issues/using-issues/adding-sub-issues): up to 100 per parent, up to eight levels. | [`gh issue`](https://cli.github.com/manual/gh_issue) `create`, `list`, `edit`, `close` and more. | VS Code's PR extension manages issues ([docs](https://code.visualstudio.com/docs/sourcecontrol/github)). | `ship roadmap sync` turns a `ROADMAP.md` into milestones, issues and native sub-issues, after you approve the plan. It writes issue numbers and boxes back to the file, and `ship undo` closes what it created. |

With ship, the roadmap is a Markdown file in the repository, reviewed like code, and GitHub follows it. When `github.project.url` names a GitHub Project, the sync also adds the issues to it and sets their status.

The sync goes one way, from the file to GitHub, and it is young. It does not manage project views. For day-to-day triage, the website and gh do more.

## Split a large change into a stack

| With git + web | With gh | With Desktop/IDE | With ship |
|---|---|---|---|
| You create each branch on the one below and open one pull request per branch, each based on the one below ([stacked pull requests](https://docs.github.com/en/pull-requests/get-started/about-stacked-prs)). GitHub's Rebase stack brings the layers up to date. | The official [`gh stack`](https://github.com/github/gh-stack) extension: `gh stack add` commits on a new branch on top, `gh stack submit` opens the pull requests and the stack, `gh stack modify` reorders or folds branches, `gh stack sync` rebases and pushes. | No stack support. | `ship stack` splits your uncommitted work into layers, by folder to start with, and you reorder them or move files on the review screen. Each layer gets the conventions, checks, secret scan and rulesets of a single delivery; `ship stack sync` and `ship stack merge` follow up. |

With ship, you do not cut the branches yourself: the stack comes from the work you have, and one `ship undo` removes all of it. An agent that stacks through ship gets drafts. GitHub's own rebase now creates [signed replacement commits](https://github.blog/changelog/2026-10-06-stacked-pull-requests-generally-available), and `ship stack sync` follows it by default.

`gh stack` is the better tool when you build a stack branch by branch as you go, or reshape a long-lived one often: it can fold, insert and rename branches, and ship cannot. `ship stack` is new and has only run against a fake GitHub so far.

## Work with AI agents

| With git + web | With gh | With Desktop/IDE | With ship |
|---|---|---|---|
| Agents run git commands themselves. | Agents can call gh. GitHub also maintains the [GitHub MCP server](https://docs.github.com/en/copilot/concepts/context/mcp), remote or local, for GitHub's API. | Copilot in VS Code; the GitHub MCP server is available remotely through Copilot Chat ([docs](https://docs.github.com/en/copilot/concepts/context/mcp)). | `ship mcp` exposes 11 tools (plan, deliver, publish, undo, status, log, doctor, config). An agent's pull request is always a **draft**, and remote undo stays human. `ship mcp install` sets it up for Claude Code, VS Code, Cursor or Codex. |

Through ship, agents deliver with the same plan, checks, secret scan and conventions as you, and the pull request discloses AI help the way the repository asks.

ship's MCP server delivers work; it does not browse issues or code like the GitHub MCP server. Other MCP clients are configured by hand, from `ship mcp install --print`.

## When ship-it is not the right tool

ship-it is the wrong choice if:

- You need a mature tool: ship-it is at 0.1, with a single maintainer. Try it on a side project first.
- Your code is not on GitHub. GitLab, Bitbucket and other forges are not supported. GitHub Enterprise Server is supported in the code but has not been tested against a real server yet. Without a GitHub remote, ship only commits locally.
- You shape history by hand, with fine-grained `git rebase -i`, fixups, squashing or splitting commits by hunk: use git. ship never rewrites existing commits, and splitting work into atomic commits is *planned*.
- You build stacks branch by branch, or reshape long-lived ones by hand. GitHub's `gh stack` extension does that better; `ship stack` starts from uncommitted work, and it is new ([how-to](../how-to/stack-pull-requests.md)).
- You need releases and changelogs: `ship release` is *planned*; use your current release tool.
- Your team already has gh aliases ([`gh alias set`](https://cli.github.com/manual/gh_alias_set)), [gh extensions](https://cli.github.com/manual/gh_extension), hooks and CI that do the job. ship then adds little, and one more tool to learn.
- Your organisation forbids unapproved tools. ship has no telemetry and no server, but it is still a binary that pushes code with your token. Ask first; [GitHub access](../reference/github-access.md) lists every request it makes and the permission it needs.
- You want AI to write the whole message. ship is deterministic by default; opt-in AI providers are *planned*. Copilot in VS Code or Desktop does that today.
- You need sign-in or an offline queue. `ship login` and the offline queue during a GitHub incident are *planned*. ship finds an existing token (`GH_TOKEN`, gh, git's credential helper) but never asks for one.

## Use them together

ship runs git and the GitHub API for you, and what it produces is visible to both. Your teammates do not need ship, and nothing ship produces is special: a branch, commits and a pull request like any other.

- Reviews and merges stay on GitHub: comment, approve and merge on the website, in gh (`gh pr merge --auto`) or in your editor. `ship status` then tells you the next step, and `ship tidy` cleans up.
- gh stays the tool for issues, releases, browsing other people's PRs, watching checks and scripting the API. ship even reuses gh's token.
- Keep push protection, rulesets and the auto-delete setting. ship checks before the push; GitHub still decides what it accepts.
- When you need a fine-grained rebase or a reflog rescue, use git; ship will read the result next time.
- The ship VS Code extension (v0.1, not published yet) adds a status bar, Ship and Undo; the Source Control view and the PR extension keep working.

## Sources

ship-it: [README](../../README.md), [roadmap](../../ROADMAP.md), [CLI](../reference/cli.md), [conventions](../reference/conventions.md), [configuration](../reference/configuration.md), [gates](../reference/gates.md).

git:
- [git-reflog](https://git-scm.com/docs/git-reflog)
- [git-revert](https://git-scm.com/docs/git-revert)

GitHub CLI manual:
- [gh issue](https://cli.github.com/manual/gh_issue)
- [gh issue develop](https://cli.github.com/manual/gh_issue_develop)
- [gh pr create](https://cli.github.com/manual/gh_pr_create)
- [gh pr status](https://cli.github.com/manual/gh_pr_status)
- [gh pr checks](https://cli.github.com/manual/gh_pr_checks)
- [gh pr update-branch](https://cli.github.com/manual/gh_pr_update-branch)
- [gh pr close](https://cli.github.com/manual/gh_pr_close)
- [gh pr merge](https://cli.github.com/manual/gh_pr_merge)
- [gh alias set](https://cli.github.com/manual/gh_alias_set)
- [gh extension](https://cli.github.com/manual/gh_extension)
- [gh stack](https://github.com/github/gh-stack) (GitHub's stack extension)

GitHub Docs:
- [About push protection](https://docs.github.com/en/code-security/secret-scanning/introduction/about-push-protection)
- [About rulesets](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/about-rulesets)
- [Creating a pull request template](https://docs.github.com/en/communities/using-templates-to-encourage-useful-issues-and-pull-requests/creating-a-pull-request-template-for-your-repository)
- [Linking a pull request to an issue](https://docs.github.com/en/issues/tracking-your-work-with-issues/using-issues/linking-a-pull-request-to-an-issue)
- [Keeping your pull request in sync with the base branch](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/proposing-changes-to-your-work-with-pull-requests/keeping-your-pull-request-in-sync-with-the-base-branch)
- [Managing the automatic deletion of branches](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/managing-the-automatic-deletion-of-branches)
- [Adding sub-issues](https://docs.github.com/en/issues/tracking-your-work-with-issues/using-issues/adding-sub-issues)
- [Creating a pull request summary with GitHub Copilot](https://docs.github.com/en/copilot/using-github-copilot/creating-a-pull-request-summary-with-github-copilot)
- [The GitHub MCP server](https://docs.github.com/en/copilot/concepts/context/mcp)
- [About stacked pull requests](https://docs.github.com/en/pull-requests/get-started/about-stacked-prs) and [stacked pull requests generally available](https://github.blog/changelog/2026-10-06-stacked-pull-requests-generally-available) (GitHub changelog)

GitHub Desktop:
- [Managing branches](https://docs.github.com/en/desktop/making-changes-in-a-branch/managing-branches-in-github-desktop)
- [Undoing a commit](https://docs.github.com/en/desktop/managing-commits/undoing-a-commit-in-github-desktop)
- [Creating an issue or pull request](https://docs.github.com/en/desktop/working-with-your-remote-repository-on-github-or-github-enterprise/creating-an-issue-or-pull-request-from-github-desktop)
- [Syncing your branch](https://docs.github.com/en/desktop/working-with-your-remote-repository-on-github-or-github-enterprise/syncing-your-branch-in-github-desktop)
- [Copilot commit message generation in GitHub Desktop 3.5](https://github.blog/changelog/2025-06-24-github-desktop-3-5-github-copilot-commit-message-generation-now-generally-available) (GitHub changelog)

VS Code:
- [Source control overview](https://code.visualstudio.com/docs/sourcecontrol/overview)
- [Staging and committing](https://code.visualstudio.com/docs/sourcecontrol/staging-commits)
- [GitHub in VS Code](https://code.visualstudio.com/docs/sourcecontrol/github)
