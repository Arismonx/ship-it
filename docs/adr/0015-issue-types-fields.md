# ADR-0015: Issue Types / Fields, falling back to labels

**Status:** accepted (user choice)

## Decision
- Organisation: **Issue Types** (task, bug, feature, or the organisation's own) + **Issue Fields** (Priority, Effort, dates). GA on July 2, 2026.
- Personal repository: a minimal set of prefixed labels (`type:*`, `priority:p0-p3`, `status:*`), without emoji, modeled on the native types and fields to ease a later move to an organisation.
- PRs get no type label: the emoji title carries it.

## To verify ❓
Actual availability of types and fields on personal repositories; REST/GraphQL write endpoints; behavior of types on PRs.
