# Undo: storage, and optimizations

> The user asks whether undo needs another storage and better optimizations, and what we may have missed. This study **challenges** ADR-0005.
> Legend: ✅ primary source · 🟡 secondary · ❓ to verify · 💡 design hypothesis (to be proven by a benchmark).

## 1. What others do

| System | Undo storage | Status | Source |
|---|---|---|---|
| **jj** | *Operation backend* (operations + **view** objects: state of all refs, heads and the working copy at the end of each operation) + *op heads backend*; **lock-free**: each command starts from an operation and records it as its parent, and divergence is resolved by the next command; worktree snapshot based on the recorded mtimes and sizes; `refs/jj/keep/*` refs so that git's GC does not delete the commits referenced by the op log | ✅ jj docs | [architecture.md](https://github.com/jj-vcs/jj/blob/main/docs/technical/architecture.md), [concurrency](https://jj-vcs.github.io/jj/latest/technical/concurrency/), [operation log](https://docs.jj-vcs.dev/latest/operation-log/) |
| Third-party project (agents) | Comparison of libgit2 / gix / jj CLI / jj-lib / an in-house BLAKE3 CAS. Decisive criterion: **the user's `git log` must show the work as real commits**, which ruled out the in-house CAS. libgit2: median checkpoint **71-78 ms** on 5,000 files | 🟡 | [issue #101](https://github.com/2youg1/sprawling-agents/issues/101) |
| Native git | reflog (local, expirable), `stash` (one ref + reflog) | known | - |
| **reftable** (ref backend) | Binary, prefix compression; deletion no longer rewrites the whole ref database; git 2.51: up to 22× faster fetch with 10,000 refs; "outperforms the files backend" | 🟡 | [GitLab, git 2.51](https://about.gitlab.com/blog/what-s-new-in-git-2-51-0/), [patches](https://lore.kernel.org/git/ZhSzDxcEYgqauWi8@tanuki/T/) |

## 2. Storage options

| Option | Description | Pros | Cons |
|---|---|---|---|
| **A. One ref per checkpoint** (current ADR-0005) | `refs/ship/checkpoints/<ts>` | Simple | 💡 Thousands of refs slow operations down on a `files` backend (the reftable gain shows that the number of refs matters); pollutes `git log --all`, gitk and third-party tools; `push --mirror` would send them ❓ |
| **B. Chain of checkpoints on a single ref** | `refs/ship/checkpoints` points to the latest; each checkpoint has the previous one as its parent (like a ref with history) | **A single ref**; walkable history; deduplication by content; natural GC | Purging = rewriting the chain (fast, it is metadata) |
| **C. Separate object repository** | `.git/ship/objects` plugged in as an *alternate* (❓ exact mechanism to verify); refs in a small dedicated repository | **Full isolation**: nothing in the user's refs, the user's GC does not touch it, and neither clone nor push can reach it | More complex; ❓ behavior of `git gc` and `fsck` with alternates |
| **D. In-house CAS** (BLAKE3 + index) | Like the third-party project | Full control, maybe faster | Not interoperable with git; dependencies; reimplements git. **Ruled out** for the same reason as the third-party project |
| **E. jj-lib** | Embed jj | Proven op log | Rust (we are in Go): **ruled out** |

### Oplog (operation log)

Proposal 💡 inspired by jj: each ship-it operation records a before/after **view** (all the local refs involved, HEAD, the worktree checkpoint, the state of the **remote effects**: PRs, remote branches, labels, release). Append-only JSON format in `.git/ship/oplog/`, one entry per file, atomic write (temp + rename), or git objects on chain B.

### Remote effects: what the current undo misses
> ✅ Implemented on 2026-10-23 for PRs and pushed branches ([ADR-0005](../adr/0005-local-checkpoints.md), revision U4). Releases will come with `ship release`.

A local undo does not undo an open PR, a pushed branch or a published release. 💡 **Compensating actions**: each remote effect records its inverse (PR opened → close it with a comment; branch pushed → delete it if it is still at the same SHA; release → turn it into a draft or delete it, **always with confirmation**). The undo plays the compensations in reverse order, and a compensation that is impossible (PR already merged) is reported, not hidden.

**Proposal:** **B (chain on one ref) + oplog with views + compensating actions**, with C as an option if measurements show an impact on the user's repository. ❓ Decision after benchmark (phase 2).

> **Decided on 2026-10-23** ([ADR-0005](../adr/0005-local-checkpoints.md), revisions U1 to U3): **C** (private repository), with **one ref per checkpoint in that repository** (chain B made ids unstable at every purge), and an **append-only JSON oplog** in `.git/ship/oplog/`. Undoing a delivery only moves refs and the index. Benchmark U1b is still to be done to calibrate retention.

## 3. Optimizations

### git
| Lever | Fact / hypothesis | Status |
|---|---|---|
| **Built-in FSMonitor + untracked cache**: `git status` from ~18 s down to < 1 s on large worktrees (Windows and macOS, git ≥ 2.37) | GitHub engineering | 🟡 [github.blog](https://github.blog/engineering/improve-git-monorepo-performance-with-a-file-system-monitor/) |
| `feature.manyFiles` (index v4, untracked cache, `index.skipHash`, which causes compatibility problems with old git versions) | git docs | 🟡 |
| → `ship doctor` **proposes** these settings on large repositories, with consent (no silent activation) | proposed decision | 💡 |
| Checkpoint: temporary index **copied from the real index**, to reuse its stat cache instead of rehashing everything | hypothesis | 💡 to measure |
| A **persistent** `cat-file --batch` git process to read many objects without one `exec` per object | known | 💡 to measure |
| `status --porcelain=v2 -z`: a single command for branch, ahead/behind and files | known | 💡 |
| The daemon's polling reads mtimes before running `git status` | hypothesis | 💡 |

### GitHub
| Lever | Fact | Status |
|---|---|---|
| **Conditional requests (ETag)**: an authenticated **304** **does not count** against the primary rate limit | GitHub docs | ✅ [best practices](https://docs.github.com/en/rest/using-the-rest-api/best-practices-for-using-the-rest-api) |
| The `x-ratelimit-*` headers are authoritative; honor `retry-after` | GitHub docs | ✅ [rate limits](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api) |
| GraphQL to aggregate reads (`ship status`: PRs, checks, reviews in one request) | hypothesis | 💡 |
| Local ETag cache (`.git/ship/cache/`), bounded parallelism | hypothesis | 💡 |

### Diff, size and "generated"
| Lever | Fact | Status |
|---|---|---|
| The **`linguist-generated`** attribute in `.gitattributes` collapses files in GitHub diffs and excludes them from statistics; it only takes effect once `.gitattributes` is committed; Linguist's automatic detection misses some generated files | GitHub docs + Linguist | ✅ [GitHub docs](https://docs.github.com/en/repositories/working-with-files/managing-files/customizing-how-changed-files-appear-on-github), [overrides](https://github.com/github-linguist/linguist/blob/main/docs/overrides.md) |
| → The size guardrail and the diff budget sent to the AI **read `linguist-generated` and `linguist-vendored`** to exclude these files. ship-it can **offer** to add these attributes when it detects generated files | proposed decision | 💡 |

### Binary and startup
`-trimpath`, `-ldflags "-s -w"` (size), lazy initialization (no network until it is needed), no request at startup for `ship --help`. 💡 To measure.

## 4. What we had missed (and is now recorded)

| # | Topic | Proposal |
|---|---|---|
| M1 | **Undoing remote effects** | Compensating actions (§2) |
| M2 | **Interruption in the middle of `deliver`** (Ctrl+C, crash, network loss) | `deliver` becomes a **persisted state machine**: each step is logged, and the rerun resumes at the next step (idempotence); fault-injection tests |
| M3 | **Concurrency** of daemon + CLI + MCP agent on the same repository | ✅ decided: **lock-free, the jj way**. ❓ Hard point: git operations themselves (rebase, push) are not lock-free; a short lock **per git step** will be needed, or git's `index.lock` can play that role. To specify in phase 2 |
| M4 | **Atomic writes** (config, oplog, cache) | temp + fsync + rename |
| M5 | **Encoding**: Windows console and emojis, non-ASCII paths, spaces, CRLF in messages | dedicated tests on the 3 OSes |
| M6 | **Generated and vendored files** | `linguist-generated` / `linguist-vendored` (§3) |
| M7 | **Local observability**: understanding what took time | `ship --trace`: local timeline (nothing sent); OpenTelemetry as an option ❓ |
| M8 | **Oplog privacy**: jj stores the host name and user in it (noted as a point of attention in the third-party project 🟡) | minimal oplog, no file content, nothing is pushed |
| M9 | **Disk growth** of checkpoints (large untracked files) | size threshold, `.gitignore` honored, `ship gc`, configurable quota |
| M10 | **Time zones and clocks** (changelog, retention) | UTC internally, local time for display |
