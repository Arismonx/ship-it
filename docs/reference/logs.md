# Logs, errors and bug reports

> This page describes what is implemented; what is not yet is marked planned.
> Sources: [clig.dev](https://clig.dev/), [log/slog](https://pkg.go.dev/log/slog), [OpenTelemetry Logs Data Model](https://opentelemetry.io/docs/specs/otel/logs/data-model/), [W3C Trace Context](https://www.w3.org/TR/trace-context/), [XDG Base Directory](https://specifications.freedesktop.org/basedir/latest/), [Flight Recorder in Go 1.25](https://go.dev/blog/flight-recorder), [os.UserCacheDir](https://pkg.go.dev/os#UserCacheDir), [creating an issue from a URL](https://docs.github.com/en/issues/tracking-your-work-with-issues/using-issues/creating-an-issue), MCP specification 2026-07-28, [Key Changes](https://modelcontextprotocol.io/specification/2026-07-28/changelog), minor change 2 and [SEP-414](https://modelcontextprotocol.io/seps/414-request-meta) (`traceparent` in `_meta`), read on 2026-10-23.

Nothing in this page ever leaves the machine: there is no telemetry.

## 1. Three flows, one business object

| Flow | Destination | Content | Rules |
|---|---|---|---|
| Result | stdout | Useful output; `--json` for machines | Never logs here (clig.dev: stdout is meant to be piped) |
| Status | stderr | Human messages, translated (i18n), progress, explained errors | No `ERR`/`WARN` level prefixes in normal mode (clig.dev) |
| Journal | JSONL file | Structured technical events (`log/slog`, `JSONHandler`) | INFO and above always written; DEBUG goes to the black box |
| *Oplog* (business) | `.git/ship/oplog/` | Operations and views for undo | Not a log: it is state, see ADR-0005 |

Global flags, accepted anywhere before `--` (`ship -d publish` and `ship publish -d` are the same):

| Flag | Effect |
|---|---|
| `-q`, `--quiet` | stderr keeps only errors, warnings, questions and what a human must do; progress and success messages are silenced |
| `-d`, `--debug` | the journal, DEBUG included, is also echoed on stderr (`key=value` text, without time and run id) |
| `--trace` | at the end, the timeline of the run's steps (offset, duration, step, details) on stderr |

No `-v`: clig.dev notes it is ambiguous between verbose and version. The short flags `-d` and `-q` are reserved: no subcommand may define them, otherwise `ship <command> -d` would change meaning (rule in [cli](cli.md); `TestNoCommandDefinesAGlobalShortFlag` asks every flag set for its flags and fails if one defines them).

## 2. The black box (flight recorder)

*Justified innovation*: clig.dev recommends writing the full log to a file on unexpected errors and showing logs only then. Go 1.25 applies the same idea to execution traces (`runtime/trace.FlightRecorder`: an in-memory ring buffer captured when the problem is detected). ship-it applies it to logs:

- every record of the run, DEBUG included, goes to a bounded in-memory ring (`ship.logs.blackBoxBytes`, default 1 MiB, at least 64 KiB; the oldest lines are dropped first);
- success → nothing more than the INFO journal;
- unexpected failure (exit code 1) or recovered panic → the ring is written to `<logs>/<run_id>.blackbox.jsonl` and the error message on stderr points to it (`cli.blackbox.saved`, English and French). A panic is recovered, journaled with its stack (scrubbed), and reported as an internal error (`cli.error.panic`) instead of a Go stack trace;
- expected refusals (red gate, policy, a human needed, usage error, nothing to undo…) are not failures of ship-it: no black box;
- under MCP (`ship mcp`), a panicking request becomes a JSON-RPC internal error naming the black box, and the server keeps serving;
- `ship bug` reads it to build a bug report (§9).

Result: no more "run it again with --debug and send me the output".

## 3. Record format (OpenTelemetry compatible)

```json
{"time":"2026-10-23T09:12:03.418Z","level":"INFO","msg":"step.done",
 "run_id":"01JB…","step":"publish/push","duration_ms":812,"lease":false,
 "trace_id":"0af7…","span_id":"b7ad…","parent_span_id":"00f0…",
 "repo_hash":"hmac-sha256:9c1e…","severity_number":9}
```

- `severity_number` follows OTel: TRACE 1-4, DEBUG 5-8, INFO 9-12, WARN 13-16, ERROR 17-20; slog levels: Debug −4, Info 0, Warn 4, Error 8.
- `msg` is a stable key, never a translated sentence; the status flow is what gets translated.
- `run_id` (one per invocation, ULID layout: time-ordered) is on every line. `op_id` is on the step that created an oplog entry (deliver, publish, undo, tidy, fix): it is only known once the operation is written, at the end of the step.
- `step` and `duration_ms` are on every timed record. Steps end with their record, so a step's start is `time − duration_ms`.
- `trace_id`/`span_id`/`parent_span_id`: W3C, see §6.
- `repo_hash` identifies the repository (§4).

### Message keys

| `msg` | Level | `step` | Other attributes |
|---|---|---|---|
| `run.start` | INFO | | `command` (name only, never its arguments), `version`, `os`, `arch`, `quiet`, `debug`, `plain` (`--plain` or `SHIP_ACCESSIBLE=1`, docs/reference/cli.md) |
| `run.end` | INFO | | `command`, `exit_code`, `duration_ms` |
| `run.fault` | WARN | | `code` (`SHIP-E####`), `key` |
| `run.error` | ERROR | | `error` (an error ship-it did not expect) |
| `run.panic` | ERROR | | `panic`, `stack` |
| `step.done` / `step.failed` | INFO / WARN | `deliver`, `publish`, `publish/fetch`, `publish/rebase`, `publish/push`, `publish/pull_request`, `undo`, `tidy`, `fix`, `gc` | `op_id`, `error` when failed, step-specific flags (`lease`, `draft`, `conflict`, `new_branch`, `undone`, `kind`, `removed`, `dry_run`) |
| `gate.done` | INFO, WARN when not passed | `gate/<name>` | `gate`, `cmd` (the project script, never its output), `status` |
| `git.exec` | DEBUG | `git` | `cmd` (arguments, configuration values redacted), `exit_code`, `detail` (last stderr line, on failure) |
| `github.request` | INFO, WARN on error | `github` | `method`, `path` (route template: `/repos/{owner}/{repo}/pulls/{number}`), `status`, `rate_remaining` |
| `mcp.request` | INFO, WARN on error | `mcp/<method>[/<tool>]` | `rpc_code`, `error` |
| `logs.pruned` / `logs.prune_failed` | DEBUG / WARN | | `removed_runs`, `freed_bytes` / `error` |

A dry run (`--dry-run`, docs/reference/cli.md) is journaled like a real run, so that `ship trace` explains it too, but every `step.done` / `step.failed` record carries `dry_run: true`: a `publish/push` step of a dry run only planned the push. Its `op_id` is absent, since nothing is logged in the operation log, and no `gate.done` record appears: the gates are listed, not run.

Keys never start with an i18n namespace (`cli.`, `publish.`…): the i18n usage test would read them as message keys.

## 4. Privacy (redaction)

Two layers, both tested:

1. At the source: what is never logged at all:
   - file contents, diffs, commit messages (git's stdin and stdout are never logged), gate output, request and response bodies;
   - tokens: `obs.Secret` and `ports.Token` render `[REDACTED]` through `slog.LogValuer` and every `fmt` verb; the GitHub client never logs headers;
   - `git config <key> <value>` and `git -c key=value` values are replaced by `[REDACTED]` (`gitexec.LogArgs`): they may hold `http.extraheader` or an e-mail;
   - GitHub paths are logged as route templates, so owner, repository, branch and label names stay out; transport errors are unwrapped from `*url.Error`, which repeats the URL;
   - the command is logged by name (`check title`), never with its arguments (titles, paths).
2. On every record: a scrubbing handler in front of every sink (file, black box, `--debug` echo) rewrites every string value, error, string list and message:
   - the repository top level becomes `.`, its git directory `$GIT_DIR`, the home directory `~`, the temporary directory `$TMP` (longest path first; native and `/` spellings);
   - remaining absolute paths (`C:\…`, `/home/…`, `/Users/…`, `/tmp/…`, `/var/…`…) become `[PATH]`;
   - e-mail addresses become `[EMAIL]`, URL credentials `scheme://[REDACTED]@`, GitHub tokens (`ghp_`, `gho_`, `ghu_`, `ghs_`, `ghr_`, `github_pat_`) and `Bearer`/`token`/`Basic` values `[REDACTED]`;
   - any other value type is logged as its scrubbed text, so a struct cannot smuggle a field out through JSON marshalling.
- `repo_hash` = `hmac-sha256:` + the first 16 hex digits of the HMAC-SHA256 of the top-level path, keyed by a per-installation salt: 32 random bytes (`crypto/rand`) created on first use in `<user config dir>/install-salt` (mode `0600`). It tells runs of the same clone apart without naming it, and nobody without the salt can test likely paths against it. The file is created with `O_EXCL`, so concurrent first runs agree on one salt (the losers read the winner's); a file left half-written by a crashed run is replaced after 5 s. Without a salt (unreadable config dir) there is no `repo_hash`, never an unsalted one. Deleting the file only changes future hashes.
- Tests: `obs` unit tests and the `FuzzScrub` fuzz target (an e-mail, a token, a URL password and a home path surrounded by arbitrary text never survive), adapter tests (no token, owner, repository, branch, title, file content or configured e-mail in the records), and the end-to-end test of `cmd/ship` (a real repository with a credential in its remote URL: no absolute path, e-mail, password or file content in the journal or the black box).
- Log files are created with mode `0600` in directories with mode `0700`.

## 5. Locations and retention

| Context | Directory |
|---|---|
| Inside a repository (so that client repositories stay apart) | `<git dir>/ship/logs/` (a linked worktree has its own) |
| Outside, Linux and others | `$XDG_STATE_HOME/ship-it/logs`, default `~/.local/state/ship-it/logs` (XDG: "actions history (logs, history…)") |
| Outside, Windows | `%LocalAppData%\ship-it\logs` (`os.UserCacheDir` = `%LocalAppData%`) |
| Outside, macOS | `~/Library/Logs/ship-it` (a convention, not an Apple API) |

Go has no `UserStateDir`: `internal/platform` computes these paths (`Environment.RunLogDir`, tested for each OS with a simulated environment). Records logged before the directory is known are buffered (256 KiB at most) and written once it is. When the directory cannot be written, the journal stays in memory: logging never stops the work.

Files of a run: `<run_id>.jsonl` (INFO journal) and, after a failure, `<run_id>.blackbox.jsonl`.

Retention: the last `ship.logs.keepRuns` runs (default 20, at least 1) within `ship.logs.maxBytes` per directory (default 10 MiB, at least 1 MiB), newest first; once a limit is reached every older run goes. The current run is never removed. Files whose name is not a run id are never touched. The three `ship.logs.*` keys are ordinary settings (user file, `.github/ship.json`, organisation policy, environment such as `SHIP_LOGS_KEEPRUNS`; JSON Schema generated); a configuration that cannot be read falls back to the defaults, because logging never stops the work. Applied:

- at the end of every run (cheap: one directory listing), except `ship gc`;
- by `ship gc`, which reports it and honours `--dry-run` (`"logs": {"removed_runs", "freed_bytes"}` in `--json`).


## 6. MCP: trace context propagation

The MCP specification 2026-07-28 (SEP-414) reserves the unprefixed `_meta` keys `traceparent`, `tracestate` and `baggage` for W3C Trace Context and Baggage; namespacing them would break trace correlation. ship-it reads `params._meta.traceparent` on every request (both protocol eras):

- the value is validated as the W3C specification requires (lengths, lower-case hex, no all-zero ids, version `ff` refused; a future version may carry more fields);
- `trace_id` = the caller's trace; `parent_span_id` = the caller's span (`parent-id`); `span_id` = a new span for ship-it's work on that request. This is the OTel meaning of a log record's `span_id` (the span active when it was written), and keeps the caller's id visible for correlation;
- every record logged while the request runs (`mcp.request`, use case steps, git commands, GitHub requests) carries them, through the request context.

`tracestate` and `baggage` are not read (nothing in ship-it uses them yet).

## 7. `ship trace`: a run's timeline

*Justified innovation*: the journal is JSONL for machines; humans asking "what took so long?" or "what did it do before failing?" should not need `jq`. `ship trace` reads a past run back and prints its steps as a timeline (offset from the first step, duration, step, details; `✗` marks a failed step):

```
Run 01M5N78ZF8TNCD09QHWD8YJ558: 5 steps
  +   0.000s      66ms  git                   cmd=status --porcelain=v2 --branch -z --untracked-files=all exit_code=0
  +   0.412s     812ms  publish/push
✗ +   1.300s     230ms  github                method=POST path=/repos/{owner}/{repo}/pulls status=422
```

- `ship trace`: the last run before this one; `ship trace <run_id>` (or its last 6+ characters); `ship trace --list`: the runs kept, with their size and whether a black box was saved; `--json` for machines.
- It prefers the black box when there is one (it has the DEBUG details), otherwise the INFO journal.
- `--trace` prints the same timeline for the current run, from its black box, at the end.

## 8. Stable error codes

- Format `SHIP-E####` (e.g. `SHIP-E0042`), one code per playbook situation ([error-playbook](error-playbook.md)), in `internal/core/fault`; the message shows the code. A fault is journaled as `run.fault` with its code.
- Planned: `ship help SHIP-E0042`. The [error reference](errors.md) is already generated from the code.
- Codes are never reused; a retired code stays documented as obsolete.
- Process exit codes: unchanged ([cli](cli.md)).

## 9. Bug reports (no telemetry)

ship sends nothing by itself. `ship bug [<run>] [--print]` builds the report of a past run, by default the last that failed (it has a black box), else the last run:

| Step | What happens |
|---|---|
| Report | Built from the run's journal only, already scrubbed: the command, ship's version, the platform, the exit code, the error (a fault's code and key, or the error, or a panic and its stack) and the timeline of `ship trace`. The git version is read now. Written to `<logs>/<run_id>.report.md`, pruned with the run, and printed for you to read |
| Address | `https://github.com/GregoireF/ship-it/issues/new?template=1-bug-report.yml&…`: the text fields `ship-version`, `git-version`, `what-happened` and `trace` are prefilled (a field's `id` is its URL parameter, per [GitHub's form schema](https://docs.github.com/en/communities/using-templates-to-encourage-useful-issues-and-pull-requests/syntax-for-githubs-form-schema)). The dropdowns are left to you. The timeline stays in the file, never in the address, which GitHub limits (`414 URI Too Long`) |
| Sending | After `[Y/n]` in a terminal, ship opens the address in your default browser (`rundll32 url.dll,FileProtocolHandler`, `open` or `xdg-open`, never a shell). You attach the file and submit the issue yourself. `--print` only prints |

Planned: a "ship-it: Report a bug" command in VS Code, and a tool for agents that prepares the report while a human sends it.

## 10. Where it lives in the code

| Package | Role |
|---|---|
| `internal/obs` | Journal (`NewJournal`, `Attach`, `SaveBlackBox`, `EchoTo`, `SetLimits`), black box, scrubber, installation salt and `RepoHash`, W3C trace context, retention (`Prune`, `ListRuns`), timeline reader (`ReadEvents`, `Steps`, `WriteTimeline`) |
| `internal/platform` | Log directories per OS (`RunLogDir`, `LogDir`, `RepositoryLogDir`, `TempDir`) and the salt file (`SaltFile`) |
| `internal/core/config` | `ship.logs.keepRuns`, `ship.logs.maxBytes`, `ship.logs.blackBoxBytes` |
| `internal/ports` | `RunLogs` (pruning for `ship gc`) |
| `internal/app` | `RunLogLimits` (reads `ship.logs`), steps of the use cases, through the injected `Workspace.Log *slog.Logger` (app never imports `obs`) |
| `internal/adapters/gitexec`, `…/github` | `git.exec` and `github.request`, through their `Log *slog.Logger` field |
| `internal/surface/cli` | Global flags, run start and end, black box on failure, panic recovery, `ship trace` |
| `internal/surface/mcp` | `traceparent`, `mcp.request`, panic recovery |
| `cmd/ship` | Opens the journal (`openJournal`) and injects its logger everywhere |
