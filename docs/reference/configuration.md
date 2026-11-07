# Configuration

ship-it works without any configuration file. When a team or a user wants something else, every setting can be changed, at the level where it belongs: a whole organisation, a client, a repository, a part of a repository, or a single run. Every key is listed, with its type, default and environment variable, in the generated [settings reference](settings.md).

Decisions: [ADR-0009](../adr/0009-layered-json-config-policy.md) (layers, JSON, policy), user choices of 2026-10-23 and 2026-10-24 (JSONC, overrides, contexts, presets, `SHIP_*`, `--set`).

## Files

| File | Who writes it | Holds |
|---|---|---|
| `.github/ship.json` | the repository's team, committed | `extends`, `ship`, `github`, `overrides` |
| User file: `%APPDATA%\ship-it\config.json`, `~/Library/Application Support/ship-it/config.json`, `$XDG_CONFIG_HOME/ship-it/config.json` (default `~/.config/ship-it/config.json`) | the developer | `extends`, `ship`, `github`, `ui`, `contexts` |
| Organisation policy: `%ProgramData%\ship-it\policy.json`, `/Library/Application Support/ship-it/policy.json`, `/etc/ship-it/policy.json` | the IT department | `organization`, `contact`, `enforce` ([cli](cli.md#organisation-policy)) |

Files are JSONC: `//` and `/* */` comments and trailing commas are accepted, as in `tsconfig.json`. Errors still give the exact line and column. Editors complete and check the keys with the JSON Schemas: [`ship.schema.json`](../../schemas/ship.schema.json) for `.github/ship.json` and presets, [`user.schema.json`](../../schemas/user.schema.json) for the user file.

```jsonc
{
  "$schema": "https://raw.githubusercontent.com/GregoireF/ship-it/main/schemas/ship.schema.json",
  // Reviews are slow on this project: keep pull requests small.
  "ship": { "pullRequest": { "maxLines": 250 } },
}
```

## Precedence

Weakest first; a later layer wins key by key:

1. built-in defaults;
2. presets named in `extends`, each right before the file that extends it;
3. the user file, then its contexts that match the repository;
4. `.github/ship.json`, then its overrides that match the delivery;
5. `SHIP_*` environment variables;
6. `--set key=value` options;
7. the organisation policy, which no other layer may contradict (exit code 5).

`ship config` shows every effective value and the layer that set it ("repository overrides[0]", "SHIP_PULLREQUEST_MAXLINES", "--set ship.gates.enabled"…). A malformed file never has a partial effect: the configuration is read and validated before anything changes.

## Presets: `extends`

```jsonc
{
  "extends": [
    "ship-it:strict",
    "ship/base.json",
    "github:my-agency/ship-config/client.json@3f2a9c1e0b7d4a6f8c2e1d0b9a8f7e6d5c4b3a21"
  ]
}
```

- Built-in presets: `ship-it:recommended` (the defaults, spelled out), `ship-it:strict` (pull requests up to 250 lines, drafts when nobody reviewed the plan, labels, mandatory checks: `--skip-gates` is refused) and `ship-it:agency` (a client's repository: deliver locally, publish after review; `Refs #n` so that the client's issues stay open; the client's labels left alone).
- A file relative to the file's root: the repository root for `.github/ship.json`, the user configuration directory for the user file. A preset never reads outside its root, links included.
- A file of another GitHub repository, pinned to a full 40-digit commit SHA: it can never change under the developer's feet. It is fetched once with the token ship-it already uses, then cached in the user cache directory (`%LocalAppData%\ship-it\cache`, `~/Library/Caches/ship-it`, `~/.cache/ship-it`). A file named in its own `extends` is read from the same repository at the same commit.
- Presets may extend presets, up to 4 levels; a loop is an error (`SHIP-E0709`), and so is a preset that cannot be read (`SHIP-E0708`).

## Per-path overrides

Settings for a part of the repository, as ESLint and Biome do:

```jsonc
{
  "overrides": [
    { "paths": ["docs/**", "*.md"], "ship": { "gates": { "enabled": false } } },
    { "paths": ["apps/web/**"], "ship": { "pullRequest": { "maxLines": 600 } } }
  ]
}
```

- An override applies to a delivery when every delivered file matches one of its patterns: a documentation-only change skips the checks, a change that also touches code does not.
- Patterns are relative to the repository root: `*` and `?` within a segment, `**` for any number of segments; a pattern without a slash matches a file name at any depth (`*.md`), as in `.gitignore`.
- Overrides apply in order, right after the file's own settings: a later match wins, and a later layer (environment, `--set`) still beats them.
- Only `ship` settings can be overridden: `github` settings describe the whole repository.

## Contexts (user file)

Settings for some repositories only (an agency's clients, a GitHub Enterprise server) without committing anything to them:

```jsonc
{
  "contexts": [
    { "match": { "owner": "acme-corp" }, "ship": { "issues": { "keyword": "Refs" } } },
    { "match": { "host": "github.example.gouv.fr" }, "ship": { "publish": { "mode": "never" } } },
    { "match": { "path": "~/clients/bank/**" }, "ship": { "pullRequest": { "maxLines": 200 } } }
  ]
}
```

Contexts hold `ship`, `github` and `ui` settings. `owner` and `host` are those of the remote, `origin` unless `ship.git.remote` names another (case ignored); `path` is a pattern of the repository's root directory, `~/` being the home directory. Every criterion given must hold; contexts apply in order.

## Environment and `--set`

Every key has an environment variable, `SHIP_` followed by the key in upper case without `ship.` (`SHIP_PULLREQUEST_MAXLINES`, `SHIP_GITHUB_LABELS_AREAS`), and can be set for one run with `--set`:

```sh
SHIP_GATES_ENABLED=false ship publish
ship --set ship.pullRequest.maxLines=600 plan
```

Values are read according to the key's type: `true`/`false` (also `1`/`0`, `yes`/`no`, `on`/`off`), integers, strings, and lists as a JSON array or comma-separated items. They are validated exactly as in files. `SHIP_*` variables that are not settings (`SHIP_TOKEN`…) are left alone; a variable that looks like a mistyped setting is reported by `ship doctor` (planned).

## The project's habits

ship-it follows what a repository already does before its own format. `ship.commits.gitmoji` is `auto` by default: the last 50 commits decide where the gitmoji goes (before the type, after the colon, or not at all); with fewer than 5 conventional commits, ship-it writes its own `✨ feat: subject`. Set `first`, `after-type` or `none` to decide explicitly.

What the project imposes beats these settings, with no setting to turn it off (rule zero, [conventions](conventions.md)):

- The project's commitlint. Installed locally (a commitlint configuration file or `package.json` field, and `node_modules/.bin/commitlint`), it shapes the title (allowed types and scopes, lengths, case, final period) and validates the exact message before every commit. When it refuses a leading gitmoji, the title goes without one even if `ship.commits.gitmoji` says `first`, and the plan says why. Nothing is downloaded: `npx --no`.
- GitHub rulesets. `ship publish` reads the branch's rules first: a branch ship-it created and never pushed is renamed to fit a branch name pattern (undoable), commits that break a message or e-mail pattern or lack a required signature are refused before the push, with the exact remedy.

Tickets of an external tracker are carried the same way: with `ship.issues.trackerKeys` (`["ACME"]`), a ticket in the branch name ends the titles, `(ACME-123)`; see [conventions](conventions.md#branches). A client context is the natural place for it:

```jsonc
{ "contexts": [ { "match": { "owner": "acme-corp" }, "ship": { "issues": { "trackerKeys": ["ACME"], "keyword": "Refs" } } } ] }
```

## Commit and branch conventions

The limits, commit types and scopes of [conventions](conventions.md) are settings. Their defaults are ship-it's own convention, so a repository without them works as before.

| Key | Default | Role |
|---|---|---|
| `ship.commits.subjectMaxLength` | `50` | longest subject (at least 20); a tracker ticket at the end does not count |
| `ship.commits.headerMaxLength` | `72` | longest title, gitmoji included (at least 30); the planner drops the scope of a longer one |
| `ship.commits.bodyWidth` | `72` | column at which the why is wrapped (at least 40) |
| `ship.commits.types` | `{}` (built-in table) | changes to the commit types, by type: `{"emoji", "branchPrefix", "label", "release"}` or `false` |
| `ship.commits.scopes` | `[]` (any) | scopes a title may use |
| `ship.commits.scopeRequired` | `false` | refuse a title without a scope (`SHIP-E0118`) |
| `ship.commits.scopeFrom` | `"folders"` | where the planner takes the scope from: the folders the changed files share, or `"none"` |
| `ship.branches.maxWords` | `4` | words kept from the subject in a branch name (at least 1) |
| `ship.branches.maxLength` | `32` | longest description part of a branch name (at least 8) |
| `ship.branches.issue` | `"first"` | put the issue number or ticket first (`feat/42-add-login`), or `"none"` |

```jsonc
// .github/ship.json
{
  "ship": {
    "commits": {
      "types": {
        "deps": { "emoji": "⬆️", "branchPrefix": "deps", "label": "dependencies" }, // a new type
        "feat": { "label": "enhancement" },                                          // kind/enhancement
        "wip": false                                                                  // no wip commits here
      },
      "scopes": ["api", "web", "infra"],
      "scopeRequired": true
    },
    "branches": { "issue": "none" }
  }
}
```

**Commit types.** Each layer changes the types it names and keeps the others: the user file may give `feat` another emoji while the repository gives it a label. In an object, `emoji` is the gitmoji of the type (not empty), `branchPrefix` the Conventional Branch prefix of its branches (lowercase letters and digits separated by single `-` or `.`), `label` its pull requests' kind, written after `kind/` (`""` for none), `release` the version increment its commits trigger: `major`, `minor`, `patch` or `none` (built-in: `feat` minor; `fix`, `perf`, `security` and `revert` patch; the others none; a breaking change is always major, whatever its type). A type the built-in table lacks is added after it; it needs an `emoji` and a `branchPrefix` (two layers may give one each), changes no version unless its `release` says so, and cannot be breaking. `false` removes a built-in type; a later layer can bring it back with an object. At least one type must be left. The effective table is the one every command uses: `ship types`, `ship check title` and `ship check branch` (a type's prefix is an accepted prefix), the planner (a removed type becomes its closest kept type, `perf` → `refactor` → `fix`, else the first type of the table, and the plan says so), branch names, the pull request title, the `kind/` labels of `ship publish`, `ship label` and `ship roadmap sync`, the kinds of `ROADMAP.md` (`deps: bump yaml`), the history habits (where the gitmoji goes, the scopes `ship init` finds), the version increments, and the adaptation to the project's commitlint (below). From the environment, give the object as JSON: `SHIP_COMMITS_TYPES='{"wip": false}'`.

**Scopes.** With `ship.commits.scopes`, a title whose scope is not listed is refused (`SHIP-E0117`) and the planner leaves out a scope it would have guessed but is not listed. `scopeRequired` refuses a title without a scope: the planner cannot invent one, so you write it at review (with `--yes` or an agent, the delivery refuses). A required scope is never dropped to shorten a title.

**With the project's commitlint (rule zero).** When the project's commitlint is installed, it decides what it has rules for, and the settings above still decide the rest:

- Case and the final period are commitlint's: ship-it's own checks of them give way.
- Lengths: commitlint's `header-max-length` and `subject-max-length` (at the error level, 2) decide those lengths, and ship-it's own check gives way. When commitlint has no such rule, `headerMaxLength` or `subjectMaxLength` still applies if a layer set it (a preset, the user file, `.github/ship.json`, a context, an override, `SHIP_*`, `--set`, even to the default value); left at its default, it gives way as before these settings existed. Without commitlint, the settings apply.
- Types: a title must use a type of the effective table, then pass commitlint's `type-enum`. When `type-enum` refuses the planned type, the replacement is chosen among the types of the effective table that commitlint allows; a type commitlint allows but the table lacks is never proposed.
- Scopes: `ship.commits.scopes` and `scopeRequired` apply first; commitlint's `scope-enum` and `scope-empty` then apply too. Both must accept the title: keep the two lists consistent.
- `bodyWidth` shapes the body ship-it writes. With commitlint's `body-max-line-length`, the body is wrapped at the narrower of the two, so commitlint's limit is never exceeded; without the rule, `bodyWidth` applies. The final message is still linted by commitlint as it is.

## Git and history

How ship commits, and how it brings a branch up to date before publishing. The defaults are what ship always did.

| Key | Default | Role |
|---|---|---|
| `ship.publish.mode` | `"auto"` | `"never"` commits only, as `--local`; `ship publish` publishes later |
| `ship.publish.update` | `"rebase"` | a branch behind its base is rebased on it (`"rebase"`, the push then uses a lease), the base is merged into it (`"merge"`, no rewrite, a normal push), or it is published as it is (`"none"`) |
| `ship.git.remote` | `"origin"` | the remote ship fetches from and pushes to, and whose GitHub repository it works with |
| `ship.commits.signoff` | `"auto"` | add `Signed-off-by` (DCO): `"always"`, `"never"`, or `"auto"` when the repository asks for it |
| `ship.commits.sign` | `"auto"` | sign commits: `"auto"` follows git's `commit.gpgSign`, `"always"` signs, `"never"` does not |
| `ship.tidy.auto` | `true` | `false`: `ship` does not run `ship tidy` first (the command itself always runs) |
| `ship.tidy.deleteMerged` | `true` | `false`: `ship tidy` keeps the local branches of merged pull requests |

```jsonc
// .github/ship.json: a project that keeps merge commits and requires the DCO
{ "ship": { "publish": { "update": "merge" }, "commits": { "signoff": "always" } } }
```

- **Update.** Either way a checkpoint comes first, and nothing happens when someone else pushed commits to the branch (their work comes first, `SHIP-W0611`). On a conflict ship cancels the rebase or the merge, publishes the branch as it is and says how to resolve it (`SHIP-W0610`, `SHIP-W0617`). The merge commit keeps git's own message, which commit conventions and the DCO check exempt.
- **Sign-off.** It uses the committer's identity, as `git commit -s` does, so `ship check signoff` accepts it. With `"auto"`, ship signs off when most of the last 50 commits carry a sign-off, when the contributing guide (`.github/CONTRIBUTING.md`, `CONTRIBUTING.md`, `docs/CONTRIBUTING.md`) mentions the DCO or `Signed-off-by`, or when `.github/dco.yml` configures the DCO app.
- **Signature.** `"always"` refuses to commit (`SHIP-E0119`, before any change) when git has no signing key: `user.signingKey`, or for SSH signatures `gpg.ssh.defaultKeyCommand`. Rebases and merges made before a publication are signed the same way, so a ruleset that requires signatures still accepts the branch. Checkpoints are ship's own records: they are never signed, whatever git says.
- **Remote.** Every command uses it: `ship`, `ship publish`, `ship tidy`, `ship status`, `ship undo` (which deletes a branch on the remote the publication used), `ship doctor` and `ship check signoff`.

## Pull requests and GitHub

| Key | Default | Role |
|---|---|---|
| `ship.pullRequest.draft` | `"unattended"` | a new pull request is a draft when nobody reviewed the plan in a terminal (`--yes`), `"always"`, or `"never"` |
| `ship.pullRequest.base` | `""` | base branch of new pull requests; empty: the repository's default branch on GitHub |
| `ship.pullRequest.labelPrefixes.kind` | `"kind/"` | prefix of the kind labels |
| `ship.pullRequest.labelPrefixes.size` | `"size/"` | prefix of the size labels |
| `ship.pullRequest.sizes` | Kubernetes' scale | size labels: name → changed lines under which it applies |
| `ship.pullRequest.sections` | all `true` | `why`, `testing`, `issue`, `changes`: the parts of ship's own body |
| `ship.pullRequest.reviewers` | `[]` | logins asked to review a new pull request |
| `ship.pullRequest.teamReviewers` | `[]` | teams of the organisation, by slug, asked to review it |
| `ship.pullRequest.assignSelf` | `false` | assign a new pull request to its author |
| `ship.pullRequest.milestone` | `"issue"` | put a new pull request in the milestone of the issue it closes, or `"none"` |

```jsonc
// .github/ship.json
{
  "ship": {
    "pullRequest": {
      "base": "develop",
      "labelPrefixes": { "kind": "type: " },
      "sizes": { "small": 100, "medium": 400, "large": 0 },
      "reviewers": ["ada"], "teamReviewers": ["backend"], "assignSelf": true
    }
  }
}
```

- **Drafts.** Work in progress commits, `ship publish --draft` and AI agents always open a draft, whatever the setting: an agent's work is reviewed on GitHub before it is ready.
- **Base.** Closing keywords only work on pull requests to the default branch (GitHub Docs, "Linking a pull request to an issue"). On another base, ship links the issue with `Refs` and leaves its project status alone. The branch is brought up to date with that base.
- **Sizes.** The default is `{"XS": 10, "S": 30, "M": 100, "L": 500, "XL": 1000, "XXL": 0}`: `XS` under 10 changed lines, generated files excluded, and so on; `0` marks the size of everything larger (one size at most). Two sizes cannot share a threshold. A layer replaces the whole table. Without a `0` size, a pull request larger than every threshold gets no size label. `ship sync` creates the labels the prefixes and sizes name; `ship publish` only uses the ones the repository has.
- **Sections.** They shape ship's own body; a repository template, when one applies, places the parts itself. `why: false` leaves out the commit bodies (several commits are still listed). `changes: false` leaves out the summary of the diff, which a repository template otherwise gets at the end of its summary part.
- **Reviews, assignee, milestone.** They apply to a new pull request, never to an updated one, and never ask the author to review their own work. The pull request exists by then, so a refusal (a reviewer without access to the repository, no triage right) is a warning. `ship undo` takes the pull request out of the milestone before closing it; the review requests and the assignee stay on the closed pull request.

### Forks

ship knows a repository is a fork when GitHub says it has a parent, or when an `upstream` remote names another repository of the same server. It then pushes to the remote (`ship.git.remote`) and opens the pull request on the parent, from `<your login>:<branch>`, against the parent's default branch (or `ship.pullRequest.base`). The base comes from the parent: through the `upstream` remote when it names the parent, else fetched by URL into `refs/ship/parent/<branch>`, without adding a remote (over SSH when your remote uses SSH, HTTPS otherwise). `ship tidy` follows the parent too, `ship status` and `ship check signoff` when an `upstream` remote names it, and `ship undo` closes the pull request on the parent and deletes the branch on your fork.

## GitHub Project

`ship roadmap sync` and `ship publish` keep a GitHub Project (v2) up to date when `github.project.url` names one ([automatisations-github §6](../design/github-automation.md#github-project-implemented-2026-11-02)). The project's settings are grouped under `github.project`, with the project's address in `url`: one object holds the project and its own field names, as `github.labels` does for labels, and a later setting of the project (a view, an iteration field) has its place next to them.

| Key | Default | Role |
|---|---|---|
| `github.project.url` | `""` (off) | `https://github.com/orgs/acme/projects/7`, `https://github.com/users/ada/projects/3` (a `/views/n` suffix is accepted), or `acme/7` (organisation tried first, then user) |
| `github.project.statusField` | `"Status"` | single-select field of the status; `""` leaves the status alone |
| `github.project.todo` / `inProgress` / `done` | `"Todo"` / `"In progress"` / `"Done"` | options: open issue without a status / linked issue of a new pull request / closed issue |
| `github.project.dateField` | `""` (off) | date field set to the milestone's due date |
| `github.project.phaseField` | `""` (off) | single-select field set to the milestone's title (the options must exist) |

```jsonc
// .github/ship.json
{
  "github": {
    "project": {
      "url": "https://github.com/orgs/acme/projects/7",
      "dateField": "Target date",
      "phaseField": "Phase"
    }
  }
}
```

Names match without regard to case. A field or option the project lacks is reported, never created. The token needs the `project` scope: `gh auth refresh -s project`.

## Checks (gates)

The project's checks (lint, typecheck, test, found from its files) run before every push. `ship check gates` runs them now, without committing or pushing; it exits with code 3 on a red check, like a publication.

`ship.gates.beforeCommit` (`false` by default) also runs them before the commit of `ship` and of the `ship_deliver` tool: a red check refuses the commit and nothing changes. It stays off by default so that a commit is instant and the checks run once; when it is on, `ship` does not run them again before its own push, unless it first rebases or merges the branch on its base. `ship.gates.enabled` false turns every check off, this one included.

```jsonc
// .github/ship.json
{ "ship": { "gates": { "beforeCommit": true } } }
```
## Secrets

The secret scan runs on the added lines before every commit and on the commits a push would send. Four keys tune it:

| Key | Default | Role |
|---|---|---|
| `ship.secrets.ignorePaths` | `[]` | files never scanned (`fixtures/**`, `*.snap`) |
| `ship.secrets.rules` | `[]` | extra rules, tried after the built-in ones |
| `ship.secrets.disabledRules` | `[]` | built-in rules turned off, by id |
| `ship.secrets.allow` | `[]` | findings accepted, by fingerprint |

```jsonc
// .github/ship.json
{
  "ship": {
    "secrets": {
      "rules": [
        { "id": "acme_token", "pattern": "\\bacme_[0-9a-f]{24}\\b", "keywords": ["acme_"] }
      ],
      "disabledRules": ["secret_assignment"],
      // tests/fixtures/aws.txt:3, a documented example key
      "allow": ["154df04c078d785552fab6920378c04ea36d73c591f7097120765869030d2804"]
    }
  }
}
```

- **Extra rules.** `id` is lower-case letters, digits and `_`, and not the id of a built-in rule; two rules cannot share an id. `pattern` is a Go regular expression ([RE2](https://github.com/google/re2/wiki/Syntax)): no back-references, and the scan time stays linear. `(?i)` ignores case. A pattern that matches an empty text is refused. `keywords` are literals, one at least: a line that holds none of them is not matched against the pattern, which keeps the scan fast on large diffs (the built-in rules work the same way). They are compared without case when the pattern has `(?i)`. A line with non-ASCII characters is always matched against the pattern, so a keyword can only skip work, never hide a match that holds it. The value reported is the first capture group, or the whole match. Unlike the built-in rules, an extra rule has no placeholder filter (`${VAR}`, `example`…): it reports every match.
- **Disabled rules.** The ids of the built-in rules are `private_key`, `github_token`, `aws_key`, `anthropic_key`, `openai_key`, `groq_key`, `google_key`, `stripe_key`, `slack_token`, `npm_token`, `url_with_password`, `secret_assignment` and `sensitive_file` (a whole `.env`, `id_rsa` or `.pem` file). `ship config explain ship.secrets.disabledRules` lists them; an unknown id is a configuration error that lists them too.
- **Accepted findings.** ship prints each finding with its **fingerprint**, in the terminal and in `--json` (`"fingerprint"`): in lower-case hexadecimal, the PBKDF2-HMAC-SHA256 of the matched value with 600 000 iterations, salted with `ship-it/secrets/fingerprint`, a NUL byte and the rule id (`pbkdf2_hmac("sha256", value, "ship-it/secrets/fingerprint\x00aws_key", 600000)`). For a whole sensitive file, the value is its path. The value itself is never printed nor stored: a value in a configuration file would leak it, and the slow derivation keeps a weak secret from being found back from its fingerprint. The plain SHA-256 fingerprints of ship-it 0.1 are still accepted during 0.x; ship prints the new one to replace them. The same value found by the same rule has the same fingerprint in any file, so one entry accepts it everywhere. When the matched value of a line is accepted, the next rules still run on that line.

**AI agents (MCP).** An agent never changes settings, and the scan always runs. For an unattended delivery or push (`ship mcp` without `--attended`), `ignorePaths`, `disabledRules` and `allow` count only when they come from `.github/ship.json` as committed in `HEAD` (its overrides that match the delivery included): they are the team's reviewed choices. The file is read from the last commit, not from the working tree, so a change the agent makes to it has no effect on its own scan; the user file, presets, contexts, `SHIP_*` and `--set` give an agent no exemption either. Extra rules apply from every layer: they only add checks. A human (the terminal, or the VS Code extension with `ship mcp --attended`) gets the exemptions of every layer.

## Interface

How `ship` talks to a person in a terminal. These are personal choices: the user file, its contexts, `SHIP_UI_*` variables and `--set` accept them. A file shared with a team refuses them with a configuration error that names the key (`SHIP-E0702`): `.github/ship.json`, a preset and a per-path override. `ship config set ui.…` writes to the user file by default, even inside a repository. AI agents (MCP) see none of them.

| Key | Default | Role |
|---|---|---|
| `ui.language` | `auto` | `en` or `fr`; `auto` follows the locale |
| `ui.reviewScreen` | `true` | `false`: the plan is printed and one question asked, `Ship it? [Y/n]`, instead of the review screen (the line-by-line prompts when a commit type must be picked) |
| `ui.accessible` | `false` | `true`: line-by-line prompts for screen readers, as `--plain` and `SHIP_ACCESSIBLE=1` |
| `ui.color` | `auto` | `never` removes the colours of the review screen, `always` keeps them when the terminal is not detected as colour-capable |

```jsonc
// user file
{ "ui": { "language": "fr", "color": "never" } }
```

- **Language**: `SHIP_LANG` wins over `ui.language`, which wins over the locale (`LC_ALL`, `LC_MESSAGES`, `LANG`). `SHIP_LANG` is an environment variable like the `SHIP_*` settings, which beat every file; `SHIP_UI_LANGUAGE` sets the setting itself.
- **Colour**: `NO_COLOR`, set to any value, always removes the colours, whatever `ui.color` says. Colour never carries meaning on its own ([cli](cli.md#the-review-screen)).
- `--plain`, `SHIP_ACCESSIBLE=1` and `TERM=dumb` still give the line-by-line prompts when `ui.accessible` is `false`: the setting can turn them on, not off.

## Planned

- `ship config get|set|unset|explain` (being implemented): change a setting without editing JSON by hand, comments and order preserved, undoable with `ship undo`; `explain` gives the documentation, the value of every layer and why the effective one wins.
- `ship.conventions.source`: the project's own conventions (commitlint, rulesets, history) already win and ship-it says so (above); the setting would only choose between them when they disagree, if that ever happens.
- Identity per context (`git.email` for a client).
