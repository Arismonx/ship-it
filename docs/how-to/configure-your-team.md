# Configure ship-it for your team

ship works without configuration. Use this guide when your team wants different defaults. Every key is in the [settings reference](../reference/settings.md); how layers combine is in [the configuration guide](../reference/configuration.md).

## 1. Create the repository file

```sh
ship init
```

It writes `.github/ship.json` (it never overwrites one) with the `area/*` labels found in your commit history. Commit it like any other file. Comments are allowed in it (JSONC), and editors complete the keys through its `$schema`.

## 2. Start from a preset

```jsonc
{
  "$schema": "https://raw.githubusercontent.com/GregoireF/ship-it/main/schemas/ship.schema.json",
  "extends": ["ship-it:strict"]
}
```

- `ship-it:strict`: pull requests up to 250 lines, drafts when nobody reviewed the plan, labels, and checks that cannot be skipped.
- `ship-it:agency`: for a client's repository. It commits locally and publishes after review, uses `Refs #n` so that the client's issues stay open, and leaves the client's labels alone.

A team can also share its own preset from another repository, pinned to a commit: `"github:my-team/ship-config/base.json@<full commit sha>"`.

## 3. Change one setting

```sh
ship config set ship.pullRequest.maxLines 300
ship config explain ship.pullRequest.maxLines   # what it does, where each value comes from
```

`ship config set` keeps your comments and the order of the file, and `ship undo` reverts it.

## 4. Treat part of the repository differently

```jsonc
{
  "overrides": [
    { "paths": ["docs/**", "*.md"], "ship": { "gates": { "enabled": false } } }
  ]
}
```

An override applies when **every** delivered file matches: a documentation-only change skips the checks, a change that also touches code does not.

## 5. Let the project's own rules lead

You rarely need to configure conventions. ship follows what the repository already has: its pull request template, its commitlint configuration, its GitHub rulesets, the gitmoji habit of its history. Set `ship.commits.gitmoji` only to decide explicitly.

## 6. Impose settings on every repository (IT departments)

An organisation policy file, deployed where developers cannot write, locks keys for everyone; a file that contradicts it is refused with exit code 5. See [the organisation policy](../reference/cli.md#organisation-policy).
