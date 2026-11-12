# Use ship-it with AI coding agents

An AI agent can deliver your work through ship instead of typing git commands. It gets the same plan, checks and undo as you, with stricter safeguards. ship talks to agents through `ship mcp`, a Model Context Protocol server over stdio that offers 11 tools.

## Declare ship-it to your agent

`ship mcp install` adds ship-it to the project MCP file of one client. It shows the file after the change, asks before writing, and keeps everything already there: your other servers, the order of keys, and in TOML every comment. A ship-it server you declared yourself (another path, other arguments) is left as it is. `ship undo` puts the file back as it was, and removes it if ship created it.

```sh
ship mcp install --client <claude|vscode|cursor|codex>   # claude when omitted
ship mcp install --client cursor --dry-run               # show the plan, write nothing
ship mcp install --client codex --print                  # print the snippet only
```

Every file written is a project file, meant to be committed so the whole team gets the server. That is why it launches `ship` from the `PATH` and never holds the absolute path of your binary: that path would only exist on your machine. Install ship so that `ship` is on the `PATH` of everyone who opens the project, or edit the file by hand on a machine where it is not.

`--print` writes nothing and needs no repository: use it to configure a client by hand, or a client not listed here.

### Claude Code

```sh
ship mcp install
```

It writes `.mcp.json` at the root of the repository ([Claude Code docs](https://code.claude.com/docs/en/mcp), project scope):

```json
{
  "mcpServers": {
    "ship-it": {
      "type": "stdio",
      "command": "ship",
      "args": [
        "mcp"
      ]
    }
  }
}
```

Claude Code asks you to approve a project server the first time it starts it.

### VS Code and GitHub Copilot agent mode

```sh
ship mcp install --client vscode
```

It writes `.vscode/mcp.json` ([VS Code docs](https://code.visualstudio.com/docs/copilot/reference/mcp-configuration)). VS Code names the object `servers`, not `mcpServers`:

```json
{
  "servers": {
    "ship-it": {
      "type": "stdio",
      "command": "ship",
      "args": [
        "mcp"
      ]
    }
  }
}
```

The ship-it extension (`extensions/vscode`, not published yet) already registers ship's server for the agents of VS Code: with it, you do not need this file.

### Cursor

```sh
ship mcp install --client cursor
```

It writes `.cursor/mcp.json` ([Cursor docs](https://cursor.com/docs/context/mcp)):

```json
{
  "mcpServers": {
    "ship-it": {
      "type": "stdio",
      "command": "ship",
      "args": [
        "mcp"
      ]
    }
  }
}
```

### OpenAI Codex CLI

```sh
ship mcp install --client codex
```

It writes `.codex/config.toml` ([Codex docs](https://developers.openai.com/codex/mcp)). Codex reads a project file only in a project you trust.

```toml
[mcp_servers.ship-it]
command = "ship"
args = ["mcp"]
```

Go has no TOML parser in its standard library, and ship adds no dependency for one. So ship never rewrites this file: it only appends the table above at the end, which keeps your settings and comments byte for byte. When appending could clash with what the file holds (`mcp_servers` written as an inline table, ship-it named in a dotted key or a comment), it writes nothing, stops with `SHIP-E0715` (exit code 2) and prints the three lines to add by hand. A JSON file it cannot read, such as one with comments, gets the same treatment with `SHIP-E0701`.

### Any other MCP client

Run `ship mcp` as a stdio MCP server, from the repository you work in. It speaks MCP 2026-07-28 and the earlier revisions 2025-11-25 and 2025-06-18. `ship mcp install --print` gives a starting point.

## The tools

| Tool | Arguments | What it returns |
|---|---|---|
| `ship_status` | `offline` | Where the work stands: branch, uncommitted files, ahead and behind the default branch, the open pull request, what `ship_undo` would revert (`undoable`) and the one next action (`next`). Read-only. |
| `ship_plan` | `only`, `issue` | What `ship_deliver` would do: conventional title, branch, files, the reasons for each choice, secrets found, the allowed types when the project's commitlint refuses the planned one (`type_choices`), and a split suggestion when the change is large (`size`). Changes nothing. |
| `ship_deliver` | `title`, `branch`, `only`, `issue`, `why`, `tested`, `dryRun` | A checkpoint, a dedicated branch when on the default branch, then one conventional commit. Returns the operation id, checkpoint, branch, whether it created the branch, commit and title. Nothing is pushed. |
| `ship_publish` | `issue`, `dryRun` | Runs the project's checks, updates the branch on the default branch, pushes it and opens a draft pull request, or updates the open one. Returns the pull request, the checks (`gates`), the labels and the warnings. |
| `ship_undo` | `allowRemote`, `dryRun` | Reverts the last local operation of ship-it, keeping your files, and returns what it undid. A publication is refused (see below). |
| `ship_log` | none | The latest operations of ship-it in this repository, newest first, with the one `ship_undo` would revert, and the agent that made each one. |
| `ship_doctor` | none | Every problem that would stop a delivery (`findings`), each with the command that fixes it, and the worst one (`worst`). |
| `ship_config_get` | `key` | The effective value of one setting and the layer it comes from (`source`, `path`). |
| `ship_config_explain` | `key` | What one setting does, its type, allowed values and default, its `SHIP_*` variable, the value every layer gives it and why the effective one wins. |
| `ship_stack` | `layers`, `local`, `dryRun` | Splits the work into layers by area (or the `layers` given, bottom first), commits each on its own branch based on the one below, then pushes them and opens one draft pull request per layer, grouped in a GitHub stack (a classic chain where the server has none). Run it from the default branch. |
| `ship_stack_status` | `offline` | The layers of the stack the current branch belongs to, bottom first, with their pull requests and checks. Read-only. |

`only` is a list of files or directories to deliver, the rest staying uncommitted. `issue` is the issue the work closes, read from the branch name when omitted. `dryRun` runs every check and returns the same result with the planned steps, changing nothing. Every tool declares an `outputSchema`, generated from the same Go types as `--json`: the result is in `structuredContent`, and the same JSON is in the text content for older clients.

## How the agent passes "why" and "tested"

`ship_deliver` takes them as arguments, as you would type them in the review screen:

```json
{
  "title": "fix(cart): handle an empty cart",
  "why": "Checkout crashed when the last item was removed.",
  "tested": "go test ./internal/cart/..."
}
```

`why` becomes the commit body and the summary of the pull request. `tested` becomes a `Tested:` trailer. Both say only what is true: the tool descriptions ask the model to state facts the user or the code gave, and to leave `tested` empty rather than guess. A title or branch the agent writes is validated like yours; without them, the deterministic plan decides. When the agent writes the title or the branch, the pull request says the plan was drafted with AI assistance, in the way your repository asks (its template, its label, or one sentence).

## When a check refuses the work

A refusal is a tool execution error: the result has `"isError": true` and one text content that starts with the error code, so the model can read it and act. Nothing is half done.

A secret in the changes stops `ship_deliver` before any commit. Each finding gives the file, the line, the rule, a masked preview and the fingerprint to allow it with:

```text
SHIP-E0901 1 secret in your changes: nothing was committed. Move each value to an ignored file (.env.local) or an environment variable; a false positive: add the fingerprint shown to ship.secrets.allow, or its file to ship.secrets.ignorePaths.
- config/deploy.env:3 aws_key AKIA…LE (fingerprint 4f1c2a9e0b7d3c6f8a5e2d1b0c9f7e6a5d4c3b2a1f0e9d8c7b6a5f4e3d2c1b0a)
```

`ship_publish` scans the commits it would push too (`SHIP-E0901` again, with the base to clean the history from). A red check stops `ship_publish` before the push. The message names the check, then gives the end of its output:

```text
SHIP-E0902 test is red (pnpm run test): nothing was pushed, your commit stays local. Fix it, then ship publish.
FAIL src/cart.test.ts
  ✕ handles an empty cart (4 ms)
```

The agent fixes the code, delivers again and publishes again. An agent cannot skip a check or allow a secret: both are settings, and settings belong to you.

## What an agent may not do

These limits hold for the MCP tools. Claude Code, Codex, Cursor and most agents can also run commands in a shell, and from there they could call `ship undo --yes`, `ship stack merge --yes`, `ship config set` or `git push` like you. Keep those commands behind your agent's permission prompts (or deny them in its settings), and let GitHub rulesets protect your default branch: ship's limits are a guardrail for the agent's normal path, not a sandbox.

- Its pull requests are always drafts. Nobody reviewed the plan of an agent before it ran, so the human gesture is to mark the pull request ready for review ([ADR-0010](../adr/0010-surfaces.md)).
- It may not undo a publication. Undoing one closes a pull request and deletes a remote branch: other people may have seen, reviewed or built on them. `ship_undo` answers `SHIP-E0407` and leaves it to you (`ship undo` in a terminal). `allowRemote` is honored only by the server the VS Code extension starts (`ship mcp --attended`), after you confirmed in VS Code.
- It may not merge a stack. `ship_stack` opens draft pull requests and `ship_stack_status` reads them, but merging stays a human gesture (`ship stack merge` in a terminal).
- It may not change settings. `ship_config_get` and `ship_config_explain` let it understand why ship behaves as it does; changing a setting is `ship config set`, in your terminal. There is no tool for it, so a model cannot turn a check off to get its work through.
- It may not weaken the secret scan. Its exemptions (`ignorePaths`, `disabledRules`, `allow`) count only as `.github/ship.json` holds them in the last commit: a change the agent makes to the file, a user file, a `SHIP_*` variable or `--set` cannot turn them on.
