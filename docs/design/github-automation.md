# GitHub automations & setup

> User choices (2026-10-22): setup **by level + config file**, **settings-as-code**, **Issue Types/Fields** (falling back to labels for personal repositories), scope **governance + project planning**, auto-assignment **author + linked issue → Project + native Teams**, roadmap **ROADMAP.md → GitHub**. Actions/Docker: **ship-it's own CI/release + a Docker image of ship-it**.
> Source legend: ✅ verified on the primary source · 🟡 secondary · ❓ to verify.

## 1. Principle: do not get in the way

Findings (🟡, general UX context rather than CLI-specific): announced, reversible defaults rather than blocking questions, ≤ 3 questions at onboarding, advanced options behind an explicit choice, visible and undoable defaults ([ConnectOnion](https://docs.connectonion.com/blog/cli-ux-progressive-disclosure), [OpenClaw](https://docs.openclaw.ai/start/onboarding-redesign.md), [Moodle/Nielsen](https://docs.moodle.org/24/en/Development:Progressive_Disclosure), [Pattern Playground](https://pattern-playground.onrender.com/patterns/progressive-disclosure)).

What it means for ship-it:

1. **`ship` works without any setup.** Setup is never a prerequisite.
2. **`ship init` asks a single question: the level.**
3. The level produces a **readable file** (`.github/ship.json`) that contains **only** what is enabled. To remove an automation, delete its line.
4. Before applying: **plan → diff → apply**, like Terraform. Nothing is applied without being shown first.
5. Afterwards, ship-it suggests **only one improvement at a time**, and only when a signal justifies it (e.g. a 3rd contributor detected → CODEOWNERS?). A declined suggestion is never offered again.

## 2. Levels

| | **Solo** (`solo`) | **Team** (`team`) | **Agency** (`agency`) | **Public sector** (`public-sector`) |
|---|---|---|---|---|
| Merge settings (squash only, squash title = PR title, branch deletion) | ✓ | ✓ | ✓ | ✓ |
| Classification (types/fields or fallback labels) | minimal | ✓ | ✓ | ✓ |
| Issue forms (bug / feature) | - | ✓ | ✓ | ✓ |
| Ruleset on the default branch (PR required, required checks) | - | ✓ | ✓ | ✓ |
| Author assigned to their PR | ✓ | ✓ | ✓ | ✓ |
| Reviewers through native Teams | - | ✓ (org) | ✓ | ✓ |
| CODEOWNERS | - | suggested | ✓ | ✓ |
| Project + ROADMAP.md → GitHub | - | option | ✓ | ✓ |
| Linked issue → assigned + Project status | - | ✓ | ✓ | ✓ |
| Releases + client notes | option | option | ✓ | ✓ |
| Locked organisation policy | - | - | option | ✓ |
| AI | off | off | off | **forbidden by default** |

`ship init --custom` offers an à la carte choice. `ship init --level agency --yes` is for agents and scripts. The `--level` values are English identifiers (`solo`, `team`, `agency`, `public-sector`) whatever the interface language; only their help text is translated.

## 3. Settings-as-code

- **File:** `.github/ship.json`. It describes the desired state: repository settings, classification, rulesets, project, assignment.
- **Reconciliation:** `ship sync` reads the actual state through the API, computes a diff, shows it, then applies it. It is idempotent.
- **Inspirations:** Probot Settings (`.github/settings.yml`, applied on merge to the default branch, labels with `oldname` to rename) 🟡 ([github.com/apps/settings](https://github.com/apps/settings)); Terraform for the plan/apply cycle.
- **Permissions:** many settings require admin rights. A non-admin sees the diff and a message "to be applied by an admin" (behavior taken from the draft).
- ❓ Eventually, the same file could be applied by an Action. It is ruled out for now: the user did not choose the server-side Action.

## 4. Classification: Issue Types / Issue Fields, falling back to labels

| Fact | Status | Source |
|---|---|---|
| Issue fields **GA on July 2, 2026** for organisations (Free, Team, Enterprise); 4 default fields: Priority, Effort, Start date, Target date; single-select, text, number and date types; reachable through the GitHub MCP server | ✅ | [Changelog 2026-07-02](https://github.blog/changelog/2026-07-02-issue-fields-are-now-generally-available/) |
| Up to 25 issue fields per organisation | 🟡 (research, not reread on the GA page) | same |
| Issue types managed at the organisation level; default types **task, bug, feature**; **25 max** | ✅ | [Issue types docs](https://docs.github.com/en/issues/tracking-your-work-with-issues/using-issues/managing-issue-types-in-an-organization) |
| Availability on **personal repositories**: not mentioned, probably restricted to organisations | ❓ | - |
| REST/GraphQL API to set a type or a field at creation: not documented in the pages read | ❓ | - |
| Sub-issues: 100 per parent, 8 levels | 🟡 | [Sub-issues docs](https://docs.github.com/en/issues/tracking-your-work-with-issues/using-issues/adding-sub-issues) |
| `gh` manages types, sub-issues and dependencies since v2.94.0 (June 10, 2026), which implies the API allows it | 🟡 | [Changelog 2026-06-10](https://github.blog/changelog/2026-06-10-manage-sub-issues-types-and-dependencies-from-github-cli/) |
| Hierarchy view in Projects (Jan. 2026) | 🟡 | [Changelog 2026-01-15](https://github.blog/changelog/2026-01-15-hierarchy-view-now-available-in-github-projects/) |

**Decision:** organisation → Issue Types + Issue Fields; personal repository → **a minimal set of labels** following the observed practices (🟡 [sensible-github-labels](https://github.com/Relequestual/sensible-github-labels), [IPFS](https://github.com/ipfs/community/blob/master/ISSUE_LABELS.md)): prefixes, a single value per category, few labels to start with, colors per group.

Proposed fallback (no emoji in labels, since no convention was found; to be confirmed):

| Category | Labels |
|---|---|
| type | `type:bug`, `type:feature`, `type:task` (modeled on GitHub's default types) |
| priority | `priority:p0` … `priority:p3` (modeled on the Priority field) |
| status | `status:blocked`, `status:needs-info` |

PRs **do not need type labels**: the `✨ feat:` title already carries the information. ❓ Issue Types only apply to issues; to verify.

## 5. Governance

| Item | Facts | Status | Source |
|---|---|---|---|
| **Rulesets** | Required checks (strict or not), commit metadata restrictions (pattern or regex, **Enterprise plans**), *push rulesets* (extension, size, path) for private and internal repositories, **evaluate** mode to test before enforcing, organisation-wide enforcement on Enterprise Cloud | ✅ docs | [Available rules](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/available-rules-for-rulesets), [REST rules](https://docs.github.com/en/rest/repos/rules) |
| Team review in rulesets | Required approval from specific teams on paths; complements CODEOWNERS without replacing it (Nov. 2025) | ✅ | [Changelog 2025-11-03](https://github.blog/changelog/2025-11-03-required-review-by-specific-teams-now-available-in-rulesets/) |
| **CODEOWNERS** | Must be on the PR's **base branch**; the last matching pattern wins | ✅ | [About code owners](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners) |
| **Teams: auto-assignment** | The team is removed and replaced by N members. *Round robin*: whoever received a request least recently. *Load balance*: recent volume and reviews in progress, balanced over 30 days | ✅ | [Team code review settings](https://docs.github.com/en/organizations/organizing-members-into-teams/managing-code-review-settings-for-your-team) |
| Assignment based on the author | Nothing native found: goes through a workflow or a bot | 🟡 | same |

**ship-it proposes:**
- a baseline "PR required + required checks" ruleset in **evaluate** mode first, then switched to active mode on confirmation;
- a CODEOWNERS file **suggested from the git history** (who touches what), always reviewed by a human before it is written;
- the author assigned to their PR at creation (through the API, during `deliver`);
- `reviewers: "team:<slug>"`, which delegates to GitHub's native algorithms.

## 6. Planning: ROADMAP.md → GitHub

Source of truth: **the markdown** (user choice). The draft already read the `## Phase N: …` phases of `ROADMAP.md`.

Format (frozen 2026-11-01, implemented in [`core/roadmap`](../../internal/core/roadmap/roadmap.go)):

```markdown
## Phase 1: Foundations          ← milestone (title = the heading)
**Due:** 2026-12-15               ← milestone due date (ISO 8601), optional
- [ ] feat: CLI skeleton #12      ← issue; a commit-type prefix gives its kind/ label (if it exists)
  Text indented under an item     ← the issue's description
  - [ ] git exec layer #13        ← native sub-issue
- [x] ADRs #14                    ← a closed issue
**Exit:** a junior ships a PR.    ← milestone description
```

A `##` heading without a checklist is not a milestone; checklists inside code fences are ignored.

`ship roadmap sync [--yes] [--file ROADMAP.md] [--json]` shows the plan, applies it after confirmation, and writes the `#n` back into the file: a change in the working tree, delivered with `ship` like any other (branch, pull request, rulesets respected). Rules:

- **Content** (titles, descriptions, milestones) flows from the file for the issues ship-it manages (an invisible `<!-- ship-it:roadmap -->` marker in their body). An issue written by hand and linked by `#n` only gets its milestone.
- **States** follow who changed them since the last sync, remembered locally in `.git/ship/roadmap.json`: an issue closed (or reopened) on GitHub ticks (or unticks) the box; a box ticked (or unticked) in the file closes (or reopens) the issue. Without that memory (first sync, another clone), GitHub wins and nothing is reopened.
- **Nothing is deleted**: an issue the file no longer names is closed as *not planned*, with a comment.
- An interrupted sync resumes without duplicates: a managed issue of the same title in the same milestone is the item's.
- **`ship undo`** compensates a sync, after explicit consent (it changes GitHub): created issues closed as not planned, created milestones deleted only when they hold nothing else, states and sub-issue links put back, the file restored.

### GitHub Project (implemented 2026-11-02)

With `github.project` set ([configuration](../reference/configuration.md#github-project)), the sync also keeps a GitHub Project (v2) of an organisation or a user up to date, over the REST API (`/orgs|users/{owner}/projectsV2/{number}`, version 2026-03-10, checked on 2026-11-01):

- **Every roadmap issue is in the project**, the existing ones included: GitHub's auto-add (1 workflow on Free, 5 on Pro/Team, 20 on Enterprise) does not pick up existing items ([adding items automatically](https://docs.github.com/en/issues/planning-and-tracking-with-projects/automating-your-project/adding-items-automatically)).
- **Status**: a closed issue gets `done`; an open issue gets `todo` only when it has no status yet; a status set by hand on an open issue is never overwritten.
- **Date field** (`dateField`, off by default): the milestone's due date, which the Roadmap view lays out. **Phase field** (`phaseField`, off by default): a single-select option named after the milestone.
- **Nothing is invented**: a field, an option or a type the project lacks is reported under the plan (`SHIP-W0714`) and left alone. The REST API has no endpoint to add an option to an existing single-select field (options are given only when a field is created), and projects cannot be created through REST: create them, and the phase options, in GitHub.
- Field and option names match without regard to case (GitHub's default status options are "Todo", "In Progress", "Done").
- **Permissions**: a classic token (`gh`) needs the `project` scope (`read:project` only reads): `gh auth refresh -s project`. A fine-grained token needs the organisation's "Projects" permission; the permission table lists no user-project permission. Without the scope GitHub may answer 404 rather than 403, so ship-it reads the token's scopes (`X-OAuth-Scopes`) to tell "missing scope" (`SHIP-E0712`, with the remedy) from "no such project" (`SHIP-E0711`).
- **`ship undo`** removes the items the sync added and puts back the field values it changed, with the same consent as the rest of the sync.

**Linked issue → In progress (`ship publish`):** when `ship publish` creates a pull request that closes an issue (`Closes`, `Fixes` or `Resolves`, not `Refs`) and that issue is in the project, its status becomes `inProgress` and, when nobody is assigned, it is assigned to the pull request's author (`POST /repos/{owner}/{repo}/issues/{n}/assignees`; GitHub silently ignores a login without push access). The pull request exists already: any problem here is a warning, like labels. The merge closes the issue and the built-in workflow moves it to "Done".

**Undoing that publication (decision of 2026-11-02):** the Publish operation records what it changed on the linked issue (the project, the item, the status field, the option before, `""` for no status, the one it set, and the login it assigned), and `ship undo` plans two more remote steps after closing the pull request and deleting its branch, under the same consent: `unassign_issue` (`DELETE /repos/{owner}/{repo}/issues/{n}/assignees`) and `restore_project_field` (the step roadmap sync already uses, here with the value publish set as expected value). Both are compare-and-swap: the assignee is removed only if still assigned (and only the login ship added: an issue that had assignees got none, so has nothing to remove); the status goes back only if it still holds the value publish set, otherwise the human's change is kept and the undo says so (`SHIP-W0411`). Like in publish, every problem on this side is a warning. `ship undo --dry-run` reads the current status and assignees and lists the steps without writing. Publications logged before this was recorded are undone as before, without touching the project.

Not done yet: creating the project's Roadmap view (`POST …/views` exists), iteration fields.

## 7. Actions & Docker (chosen scope)

**For ship-it itself only:**

| Topic | Observed good practices | Status | Source |
|---|---|---|---|
| Pinning | Actions pinned by **full SHA** (tj-actions incident: moved tags) + a version comment; Dependabot updates the SHAs | 🟡 | [devtoolsdaily](https://www.devtoolsdaily.com/blog/github-actions-permissions-sha-pinning/), [safeguard.sh](https://safeguard.sh/resources/blog/github-actions-supply-chain-security) |
| Permissions | Read-only by default, widened per job | 🟡 | same |
| Audit | zizmor, OpenSSF Scorecard | 🟡 | same |
| Provenance | `actions/attest-build-provenance` is a wrapper around **`actions/attest`** since v4 (prefer the latter); permissions `id-token: write` + `attestations: write`; public Sigstore for public repositories; verification with `gh attestation` | ✅ README | [actions/attest-build-provenance](https://github.com/actions/attest-build-provenance) |
| SLSA | Provenance = level 1; attestations on hosted runners = level 2; level 3 = isolated signing through a reusable workflow | 🟡 GitHub blog | [github.blog SLSA 3](https://github.blog/enterprise-software/devsecops/enhance-build-security-and-reach-slsa-level-3-with-github-artifact-attestations/) |
| Docker image | **Free Docker Hardened Images** (Apache-2.0) since Dec. 2025, distroless, non-root (UID 65532), signed SBOM, SLSA L3 provenance; pulled from `dhi.io` after login | 🟡 Docker blog | [docker.com](https://www.docker.com/blog/docker-hardened-images-for-every-developer/) |

ship-it's image: a static Go binary + git. Candidate base: a DHI that includes git, or a distroless image with git (❓ to assess). Published on `ghcr.io/<owner>/ship-it`, signed and attested.

## 8. AI agents on GitHub (context)

🟡 Copilot coding agent: you assign it an issue, it opens a PR and requests a review. It does not see comments posted after the assignment. **Agents** tab (Jan. 26, 2026). **Automations** that are scheduled or triggered by an event. Claude and Codex can be assigned (preview in Feb. 2026) ([Agents tab](https://github.blog/changelog/2026-01-26-introducing-the-agents-tab-in-your-repository/), [cloud agent](https://docs.github.com/en/copilot/how-tos/use-copilot-agents/cloud-agent/use-cloud-agent-on-github), [Claude & Codex](https://github.blog/changelog/2026-02-04-claude-and-codex-are-now-available-in-public-preview-on-github/)).
**Consequence:** PRs will be opened by server-side agents, outside ship-it. `ship status` must show them, and auto-assignment or the Project must treat them like any other.
