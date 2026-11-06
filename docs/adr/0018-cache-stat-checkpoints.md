# ADR-0018: Checkpoint stat cache trusts stat data, as git's index does

**Status:** accepted (2026-11-02, user choice)

## Context
A checkpoint ([ADR-0005](0005-local-checkpoints.md)) copies every non-ignored file of the working tree into the private store. Reading and hashing all of them made a checkpoint cost about 1.1 s on 2,000 files on Windows, paid before every `ship`. The stat cache (`internal/adapters/checkpointstore/cache.go`) keeps the blob of every file of the last checkpoint with the file's stat data, and reuses the blob of a file whose stat data has not changed: the warm checkpoint went from 1,146 ms to 421 ms ([performance](../design/performance.md)).

The cache compares **mode, size, modification time (nanoseconds) and, on Unix, the inode number**. A file modified less than 3 s before the cache was written, or later, is always hashed again (the racy window, against coarse clocks and FAT's two-second times). Windows has no inode in `os.FileInfo`. So on Windows, a file **replaced by one of identical size and identical modification time**, that time being more than 3 s older than the cache, keeps its old blob, and the next checkpoint stores the old content. An ordinary edit cannot do that: it gives the file a new time, later than the cache, and the file is hashed again. It needs a tool that preserves or sets modification times (`touch -r`, some archive extractors, some sync tools) and a content of exactly the same length.

git's index makes the same bet. It compares stat data instead of contents and only rereads files whose time is too close to the index's own: the "racy git" problem and its handling are described in [git's documentation on racy git](https://git-scm.com/docs/racy-git). That page lists the fields git compares (`st_mode`, `st_mtime`, `st_ctime`, `st_uid`, `st_gid`, `st_ino`, `st_size`) and notes that some filesystems still have coarse timestamps (it names CEPH, CIFS, NTFS and UDF).

## Options
| | Pros | Cons |
|---|---|---|
| **Trust mode, size, mtime (+ inode on Unix), racy window of 3 s** | Keeps the gain on Windows (1,146 → 421 ms); same trade-off as git's index, which the user already relies on for `git status` and `git add` | A same-size, same-mtime replacement on Windows outside the racy window is missed |
| Also compare the Windows file index (`GetFileInformationByHandle`, the closest thing to an inode) | Catches replacement by a new file | Needs a handle, so an open of every file: most of the cost of reading it, so most of the gain is lost; a `syscall`/`golang.org/x/sys/windows` path for one platform |
| Disable the cache on Windows | Never misses a change | Windows, where processes and file I/O are the slowest, loses the whole gain: about 1.1 s per checkpoint on 2,000 files |

## Decision
Keep the stat cache on every platform, trusting mode, size, modification time and, where there is one, the inode, with the 3 s racy window. The trade-off is the one git's index makes: stat data stands for contents, and only racy entries are read again.

## Consequences
- On Windows, a file replaced by one of identical size and modification time (older than the racy window) since the last checkpoint comes back with its older content on undo. On Unix, the inode catches a replacement by a new file; an in-place rewrite of the same size that restores the time is missed there too (git's index would notice it through `st_ctime`, which the cache does not compare).
- The cache is only an optimisation: removing `.git/ship/store.git/checkpoint-cache` makes the next checkpoint hash every file. A stale cache naming a pruned blob makes `write-tree` fail, and the snapshot is taken again without it.
- The blob ids do not depend on how they were written: `hash-object --no-filters` for a few files, one `git fast-import` run from 100 new files (byte-exact, tested against `hash-object --no-filters`).
- To revisit if a user reports a change missed by a checkpoint.
