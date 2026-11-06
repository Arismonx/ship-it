# ADR-0009: Layered JSON config + organisation policy

**Status:** accepted. Default, user and repository layers implemented on 2026-10-23 in [`internal/core/config`](../../internal/core/config/config.go); organisation policy through a system file implemented the same day ([`policy.go`](../../internal/core/config/policy.go), see [cli](../reference/cli.md#organisation-policy)). The private `<org>/ship-policy` repository is still to do.

## Decision
- **A single repository file: `.github/ship.json`** (user choice), with the sections `ship` (the tool's behavior), `github` (settings-as-code, ADR-0014) and, later, `roadmap`.
- JSON (stdlib) + a published JSON Schema, for completion in VS Code.
- Layers: defaults < user (`config.json` in the OS config folder) < repository < organisation policy (locked, coming).
- **Progressive configuration**: a repository without `ship.json` works perfectly. `ship init` only writes `$schema`, `version` and the keys that differ from the defaults.

## Implementation (2026-10-23)
- **A single source of truth**: the Go `Settings` struct and its tags (`json`, `enum`, `doc`). The parser walks it by reflection to validate and stack the layers, and [`tools/gen`](../../tools/gen/main.go) derives [`schemas/ship.schema.json`](../../schemas/ship.schema.json) from it. A test fails if the versioned schema is out of date.
- **Only the keys actually wired exist**, because a key accepted with no effect would mislead the user. Any unknown key is an error that gives the file, the key's path and the keys known at that location.
- **Precise errors**: invalid JSON with `file:line:column` (`SHIP-E0701`), unknown key (`E0702`), wrong type (`E0703`), value outside the list (`E0704`), version too recent (`E0705`, "update ship-it"). The config is read **before** any change: a broken file never has a partial effect.
- **Provenance**: `ship config` shows each effective setting and the layer that set it.
- `version`: absent = current version (1); higher than the one the tool knows = explicit refusal (N9: no silent rewrite).
- v1 keys: `ship.publish` (`auto` | `never`), `ship.pullRequest.draftWhenUnattended`, `ship.pullRequest.labels`, `ship.issues.keyword`, `github.managedBy`, `github.labels.taxonomy` (`ship-it` | `none`), `github.labels.areas`.
- **Strict JSON** (no comments): JSONC remains possible later.

## Options ruled out
- TOML or YAML: nicer, but they require a parsing dependency (ADR-0012).

## Open question
Exact format and location of the policy ([cli.md](../reference/cli.md#organisation-policy)).
