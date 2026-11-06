# git & GitHub standards: the real values

## Decisions (2026-10-23)

| # | Topic | Decision |
|---|---|---|
| NC1 | Lengths | **Subject ≤ 50**, **full header ≤ 72**; ship-it shortens or drops the scope to fit. Compatible with `@gregoiref/commitlint-config` (≤ 100) |
| NB1 | Branches | **100 % Conventional Branch 1.1.0**: `feat/`, `fix/`, `hotfix/`, `release/`, `chore/`; the types `docs`, `refactor`, `test`, `style`, `ci`, `build`, `perf`, `security`, `revert`, `wip` → `chore/` (except `security` → `fix/`); an agent that delivers → `claude/`, `codex/`, `copilot/`, `cursor/` or `ai/` |
| NB2 | Ticket | Issue number at the start of the slug: `feat/123-add-login` |
| NB3 | Model | **Trunk-based**: `main` is the only long-lived branch, single-session branches, stacks; release from `main` (fix forward); `release/x.y` only if maintenance is needed |
| NB4 | Issues and stacks | `Closes #n` in the **message of the squashed commit on `main`** (a keyword in a commit merged into the default branch closes the issue ✅) + `Refs #n` in the body of each PR of the stack |
| NL1 | Labels | **Kubernetes-style prefixed taxonomy `group/value`** (§5 bis); `good first issue` and `help wanted` kept as they are |
| NI1 | Issue forms | Bug, Feature; `config.yml`: `blank_issues_enabled: false` + `contact_links` to Discussions (questions, docs) and to private reporting (security) |
| NX1 | Extras | Automatic `.git-blame-ignore-revs`, Conventional Comments in `ship inbox` and CONTRIBUTING, an existing `pull_request_template.md` honored. Immutable releases: **not adopted** for automation |
| NZ1 | Zero config | First `ship` in an unknown repository: **detect and announce** in 3 lines (base, ecosystem, gates, existing convention); no file written before `ship init` |
| TEL | Telemetry | **None** (neither collection nor sending); only voluntary bug reports (`ship bug`), see [logs](../reference/logs.md) |

> Checkup of 2026-10-23. **Only values read on the primary source** (✅) or clearly marked (🟡 secondary, ❓ not found). The ship-it decisions that follow from them are marked with ⚙️.

## 1. Commit message

