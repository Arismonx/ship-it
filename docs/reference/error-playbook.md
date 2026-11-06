# Error playbook

Common git and GitHub situations, how ship detects each one, and what it does about it. An error should never leave you stuck: each situation has a detector (before git fails, when possible), a message in plain words, and a remedy.

Remedy levels:

- 🟢 automatic: safe and reversible; ship does it and records it in the operation log.
- 🟡 guided: ship proposes a precise action, and you say yes or no.
- 🔴 manual: ship explains what to do, and points to the right file or page.

A remedy marked *planned* does not exist yet; the others do. This page describes situations, not exact messages: every message and its code is in the [error reference](errors.md).

## Environment and access

| Situation | Detection | Remedy | Level |
|---|---|---|---|
| `git` missing or too old | `git --version` (`ship doctor`) | The install command for your system (winget, brew, apt). | 🔴 |
| No GitHub token | no source gives a token ([GitHub access](github-access.md)) | `gh auth login`, or set `GH_TOKEN`. `ship login` with the device flow is *planned*. | 🔴 |
| Token expired or revoked | 401 from the API | Renew the token. Signing in again with the device flow is *planned*. | 🔴 |
| Organisation with SAML SSO, token not authorised | 403 from the API | Authorise the token for the organisation. Showing the authorisation link is *planned*. | 🔴 |
| Missing git identity (`user.name`, `user.email`) | `git config` | `ship fix` sets the name and the noreply address of your GitHub account (`ID+LOGIN@users.noreply.github.com`, see GitHub Docs, "Email addresses reference") in your global git configuration. Undoable. | 🟡 |
| git asks for a password over HTTPS | `ls-remote` fails | A ship credential helper using the keychain token is *planned*. | 🟡 |
| Company proxy, private certificate authority | TLS or connection error | `ship doctor` showing the proxy and testing the system store is *planned*. | 🔴 |
| Windows: long paths | `core.longpaths` | `ship fix` sets `core.longpaths true` in the repository. Undoable. | 🟡 |
| Mirror remote (`git push --mirror` would send local data) | `remote.*.mirror` | `ship fix` removes the setting from the repository. Undoable. | 🟡 |
| Windows: line endings | no `.gitattributes` | Proposing a `.gitattributes` (`* text=auto`) is *planned*. | 🟡 |

## Repository

| Situation | Detection | Remedy | Level |
|---|---|---|---|
| Not a git repository | `rev-parse` | Running `git init` and writing a `.gitignore` for the ecosystem is *planned*; today `ship init` writes `.github/ship.json` only. | 🟡 |
| No remote, or the repository does not exist on GitHub | `remote -v` | ship commits locally. Creating the repository (private by default) and adding `origin` is *planned*. | 🟡 |
| Detached HEAD | `branch --show-current` is empty | `ship` creates a branch from HEAD. | 🟢 |
| Merge, rebase or cherry-pick in progress | files in `.git/` | Choosing to continue (when no conflict is left) or abort is *planned*. | 🟡 |
| Unresolved conflicts | `diff --diff-filter=U` | Listing the files and opening them in the editor is *planned*. | 🔴 |
| `node_modules`, `.env`, `dist` tracked or about to be | path scan | `ship fix` handles `node_modules/` and `.env*` (except templates such as `.env.example`): adds them to `.gitignore` and removes them from the index, keeping the files on disk. Undo puts `.gitignore` back byte for byte and indexes the files again. A `.env` already committed stays in the history: revoke its secrets. `dist/` is left alone, since some projects commit it. | 🟡 |
| File over 50 MiB (GitHub warns) or 100 MiB (GitHub blocks) | size before the commit | Excluding it, ignoring it or moving it to Git LFS is *planned*. | 🟡 |
| A secret in the changes | scan of the added lines | `SHIP-E0901`, exit code 3, before the review. ship lists `file:line` with the value masked and a fingerprint. Move the value to `.env.local` and ignore that file; for a false positive, add its fingerprint to `ship.secrets.allow` or its file to `ship.secrets.ignorePaths`. `ship.secrets.rules` and `ship.secrets.disabledRules` add or turn off rules. | 🟢 |
| A secret already in a local commit not pushed yet | scan of the unpushed commits | `SHIP-E0901` before the push, with the base to clean the history from. Rewriting those commits for you (after a checkpoint) is *planned*. | 🟡 |
| GitHub push protection refuses a secret | refusal at push time | Same remedy, then rotate the secret. | 🟡 |

## Branches and syncing

