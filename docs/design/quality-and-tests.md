# Quality: tests, coverage, benchmarks, cookbook

> User goal: **maximum coverage of every kind**. Here, "coverage" does not only mean a percentage of lines: every promise of the product (undo, errors, conventions, surfaces, performance, docs) must be **proven by an automated test**.
> Legend: ✅ primary source · 🟡 secondary · ❓ to verify.
>
> Status: the pyramid below is the proposal of phase 0. testscript was not adopted, so layer 5 is not done as written: the situations ship handles are tested end to end in `cmd/ship/*_test.go`, with real git, a bare remote and a fake GitHub HTTP server (layer 6 lives there too, not in `internal/e2e/`). The RPC contract of layer 7 was dropped with `ship rpc` ([ADR-0010](../adr/0010-surfaces.md)). Since 2026-11-07, the merged coverage is sent to Codecov by `ci.yml` and the real E2E of layer 8 runs every night (`nightly-real.yml`, [workflows](workflows.md) §11); mutation testing and the agent evals are not done yet.

## 1. Research

| Fact | Status | Source |
|---|---|---|
| Since **Go 1.20**, you can build an **instrumented binary** (`go build -cover`) that writes its coverage to `GOCOVERDIR`; `go tool covdata` can do `percent`, `textfmt` (`-coverprofile` format) and `merge` (e.g. merging the Windows and macOS runs). Limit: an unrecovered panic loses that run's data | ✅ | [go.dev/doc/build-cover](https://go.dev/doc/build-cover) |
| **testscript**: CLI tests as txtar scripts (the harness of the `go` command), golden files, integration with `go test` and coverage | 🟡 | [rednafi](https://rednafi.com/go/testscript-cli/) |
| **`testing.B.Loop`** (Go 1.24): more reliable benchmarks (no spurious optimization, setup excluded from the timing); compare with **benchstat** over `-count=10` | ✅ Go blog | [go.dev/blog/testing-b-loop](https://go.dev/blog/testing-b-loop) |
| Native Go fuzzing suits **parsers, encoders, validators**; the inputs that make it fail become fixtures | 🟡 | [tessl (secondary reference)](https://tessl.io/registry/testland/coverage-guided-fuzzing/files/references/go-native-fuzzing.md) |
| Go mutation testing: **Gremlins** (pre-1.0, incremental mode on PRs, quality thresholds; can take hours on large modules); **go-mutesting** (jonbaldie v2 fork, May 2026: baseline, filtering by changed lines) | 🟡 | [gremlins](https://redirect.github.com/go-gremlins/gremlins), [go-mutesting v2](https://pkg.go.dev/github.com/jonbaldie/go-mutesting/v2) |
| Property-based testing: `pgregory.net/rapid` | ❓ not covered by the research | - |
| 2026 MCP agent benchmarks: **MCP-Bench** (ICLR 2026, 28 servers, 250 tools, scored at the tool and trajectory level), **DynamicMCPBench** (deterministic replay, scored on the **effects** produced), MCP-Atlas, LiveMCPBench | 🟡 | [MCP-Bench](https://mlanthology.org/iclr/2026/wang2026iclr-mcpbench/), [DynamicMCPBench](https://www.alphaxiv.org/abs/2607.20531) |
| **Diátaxis**: 4 separate kinds of documentation (tutorials, how-to, reference, explanation); adopted by Canonical | 🟡 | [Canonical](https://ubuntu.com/blog/diataxis-a-new-foundation-for-canonical-documentation) |

## 2. The ship-it pyramid (proposal)

| Layer | Tool | What it proves | Where |
|---|---|---|---|
| 1. Unit | `go test` | git parsers, deterministic plan, semver, format, schemas | `internal/**/_test.go` |
| 2. **Golden** | expected files | Plan, title, PR body and changelog generated from real, frozen diffs | `testdata/golden/` |
| 3. **Fuzz** | `go test -fuzz` | Parsers of diffs, `status -z`, emoji titles, config, ROADMAP.md: no panic, stable round trip | corpus in `testdata/fuzz/` |
| 4. **Properties** | ❓ rapid or in-house | Invariants: *undo(op(x)) = x*; checkpoint then restore = identical tree; split then recombine = original diff | `internal/**` |
| 5. **CLI scenarios** | testscript | Every situation of the error playbook ([error-playbook](../reference/error-playbook.md)) and of the cookbook; a real temporary git repository | `testdata/script/*.txtar` |
| 6. **Offline E2E** | fake GitHub (`httptest`) | Full delivery, stacks, rate limit, 5xx, offline mode | `internal/e2e/` |
| 7. **Surface contracts** | single schema | CLI `--json`, RPC and MCP return the same structure for the same operation | generated + test |
| 8. **Real E2E** (nightly) | GitHub sandbox repository | Real API behavior (stacks, rulesets), API drift | scheduled CI |
| 9. **Mutation** | Gremlins or go-mutesting | The suite really catches regressions (mutation score) | weekly CI + incremental mode on PRs |
| 10. **Agent evals** | in-house harness inspired by DynamicMCPBench | An agent completes the task with ship-it MCP, measured on the **effects** (PR created, compliant, nothing lost) | `bench/agents/` |

**Measured coverage:** unit (`-coverprofile`) + instrumented binary (`go build -cover`) run by testscript and the E2E → merged with `covdata merge` across the 3 OSes, published per PR.
**CI of the ship-it repository, blocking from phase 1** (user decision; it concerns **our** CI only, ship-it does not offer a quality CI to client projects):
- the merged coverage (unit + instrumented binary, on 3 OSes) does not drop;
- cookbook, tutorials and error playbook run as testscript on Windows, macOS and Linux;
- no significant performance regression (benchstat) on the key benchmarks (❓ shared-runner noise to handle: repeated runs, comparison on the same machine);
- short fuzzing per parser + `-race`.

The mutation score is tracked every week, without blocking at first.

## 3. Benchmarks

| Family | Measure | Tool |
|---|---|---|
| Micro | parsers, plan, secret scan | `testing.B.Loop` + benchstat, regression flagged in the PR |
| Macro | `ship --dry-run`, checkpoint, `status` on synthetic repositories (1k, 10k, 100k files; 100 MB diff) | real binary, Windows first |
| Startup | time to first output | `hyperfine` ❓ or a Go harness |
| **Agents** | tool calls, tokens (real tokenizer), duration, success rate: ship-it MCP vs Pare vs plain Bash | `bench/agents/` |
| Network | API calls per operation, ratio of 304 responses | counter in the `forge` client |

**Performance budgets**: `ship status` < 200 ms wall clock without network on Windows, process start included (decision of 2026-11-02; measured: about 155-185 ms, 5 git processes); checkpoint < 150 ms on 5,000 files (order of magnitude reported for libgit2: 71-78 ms median in a third-party project 🟡 [issue](https://github.com/2youg1/sprawling-agents/issues/101); measured: about 420 ms on 2,000 files with the stat cache, not met yet). The micro, checkpoint and end-to-end benchmarks exist; `go run ./tools/bench` checks their budgets (time ceilings, and git processes per command, machine-independent) and CI runs it: see [docs/design/performance.md: Performance](performance.md) for the numbers and how to run them.

## 4. Documentation and cookbook (Diátaxis)

| Kind | ship-it content | Tested? |
|---|---|---|
| Tutorials | "Your first ship in 2 minutes" (junior), "Connect ship-it to Claude Code" | *planned*: every tutorial as a testscript scenario |
| **How-to / Cookbook** | One recipe per situation: "I committed on main", "undo my last ship", "deliver as a stack", "configure a client repository at the Agency level", "secret pushed"… | ✅ **every recipe runs in CI** (the markdown is generated from the txtar or checked against it) |
| Reference | Commands, `.github/ship.json`, MCP tools, exit codes | **generated** from the single schema |
| Explanation | Why checkpoints, why intents rather than verbs, security model | docs/ (this folder) |

Innovation: **no executable doc can lie**, since tutorials, cookbook and error playbook are played in CI on the 3 OSes.

## 5. Other quality guardrails

- Lint: `go vet`, staticcheck (❓ through the `tool` directive), `govulncheck` (❓ to verify); zizmor on the workflows.
- Concurrency tests with `-race`.
- **Crash** tests: kill the process at every step of `deliver` (fault injection), then check that the rerun resumes without loss.
- i18n tests: every key exists in fr and en; no hard-coded string.
- CLI accessibility: `--plain` output checked by golden files; `NO_COLOR` honored.
