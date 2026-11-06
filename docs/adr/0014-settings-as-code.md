# ADR-0014: Settings-as-code (`.github/ship.json`) with plan/apply

**Status:** accepted (user choice)

## Decision
The desired state of the GitHub repository (merge, classification, rulesets, assignment, project) is described in `.github/ship.json`. `ship sync` reads the actual state, shows the diff and applies it after confirmation. The file is generated per level (`ship init`) and only contains what is enabled.

## Implemented (2026-10-23): labels
- `ship init` writes `.github/ship.json` without ever overwriting an existing file (`O_EXCL`). It proposes `area/*` labels from the commit scopes used at least 3 times in the last 500 commits, and does not touch GitHub.
- `ship sync` reads the repository's labels and computes a pure plan ([`core/settings`](../../internal/core/settings/labels.go)): **rename** GitHub's default labels into the taxonomy (`bug` → `kind/bug`…, issues keep their label), **create** the missing ones, **update** color and description. It **never deletes** anything and never renames over an existing label.
- The plan is shown, then Enter applies it. Without a terminal and without `--yes`, nothing is applied (exit code 2). What was applied is recorded in the oplog (`kind: sync`). On failure, applying stops and says how many changes went through.
- `github.managedBy` (for example `opentofu`) makes `ship sync` **read-only**: it shows the differences and refuses to apply (`SHIP-E0707`).
- Verified end to end (`cmd/ship/sync_test.go`) against a fake GitHub that keeps its labels: the second `ship sync` finds nothing left to do.
- Remaining: rulesets, merge, assignment, Project.

## Pros / cons
- ✅ Auditable, versioned, replicable from one client to another, reviewed in a PR.
- ⚠️ Possible drift if someone changes the repository in the UI: `ship sync` reports it (drift) without silently overwriting.
- ⚠️ Admin rights required for many settings.

## Coexistence with IaC (decision H1)
Repositories already managed by IaC (the OpenTofu module `GregoireF/iac`, Terraform, Probot Settings…) are **not** touched: `.github/ship.json` can declare `"github": { "managedBy": "opentofu" }`, and `ship sync` then becomes read-only (it reports the differences without applying anything). `ship sync` is meant for agencies and clients without IaC.

## Open questions
Merge with a root `ship.json` (tool config) or separate files? Settled: one `.github/ship.json`.
