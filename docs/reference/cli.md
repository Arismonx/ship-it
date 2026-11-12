# Commands, flags and exit codes

Every command of `ship`, with its flags and the exit codes worth knowing. `ship help` prints the short list; `ship <command> --help` prints the flags of one command. Commands that are not built yet are listed under [Planned](#planned).

Results go to stdout; progress, warnings, questions and errors go to stderr. Most commands take `--json`, which prints a machine-readable result on stdout, and the commands that change something take `--dry-run`: see [Dry runs](#dry-runs). Each command's section lists its flags, and flags can go anywhere among a command's words: `ship branch feat "add login" --issue 42` works. `--help` on any command prints its usage and changes nothing.

## Global flags

These work with every command, before or after its name (`ship -d publish` and `ship publish -d` are the same).

| Flag | Meaning |
|---|---|
| `-q`, `--quiet` | Only errors, warnings and questions on stderr. |
| `-d`, `--debug` | Also print the technical journal on stderr, debug records included (see [logs](logs.md)). |
| `--trace` | Print the timeline of the run's steps on stderr when it ends. |
| `--plain` | No full-screen interface: one line per prompt, for screen readers. `SHIP_ACCESSIBLE=1` and the setting `ui.accessible` do the same. |
| `--set <key>=<value>` | A setting for this run only, as a `SHIP_<KEY>` variable would set it (see [settings](settings.md)). Repeatable. |

No command defines `-d`, `-q` or the long flags above for itself, so they always keep this meaning. Everything after `--` is passed to the command as is.

## Exit codes

The codes are stable, so scripts and agents can rely on them.

| Code | Meaning |
|---|---|
| `0` | Success. |
| `1` | Error: a git or GitHub failure, an invalid title, a broken configuration. |
| `2` | A human must act: a review or a confirmation is needed and there is no terminal, a token is missing, or GitHub asks for a decision. Add `--yes`, run it in a terminal, or follow the remedy printed. |
| `3` | A safety check refused the work: a secret in the changes, a red [gate](gates.md), a commit the project's commitlint refuses. |
| `5` | Refused by the [organisation policy](#organisation-policy). |
| `64` | Bad usage: unknown command or flag, missing argument (`EX_USAGE` from `sysexits.h`). |
| `130` | You cancelled: Esc, Ctrl+C, or "no" to a confirmation. Nothing changed. |

Each refusal prints a `SHIP-E####` code; the [error reference](errors.md) explains every one.

## Deliver

### `ship`

Plans the delivery of your work in progress, shows it on the [review screen](#the-review-screen), then commits it on a branch, runs the gates, pushes and opens the pull request.

```text
ship [--yes] [--local] [--only <path>]… [--why "…"] [--tested "…"] [--issue N] [--agent <name>] [--skip-gates] [--dry-run] [--json]
```

| Flag | Meaning |
|---|---|
| `-y`, `--yes` | Accept the plan without the review. The pull request then opens as a draft, so the review happens on GitHub ([ADR-0010](../adr/0010-surfaces.md)). |
| `--local` | Commit only: no push, no pull request. |
| `--only <path>` | Deliver only this file or directory; the rest stays uncommitted and untouched. Repeatable. See [deliver only part of your work](../how-to/deliver-part-of-your-work.md). |
| `--why "…"` | Why the change was made: the commit body and the summary of the pull request. |
| `--tested "…"` | How you tested it, written as a `Tested:` trailer ([conventions](conventions.md#pull-request-body)). |
| `--issue N` | The linked issue: its number goes in the branch name and the pull request closes it. |
| `--agent <name>` | Prefix the branch for an AI agent: `ai`, `claude`, `codex`, `copilot` or `cursor`. |
| `--skip-gates` | Push without running the gates. The pull request says so. |
| `--dry-run` | Show every step without changing anything. |

On the default branch, ship creates a branch first; it never commits there, except the very first commit of an empty repository. Before planning, it leaves a branch whose pull request was merged (see [`ship tidy`](#ship-tidy)). Without a GitHub remote, the work is committed locally. When a GitHub step fails, the commit stays and ship tells you to run `ship publish` later.

Exit codes: `2` when there is no terminal and no `--yes`, or no token; `3` when a secret is found (nothing is committed, and each finding prints its fingerprint for `ship.secrets.allow`, see [secrets](configuration.md#secrets)) or a gate is red (the commit stays local); `130` when you cancel.

On the top layer of a stack, `ship` adds the work as a new layer: a branch based on the top one, with its pull request added to the stack.

### `ship plan`

Shows the title, branch, files and the reasons of the plan `ship` would follow. Changes nothing.

```text
ship plan [--issue N] [--agent <name>] [--only <path>]… [--json]
```

The flags mean what they mean for `ship`. Exits `3` when the changes hold a secret, so the plan also tells whether the delivery would get through.

### `ship publish`

Pushes the current branch and opens its pull request, or updates the one already open.

```text
ship publish [--draft] [--issue N] [--skip-gates] [--dry-run] [--json]
```

| Flag | Meaning |
|---|---|
| `--draft` | Open the pull request as a draft. |
| `--issue N` | The linked issue. By default it is read from the branch name: `feat/42-add-login` closes `#42`. |
| `--skip-gates` | Push without running the gates. The pull request says so. |

Before the push, ship rebases on the base when it moved, checks the branch and commits against the repository's rulesets, and runs the gates. The pull request gets the `kind/`, `area/` (from the commit scope) and `size/` labels the repository has. The URL of the pull request is the last line of stdout.

### `ship stack`

Splits the work into layers and ships them as a stack of pull requests, each based on the one below. See [ship your work as a stack](../how-to/stack-pull-requests.md).

```text
ship stack [--yes] [--layer <paths>]… [--issue N] [--agent <name>] [--local] [--draft] [--skip-gates] [--dry-run] [--json]
```

| Flag | Meaning |
|---|---|
| `-y`, `--yes` | Accept the layers without the review. |
| `--layer <paths>` | One layer, bottom first: files or directories separated by commas. Repeatable. Files no layer names stay uncommitted. Without it, ship splits by area, up to `ship.stack.maxLayers`. |
| `--issue N` | The linked issue: closed by the bottom layer, referred to by the others. |
| `--local` | Commit the layers only: no push, no pull request. `ship stack publish` publishes them later. |
| `--draft` | Open the pull requests as drafts. |
| `--skip-gates` | Push without running the gates. |

Run it from the default branch. The layers are reviewed on a screen like the one of `ship` (see [the stack review screen](#the-stack-review-screen)). Exits `2` when there is no terminal and no `--yes`.

On github.com the pull requests form a native stack. On a server without the stacks API (GitHub Enterprise Server), they form a chain, and each body ends with a line such as "Part 2 of 3, after #41.", kept up to date.

### `ship stack status`

Lists the layers from the top down, with the state of each pull request (open, draft, merged, closed) and its checks. `▸` marks the current branch.

```text
ship stack status [--offline] [--json]
```

`--offline` does not ask GitHub.

### `ship stack sync`

After a lower layer merged: fetches, removes the merged layers (local and remote branches), rebases the others on the layer below that stays, and retargets the pull requests of a chain.

```text
ship stack sync [--yes] [--dry-run] [--json]
```

The working tree must be clean (`SHIP-E1014`). With `ship.stack.rebase` set to `github` (the default), ship follows the rebase GitHub does, which creates signed replacement commits, and leaves alone a layer with commits never pushed (`SHIP-W1010`). With `local`, the layers are rebased here and pushed with a lease, your own signatures kept. `ship undo` puts the branches back; on GitHub it does so only where nobody pushed since (`SHIP-W1015`).

### `ship stack merge`

Asks GitHub to merge the stack, then waits for the result.

```text
ship stack merge [-y|--yes] [--dry-run] [--json]
```

ship shows what merges and asks for confirmation; `--yes` merges without asking. It merges the top layer, which merges the whole stack (the bottom layer for a chain). GitHub's rules always apply: ship never bypasses them, and a merge queue stays in charge (`SHIP-W1009`). Exits `2` without a terminal and without `--yes` (`SHIP-E1006`).

### `ship stack publish`

Publishes a stack made with `ship stack --local`. Run it from the top layer.

```text
ship stack publish [-y|--yes] [--issue N] [--draft] [--skip-gates] [--dry-run] [--json]
```

The flags mean what they mean for `ship stack`.

### `ship check gates`

Runs the project's checks now, the same [gates](gates.md) a push runs, without committing or pushing anything.

```text
ship check gates [--json]
```

Use it to see whether your work would get through before you ship it. When `ship.gates.beforeCommit` is set, `ship` also runs them before each commit.

## Undo and history

### `ship undo`

Undoes the last operation of ship. Run it again to undo the one before.

```text
ship undo [-y|--yes] [--dry-run] [--json]
```

`--yes` agrees in advance to the changes on GitHub; without it, ship asks in a terminal and exits `2` elsewhere.

| Last operation | What `ship undo` does |
|---|---|
| A delivery | The commit becomes uncommitted changes again, the branch ship created is deleted, and you are back where you were. Your files are not touched. |
| A publication | The pull request is closed with a comment, and the remote branch is deleted if nobody else pushed to it. A linked issue moved in the GitHub Project gets its status back, and loses the assignee ship added. |
| A tidy | The deleted branches are created again. |
| A stack, a stack sync | The layers and branches are put back; on GitHub, the pull requests are closed or retargeted. |
| A roadmap sync | The issues ship created are closed as not planned, empty milestones it created are deleted, the project fields and states are put back. |
| A `ship fix`, a `ship config set` | The files and settings are put back. |

ship saves a checkpoint before undoing. It refuses when the repository moved since (`SHIP-E0404`) or files changed (`SHIP-E0405`). A status someone changed on GitHub since is kept (`SHIP-W0411`). See [the safety net](../explanation/safety-net.md).

### `ship log`

Lists what ship did, newest first. `↶` marks what `ship undo` reverts next; undone operations are flagged, and so is what an AI agent did through MCP, with its name.

```text
ship log [-n N] [--agent [<name>]] [--json]
```

`--agent` keeps only what agents did; `--agent claude` (or `--agent=claude`) what one agent did. The name is the one `ship mcp install` gave the server with `--client`, or the one the agent gives itself when it connects.

`-n N` sets how many operations to show (20 by default).

### `ship checkpoint`

Saves an exact snapshot of the working tree, lists the snapshots, or restores one.

```text
ship checkpoint [--label <text>] [--dry-run] [--json]
ship checkpoint list [--json]
ship checkpoint restore <id> [--dry-run] [--json]
```

`--label` names the snapshot. `list` shows the 20 most recent. `restore` saves a safety checkpoint first, so a restore can be undone with another restore.

### `ship gc`

Removes old checkpoints and run logs.

```text
ship gc [--dry-run] [--json]
```

It keeps the 20 newest checkpoints, the newest of each day for 30 days, and every checkpoint an undo still needs. Run logs are kept within their limits: the last 20 runs and 10 MB (see [logs](logs.md)).

### `ship trace`

Shows the timeline of a past run from its journal: steps, durations, git commands and GitHub requests, and its black box if it failed.

```text
ship trace [<run>] [--list] [--json]
```

Without an argument, the last run. `--list` lists the runs kept. See [logs](logs.md).

## Status and repairs

### `ship preview`

Shows the pull request `ship publish` would open, as reviewers will read it: the title, the body as the repository's template renders it, the labels, the base, and what the publication would warn about. It rehearses the publication itself, so the preview is what you would get; nothing is pushed or opened, and GitHub is only read. When the branch already has an open pull request, it says so: publishing would push to it and keep its title and body.

```text
ship preview [--draft] [--issue N] [--json]
```

It previews committed work: run `ship --local` first, or `ship --dry-run` for uncommitted changes.

It ends with a review risk, low, medium or high, and the facts behind each point, so a reviewer knows how carefully to read:

| Fact | Points |
|---|---|
| 100 changed lines or more (500 or more) | 1 (2) |
| Source files changed without any test file | 2 |
| A sensitive path: continuous integration, dependencies, or `ship.pullRequest.sensitivePaths` (migrations, auth, security, SQL, Dockerfile by default) | 2 |
| Files or declarations removed | 1 |

Generated files and lockfiles do not count as changed lines, as for the `size/` labels. Medium starts at 2 points, high at 4. The score is not added to the pull request.

### `ship status`

Shows where you stand and the one next thing to do: branch, ahead and behind the default branch, files not shipped, the remote branch, the open pull request with its checks and reviews, the open pull requests that change the same lines as your branch (`ship.publish.overlaps`), and what `ship undo` would revert.

```text
ship status [--offline] [--json]
```

It asks GitHub for at most 5 seconds. `--offline` does not ask GitHub at all.

### `ship doctor`

Checks git, your identity, the repository, then GitHub with a single read: token found (and where), refused, repository not visible, or GitHub unreachable. Each problem comes with the command that fixes it.

```text
ship doctor [--json]
```

GitHub problems are warnings, since local work goes on. Exits `1` on a blocking problem. Which token ship uses: [GitHub access](github-access.md).

### `ship fix`

Lists the remedies for what `ship doctor` and the [error playbook](error-playbook.md) detect, then applies them after one confirmation, with a checkpoint.

```text
ship fix [--yes] [--dry-run] [--json]
```

It handles your identity (a GitHub noreply address), long paths on Windows, a mirror remote, tracked `node_modules` or `.env` files, and merged branches. `ship undo` reverts the remedies. `--yes` applies them without asking; without a terminal and without `--yes`, it lists them and exits `2`.

### `ship tidy`

After your pull request merged: goes back to the up-to-date base with your work in progress, and deletes the merged branches.

```text
ship tidy [--dry-run] [--json]
```

The work in progress moves as a whole or not at all. `ship` runs it first, except with `--local`; offline, it only warns. `ship undo` creates the branches again.

## Settings and GitHub

### `ship config`

Shows every effective setting and the layer it comes from, plus the files read. A key the organisation policy imposes is marked.

```text
ship config [--json]
ship config get <key> [--json]
ship config set <key> <value> [--user | --repo] [--dry-run] [--json]
ship config unset <key> [--user | --repo] [--dry-run] [--json]
ship config explain <key> [--json]
```

| Subcommand | What it does |
|---|---|
| `get <key>` | Prints the effective value on stdout (a string without quotes, anything else as JSON), and on stderr the layer and file it comes from. |
| `set <key> <value>` | Writes the key into one file. The value is read by the key's type, as `--set` reads it: `true` or `false` (also `1`/`0`, `yes`/`no`, `on`/`off`), an integer, a string, or a list (a JSON array or items separated by commas). Put `--` before a value that starts with `-`. |
| `unset <key>` | Removes the key from one file, with the objects it leaves empty. |
| `explain <key>` | What the key does, its type, allowed values and default, the value each layer gives it, and which layer wins. |

`--repo` edits `.github/ship.json` and is the default inside a repository; `--user` edits your user file and is the only choice outside one.

The file is edited as text: comments, key order, indentation and line endings stay as they are. The edit is validated before anything is written, so an unknown key (with a suggestion), a wrong type or a value out of range leaves the file untouched; a key the policy imposes is refused with exit code `5`. If the file changed since ship read it, nothing is written (`SHIP-E0410`). Inside a repository, `ship undo` reverts the edit.

```console
$ ship config get ship.pullRequest.maxLines
Built-in default.
400
```

### `ship init`

Creates `.github/ship.json` with the `area/*` labels found in the commit scopes of the history. It never overwrites an existing file and does not touch GitHub.

```text
ship init [--dry-run] [--json]
```

### `ship sync`

Brings the repository's labels on GitHub in line with `.github/ship.json`: shows the labels to create, rename or update, then applies them once you agree. It never deletes a label.

```text
ship sync [--yes] [--dry-run] [--json]
```

`--yes` applies without asking; without a terminal and without `--yes`, it exits `2`. When `github.managedBy` names another tool, it only shows the plan.

### `ship roadmap sync`

Brings milestones, issues and the GitHub Project in line with `ROADMAP.md`: shows the plan, then applies it once you agree. See [plan your project with ROADMAP.md](../how-to/plan-with-a-roadmap.md).

```text
ship roadmap sync [--file <path>] [--yes] [--dry-run] [--json]
```

`--file` names the roadmap file, relative to the repository root (`ROADMAP.md` by default). `--yes` applies without asking; without a terminal and without `--yes`, it exits `2`. `ship undo` reverts what the sync did.

### `ship label`

Puts on pull request N the `kind/`, `area/` and `size/` labels ship would have set, computed from its commits and files as GitHub lists them (up to 250 commits and 3,000 files). It is meant for pull requests not opened with ship, from forks included, for example in a workflow.

```text
ship label --pr N [--dry-run] [--json]
```

Only labels the repository has are used. Once a `size/` label is there, only the size is refreshed.

## Conventions

### `ship check title`

Checks a commit or pull request title against the repository's conventions ([conventions](conventions.md)). Exits `1` when it is invalid.

```text
ship check title [--json] <title>
```

```console
$ ship check title "add vat rate"
SHIP-E0102  The title must look like "✨ feat(scope): subject".
```

### `ship check branch`

Checks a branch name: [Conventional Branch](https://conventional-branch.github.io/) 1.1.0 and `git check-ref-format`. Exits `1` when it is invalid.

```text
ship check branch [--json] <name>
```

### `ship check signoff`

Checks that every commit of a range carries a [Developer Certificate of Origin](https://developercertificate.org/) sign-off, the `Signed-off-by: Name <email>` trailer that `git commit -s` writes.

```text
ship check signoff [--from <base>] [--to <rev>] [--json]
```

| Flag | Meaning |
|---|---|
| `--from <base>` | First excluded revision. By default the default branch of `origin`, or of the `upstream` remote in a fork, so the range is what a pull request brings. Without a known default branch, ship asks for it (`SHIP-E0116`). |
| `--to <rev>` | Last revision, `HEAD` by default. |

Only the trailer block counts, read the way git reads it: a `Signed-off-by` line in the middle of the body does not. As with the [DCO GitHub App](https://github.com/dcoapp/app), a sign-off must carry the name and the email of the author or the committer, compared without case (`SHIP-E0115`; `SHIP-E0114` when there is none). Merge commits and GitHub App bots are exempt. On failure it exits `1`, lists the commits and prints the fix: `git rebase --signoff <base>`, then a push with `--force-with-lease`.

### `ship branch`

Suggests a short, valid branch name.

```text
ship branch [--issue N] [--agent <name>] [--hotfix] [--json] <type> <subject>
```

`--hotfix` marks an urgent production fix.

```console
$ ship branch --issue 42 feat add login page
feat/42-add-login-page
```

### `ship types`

Lists the commit types with their gitmoji, branch prefix and description.

```text
ship types [--json]
```

## Agents

### `ship mcp`

Starts the MCP server on stdin and stdout, for AI agents. It offers 11 tools; see [use ship-it with AI coding agents](../how-to/use-with-ai-agents.md). `--client <name>` names the agent, which every operation made through the server records (`ship log --agent`); `ship mcp install` writes it in the agent's project file. Without it, the server uses the name the agent gives in its handshake.

```text
ship mcp [--attended]
```

`--attended` is for an interface where a human reviews each plan, such as the VS Code extension: pull requests then follow your settings instead of always opening as drafts.

### `ship mcp install`

Declares ship-it in the project's MCP file of an agent, after showing the new file and asking.

```text
ship mcp install [--client <name>] [--print] [--yes] [--dry-run] [--json]
```

| Flag | Meaning |
|---|---|
| `--client <name>` | `claude` (the default) writes `.mcp.json`, `vscode` writes `.vscode/mcp.json`, `cursor` writes `.cursor/mcp.json`, `codex` appends to `.codex/config.toml`. |
| `--print` | Print the entry and write nothing. |
| `--yes` | Write without asking. |

Other servers in the file are kept. The command is `ship` from the `PATH`, never an absolute path, since the file is committed. When the file cannot be changed safely, ship prints the entry to add by hand and exits `2` (`SHIP-E0715`). `ship undo` reverts the change.

## Other commands

### `ship version`

Prints the version, the commit and the build date; `--json` prints them with the Go version. `ship --version` does the same. A binary built with `go install` reads them from Go's build information; as it records no commit and no date, such a binary prints the version alone (`ship v0.1.0`).

### `ship help`

Prints the list of commands and global flags. `ship --help` and `ship -h` do the same.

## Interface language

ship picks its language from `SHIP_LANG`, then the setting `ui.language` (`en` or `fr`; `auto`, the default, skips it), then `LC_ALL`, `LC_MESSAGES` and `LANG`. English is the default.

## Dry runs

The commands that change something take `--dry-run` (each command's section says so). A dry run makes every read and every check the real run would make (`ship doctor`, title and branch rules, the project's commitlint, the secret scan, the rulesets, the open pull request, the labels of the repository), then prints what would change as a list of steps. It changes nothing: no git or GitHub write, no checkpoint, no entry in the log, no file written. It asks nothing either.

| Command | What a dry run does not do |
|---|---|
| `ship` | The review screen: the plan is taken as proposed, as with `--yes`. |
| `ship publish` | Fetch (it works with the remote branches of the last fetch) and run the gates (they are listed with their command). |
| `ship undo` | Ask for consent: the plan says the real run will ask. |
| `ship tidy` | Check that the work in progress applies on the base without conflict, which would need a stash. |
| `ship fix`, `ship sync`, `ship roadmap sync`, `ship mcp install` | Ask for confirmation. |

The exit code is the one of the real run: a secret gives `3`, a policy refusal `5`, an invalid title or a broken configuration `1`. So `ship --dry-run` in a CI job tells whether the real run would get through.

With `--json`, a dry run prints the result the real run would return, plus `"dry_run": true` and `"planned"`: the ordered steps, each a message key with its parameters, and `"remote": true` when it reaches GitHub. Values that only exist after a write (commit id, pull request number and URL) stay empty. The MCP tools `ship_deliver`, `ship_publish` and `ship_undo` take `"dryRun": true` for the same result.

`--dry-run` is not a global flag: on a command that only reads, it would promise something with nothing behind it.

## The review screen

When stdin and stderr are both terminals, `ship` shows the plan as a form you can edit before anything changes. It is drawn on stderr, inline, for 80 columns; `--quiet` does not hide it.

```text
⛵ ship · main → feat/add-vat-rate

  Title    ✨ feat(cart): add vat rate
           ✓ valid · 26/72
  Branch   feat/add-vat-rate
           ✓ valid
  Why?     optional
  Tested?  optional

› Files    ▾ 3/3 selected
           [x] internal/cart/vat.go
           [x] internal/cart/vat_test.go
           [x] docs/vat.md

Enter ship · Ctrl+L commit only · Tab field · Space toggle · ? help · Esc cancel
```

- Title: checked as you type, with the rules of `ship check title`.
- Branch: editable when the delivery creates one. Until you edit it, it follows the title.
- Why? and Tested?: optional; they fill the commit body and the `Tested:` trailer, like `--why` and `--tested`.
- Files: the boxes are the `--only` selection. Changing it plans again and keeps what you edited. Nothing selected blocks shipping.
- Type: shown only when the project's commitlint refuses the planned type and several allowed types fit. Pick one; shipping stays blocked until the title has an allowed type. With `--yes` or through an agent, nobody picks, and the delivery refuses with `SHIP-E0906`, exit code `3`.
- Warnings: secrets are listed `path:line` with the value masked, and block shipping until you unselect their files. Above `ship.pullRequest.maxLines`, the parts that can ship on their own are listed.

The screen opens on the files, so Enter alone ships the plan as proposed.

| Key | Effect |
|---|---|
| Enter | Ship: commit, then publish as `ship.publish.mode` says. In a text field, go to the next field. |
| Ctrl+Enter, Ctrl+J, F2 | Ship from anywhere, a text field included. |
| Ctrl+L | Commit only, as `--local`. |
| Esc, Ctrl+C | Cancel: nothing changes, exit code `130`. |
| Tab, Shift+Tab | Next or previous field. |
| ↑ ↓ (or k j) | Move in the files and in the parts. |
| Space | Select or unselect a file; on the Files line, fold or unfold; on a part, ship only that part. |
| ← → (or h l) | Fold or unfold the files; on the type line, pick a type. |
| `?` | Help, outside a text field. Any key closes it. |
| Ctrl+A, Ctrl+E, Ctrl+U, Ctrl+K, Ctrl+W | In a text field: start, end, delete to the start, delete to the end, delete a word. |

Some terminals send Ctrl+Enter as Enter: Ctrl+J and F2 always work.

Nothing is said by colour alone: `✓`, `✗`, `⚠`, `›` (focus), `[x]` and `(•)` are always written. Colours follow the terminal and disappear with `NO_COLOR`. The setting `ui.color` set to `never` removes them, `always` keeps them on a terminal not detected as colour-capable.

### Without the screen

| Situation | Review |
|---|---|
| `--yes` | None. The pull request opens as a draft. |
| stdin is not a terminal | None: exit code `2`. |
| `--plain`, `SHIP_ACCESSIBLE=1`, `ui.accessible`, or `TERM=dumb` | One prompt per line on stderr. |
| stderr is not a terminal (`ship 2>log`), or `ui.reviewScreen` is `false` | The plan on stdout and one question, `Ship it? [Y/n]`. |
| The screen cannot start | One prompt per line. |

The line-by-line prompts look like this; Enter keeps the value in brackets, an invalid title or branch is explained and asked again:

```text
Title [🐛 fix: update run]:
Branch [fix/update-run]:
Why? (optional, Enter to skip):
Tested? (optional, Enter to skip):
Ship it? [Y/n]
```

### The stack review screen

`ship stack` shows its layers bottom first, each with its title, branch and files, and checks titles and branches as you type.

| Key | Effect |
|---|---|
| Enter, Ctrl+Enter, Ctrl+J, F2 | Ship the layers. In a text field, Enter goes to the next line. |
| Ctrl+L | Commit the layers only. |
| Esc, Ctrl+C | Cancel: nothing changes. |
| ↑ ↓ (or k j) | Move in the list. |
| Tab, Shift+Tab | Next or previous layer. |
| Shift+↑, Shift+↓ (or K, J) | Move the layer up or down the list; on a file, move it to the layer above or below. |
| `1` to `9` | On a file, move it to that layer. |
| `?` | Help. |

## Configuration

Settings come from several layers, weakest first: built-in defaults, presets, your user file, the repository's `.github/ship.json`, `SHIP_*` environment variables, `--set`, and the organisation policy. How they combine is in [the configuration guide](configuration.md); every key is in [the settings reference](settings.md). Read and edit them with [`ship config`](#ship-config).

## Organisation policy

An organisation can impose settings that developers cannot change locally. The policy is a file an administrator deploys on each machine:

| System | File |
|---|---|
| Windows | `%ProgramData%\ship-it\policy.json` |
| macOS | `/Library/Application Support/ship-it/policy.json` |
| Linux | `/etc/ship-it/policy.json` |

No environment variable can move this file. Its format is [`schemas/policy.schema.json`](../../schemas/policy.schema.json):

```json
{
  "$schema": "https://raw.githubusercontent.com/GregoireF/ship-it/main/schemas/policy.schema.json",
  "version": 1,
  "organization": "Acme",
  "contact": "it@acme.example",
  "enforce": { "ship": { "pullRequest": { "draft": "always" } } }
}
```

`enforce` has the shape of `.github/ship.json`. An imposed key wins over every layer. When a user or repository file gives it another value, ship refuses with `SHIP-E0801` and exit code `5`, naming the key, both values, the file, the organisation and the contact. `ship config` marks the imposed keys and `ship doctor` shows the active policy.

The policy governs ship, not git: a developer can still run git directly. To enforce rules on GitHub itself, use [rulesets](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/about-rulesets).

On Windows, a standard user can create a folder under `%ProgramData%`. Until an administrator deploys the file, a developer could write one there, but it would only constrain that developer.

## Planned

These commands and features do not exist yet; see the [roadmap](../../ROADMAP.md).

- Automatic atomic commits: split the work into several commits by intent (0.2).
- `ship watch` and `ship rewind`: checkpoints taken as files change, and a way back to any of them.
- `ship explain <code>`, `ship bug`, `ship skill install`, `ship uninstall` and `ship log --agent` (see the [roadmap](../../ROADMAP.md) for 0.2).
- `ship release`: compute the version, write the changelog, create the tag and the GitHub release.
- `ship login` and `ship logout`: sign in with the GitHub device flow and keep the token in the system keychain.
- `ship rewind`: go back to any moment of your files, from checkpoints taken while you work (0.2; today, `ship checkpoint restore <id>`).
- `ship daemon`: run deliveries automatically in the background.
- An organisation policy read from a private repository, `<org>/ship-policy`.
