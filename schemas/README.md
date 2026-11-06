# schemas

**Generated files: do not edit by hand.** Run `go run ./tools/gen`; `tools/gen`'s test fails when a file here is out of date.

| File | Source | Used by |
|---|---|---|
| `ship.schema.json` | `internal/core/config` (`Settings` and its `json`, `enum`, `doc` tags) | the `$schema` of `.github/ship.json`: completion and validation in VS Code |
| `policy.schema.json` | `internal/core/config` (same `Settings` under `enforce`) | the organisation policy file deployed by IT |
