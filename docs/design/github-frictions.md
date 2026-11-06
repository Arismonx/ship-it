# Developer frictions with git & GitHub

> Goal: **zero friction**. Each friction is **documented by a source**, then paired with a ship-it answer (already in the docs or new).
> 🟢 answer already designed · 🆕 new idea from this research · ❓ to dig into.

## 1. Undo and recover (friction no. 1 according to the data)

| Finding | Source |
|---|---|
| Most upvoted git question on Stack Overflow: "How do I undo the most recent local commits in Git?" (score 22,579 in a 2021 snapshot) | 🟡 [SO mirror](https://apify.com/reapx/stackoverflow-scraper/examples/so-top-questions-git.md) |
| Study: the most viewed questions are about `revert` and `reflog`, then `stash`; complex topics (submodules) are the least well answered | 🟡 [neverworkintheory](https://neverworkintheory.org/2022/09/19/developer-git-knowledge.html) |
| Classic mistakes: committing on the wrong branch, undoing an `add`, a pushed commit or a `reset --hard`, renaming a branch | 🟡 [GitLab "git happens"](https://about.gitlab.com/blog/git-happens), [dev.to](https://dev.to/marcelxv/10-most-common-git-errors-and-how-to-fix-them-1beg) |

**Answers:** 🟢 checkpoints + oplog + `ship undo` (ADR-0005). 🆕 `ship undo` in the **natural language of situations**: "I committed on the wrong branch", "I want to get back a file from yesterday", "undo my last ship". A menu of situations rather than of git commands.

## 2. Authentication

| Finding | Source |
|---|---|
| Passwords refused over HTTPS since **August 13, 2021**; a PAT or SSH is required | 🟡 [betterstack](https://betterstack.com/community/questions/how-to-fix-support-for-password-authentication-was-removed/) |
| `Permission denied (publickey)`: key missing, not loaded or not registered; HTTPS and SSH are often confused, hence the advice to check `git remote -v` first | 🟡 [techearl](https://techearl.com/git-password-authentication-removed), [mkyong](https://mkyong.com/git/github-keep-asking-for-username-password-when-git-push/) |
| Other causes: expired or revoked token, insufficient scopes, credential cache, **SSO authorization** | 🟡 same |

**Answers:** 🟢 `ship login` (device flow) + ship-it credential helper, hence **a single** authentication. 🆕 `ship doctor` detects HTTPS or SSH on the remote and offers to convert to the method that works. 🆕 Detection of missing scopes on the first 403 error, with a re-authorization link.

## 3. Code review: the wait

| Finding | Source |
|---|---|
| 2026 benchmark (8.1 M PRs): median review **15 h**, median cycle ~**83 h**; pickup beyond 16 h is considered problematic | 🟡 vendor ([LinearB via DEV](https://dev.to/tessainsley/your-pull-request-review-time-is-mostly-waiting-and-rework-2g6n), [LinearB](https://linearb.io/library/ai-in-software-development)) |
| AI-assisted PRs: **2.6× larger**, **4.6× more waiting** before the first review, 30-day acceptance of 32.7 % versus 84.4 % | 🟡 vendor, same |
| Survey (2,847 developers, Q1 2026): review overtakes writing as the main time sink among heavy AI users | 🟡 [Digital Applied](https://www.digitalapplied.com/blog/ai-coding-tool-adoption-2026-developer-survey) |
| 2 or 3 people often do 70 %+ of the reviews | 🟡 opinion ([Medium](https://medium.com/@chvishal7017/why-your-pull-request-reviews-are-still-broken-in-2026-and-what-actually-fixes-it-69c527a07518)) |
| Merge conflicts: the **size of the merge** matters more than the size of the conflict; small pieces speed up resolution | 🟡 study (Vale et al., 66 projects, ~81 k merges, 140 developers) [uni-saarland](https://www.se.cs.uni-saarland.de/publications/docs/VHF+22.pdf) |
| 43 % of conflicts involve refactorings, which take longer to resolve | 🟡 [SANER 2025](https://conf.researchr.org/details/saner-2025/saner-2025-papers/15/The-Hidden-Challenges-of-Merging-A-Tool-Based-Exploration) |

**Answers:** 🟢 automatic split + native stacks, that is **small PRs by construction**, the lever shared by all three findings. 🆕 **Size guardrail**: beyond a (configurable) threshold, `ship` offers to split before pushing, and always for PRs produced by an agent. 🆕 **Separate refactor**: moves and renames are detected and isolated in their own commit or PR, to reduce conflicts and ease review. 🆕 Reviewer-oriented PR summary: "where to start", risky files first. 🟢 Reviewers through Teams load balance. ❓ Gentle reminder if a PR has been waiting for more than X hours (a notification to the author, no spamming of reviewers).

## 4. Notifications

| Finding | Source |
|---|---|
| Bug reported in March 2026: notifications marked as read without being opened, "for years" | 🟡 [devactivity](https://devactivity.com/insights/github-s-phantom-reads-how-notification-bugs-impact-engineering-team-metrics) |
| Subscribed to a whole Discussion after a single reply; maintainers watching hundreds of repositories | 🟡 [Foreman](https://community.theforeman.org/t/how-to-not-get-overwhelmed-by-github/23768) |
| Notification delays during incidents (Oct. 2026) | 🟡 [Pulsetic](https://pulsetic.com/status/github/incidents/147512) |

**Answers:** 🟢 `ship watch` (native notifications limited to **my** PRs). 🆕 `ship inbox`: only what requires **my action** (review requested, changes requested on my PR, red CI on my PR, mention), computed from the API rather than from GitHub notifications, which makes it immune to the "phantom reads" bug. ❓ Bulk unsubscribe from inactive threads.

## 5. CI: slowness, flakiness, opacity

| Finding | Source |
|---|---|
| 3.2 % of builds rerun, **67.7 % of them flaky**; more than half of the 1,960 projects studied affected | 🟡 [arXiv 2602.02307](https://arxiv.org/abs/2602.02307v1) |
| No remote console on a runner: debugging through logs, a loop of several minutes | 🟡 [rwx](https://rwx.com/blog/what-would-github-actions-look-like-if-you-designed-it-today) |
| Testimony: 13.9 % failures, 31.5 h/month of investigation | 🟡 anecdote (vendor) |

**Answers:** 🟢 **local** gates before push, limited to the affected tests (inherited from the draft), hence fewer CI round trips. 🆕 On red CI: `ship status` shows **the relevant log excerpt** (failed step, last lines) without opening GitHub, and says whether the same job failed recently on the base (a sign of flakiness, ❓ heuristic to validate). 🆕 Rerun in one gesture (`ship ci rerun --failed`).

## 6. GitHub availability

| Finding | Source |
|---|---|
| Official 2026 reports: 6 incidents in February, 4 in March, 8 in July; Actions degraded on March 5 (95 % of runs started more than 5 min late); GitHub says it "fell short of its commitments" and is moving Actions to Azure | ✅ [Availability report July 2026](https://github.blog/news-insights/company-news/github-availability-report-july-2026/), [March 2026](https://github.blog/news-insights/company-news/github-availability-report-march-2026/) |
| Writes unavailable for ~19 min on Oct. 7, 2026 | 🟡 third party |

**Answers:** 🆕 **Offline mode / queue**: if GitHub does not respond, `ship` finishes all the local work (commits, checkpoint) and queues the delivery. The daemon (or the next `ship`) replays it when the API is back. 🆕 A clear message, "GitHub is having an incident", by reading `githubstatus.com` (❓ status API to verify).

## 7. Secrets

| Finding | Source |
|---|---|
| ~39 M secrets leaked on GitHub in 2024 | 🟡 [I Programmer](https://www.i-programmer.info/news/149-security/17958-github-launches-secret-protection.html) |
| Push protection on by default on public repositories since early 2024 | 🟡 [developer-tech](https://www.developer-tech.com/news/github-enables-secret-scanning-push-protection-default/) |

**Answers:** 🟢 local scan before commit, push and anything sent to an AI. 🆕 If a secret was pushed: a guided "**revoke first**" path (a link to the revocation page of the detected provider), then clean-up. Rewriting history is not enough.

## 8. File system traps

| Finding | Source |
|---|---|
| A rename that only changes case is ignored on a case-insensitive file system (Windows, macOS); `git mv` is the workaround; setting `core.ignorecase` to false is discouraged | 🟡 [adamj.eu](https://adamj.eu/tech/2022/12/09/git-change-case-of-filenames/), [Hanselman](https://www.hanselman.com/blog/git-is-casesensitive-and-your-filesystem-may-not-be-weird-folder-merging-on-windows) |
| Folders that differ only by case silently merged on Windows | 🟡 Hanselman |
| CRLF / "whole file" diffs | ❓ not covered by the research, known from the draft (`core.autocrlf`, `.gitattributes`) |

**Answers:** 🆕 A "case rename" detector that automatically uses `git mv` in two steps. 🆕 A detector of case collisions in the tree. 🟢 `.gitattributes` proposed by `ship fix`.

## 9. Forks and aging PRs

| Finding | Source |
|---|---|
| Fork sync: GitHub offers a PR on conflict; `gh repo sync` does not handle conflicts | 🟡 [Syncing a fork docs](https://docs.github.com/en/enterprise-server@3.14/pull-requests/collaborating-with-pull-requests/working-with-forks/syncing-a-fork) |
| Bots mark PRs not rebased for 30 days as "stale" (e.g. coq/bot) | 🟡 [coq bot](https://github.com/erikmd/bot) |

**Answers:** 🆕 `ship` in a fork: detects the upstream, syncs and targets the upstream for the PR. 🟢 `ship status` flags PRs that are behind or in conflict, and `ship` on the branch rebases. ❓ Forks out of v1 scope (stacks impossible across forks, verified in §1 of the research log).

## Summary: new capabilities from this research

| Capability | Friction addressed | Proposed phase |
|---|---|---|
| Undo by situations | §1 | 2 |
| HTTPS ↔ SSH conversion, missing scopes | §2 | 1 |
| PR size guardrail, isolation of refactors | §3 | 3 |
| `ship inbox` (required actions only) | §4 | 5 |
| CI log excerpt + rerun of failed jobs | §5 | 5 |
| Offline queue during a GitHub incident | §6 | 6 |
| "Revoke first" path | §7 | 2 |
| Case renames, collisions | §8 | 2 |
| Fork mode | §9 | ❓ |
