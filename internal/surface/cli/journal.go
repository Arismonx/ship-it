// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/GregoireF/ship-it/internal/app"
	"github.com/GregoireF/ship-it/internal/i18n"
	"github.com/GregoireF/ship-it/internal/obs"
)

// globalFlags apply to every command (docs/reference/logs.md §1).
type globalFlags struct {
	quiet bool // -q, --quiet: only errors, warnings and questions on stderr
	debug bool // -d, --debug: also echo the journal, DEBUG included, on stderr
	trace bool // --trace: the timeline of the run's steps on stderr at the end
	// --plain: no full-screen interface, line-by-line prompts (screen
	// readers); SHIP_ACCESSIBLE=1 does the same (docs/reference/cli.md).
	plain bool
	// set holds the --set key=value assignments: settings for this run only.
	set []string
}

// parseGlobalFlags removes the global flags from args, wherever they are
// before a "--" terminator, so that `ship -d publish` and `ship publish -d`
// both work.
func parseGlobalFlags(args []string) (globalFlags, []string) {
	var g globalFlags
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			rest = append(rest, args[i:]...)
			break
		}
		if value, ok := strings.CutPrefix(arg, "--set="); ok {
			g.set = append(g.set, value)
			continue
		}
		switch arg {
		case "--set":
			if i+1 < len(args) {
				i++
				g.set = append(g.set, args[i])
			} else {
				g.set = append(g.set, "") // reported as a malformed assignment
			}
		case "-q", "--quiet":
			g.quiet = true
		case "-d", "--debug":
			g.debug = true
		case "--trace":
			g.trace = true
		case "--plain":
			g.plain = true
		default:
			rest = append(rest, arg)
		}
	}
	return g, rest
}

// commandName names the command for the journal without its arguments,
// which may hold titles or paths: "publish", "checkpoint restore", "ship".
func commandName(args []string) string {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return "ship"
	}
	name := args[0]
	if len(args) > 1 && !strings.HasPrefix(args[1], "-") {
		switch name {
		case "checkpoint", "check", "mcp", "config":
			name += " " + args[1]
		}
	}
	return name
}

// opportunisticPrune keeps the log directory within the retention limits
// after each run; `ship gc` does it explicitly (and honours --dry-run).
func (r runner) opportunisticPrune(command string) {
	if command == "gc" {
		return
	}
	if dir := r.journal.Dir(); dir != "" {
		if report, err := obs.Prune(dir, r.journal.RunID, r.journal.Retention(), false); err != nil {
			r.journal.Logger.Warn("logs.prune_failed", "error", err)
		} else if len(report.Removed) > 0 {
			r.journal.Logger.Debug("logs.pruned", "removed_runs", len(report.Removed), "freed_bytes", report.FreedBytes)
		}
	}
}

// finish closes a run: its end in the journal, the black box on disk when
// it failed, and the timeline with --trace.
func (r runner) finish(command string, code int, panicked any) int {
	if panicked != nil {
		r.journal.Logger.Error("run.panic", "panic", fmt.Sprint(panicked), "stack", string(debug.Stack()))
		r.problem(i18n.Render(r.lang, "cli.error.panic", map[string]string{"panic": fmt.Sprint(panicked)}))
		code = ExitError
	}
	r.journal.Logger.Info("run.end", "command", command, "exit_code", code,
		obs.KeyDurationMS, r.journal.Elapsed().Milliseconds())
	// An expected refusal (gate, policy, a human needed, usage, a known
	// fault such as an invalid title) is not a failure of ship-it: only
	// unexpected errors and panics keep the black box.
	if code == ExitError && (panicked != nil || r.journal.Unexpected()) {
		if path, err := r.journal.SaveBlackBox(); err == nil && path != "" {
			r.problem(i18n.Render(r.lang, "cli.blackbox.saved", map[string]string{"path": displayPath(path), "run": string(r.journal.RunID)}))
		}
	}
	if r.global.trace {
		r.printTimeline(r.streams.Err, r.journal.Events(), string(r.journal.RunID))
	}
	r.opportunisticPrune(command)
	return code
}

// displayPath shows a log file relative to the working directory when it is
// below it (.git/ship/logs/…), so that it is short and can be copied.
func displayPath(path string) string {
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, path); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	return path
}

func (r runner) printTimeline(w interface{ Write([]byte) (int, error) }, events []obs.Event, run string) {
	steps := obs.Steps(events)
	if len(steps) == 0 {
		fmt.Fprintln(w, i18n.Render(r.lang, "cli.trace.empty", map[string]string{"run": run}))
		return
	}
	fmt.Fprintln(w, i18n.Render(r.lang, "cli.trace.header", map[string]string{"run": run, "steps": strconv.Itoa(len(steps))}))
	_ = obs.WriteTimeline(w, steps)
}

