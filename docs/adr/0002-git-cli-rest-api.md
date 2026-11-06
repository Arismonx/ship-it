# ADR-0002: git CLI + GitHub REST API, without `gh`

**Status:** accepted

## Context
The draft depended on `git` and `gh`, that is two binaries to install and authenticate.

## Options
| | Pros | Cons |
|---|---|---|
| **Exec `git` + REST over net/http** | `git` is already on every developer's machine; honors hooks, config, credential helpers and signing; a single binary of our own | Output parsing (mitigated by `-z` and porcelain) |
| git + `gh` | Simple | A second binary, a second authentication |
| Native git library (go-git) | No exec | Partial coverage (hooks, some configs, rebase): behaves differently from the user's git |

## Decision
Exec `git` for every local operation; GitHub REST API (GraphQL for reads where useful) through the stdlib.

## Consequences
- Compatibility contract: **a recent git is required** (user choice). Settled: **git ≥ 2.43** (the version of Ubuntu 24.04 LTS); `ship doctor` recommends the latest release, and newer options are used when available.
- If `gh` is installed and authenticated, we can **reuse its token** (see ADR-0003), without depending on it.
