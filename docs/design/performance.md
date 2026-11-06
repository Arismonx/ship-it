# Performance

ship-it sits between the developer and every commit: `ship status` must answer at once, and `ship` must not make delivering slower than typing the git commands. This page gives the measured numbers, the budgets CI enforces, how to run them, and where the time still goes.

## What costs time

On Windows, **every git process costs about 20 ms** before it does any work (process creation, the git for Windows runtime, antivirus scanning): it is the dominant cost of every command. On Linux and macOS a process costs 1 to 3 ms. Hence two kinds of budget:

- **git processes per command (`git/op`)**: the same on every machine, so a hard ceiling that never flakes. A new process in a hot path shows up here first.
- **time per operation**: the median of a few runs, with a wide headroom, since shared CI runners are noisy (on the development machine the same `ship status` run varied from 206 to 394 ms while other work was running).

## Benchmarks

Inputs are generated, deterministic and realistic (`internal/testkit/benchdata`):

| Benchmark | Package | Input |
|---|---|---|
| `BenchmarkParseUnifiedDiff`, `BenchmarkNew` | `internal/core/snapshot` | a `git diff` of 500 files, about 10,000 lines (new, deleted, renamed, binary files) |
| `BenchmarkScan` | `internal/core/secrets` | the ~3,000 added lines of that diff |
| `BenchmarkPropose` | `internal/core/plan` | the plan of those 500 files |
| `BenchmarkMergeFor`, `BenchmarkStripJSONC` | `internal/core/config` | preset, user file with contexts, repository file with overrides, `SHIP_*`, `--set`, for 500 paths |
| `BenchmarkAny` | `internal/core/glob` | 50,000 monorepo paths against 6 patterns |
| `BenchmarkCreateWarm`, `BenchmarkCreateCold` | `internal/adapters/checkpointstore` | a checkpoint of the repository below, with the store already there (any ship) or not (the first one) |
| `BenchmarkStatusOffline`, `BenchmarkPlan`, `BenchmarkShipLocal` | `cmd/ship` | `ship status --offline`, `ship plan`, `ship --local --yes` run in-process as `main` does, every real adapter |
| `BenchmarkBinaryStatusOffline` | `cmd/ship` | the built executable, process start included |

The repository of the end-to-end benchmarks has **2,000 files and 300 commits** (written by one `git fast-import`, files dated like a checkout), on a feature branch three commits ahead of `origin/main`, with **30 changed and 5 new files**. Remote refs are local: GitHub is never reached. A handler on the journal counts the `git.exec` records, one per git process (the startup `rev-parse` included), reported as `git/op`.

All benchmarks use `testing.B.Loop` (Go ≥ 1.24): setup is excluded from the timing and results cannot be optimised away.

## Running them

```sh
go run ./tools/bench              # every budget; exit 1 when one is exceeded
go run ./tools/bench -run Status  # those whose name matches
go run ./tools/bench -count=5     # more runs (the median is compared)
go run ./tools/check -bench       # the pull request checks, then the budgets
```

`tools/bench` runs each benchmark of [`tools/bench/budgets.json`](../../tools/bench/budgets.json) with `go test -bench`, prints a table (median time, its ceiling, git processes, their ceiling) and fails on any excess or on a benchmark that did not run. CI runs it on Ubuntu in the `bench` job of `ci.yml`, blocking.

To look at one command by hand, generate the repository and run ship with `--debug`, which prints every git process with its duration:

```sh
SHIP_BENCH_REPO=/tmp/widget go test -run TestMakeBenchRepository ./cmd/ship
cd /tmp/widget && ship --debug status --offline
```

For a CPU profile: `go test -run '^$' -bench BenchmarkPlan -benchtime=10x -cpuprofile cpu.out ./cmd/ship`, then `go tool pprof cpu.out`. To compare two versions, run `-count=10` on each and compare with [benchstat](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat).

## Measured

Medians from `go run ./tools/bench` on the development machine (Windows 11, i7-12700F, other work running), and the budgets of `tools/bench/budgets.json` (time: about 4× the measure; git processes: exact).

