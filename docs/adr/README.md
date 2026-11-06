# Architecture Decision Records

Format: context → options (pros/cons) → decision → consequences. Statuses: `proposed`, `accepted`, `superseded by ADR-xxxx`.

| # | Decision | Status |
|---|---|---|
| [0001](0001-go-language.md) | Go for the core | accepted |
| [0002](0002-git-cli-rest-api.md) | git CLI + GitHub REST API, without `gh` | accepted |
| [0003](0003-authentication.md) | Authentication chain and device flow | accepted (token resolution); device flow proposed |
| [0004](0004-opt-in-ai.md) | Opt-in AI, three plan sources | accepted |
| [0005](0005-local-checkpoints.md) | Checkpoints in local hidden refs | accepted |
| [0006](0006-native-github-stacks.md) | Native GitHub stacks through REST | accepted |
| [0007](0007-emoji-header-format.md) | `✨ feat(scope): subject` format | accepted |
| [0008](0008-license.md) | License: Apache-2.0 (EUPL-1.2 until 2026-11-06) | accepted, revised |
| [0009](0009-layered-json-config-policy.md) | Layered JSON config + organisation policy | accepted (layers + system policy); policy repository to come |
| [0010](0010-surfaces.md) | One core, CLI / VS Code / MCP / daemon surfaces | accepted |
| [0011](0011-i18n.md) | Separately configurable languages | accepted |
| [0012](0012-dependency-budget.md) | Dependency budget (+ Charm v2 exception) | accepted |
| [0013](0013-built-in-releases.md) | Built-in releases | proposed |
| [0014](0014-settings-as-code.md) | Settings-as-code with plan/apply | accepted |
| [0015](0015-issue-types-fields.md) | Issue Types/Fields, falling back to labels | accepted |
| [0016](0016-roadmap-markdown-source.md) | ROADMAP.md → GitHub | accepted |
| [0017](0017-hexagonal-architecture.md) | Lightweight hexagonal architecture + architecture test | accepted |
| [0018](0018-cache-stat-checkpoints.md) | Checkpoint stat cache trusts stat data, as git's index | accepted |
