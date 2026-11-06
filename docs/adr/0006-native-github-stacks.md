# ADR-0006: Native GitHub stacks through REST

**Status:** accepted

## Context
Stacked PRs **GA on Oct. 6, 2026** on github.com, not yet on GHES. REST API: `GET/POST /repos/{o}/{r}/stacks`, `…/{n}`, `…/{n}/add`, `…/{n}/unstack`. GraphQL is read-only. Server-side cascading rebase, merged from the bottom up, merge queue as a single group.

## Options
| | Pros | Cons |
|---|---|---|
| **Native stacks REST API** | No dependency, native GitHub UI for reviewers, server-side rebase | Recent API (possible bugs, schema to learn), no GHES |
| In-house engine (chained bases) | Works everywhere | Reinvents what GitHub now does better |
| `gh stack` | The simplest | Brings `gh` back |

## Decision
Native REST. **GHES fallback**: a classic chain of PRs (each PR targets the branch of the previous one) with a local `--update-refs` rebase (❓ to validate).

## Consequences
- Locally: create N stacked branches, push, then `POST /stacks` with the PR numbers from the bottom up.
- `ship stack sync`: after a merge, fetches, realigns the local branches and cleans up.
- Verified at the time: no REST endpoint for merging a stack is documented. **ship-it does not merge**: merging stays a human gesture in GitHub (bottom up), and ship-it cleans up afterwards (`ship stack sync`).

## Revision of 2026-11-02 (design of `ship stack`)

Facts checked on 2026-11-02:

- REST, API version 2026-03-10 ([stacks endpoints](https://docs.github.com/en/rest/pulls/stacks)): `GET|POST /repos/{owner}/{repo}/stacks` (`pull_requests`: numbers from bottom to top; "each PR's base ref must match the previous PR's head ref"), `GET …/stacks/{stack_number}`, `POST …/{stack_number}/add` (409 when another request modifies the stack), `POST …/{stack_number}/unstack` (204 when the stack is dissolved). Pull request resources carry a `stack` object; GraphQL is read-only ([APIs and webhooks](https://docs.github.com/en/pull-requests/reference/stacked-pull-requests-apis-and-webhooks)).
- A stack is two or more pull requests of one repository, no forks; merge requirements come from the bottom pull request's base; merges go bottom up, and merging one merges those below it; the pull requests above are retargeted and rebased by GitHub ([about stacked pull requests](https://docs.github.com/en/pull-requests/get-started/about-stacked-prs)).
- Merging a stack through the API needs the asynchronous merge API, generally available since 2026-10-01 ([changelog](https://github.blog/changelog/2026-10-01-github-async-merge-api-generally-available)); it is all or nothing and rules are evaluated when it runs.
- A third-party article says GitHub's server-side rebase produces unsigned commits ([cubic.dev](https://www.cubic.dev/blog/stacked-pull-requests)) 🟡 not confirmed by GitHub's documentation yet. Contradicted by GitHub on 2026-10-06, see the revision of 2026-11-06.
- No statement about GitHub Enterprise Server was found.

Decisions (user, 2026-11-02):

1. **Creating**: both ways. `ship stack` splits the work into layers (by area, as the split suggestion does, adjustable on the review screen); `ship` run on a branch of a stack adds a layer on top.
2. **Merging**: `ship stack merge`, explicit, with the plan shown and a confirmation, never for an AI agent, through the asynchronous merge API; GitHub's rules always apply, no bypass. This replaces "ship-it does not merge" above for stacks.
3. **Rebasing the layers above a merged one**: configurable, `ship.stack.rebase` = `local` (default: `ship stack sync` rebases locally, so signed commits stay signed, then pushes with a lease; checkpoint first, undoable) or `github`.
4. **GitHub Enterprise Server**: the classic chain (each pull request targets the branch of the previous one), with the same commands.
5. **Titles**: nothing added; GitHub shows the stack. In a classic chain, one line in the body: "Part 2 of 3, after #41".
6. **Undo of a published stack**: the whole stack, with consent: the unmerged pull requests are closed with a comment, the stack dissolved (unstack), the remote branches deleted.
7. **Drafts**: as for a single pull request (`ship.pullRequest`).
8. **Layers**: `ship.stack.maxLayers`, 5 by default; smaller parts are grouped beyond it.

Implemented on 2026-11-05 ([how-to](../how-to/stack-pull-requests.md)). Read that day:

- The asynchronous merge API ([merge a pull request asynchronously](https://docs.github.com/en/rest/pulls/pulls#merge-a-pull-request-asynchronously)): `PUT /repos/{owner}/{repo}/pulls/{pull_number}/merge-async` with `sha`, `merge_action` (`default`, `direct_merge`, `merge_queue`) and `bypass_rules` (ship-it sends `default` and `false`), answering 200 (merged or queued already) or 202 (accepted); `GET …/merge-async/{uuid}` polls it, kept 24 hours. Both answer a `status` (`pending`, `merged`, `enqueued`, `failed`) and `details`. The page gives the schema, not a literal example.
- Merging a stacked pull request merges "every pull request in the stack up to and including the one you request", atomically ([APIs and webhooks](https://docs.github.com/en/pull-requests/reference/stacked-pull-requests-apis-and-webhooks)): ship-it asks on the top one.
- The pull request's `stack` object holds `id`, `number`, `size`, `position` and `base`. `unstack` keeps the pull requests it cannot remove (queued) and answers 200 then, 204 once the stack is dissolved.
- Not verified against the real API: a 404 on `/stacks` as the sign of a server without stacks (ship-it reads it so, the repository having been read just before), and what GitHub does to a stack when one of its pull requests is closed.

## Revision of 2026-11-06 (default rebase, positioning next to `gh stack`)

Facts read on 2026-11-06 in GitHub's [general availability changelog](https://github.blog/changelog/2026-10-06-stacked-pull-requests-generally-available):

- "GitHub creates signed replacement commits during Rebase stack, preserving original authorship."
- "Automatic rebases after partial merges also sign replacement commits when branch rules require signatures."
- "Rebase stack now preserves approvals, even in repositories that dismiss stale approvals."
- Stacked pull requests "are available on all github.com plans" and "will be included in an upcoming GitHub Enterprise Server release".

The reason for a local default is gone: GitHub's rebase creates signed commits that keep their author, and keeps approvals. They are signed by GitHub, not with the developer's key. Decisions (user, 2026-11-06):

1. The default of `ship.stack.rebase` becomes `github`. `local` stays for a team that wants every commit signed with the developer's own key, and a classic chain on GitHub Enterprise Server is still rebased locally.
2. `ship stack` is kept and positioned next to GitHub's official [`gh stack`](https://github.com/github/gh-stack) extension, not against it. `gh stack` manages a stack you build branch by branch (`gh stack add` commits on a new branch at `HEAD`, `gh stack modify` reorders, folds or drops branches, `gh stack sync` rebases and pushes); it does not split work in progress. `ship stack` starts from uncommitted work: it proposes layers by folder as a starting point, which the review screen lets you reorder and move files between, and then applies to every layer what `ship` applies to one pull request (conventions, project checks, secret scan, rulesets, drafts for agents). One `ship undo` removes the whole stack. Someone who already cuts branches by hand, or reshapes long-lived stacks, is better served by `gh stack`; the docs say so.
