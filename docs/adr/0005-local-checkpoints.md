# ADR-0005: Checkpoints in local hidden refs

**Status:** accepted; **storage revised on 2026-10-23: option C, an isolated private repository, one ref per checkpoint** (implemented in [`internal/adapters/checkpointstore`](../../internal/adapters/checkpointstore/store.go)), **undo through the oplog** ([`internal/core/oplog`](../../internal/core/oplog/oplog.go)).

## Revision U3 of 2026-10-23: oplog and `ship undo`
- **Append-only log**: one JSON entry per operation in `.git/ship/oplog/<id>.json`, written atomically (temporary file, `fsync`, rename). The id is a 26-character time-ordered identifier (ULID layout, `obs.NewRunID`). An entry holds the before/after *view* (branch, HEAD, detached HEAD), the checkpoint taken before, the commit and the branch created. An undo is **a new entry** (`kind: undo`, `undoes: <id>`), never a modification: this is jj's model.
- **Pure decision**: `oplog.PlanUndo` (core, no I/O) computes the compensating steps; `app.Undo` applies them through the ports.
- **Do not touch the files**: right after a delivery, the worktree is exactly the delivered commit, hence the work as it was before. The undo only moves HEAD and the refs (`symbolic-ref`, `update-ref` as a **compare-and-swap**), then resets the index to the old HEAD (`read-tree`). A file deleted before the delivery therefore stays deleted, whereas a `git switch` would have recreated it (case covered by the end-to-end test `cmd/ship/undo_test.go`). The checkpoint is then restored byte for byte, in case a hook modified files during the commit.
- **Guardrails**: the undo **refuses** if HEAD or the branch moved since the delivery (`SHIP-E0404`) or if files changed since (`SHIP-E0405`). Branches are only deleted if they still point to the delivered commit. A `before-undo` checkpoint is taken before acting.
- **Interrupted delivery**: if the delivery fails after creating the branch, the entry is still written and the undo knows how to roll back.
- ~~Still to do: remote effects~~ → revision U4 below. Remaining: `ship redo`.

## Revision U4 of 2026-10-23: undoing remote effects and tidy
- **Publication**: if it **opened** the PR (`pull_request_created` in the oplog), the undo closes the PR with a comment explaining why, then deletes the remote branch **only if it still points to the pushed commit** (`push --force-with-lease=<ref>:<sha> --delete`). git's docs do not say whether the lease protects a deletion: **verified on git 2.52** (a stale lease is rejected with `stale info`), then pinned by `TestDeleteRemoteBranchIsLeased`, which runs in CI on all three OSes.
- **Consent**: every remote step requires explicit approval. `app` never asks a question; it returns `SHIP-E0407`, then the CLI asks in the terminal or accepts `--yes`. Without a terminal and without `--yes`, nothing changes (exit code 2).
- The actual state of the PR is read again before acting. A **merged** PR is refused (`SHIP-E0408`: a merge cannot be undone, it takes a revert). A PR that is already closed is not closed again. A PR that was only **updated** is refused (`SHIP-E0406`), because a shared branch is not rewritten.
- **Tidy** (`ship tidy`): each deleted branch is recorded with its commit, and the undo recreates it exactly (`update-ref <ref> <sha> ""`, which refuses if it already exists). Deleting a branch also removes its config section (upstream), like `git branch -D`.
- **Full chain** verified end to end (`cmd/ship/tidy_test.go`): tidy, delivery, publication, then three `ship undo` that revert everything in reverse order.

## Revision U2 of 2026-10-23: one ref per checkpoint in the store
- **Finding**: with a chain (each checkpoint has the previous one as its parent), purging old ones forces the chain to be rewritten, which **changes the id of every checkpoint kept**. Yet the oplog and the user (`ship checkpoint restore <id>`) refer to them by id.
- **Decision**: each checkpoint is a commit **without a parent**, under its own ref `refs/checkpoints/<UTC nanoseconds on 19 digits>-<id>` *in the private store*. The name gives the exact creation order (`for-each-ref --sort=-refname`), even for two checkpoints taken in the same second.
- The drawbacks of option A (many refs, polluted `git log --all`, `push --mirror`) concerned **the user's repository**; they do not apply to option C's private store.
- **Purge** (`ship gc`): refs deleted in a single `update-ref --stdin` transaction (each deletion checks the expected id), then `git gc --prune=now` *in the store*. The store is a bare repository: no reflog by default, so the objects are really freed.
- **Retention** ([`internal/core/retention`](../../internal/core/retention/retention.go)): the **last 20**, plus **the most recent of each day (UTC) over 30 days**, plus **every checkpoint of a delivery that can still be undone**. These values are defaults, to recalibrate after benchmark U1b. If a checkpoint has disappeared anyway, the undo simply skips the defensive restore.

## Revision of 2026-10-23 (decision U1)
- **Actual finding**: the demo of option B (a `refs/ship/checkpoints` ref in the user's repository) showed the "ship-it checkpoint" commits in `git log --all`, which also makes them visible in gitk and Git Graph.
- **Decision** ("as well as possible"): **option C**, a private bare repository `.git/ship/store.git` with **its own objects** (copied, not shared through *alternates*). It is invisible in `log --all` and in graphical interfaces, never sent (not even by `push --mirror`), and the user's `gc` cannot break it.
- **Byte-exact**: blobs written by `hash-object --no-filters` (no attribute filter and no line-ending conversion, ✅ git docs), index built by `update-index -z --index-info`, raw restore through `cat-file --batch`. The test `TestRestoreIsByteExact` proves it under `core.autocrlf=true` and `* text=auto`.
- ~~**Chain on a single ref** in the store, updated by compare-and-swap~~: replaced by one ref per checkpoint (revision U2).
- **Cost**: the first snapshot copies the files (compressed); every checkpoint rehashes everything. The optimization (reusing unchanged blobs) will be measured by benchmark U1.
- **Known limit**: `RestoreFiles` rewrites the checkpoint's files but does not delete those created afterwards. `ship undo` (revision U3) does not have this limit for deliveries, since it works on the refs and the index.

The text below describes the initial design (option A), kept for the record.

## Decision
- A checkpoint is a git commit (the full tree of the worktree, untracked non-ignored files included) created with a **temporary index** (`GIT_INDEX_FILE`), then `write-tree` and `commit-tree`. Neither the user's index, nor their worktree, nor HEAD are touched.
- It is stored under `refs/ship/checkpoints/<timestamp>` with the metadata in the message (branch, HEAD, triggering operation).
- These refs are **never pushed**: a default refspec does not include them (❓ to verify for `push --mirror` and third-party tools).
- Retention: last N + 1 per day over 30 days (configurable), purged by `ship gc`.
- Triggers: before every ship-it operation, on demand, and periodically by the daemon.

## Pros / cons
- ✅ No new storage (we use the git object database), time travel, undo of everything.
- ⚠️ Can bloat the object database with large untracked files: size threshold and `.gitignore` honored.
- ⚠️ An unscanned secret can end up in a local object, which is acceptable since nothing is pushed.

## Inspirations
jj's oplog and GitButler's undo.