| Rule | Real value | Source |
|---|---|---|
| Title | "single short (**no more than 50 characters**) line", then **a blank line**, then the body; the text up to the first blank line is the title | ✅ [git-commit DISCUSSION](https://git-scm.com/docs/git-commit) |
| 50 limit | "**soft limit**" (not an error); no trailing period | ✅ [SubmittingPatches](https://git-scm.com/docs/SubmittingPatches) |
| Mood | **Imperative** ("make xyzzy do frotz"); the problem described in the present tense; no capital letter after the `area:` prefix | ✅ same |
| Body | Explains the **why**: the problem, why this solution, the alternatives ruled out. If it gets long, split the commit | ✅ same |
| Body width | **Not specified by git** ("follow the wrapping convention of the surrounding project") | ✅ same |
| Trailers | At the end of the message, in chronological order, `Signed-off-by:` **last**; only the first letter capitalized (`Signed-off-by`, not `Signed-Off-By`); common ones: `Reported-by`, `Acked-by`, `Reviewed-by`, `Tested-by`, `Co-authored-by`, `Helped-by`, `Suggested-by` | ✅ same |
| GitHub co-authors | `Co-authored-by: name <email>`, one line per co-author, a **blank line** before the trailers, an e-mail linked to the GitHub account (otherwise the noreply address) | ✅ [GitHub Docs](https://docs.github.com/en/pull-requests/committing-changes-to-your-project/creating-and-editing-commits/creating-a-commit-with-multiple-authors) |
| Conventional Commits | 1.0.0 (latest version), type first, `!` or `BREAKING CHANGE:` | ✅ |
| Your current config | header ≤ 100, body and footer ≤ 100 per line (`@gregoiref/commitlint-config`); subject ≤ 50 (`cz-config`) | ✅ |

⚠️ **Real tension**: git recommends a **whole** title ≤ 50 characters, whereas our `✨ feat(scope): subject` format already uses 10 to 20 characters before the subject. See question NC1.

## 2. Branch names

| Rule | Real value | Source |
|---|---|---|
| git validity | No component starting with `.` or ending with `.lock`; no `..`; no control character, space, `~`, `^`, `:`, `?`, `*`, `[`, `\`; no leading or trailing `/` and no `//`; no trailing `.`; no `@{`; not `@` alone | ✅ [git-check-ref-format](https://git-scm.com/docs/git-check-ref-format) |
| **Conventional Branch 1.1.0** | `<type>/<description>`; types: `feature/` (`feat/`), `bugfix/` (`fix/`), `hotfix/`, `release/`, `chore/`; **AI agent prefixes** (v1.1.0): `ai/`, `claude/`, `codex/`, `copilot/`, `cursor/`; trunk without a prefix (`main`, `master`, `develop`) | ✅ [conventionalbranch.org](https://conventionalbranch.org/) |
| Characters | `a-z`, `0-9`, `-` (and `.` for versions); no capitals, `_` or spaces; no `--`, `..`, `-.`, and no leading or trailing `-`/`.`; ticket number recommended where applicable | ✅ same |
| Invalid (official examples) | `Feature/Add-Login`, `fix/header bug`, `fix/header_bug`, `feature/new--login`, `feature/-new-login`, **`unknown/some-task`** (unknown prefix) | ✅ same |
| Length | No limit found in git or GitHub; only the file system imposes a per-component limit | ❓ |

⚠️ Our current plan (`docs/`, `refactor/`, `test/`… as prefixes) **is not compliant** with Conventional Branch ("unknown prefix"). See question NB1.

## 3. Branch management

| Model | What the source says | Source |
|---|---|---|
| **Trunk-based** | One shared branch (`main`), **short** branches (one work session); "all team members commit to trunk **at least once every 24 hours**"; **optional** release branches, cut just in time then deleted, or release straight from the trunk with "fix forward"; feature flags and branch by abstraction for large changes | ✅ [trunkbaseddevelopment.com](https://trunkbaseddevelopment.com/) |
| GitHub stacks | Bottom of the stack → trunk; each layer targets the branch below; merged from the bottom up | ✅ |

### ⚠️ Trap found: closing keywords and stacks
"The keywords **work only when the pull request targets the repository's default branch**. For a PR aimed at any other branch, the keywords are ignored" ✅ [GitHub Docs](https://docs.github.com/en/issues/tracking-your-work-with-issues/using-issues/linking-a-pull-request-to-an-issue). Keywords: close(s/d), fix(es/ed), resolve(s/d), capitals and `:` accepted, cross-repo `owner/repo#n`. **In a stack, only the bottom PR targets `main`**: a `Closes #12` in a higher layer does nothing. ❓ Does GitHub's automatic rebase and retarget (when the PR below is merged) reactivate the link? Not documented. See question NB4.

## 4. Tags and versions

| Rule | Real value | Source |
|---|---|---|
| SemVer | "**v1.2.3 is not a semantic version**"; the `v` is a common convention; pre-release `-` + identifiers `[0-9A-Za-z-]`, no leading zero; build `+…` ignored in precedence | ✅ [semver.org](https://semver.org/) |
| Go | Module versions **always prefixed with `v`**; `+incompatible` tag ignored; module in a subdirectory → tag `subdirectory/vX.Y.Z`; from v2 on → `/v2` suffix in the module path | ✅ [go.dev/ref/mod](https://go.dev/ref/mod) |
| Immutable releases | GA on **Oct. 28, 2025**: tags and assets protected after publication, signed attestation (tag, SHA, assets), verifiable with `gh release verify`; not on GHES | ✅ [Changelog](https://github.blog/changelog/2025-10-28-immutable-releases-are-now-generally-available/) |

⚙️ ship-it: `vX.Y.Z` tags (mandatory for Go, and the dominant convention); monorepo → `<package>/vX.Y.Z` (the Go convention reused for every ecosystem); **immutable releases enabled** by `ship init` (`agency` and `public-sector` levels).

## 5. Labels

| Fact | Source |
|---|---|
| Labels created by default in a new repository: `accessibility`, `bug`, `documentation`, `duplicate`, `enhancement`, `good first issue` (feeds the `contribute` page), `help wanted`, `invalid`, `question`, `wontfix` | ✅ [GitHub Docs](https://docs.github.com/en/issues/using-labels-and-milestones-to-track-work/managing-labels) |
| Maximum length of a label name | ❓ not documented |

⚠️ Our planned fallback (`type:bug`, `priority:p1`…) **would duplicate** the default labels (`bug`, `enhancement`). See question NL1.

## 5 bis. ship-it label taxonomy (NL1)

`group/value` format, inspired by Kubernetes' real taxonomy ✅ ([labels.md](https://github.com/kubernetes/test-infra/blob/master/label_sync/labels.md): `kind/*`, `priority/*`, `area/*`, `triage/*`, `lifecycle/*`, `size/*`). GitHub's default labels are **renamed** (the `oldname` field of settings-as-code), not deleted, so that existing issues keep their labels.

| Group | Labels | Notes |
|---|---|---|
| `kind/` | `bug`, `feature`, `docs`, `refactor`, `perf`, `security`, `test`, `ci`, `build`, `chore`, `regression`, `breaking`, `support` | Aligned with our commit types, plus `regression` and `support` (taken from Kubernetes). Renames: `bug`→`kind/bug`, `enhancement`→`kind/feature`, `documentation`→`kind/docs`, `question`→`kind/support` |
| `priority/` | `p0`, `p1`, `p2`, `p3` | p0 = "must be worked on now" (assignee mandatory), p1 = next release, p2 = long term, p3 = backlog; aligned with the Priority field of Issue Fields to ease a move to an organisation |
| `triage/` | `needs-triage`, `accepted`, `needs-info`, `duplicate`, `not-reproducible`, `invalid`, `wontfix` | Renames: `duplicate`, `invalid`, `wontfix` → `triage/*` |
| `lifecycle/` | `active`, `stale`, `frozen` | |
| `size/` | `XS` (0-9), `S` (10-29), `M` (30-99), `L` (100-499), `XL` (500-999), `XXL` (1000+) | Kubernetes' exact thresholds ✅, **generated files ignored**; set automatically by ship-it on its PRs. The 400-line guardrail falls into `size/L` |
| `area/` | proposed by `ship init` from the commit scopes and the folders | |
| `a11y/` | `accessibility` → `kind/accessibility` | |
| no prefix | `good first issue`, `help wanted` | Recognized by GitHub (`contribute` page) |

**Automatic labeling (implemented on 2026-10-23, [`core/pullrequest/labels.go`](../../internal/core/pullrequest/labels.go))**: when a PR is created, ship-it sets `kind/*` (from the commit types, plus `kind/breaking`) and `size/*`. The line count excludes lockfiles and `linguist-generated` files (`git check-attr`).
- **ship-it never creates a label when publishing**: it only sets those that **already exist** in the repository. Creation goes through `ship init` then `ship sync`, with a plan reviewed before applying.
- In a repository that kept GitHub's default labels, `kind/feature`, `kind/bug` and `kind/docs` fall back to `enhancement`, `bug` and `documentation`.
- A labeling failure does not undo the publication: it is a warning.
- **Updating a PR** (new commits pushed): only `size/*` is recomputed. The new size label **replaces** the old one, of which ship-it is the only author, which avoids `size/S` and `size/M` side by side. `kind/*` labels are only set at creation, so as not to undo a reviewer who removed one on purpose. Other labels are never touched.

Colors (revised on 2026-10-23, user choice): **color carries meaning**, as in kubernetes/kubernetes (labels read through the API ✅): red `e11d21` for what breaks or blocks (bug, regression, breaking change, security, p0), orange `eb6420` then yellow `fbca04` for urgency, a green → red gradient for `size/*` (`009900` … `ee0000`), neutral blue `c7def8` for the other `kind/*` and `0052cc` for `area/*`. The former "one color per group" rule (secondary source 🟡) is dropped. Descriptions are always filled in.

## 6. Issues and PRs

| Rule | Real value | Source |
|---|---|---|
| Issue forms | `.github/ISSUE_TEMPLATE/*.yml`; required keys `name`, `description`, `body`; optional `title`, `labels` (labels that do not exist are **ignored**), `assignees`, `type` (organisation issue type), `projects` (`OWNER/NUMBER`, write access required); elements `markdown`, `input`, `textarea`, `dropdown`, `checkboxes`, `upload`; `validations.required` | ✅ [syntax for issue forms](https://docs.github.com/en/communities/using-templates-to-encourage-useful-issues-and-pull-requests/syntax-for-issue-forms) |
| `config.yml` | `blank_issues_enabled: false` (the "Blank issue" button stays visible only to the Write, Maintain or Admin roles); `contact_links` (`name`, `url`, `about`) | ✅ [configuring issue templates](https://docs.github.com/en/communities/using-templates-to-encourage-useful-issues-and-pull-requests/configuring-issue-templates-for-your-repository) |
| PR template | `pull_request_template.md` at the root, in `docs/` or in `.github/`; several: a `PULL_REQUEST_TEMPLATE/` folder + the `template=` URL parameter | ✅ [PR template](https://docs.github.com/en/communities/using-templates-to-encourage-useful-issues-and-pull-requests/creating-a-pull-request-template-for-your-repository) |

⚙️ ship-it fills the PR body itself (generated template). An existing `pull_request_template.md` is **honored**: ship-it fills its sections instead of overwriting it (❓ to design).

## 7. Things we had not thought of

| Practice | What it is | Source | Proposal |
|---|---|---|---|
| **`.git-blame-ignore-revs`** | At the root, one SHA per line (with comments); GitHub hides these commits in the blame view, with a banner | ✅ [GitHub Docs](https://docs.github.com/en/repositories/working-with-files/using-files/viewing-and-understanding-files) | ship-it automatically adds **pure formatting** commits (`style`, `🎨`) to it and sets `blame.ignoreRevsFile` locally |
| **Conventional Comments** | Review comments `label [decorations]: subject`; labels `praise`, `nitpick`, `suggestion`, `issue`, `todo`, `question`, `thought`, `chore`, `note`; decorations `(blocking)`, `(non-blocking)`, `(if-minor)` | ✅ [conventionalcomments.org](https://conventionalcomments.org/) | `ship inbox` sorts the comments received (blocking or not); recommended in CONTRIBUTING |
| **Immutable releases** | See §4 | ✅ | enabled by `ship init` |
| **DCO `Signed-off-by`** | See §1 | ✅ | already decided (N14); `ship` adds it automatically if the repository requires it |
| `.mailmap` | Unifies the identities of the same author in `git log` and `shortlog` | ❓ not verified in this session | to research |
| `CITATION.cff`, `FUNDING.yml` | Academic citation, Sponsor button | ❓ not verified | to research |