| Benchmark | Time/op | Budget | git/op | Budget |
|---|---|---|---|---|
| `BenchmarkParseUnifiedDiff` (10,000 lines) | 1.2 ms | 5 ms | not counted | not counted |
| `BenchmarkScan` (3,000 added lines) | 6.5 ms | 25 ms | not counted | not counted |
| `BenchmarkPropose` (500 files) | 2.0 ms | 10 ms | not counted | not counted |
| `BenchmarkMergeFor` (full stack, 500 paths) | 62 µs | 300 µs | not counted | not counted |
| `BenchmarkAny` (50,000 paths) | 59 ms | 250 ms | not counted | not counted |
| `BenchmarkCreateWarm` (checkpoint, 2,000 files) | 432 ms | 1.5 s | 7 | 7 |
| `BenchmarkCreateCold` (first checkpoint) | 760 ms | 3 s | 9 | 9 |
| `BenchmarkStatusOffline` | 129 ms | 600 ms | 5 | 5 |
| `BenchmarkPlan` | 259 ms | 1 s | 7 | 7 |
| `BenchmarkShipLocal` | 1.0 s | 4 s | 23 | 23 |
| `BenchmarkBinaryStatusOffline` (process start included) | 185 ms | 800 ms | 5 | 5 |

When a change saves a git process, lower its `max_git`: the budget only ever goes down.

## Changes made after measuring

Each number is from the development machine (Windows 11, i7-12700F), before → after:

| Change | Measure | Before | After |
|---|---|---|---|
| `ship status`: compare with the base in one process (`rev-list` alone, no `show-ref` first) | git processes | 6 | 5 |
| | real binary, median of 7 (`run.end`) | 138 ms | 110 ms |
| `ship plan`: read `git status` once (was twice) and the history once (was up to three times, two processes each); `RecentSubjects` runs `log` alone | git processes | 11 | 7 |
| | real binary, median of 7 | 329 ms | 214 ms |
| Secret scan: each rule's literals skip the patterns a line cannot match | 3,000 added lines | 42.5 ms | 6.2 ms |
| `glob.Any`: split the path once; a name pattern matches the last segment only | 50,000 paths × 6 patterns | 89.9 ms | 53.5 ms |
| Checkpoint: reuse the blob of files unchanged since the last checkpoint (stat cache, as git's index; trade-off in [ADR-0018](../adr/0018-cache-stat-checkpoints.md)) | warm checkpoint, 2,000 files | 1,146 ms | 421 ms |
| Checkpoint: from 100 new blobs, write them with one `git fast-import` into a pack instead of one loose object each (`hash-object -w`) | first checkpoint, 2,000 files, median of 3 | 5.9 s | 760 ms |
| Delivery: reuse the status read before the work in progress | git processes of `ship --local` (warm store) | 29 | 28 |
| Delivery right after the plan (`--yes`, an agent): reuse the plan's status, diff, `origin/HEAD` and history; the doctor shares the delivery's status | git processes of `ship --local --yes` (warm store) | 28 | 23 |
| | `BenchmarkShipLocal`, median of 3 | 1.2 s | 1.0 s |

## Where the time still goes

The status target of [docs/design/quality-and-tests.md](quality-and-tests.md) is under 200 ms wall clock without the network on Windows, process start included (decision of 2026-11-02, replacing 300 ms). `ship status --offline` meets it: about 110 ms of its own time, ≈ 155-185 ms wall clock with process start. What remains is git itself:

- **5 git processes**, about 20 ms each on Windows: the `rev-parse` of the journal at start-up, then `git status`, `config --get remote.origin.url`, `symbolic-ref origin/HEAD` in parallel, then `rev-list` against the base. `git status` on 2,000 files is 30 to 60 ms by itself.
- Going further means fewer processes: reading `remote.origin.url` and `origin/HEAD` from the files of `.git` instead of asking git (fragile: includes, reftable, worktrees), or comparing with the base inside the parallel phase (`origin/HEAD...HEAD`). Below 100 ms is not reachable on Windows while status needs `git status` plus one other process after start-up.

`ship --local` spends most of its time in the checkpoint (`write-tree` alone is about 120 ms on a fresh private index). With `--yes` it reads each fact once: the plan's reads (`app.PlanReads`) go with the request to the delivery, and to the publication for `origin/HEAD`. After a review they do not, on purpose: the review can last minutes, the developer may edit files meanwhile, and the delivery must scan what it commits; it then reads `git status` (shared with its doctor), the diff, `origin/HEAD` and the history again, four processes. The publication always reads `git status` again: the delivery has just moved HEAD. The first checkpoint of a repository writes its blobs with one `git fast-import` run, into a pack: 760 ms for 2,000 files on Windows (`BenchmarkCreateCold`), of which `write-tree` is about 400 ms and fast-import about 150 ms. Files written a moment before are slow to read the first time on Windows (9 s for 1,847 fresh files read from the test binary, 75 ms the second time, most likely the antivirus): the benchmark reads its generated files once before timing, as a real repository's files are not new.
