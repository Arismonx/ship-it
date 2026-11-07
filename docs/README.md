# Documentation

The documentation follows [Diátaxis](https://diataxis.fr/): tutorials to learn, how-to guides to get a task done, reference to look things up, explanation to understand. How we write it: [STYLE.md](STYLE.md). Unknown words: [glossary](glossary.md).

## Tutorials

For your first steps; no experience with git needed.

- [Your first pull request](tutorials/first-pull-request.md)
- [Undo a mistake](tutorials/undo-a-mistake.md)

## How-to guides

For a task you already know you need.

- [Install, update and remove ship-it](how-to/install.md)
- [Deliver only part of your work](how-to/deliver-part-of-your-work.md)
- [Ship your work as a stack of pull requests](how-to/stack-pull-requests.md)
- [Configure ship-it for your team](how-to/configure-your-team.md)
- [Work on client repositories](how-to/client-repositories.md)
- [Plan your project with ROADMAP.md](how-to/plan-with-a-roadmap.md)
- [Use ship-it with AI coding agents](how-to/use-with-ai-agents.md)

## Reference

- [Commands, flags and exit codes](reference/cli.md)
- [GitHub access](reference/github-access.md): the token ship uses, every API request and the permission it needs
- [Settings](reference/settings.md): every key, its type, default and environment variable (generated)
- [Error codes](reference/errors.md): every `SHIP-E####` code, the message ship prints and how to fix it (generated)
- [Generated conventions](reference/conventions.md): titles, gitmoji, branches, pull request bodies, AI disclosure
- [Gates](reference/gates.md): the checks run before a push, per ecosystem
- [Error playbook](reference/error-playbook.md): common git and GitHub situations and what ship does
- [Logs and reports](reference/logs.md): journal, black box, `ship trace`

## Explanation

- [How ship-it compares to GitHub's own tools](explanation/compared-to-github.md): task by task, with the trade-offs
- [How a delivery works](explanation/how-a-delivery-works.md): every step of `ship`, in order
- [The safety net](explanation/safety-net.md): checkpoints, the operation log, what can be undone
- [Configuration layers](reference/configuration.md): files, precedence, presets, overrides, contexts, policy
- [Architecture](../ARCHITECTURE.md) and [architecture decisions](adr/)

## For maintainers

How ship-it was designed, for contributors.

| Document | What it covers |
|---|---|
| [Vision](design/vision.md) | Principles, personas, non-goals |
| [Architecture](design/architecture.md) | Modules, flows, protocols |
| [Repository architecture](design/architecture-repo.md) | Tree, single schema, dogfooding |
| [Quality & tests](design/quality-and-tests.md) | Test pyramid, multi-OS, fuzzing, benchmarks |
| [Performance](design/performance.md) | Measured numbers, budgets checked in CI, where the time goes |
| [Undo, storage, optimisations](design/undo-storage-optimizations.md) | Checkpoints, remote effects, git and GitHub optimisations |
| [GitHub automation](design/github-automation.md) | Tiered setup, settings as code, governance, roadmap sync |
| [GitHub frictions](design/github-frictions.md) | Documented developer frictions and ship-it's answers |
| [VS Code extension](design/vscode-extension.md) | Entry points, review, undo timeline, MCP provider |
| [git & GitHub standards](design/git-github-standards.md) | Commits, branches, tags, labels, issues, pull requests, reviews |
| [Compliance](design/compliance.md) · [CRA commitments](design/cra-commitments.md) | Sovereignty, licence, signing, SBOM, Cyber Resilience Act |
| [Roadmap work log](design/roadmap-log.md) | The detailed history of the work, phase by phase, with exit criteria |
| [Workflows](design/workflows.md) | ship-it's own CI, release, supply chain, "going public" checklist |

