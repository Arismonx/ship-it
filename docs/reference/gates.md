# Gates: the project's checks before a push

Before pushing, ship-it runs the checks the project declares, cheapest first, and stops at the first red one: the work stays committed locally (undoable), nothing reaches GitHub. `ship check gates` runs the same checks now, without committing or pushing, and a repository can ask for them before every commit with `ship.gates.beforeCommit`; a red check then refuses the commit.

**The rule:** ship-it runs what the project declares, never a tool it would impose. A formatter or linter runs when its configuration is in the repository; a test runner when the ecosystem's standard tool runs tests by itself, or when the project declares it. Several ecosystems in one repository (a Go API and a Node front end) each run theirs, in the order of the table.

| Ecosystem | Detected by (repository root) | Gates, in order |
|---|---|---|
| Node | `package.json` | its scripts `lint`, `typecheck`, `test`, with the package manager of `packageManager` or the lockfile; Vitest runs only the tests affected since the base (`--changed`) |
| Go | `go.mod` | `gofmt -l` on the changed `.go` files (any output fails), `go vet ./...`, `go test ./...` |
| Rust | `Cargo.toml` | `cargo fmt --all -- --check`, `cargo clippy --all-targets` (errors only), `cargo test`; rustup's default profile installs rustfmt and clippy |
| Python | `pyproject.toml` sections, or the tools' own files | `ruff check .` (`[tool.ruff]`, `ruff.toml`), `ruff format --check .` (`[tool.ruff.format]`), `black --check .` (`[tool.black]`), `mypy .` (`[tool.mypy]`, `mypy.ini`), `pytest` (`[tool.pytest.ini_options]`, `pytest.ini`, `conftest.py`); through `uv run --frozen`, `poetry run` or `pdm run` when their lockfile is there |
| PHP | `composer.json` scripts | `composer run-script` of `lint`, `analyse`, `phpstan`, `test` (those defined) |
| Ruby | `Gemfile` | `bundle exec rubocop` (`.rubocop.yml`), `bundle exec rspec` (`.rspec`) |
| JVM | `build.gradle(.kts)`, `gradlew`; `pom.xml` | `gradle check` or `mvn --batch-mode verify`, through `./gradlew` / `./mvnw` (`.bat` / `.cmd` on Windows) when the project has a wrapper |
| .NET | one `.sln`/`.slnx`, or one project file | `dotnet format <it> --verify-no-changes`, `dotnet test <it>`; several solutions: no guess, no gate |

- Commands run without a shell, at the repository root, with no standard input, each bounded to 15 minutes.
- A declared tool that is not installed stops the push (`SHIP-E0903`): ship-it never pretends a check passed.
- `--skip-gates` is written in the pull request ("Local checks were skipped… only CI vouches for this change"); `ship.gates.allowSkip: false` (preset `ship-it:strict`) refuses it; `ship.gates.enabled: false` turns the gates off, for example for documentation-only deliveries through an override ([configuration](configuration.md#per-path-overrides)).
- The commands that passed are listed in the pull request's Testing section ([conventions](conventions.md#pull-request-body)).

Planned: custom gates declared in `.github/ship.json`, monorepo sub-projects, Swift and Dart.
