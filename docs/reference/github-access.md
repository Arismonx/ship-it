# GitHub access

Which token ship uses, every GitHub API request it makes, and the permission each one needs. Use it to create a token with no more access than ship needs, or to understand a refusal.

## Which token ship uses

ship reads a token you already have; there is no sign-in of its own yet (`ship login` is [planned](cli.md#planned)). It takes the first one it finds, once per host and per run:

1. `SHIP_TOKEN`.
2. The variables of the GitHub CLI: `GH_TOKEN`, then `GITHUB_TOKEN` for github.com and `*.ghe.com`; `GH_ENTERPRISE_TOKEN`, then `GITHUB_ENTERPRISE_TOKEN` for GitHub Enterprise Server.
3. `gh auth token --hostname <host>`, when the [GitHub CLI](https://cli.github.com/) is installed and signed in.
4. Your git credential helpers (`git credential fill`), for example Git Credential Manager or the macOS keychain. ship never lets them prompt you.

`ship doctor` says which source it used (`token from gh`, for example) and whether GitHub accepts the token. When GitHub refuses a token during a long run, such as `ship mcp`, ship looks it up again once, in case it was refreshed.

ship sends the token only to the GitHub API of the repository's host, in the `Authorization` header. It never writes it to a file, a log or the run journal ([logs](logs.md)).

### Pushes go through git

ship pushes, fetches and deletes remote branches with git, so these use git's own credentials (SSH key or credential helper), not the token above. When git uses a token over HTTPS, that token needs write access to the code: the `repo` scope (or `public_repo`) on a classic token, Contents: write on a fine-grained one. Pushing a change to a file under `.github/workflows/` also needs the `workflow` scope or the Workflows: write permission.

## The permissions, in short

| What you use | Classic token scopes | Fine-grained token permissions |
|---|---|---|
| `ship`, `ship publish`, `ship stack`, `ship undo`, `ship status` | `repo` (`public_repo` is enough for public repositories) | Metadata: read, Contents: read, Pull requests: write, Issues: write, Commit statuses: read |
| `ship stack merge` | `repo` | Also Contents: write |
| `ship sync`, `ship label` | `repo` | Issues: write, Pull requests: write |
| `ship roadmap sync` | `repo` | Issues: write |
| A GitHub Project (`github.project.url`) | Also `project` (`read:project` to read only) | Also the organisation permission Projects: read and write |

The GitHub CLI's token has the `repo` scope; when it lacks `project`, add it with `gh auth refresh -s project`.

Fine-grained tokens have limits GitHub documents in [managing your personal access tokens](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens). Three of them matter for ship:

- They cannot call the Checks API, so `ship status` and `ship stack status` show no checks with one (commit statuses still show).
- They cannot reach a project owned by a user account, only one owned by an organisation.
- They cannot contribute to a public repository where you are not a member, so a pull request from your fork to someone else's repository needs a classic token or the GitHub CLI's token.

## Every request

ship uses the REST API only, version `2026-03-10`. The paths below start with `/repos/{owner}/{repo}` unless they start with `/user`, `/orgs` or `/users`. "Classic" is the scope a classic token needs on a private repository: on a public one, reads need no scope and writes need `public_repo`, per [scopes for OAuth apps](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/scopes-for-oauth-apps). "Fine-grained" is the permission listed in [permissions required for fine-grained personal access tokens](https://docs.github.com/en/rest/authentication/permissions-required-for-fine-grained-personal-access-tokens), checked on 2026-11-06. Where that page lists an endpoint under both Issues and Pull requests, the table says "Issues or Pull requests"; grant both to be safe.

### Repository and user

| Request | When | Classic | Fine-grained |
|---|---|---|---|
| `GET /repos/{owner}/{repo}` | Every command that reaches GitHub: the default branch, and the parent of a fork. `ship doctor` makes only this request. | `repo` | Metadata: read |
| `GET /user` | `ship fix` (your noreply address), `ship publish` when it assigns you or requests reviews (to leave you out). | none | none (the endpoint is available to fine-grained tokens and no permission is listed) |
| `GET …/rules/branches/{branch}` | Before a push: the rulesets that apply to the branch and its commits. | `repo` | Metadata: read |
| `GET …/contents/{path}?ref=` | When a setting preset (`extends`) names a file of a GitHub repository. | `repo` | Contents: read |

### Pull requests

| Request | When | Classic | Fine-grained |
|---|---|---|---|
| `GET …/pulls?head=…` | `ship`, `ship publish`, `ship status`, `ship tidy`, `ship undo`, `ship stack`: the pull request of a branch. | `repo` | Pull requests: read |
| `GET …/pulls?state=closed` | `ship tidy`, `ship fix`: the merged pull requests, to find merged branches. | `repo` | Pull requests: read |
| `POST …/pulls` | `ship`, `ship publish`, `ship stack`: open a pull request. | `repo` | Pull requests: write |
| `PATCH …/pulls/{n}` | `ship undo` (close it), `ship stack sync` and its undo (change the base, the "Part n of N" line of a chain). | `repo` | Pull requests: write |
| `GET …/pulls/{n}` | `ship stack`, `ship stack sync`, `ship undo`, `ship label`. | `repo` | Pull requests: read (the page also lists Contents: read, with additional permissions) |
| `GET …/pulls/{n}/commits`, `GET …/pulls/{n}/files` | `ship label`: what the pull request holds. | `repo` | Pull requests: read |
| `GET …/pulls/{n}/reviews` | `ship status`: the review decision. | `repo` | Pull requests: read |
| `POST …/pulls/{n}/requested_reviewers` | `ship publish`, when `ship.pullRequest.reviewers` or `teamReviewers` is set. | `repo` | Pull requests: write |
| `GET …/commits/{ref}/check-runs` | `ship status`, `ship stack status`: the checks. | `repo` | Not available: fine-grained tokens cannot call the Checks API (see above). |
| `GET …/commits/{ref}/status` | `ship status`, `ship stack status`: the commit statuses. | `repo` (or `repo:status`) | Commit statuses: read |

### Stacks and merges

| Request | When | Classic | Fine-grained |
|---|---|---|---|
| `GET …/stacks` | `ship stack`, `ship stack status`, `ship stack sync`. A 404 means the server has no stacks: ship falls back to a chain. | `repo` | Pull requests: read |
| `POST …/stacks` | `ship stack`, `ship stack publish`: group the pull requests. | `repo` | Pull requests: write |
| `POST …/stacks/{n}/add` | `ship` on the top layer: add the new layer. | `repo` | Pull requests: write |
| `POST …/stacks/{n}/unstack` | `ship undo` of a stack. | `repo` | Pull requests: write |
| `PUT …/pulls/{n}/merge-async` | `ship stack merge`, with `bypass_rules` always `false`. | `repo` | Contents: write |
| `GET …/pulls/{n}/merge-async/{uuid}` | `ship stack merge`, while it waits for the result. | `repo` | Contents: write (as listed, although it is a read) |

The [stacks](https://docs.github.com/en/rest/pulls/stacks) and [merge](https://docs.github.com/en/rest/pulls/pulls) pages give no classic scope for these endpoints; `repo` is inferred from the scope rules above, not read on those pages.

### Labels, issues and milestones

| Request | When | Classic | Fine-grained |
|---|---|---|---|
| `GET …/labels` | `ship publish`, `ship label`, `ship sync`, `ship roadmap sync`: the labels the repository has. | `repo` | Issues or Pull requests: read |
| `POST …/labels`, `PATCH …/labels/{name}` | `ship sync`: create or update a label. | `repo` | Issues or Pull requests: write |
| `POST …/issues/{n}/labels`, `DELETE …/issues/{n}/labels/{name}` | `ship publish`, `ship label`: the `kind/` and `size/` labels of a pull request. | `repo` | Issues or Pull requests: write |
| `GET …/issues/{n}` | `ship publish` (the issue's title and milestone), `ship undo`. | `repo` | Issues: read |
| `GET …/issues?state=all` | `ship roadmap sync`: the issues the roadmap tracks. | `repo` | Issues: read |
| `POST …/issues` | `ship roadmap sync`: create an issue. | `repo` | Issues: write |
| `PATCH …/issues/{n}` | `ship roadmap sync`, `ship publish` (give the pull request the issue's milestone), `ship undo` (close or reopen, clear the milestone). | `repo` | Issues or Pull requests: write |
| `POST …/issues/{n}/comments` | `ship undo`: the comment that says why a pull request or a created issue is closed. | `repo` | Issues or Pull requests: write |
| `POST`, `DELETE …/issues/{n}/assignees` | `ship publish` (assign you, or the issue's author), `ship undo` (remove that assignee). | `repo` | Issues or Pull requests: write |
| `GET`, `POST …/issues/{n}/sub_issues`, `DELETE …/issues/{n}/sub_issue` | `ship roadmap sync` and its undo: the sub-issue links. | `repo` | Issues: read, write |
| `GET`, `POST …/milestones`, `PATCH`, `DELETE …/milestones/{n}` | `ship roadmap sync` and its undo. A milestone is deleted only when ship created it and it holds nothing else. | `repo` | Issues or Pull requests: read, write |

### GitHub Projects

These requests use `/orgs/{org}/projectsV2/{number}` or `/users/{username}/projectsV2/{number}`. They are made by `ship publish` (move the linked issue to its status in the project of `github.project.url`) and its undo, and by `ship roadmap sync`.

| Request | Classic | Fine-grained |
|---|---|---|
| `GET …/projectsV2/{number}`, `GET …/fields`, `GET …/items` | `read:project` or `project` | Organisation permission Projects: read |
| `POST …/items`, `PATCH …/items/{id}`, `DELETE …/items/{id}` | `project` | Organisation permission Projects: write |

The scopes come from [scopes for OAuth apps](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/scopes-for-oauth-apps). A project owned by a user account cannot be reached with a fine-grained token.

## What ship never does

ship needs no administration permission, and asks for none. It never:

- creates, renames or deletes a repository, or changes its settings;
- changes branch protections or rulesets: it only reads them;
- merges a pull request, except with `ship stack merge` after you confirm, and never bypassing the rules;
- deletes a label, an issue or a comment;
- touches GitHub Actions, secrets, webhooks, deploy keys or organisation membership.

The only deletions it makes undo its own work, or tidy after a merge: a remote branch it pushed (when you undo a publication, or after a layer merged), a label it set, an assignee it added, a project item it added, a sub-issue link it made, and an empty milestone it created.

## Not verified

The fine-grained permission of `GET /user` is not stated on GitHub's permissions page: the endpoint is listed as available to fine-grained tokens with no permission. The rendered endpoint pages of the [stacks](https://docs.github.com/en/rest/pulls/stacks), [pull requests](https://docs.github.com/en/rest/pulls/pulls) and [projects](https://docs.github.com/en/rest/projects/items) APIs did not show their token sections when checked, so the fine-grained permissions above come from the permissions page alone, and the classic scopes for stacks and merges are inferred. The permissions git needs to push are GitHub's general rules for code access, not read on an endpoint page.
