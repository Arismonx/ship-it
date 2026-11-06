# Generated conventions

> Decision: [ADR-0007](../adr/0007-emoji-header-format.md).
>
> Rule zero: what the project imposes wins. A repository's pull request and issue templates, its commitlint/commitizen configuration, its GitHub rulesets and the habits visible in its history take precedence over ship-it's defaults. ship-it's formats below apply where the project says nothing.
>
> What is implemented (decisions of 2026-10-24):
>
> - History: `ship.commits.gitmoji: "auto"` follows where the last 50 commits put the gitmoji ([configuration](configuration.md#the-projects-habits)).
> - commitlint, read and validated. When the project has a commitlint configuration (the files commitlint reads, or a `commitlint` field in `package.json`) and the local binary `node_modules/.bin/commitlint`, ship-it reads the resolved configuration (`npx --no -- commitlint --print-config json`, never a download) and writes its title to fit the level-2 rules: `type-enum` (a type the project does not allow becomes the one type that means the same (`security` → `fix`, `wip` → `chore`); when several could fit, or none of ship-it's close types is allowed, the plan lists the allowed types and the developer picks one at review ([cli](cli.md#the-review-screen)); with `--yes` or an agent, nobody picks and the delivery refuses, `SHIP-E0906`), `scope-enum` and `header-max-length` (the scope goes), `scope-case`, `subject-case`, `subject-full-stop`; a subject is never cut (`subject-max-length` → `SHIP-E0906`). Whether a leading gitmoji passes is found by linting, since the parser's pattern does not survive JSON: when commitlint refuses it, the title goes without gitmoji, even against `ship.commits.gitmoji`, and the plan says so. Right before committing, the exact message goes through the project's commitlint on standard input; a refusal stops everything with commitlint's own output (`SHIP-E0905`, exit code 3, nothing changed). ship-it's own style rules (lengths, case, period) then give way to the project's.
> - GitHub rulesets, before the push. `ship publish` reads the active rules of the branch (`GET /repos/{owner}/{repo}/rules/branches/{branch}`) and evaluates them as GitHub does (literal `starts_with`/`ends_with`/`contains`, RE2 regex, `negate`). A branch name that breaks a pattern is renamed, then pushed, when ship-it created the branch and never pushed it: the name stays as close to ship-it's as possible (`feat/` → `feature/`, a required prefix in place of ship-it's, a required suffix, no prefix, an upper-case ticket key, never invented text), is checked against its own rules, logged, and `ship undo` gives the old name back (`SHIP-W0616`). Otherwise ship-it refuses, quoting the rule and giving the exact `git branch -m` command (`SHIP-E0612`). Commits whose message or author/committer e-mail break a pattern, or unsigned commits where signatures are required, are refused before the push with the commit and the remedy (`SHIP-E0613` to `SHIP-E0615`); ship-it never rewrites history. No access to the rules (404, 403) means none: GitHub still enforces them.

## Commit and pull request titles

```
<emoji> <type>(<scope>)<!>: <subject>
```

- `emoji`: Unicode (no `:shortcode:`, which renders poorly outside GitHub), from the official gitmoji table.
- `type`: Conventional Commits vocabulary (below).
- `scope`: optional, derived from the paths (component, package, module); `ship.commits.scopes` lists the allowed ones, `ship.commits.scopeRequired` makes it mandatory, `ship.commits.scopeFrom: "none"` stops the planner from guessing it ([configuration](configuration.md#commit-and-branch-conventions)).
- `!`: breaking change. The `BREAKING CHANGE:` footer is also written in the body.
- `subject`: imperative mood, lower-case first letter, no final period, at most 50 characters; the whole header at most 72 (by default: `ship.commits.subjectMaxLength` and `ship.commits.headerMaxLength` change them; `ship check title` validates both). English by default.
- Types (those of `@gregoiref/commitlint-config`, the built-in table; `ship.commits.types` changes an emoji, a branch prefix or a kind label, adds a type or removes one, and `ship types` lists the result): `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `security` (🔒️), `revert`, `wip` (🚧, never in a ready pull request: ship-it opens a draft when a `wip` commit is present).
- Emojis are written with the variation selector (U+FE0F); the parser accepts both forms (🔒 and 🔒️), like the commitlint `headerPattern`.
- With a squash merge, the pull request title becomes the commit on the base branch, so it follows exactly the same format. For several commits, ship-it takes the most significant header (the first breaking change, else the first feature, else the oldest commit); the title stays editable at review.

### Type → emoji table

Emojis and descriptions from the official gitmoji table ([gitmojis.json](https://raw.githubusercontent.com/carloscuesta/gitmoji/master/packages/gitmojis/src/gitmojis.json)).

| Type | Default emoji | Official gitmoji description | Contextual variants (detection) |
|---|---|---|---|
| `feat` | ✨ | Introduce new features | 🚸 UX, ♿️ accessibility, 🌐 i18n, 📱 responsive, 🚩 feature flag, 💫 animations, 👔 business logic |
| `fix` | 🐛 | Fix a bug | 🚑️ critical hotfix, 🩹 minor fix, ✏️ typo, 🔒️ security, 🚨 linter warnings, 💚 broken CI |
| `docs` | 📝 | Add or update documentation | 💡 code comments |
| `style` | 💄 | Add or update the UI and style files | 🎨 code format/structure |
| `refactor` | ♻️ | Refactor code | 🚚 move/rename, ⚰️ dead code, 🔥 removal, 🏗️ architecture |
| `perf` | ⚡️ | Improve performance | |
| `test` | ✅ | Add, update, or pass tests | 🧪 failing test added, 📸 snapshots, 🤡 mocks |
| `build` | 📦️ | Add or update compiled files or packages | ➕ add dependency, ➖ remove, ⬆️ upgrade, ⬇️ downgrade, 📌 pin |
| `ci` | 👷 | Add or update CI build system | 💚 fix the CI |
| `chore` | 🔧 | Add or update configuration files | 🔨 dev scripts, 🙈 .gitignore, 📄 license, 🧱 infrastructure, 🧑‍💻 DX |
| `revert` | ⏪️ | Revert changes | |
| `security` | 🔒️ | Fix security or privacy issues | 🔐 secrets, 🛂 authorization |
| `wip` | 🚧 | Work in progress | never outside a draft |
| release | 🔖 | Release / Version tags | |
| breaking | 💥 | Introduce breaking changes | replaces the type's emoji when `!` is present |
| initial commit | 🎉 | Begin a project | the first commit of a repository: `🎉 chore: initial commit` |

Deliberately excluded: 💩, 🍻 and 🥚 are never generated (professional and public-sector contexts).

## Branches

[Conventional Branch](https://conventional-branch.github.io/) 1.1.0 (see [git and GitHub standards](../design/git-github-standards.md)), implemented in [`core/convention/branch.go`](../../internal/core/convention/branch.go):

```
<prefix>/<issue>-<description>      feat/42-add-login-page · fix/crash-startup · claude/7-handle-nil
```

- Prefix: `feat/`, `fix/` (also for `security`), `hotfix/` (explicit urgent fix), `release/`, `chore/` (docs, style, refactor, perf, test, build, ci, chore, revert, wip), or the `branchPrefix` a repository gives a type in `ship.commits.types` (accepted by `ship check branch` too); an agent replaces the prefix with `claude/`, `codex/`, `copilot/`, `cursor/` or `ai/`.
- Short description (ship-it values; the specification only says "short"): at most 4 words and 32 characters by default (`ship.branches.maxWords`, `ship.branches.maxLength`); stop words removed (English and French: *the, for, de, le…*); the first word is dropped when it repeats the prefix ("fix crash on startup" → `fix/crash-startup`); accents transliterated; uniqueness guaranteed (`-2`, `-3`…).
- The linked issue number comes first (`ship.branches.issue: "none"` leaves it, and the ticket, out).
- The very first commit of a repository lands on the current branch: there is no base to branch from yet.
- External tickets (Jira, Linear): with `ship.issues.trackerKeys` (for example `["ACME"]`, usually in a client context of the user file), a ticket found in the branch name, whatever its case, ends the commit and pull request titles: `✨ feat(auth): add login (ACME-123)`, still valid Conventional Commits, and the branch keeps it in upper case (`feat/ACME-123-add-login`), the one exception to Conventional Branch's lower case, because Jira only recognises upper-case keys. The reference does not count in the 50-character subject.
- A branch name pattern imposed by a ruleset wins (rule zero above: rename before the first push, or refusal with the command). Planned: a different convention visible in the repository's branches.
- Stacks: one branch per layer, same rule.

## Pull request body

The body says only what is true and known, the way a careful author writes it:

- The why comes from the developer: the prose of the commit bodies (trailers such as `Signed-off-by:` and lines that only link issues are dropped). Google's guidance on change descriptions asks for "the problem being solved" and "why this approach"; a diff cannot tell it, so ship-it never makes it up.
- One commit: the title says what changed, the body gives the why. Several commits: their subjects are listed as sentences, without the `type(scope):` syntax.
- Testing lists the checks ship-it really ran before the push (the gates), or says plainly that they were skipped (`--skip-gates`).
- No empty section, no placeholder. A one-line commit without issue gives an empty body: the title says it all.
- No visible signature. An invisible `<!-- ship-it -->` comment marks the pull requests ship-it wrote.
- AI assistance is disclosed the repository's way (policies differ: Kubernetes forbids assisted-by trailers, Linux requires them, Rust cites "Generated with Claude Code" as poor disclosure): 1) the AI section of the repository's template, filled; 2) else the repository's own AI label (`ai-assisted`, `ai-generated`, `llm-generated`…), applied; 3) else one factual sentence, without a tool signature: *The commit message was drafted with AI assistance (mistral-small).*, or `host agent` when the MCP client's model wrote it. Never a git trailer.

Without a repository template:

```markdown
Closes #42 (Invoices ignore VAT)

The EU changed its VAT rules in July.

- Add the July VAT rates
- Explain the rounding in the invoice footer

### Testing

`npm run lint`, `npm run typecheck` and `npm test` pass locally.

### Notes

**Breaking change:** Drop the v1 invoice API.

The commit message was drafted with AI assistance (host agent).

<!-- ship-it -->
```

With a repository template (`ship.pullRequest.template: "auto"`, the default): ship-it reads the template GitHub itself would offer, from the default branch, in GitHub's order (`.github/`, the root, `docs/`; file name `pull_request_template.md`, any case). It fills the sections it has facts for: summary/description/why, testing, related issue, breaking changes, AI usage, and only when the section holds nothing but guidance comments. Everything else stays as the maintainers wrote it: comments, checklists (never ticked for the author), release-note blocks, type-of-change questions. What has no section in the template is placed first (issue, summary) or left out (testing, notes). With several templates in `PULL_REQUEST_TEMPLATE/`, GitHub offers none by default, and neither does ship-it unless `ship.pullRequest.template` names one. `"ship"` always uses ship-it's format.

The developer's words: `ship --why "…" --tested "…"` (and the `why`/`tested` arguments of `ship_deliver` over MCP, and the Why? and Tested? fields of the review screen) writes the why as the commit body, wrapped at 72 columns (`ship.commits.bodyWidth`), and how the change was tested as a `Tested:` trailer. The history keeps them; the pull request quotes the why as its summary and the trailer in its Testing section ("Tested by the author: …"). Nothing is written when they are empty.

Planned: organisation-wide templates from a public `.github` repository.

## Changelog

Planned: [Keep a Changelog](https://keepachangelog.com/) (which version of the format is still to be checked), with sections derived from the types, and optional plain-language client release notes, in the client's language, for agencies.

## Third-party tooling compatibility

With the emoji first, commitlint and release-please do not parse the messages as they are. ship-it provides:

- its own parser, which accepts an optional leading emoji;
- `ship.commits.gitmoji` (`"after-type"`: `feat(ui): ✨ subject`, or `"none"`) for teams that depend on those tools, and the project's own commitlint, which ship-it follows and runs before every commit (rule zero above);
- a commitlint preset, `@gregoiref/commitlint-config`, which already accepts the optional leading emoji.