// trace shows the timeline of a past run from its journal: the black box
// when the run failed (it has the DEBUG details), the INFO journal otherwise.
func (r runner) trace(args []string) int {
	fs, asJSON := r.flags("trace")
	list := fs.Bool("list", false, "list the runs kept in the log directory")
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	dir := r.journal.Dir()
	runs, err := obs.ListRuns(dir)
	if dir == "" || err != nil {
		r.problem(i18n.Render(r.lang, "cli.trace.no_logs", nil))
		return ExitError
	}
	// The current run is this very command: it is not history yet.
	past := runs[:0:0]
	for _, run := range runs {
		if run.ID != r.journal.RunID {
			past = append(past, run)
		}
	}
	if *list {
		if *asJSON {
			return r.writeJSON(past)
		}
		for _, run := range past {
			mark := ""
			if run.BlackBox {
				mark = "  " + i18n.Render(r.lang, "cli.trace.failed", nil)
			}
			fmt.Fprintf(r.streams.Out, "%s  %s  %6d B%s\n", run.ID, run.Time.Local().Format("2006-01-02 15:04:05"), run.Bytes, mark)
		}
		return ExitOK
	}
	var chosen *obs.RunFiles
	for i := range past {
		if fs.NArg() == 0 || strings.EqualFold(string(past[i].ID), fs.Arg(0)) ||
			(len(fs.Arg(0)) >= 6 && strings.HasSuffix(string(past[i].ID), strings.ToUpper(fs.Arg(0)))) {
			chosen = &past[i]
			break
		}
	}
	if chosen == nil {
		r.problem(i18n.Render(r.lang, "cli.trace.not_found", map[string]string{"run": fs.Arg(0)}))
		return ExitError
	}
	name := string(chosen.ID) + obs.JournalSuffix
	if chosen.BlackBox {
		name = string(chosen.ID) + obs.BlackBoxSuffix
	}
	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return r.fail(err)
	}
	defer f.Close()
	events, err := obs.ReadEvents(f)
	if err != nil {
		return r.fail(err)
	}
	if *asJSON {
		return r.writeJSON(obs.Steps(events))
	}
	r.printTimeline(r.streams.Out, events, string(chosen.ID))
	return ExitOK
}

// humanBytes renders a size for humans: 512 B, 3.2 KiB, 10.0 MiB.
func humanBytes(n int64) string {
	switch {
	case n < 1<<10:
		return strconv.FormatInt(n, 10) + " B"
	case n < 1<<20:
		return strconv.FormatFloat(float64(n)/(1<<10), 'f', 1, 64) + " KiB"
	}
	return strconv.FormatFloat(float64(n)/(1<<20), 'f', 1, 64) + " MiB"
}

// startRun records the start of a run: what, which build, where.
func (r runner) startRun(command string) {
	r.journal.Logger.Info("run.start", "command", command, "version", r.build.Version,
		"os", runtime.GOOS, "arch", runtime.GOARCH, "quiet", r.global.quiet, "debug", r.global.debug, "plain", r.accessibleMode())
}

// applyRunSettings applies ship.logs to the journal (black box size now,
// retention after the run) and the ui settings to the run. Messages written
// before, if any, use the language of SHIP_LANG or the locale.
func (r runner) applyRunSettings() runner {
	limits, ui := app.RunSettings(r.ctx, r.deps)
	r.journal.SetLimits(obs.Retention{Runs: limits.KeepRuns, Bytes: limits.MaxBytes}, limits.BlackBoxBytes)
	r.lang = r.language(ui.Language)
	r.ui = uiPrefs{noReviewScreen: !ui.ReviewScreen, accessible: ui.Accessible, color: ui.Color}
	return r
}

// language resolves the language of messages: SHIP_LANG first (an
// environment variable beats the files, as SHIP_* settings do), then
// ui.language unless it is auto, then the locale, which the composition root
// already read into r.lang along with SHIP_LANG.
func (r runner) language(setting string) i18n.Language {
	if v := r.env("SHIP_LANG"); v != "" {
		return i18n.ParseLanguage(v)
	}
	if setting != "" {
		return i18n.ParseLanguage(setting)
	}
	return r.lang
}

// echoDebug turns --debug on: the journal, DEBUG included, also on stderr.
func (r runner) echoDebug() {
	if r.global.debug {
		r.journal.EchoTo(r.streams.Err, slog.LevelDebug)
	}
}