| Situation | Detection | Remedy | Level |
|---|---|---|---|
| Work done directly on `main` | the current branch is the base | `ship` creates a dedicated branch and puts the local `main` back. | 🟢 |
| Branch behind its base | `rev-list --left-right --count origin/base...HEAD`, after a fetch | When publishing: checkpoint, `rebase --autostash`, then a push under a lease if the branch was already pushed (`--force-with-lease=<ref>:<old tip>`). No rebase when someone else pushed commits you do not have (`SHIP-W0611`). | 🟢 |
| Rebase conflict | the rebase fails | The rebase is aborted and nothing is lost. The branch is published as it is, so the pull request shows the conflict, with a warning that explains how to resolve it (`SHIP-W0610`). | 🟢 |
| Conflict on a lockfile only | the conflicting paths are lockfiles | Taking the base's version, running the install again and committing is *planned*. | 🟢 |
| Push refused: not a fast-forward | refusal at push time | Fetch, rebase, then `--force-with-lease`, only on a ship branch that belongs to you. | 🟢 |
| Push refused: protected branch | refusal at push time | Moving to a branch and a pull request. | 🟢 |
| Push refused: signature required | refusal by a rule | A guided SSH signing setup (`gpg.format ssh`) is *planned*. | 🟡 |
| The pull request of the current branch is already merged | the branch's latest pull request (`state=all`), `merged_at` | `ship tidy`, also run at the start of `ship`: checkpoint, work set aside, then a check that changes nothing (`git merge-tree --merge-base`, git 2.40 or later) that it applies cleanly on `origin/<base>`. If it does, ship goes back to the updated base, applies the work again, and deletes the branch when it holds nothing more than the pull request. Otherwise nothing moves, and the message says what to do. | 🟢 |
| Merged local branches left behind | recent merged pull requests whose head is the branch tip or a descendant of it | Deleted with a compare-and-swap, announced, and undoable (`ship undo` creates them again). A branch with commits added after the merge is kept. | 🟢 |
| Stack: the bottom pull request merged | state of the stack | GitHub rebases and retargets on its side; `ship stack sync` then cleans up locally. | 🟢 |

## Gates and hooks

| Situation | Detection | Remedy | Level |
|---|---|---|---|
| Lint or format red | exit code | Running the fix scripts (`lint:fix`, `format`) and the gate again is *planned*. | 🟢 |
| Tests red | exit code | The end of the output is shown and the commit stays local; run `ship publish` once it is fixed. | 🔴 |
| A `pre-commit` hook fails | the commit fails | The hook's output is shown. ship never adds `--no-verify`. | 🔴 |
| Typecheck red | exit code | The output is shown. Grouping errors by file and opening them in VS Code is *planned*. | 🔴 |
| The project's commitlint refuses the message | `npx --no -- commitlint` on the exact message (on stdin), right before committing | `SHIP-E0905`, exit code 3, nothing changed (no checkpoint, no branch, no commit), with commitlint's own output. Change the title, then ship again. | 🔴 |
| No title fits the project's commitlint | its resolved rules (`--print-config json`): a type it does not allow with no single equivalent, a scope it requires, a subject or header over its limit | `SHIP-E0906`, exit code 3, before anything changes; the message names the rule and the allowed values. ship never cuts a subject or picks between two types: you pick the type at review. With `--yes` or an agent, the refusal stands. | 🔴 |

## On GitHub

| Situation | Detection | Remedy | Level |
|---|---|---|---|
| A label does not exist | the labels of the repository | ship only sets labels the repository has; `ship sync` creates them. | 🟢 |
| A reviewer cannot be requested | 422 | A warning; the pull request is created anyway. | 🟢 |
| Rate limit | 403 or 429, and the rate limit headers | ship reports it. Waiting for `Retry-After` or `X-RateLimit-Reset` and trying again is *planned*. | 🔴 |
| CI red after the push | `ship status` | `ship status` names the failing checks. ship does not notify you. | 🔴 |
| Changes requested in review | `ship status` | `ship status` says so; `ship` on the branch updates the pull request. | 🔴 |
| A ruleset refuses the branch name, the branch was created by ship and never pushed | `GET /repos/{owner}/{repo}/rules/branches/{branch}`, patterns evaluated as GitHub does | Renamed to the closest name that fits (checked against the rules), logged and announced (`SHIP-W0616`). `ship undo` gives the old name back. | 🟢 |
| A ruleset refuses the branch name, the branch was already pushed or is not ship's, or no name fits | same | `SHIP-E0612` before the push, quoting the rule's name and requirement, with the exact `git branch -m` command. | 🔴 |
| A commit breaks a ruleset's message or author or committer e-mail pattern | same, on `origin/<default>..HEAD` | `SHIP-E0613` or `SHIP-E0614` before the push, naming the commit. Fix it with `git commit --amend` or `git rebase -i`: ship never rewrites history. | 🔴 |
| A ruleset requires signed commits and one is not | the `gpgsig` header of the commits (not `%G?`, which says "N" for an SSH signature it cannot verify) | `SHIP-E0615` before the push, with the signing setup and `git rebase --exec "git commit --amend --no-edit -S"`. | 🟡 |
| The rules cannot be read (404 on GitHub Enterprise Server without rulesets or without access, or 403) | API status | ship goes on without rules and logs it at debug level; GitHub still enforces them at push time. | 🟢 |

## How `ship fix` uses this playbook

- `ship fix` runs every detector and proposes the remedies together: a list, one confirmation (Enter, or `--yes`), a checkpoint, and one `fix` entry in the operation log that keeps the previous state of each change. `ship undo` puts everything back in reverse order. Problems that need a human decision stay listed with their command.
- Every 🟢 or 🟡 remedy is preceded by a checkpoint and appears in `ship log`, so it can be undone.
- Every situation ship handles is tested by a scenario on a real temporary repository.
