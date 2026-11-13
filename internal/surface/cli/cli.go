// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/GregoireF/ship-it/internal/app"
	"github.com/GregoireF/ship-it/internal/i18n"
	"github.com/GregoireF/ship-it/internal/obs"
)

// Exit codes are stable and documented (docs/reference/cli.md).
const (
	ExitOK         = 0
	ExitError      = 1
	ExitNeedsHuman = 2
	ExitGate       = 3   // a safety gate refused the work (secrets…)
	ExitPolicy     = 5   // refused by the organisation policy
	ExitUsage      = 64  // EX_USAGE from sysexits.h
	ExitAborted    = 130 // the human declined or interrupted
)

// BuildInfo is injected at link time by the release pipeline.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Streams separates the result (stdout) from status messages (stderr), as
// clig.dev recommends, and carries the third flow: the technical journal
// (docs/reference/logs.md §1).
type Streams struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
	// Interactive is true when a human can answer prompts on In.
	Interactive bool
	// TerminalUI is true when Err is a terminal too: the review screen of
	// ship can draw there (stdout stays for the results).
	TerminalUI bool
	// Journal is the run's technical journal, opened by the composition
	// root; nil keeps one in memory only (nothing written to disk).
	Journal *obs.Journal
}

// Deps are the adapters the composition root (cmd/ship) injects: the same
// workspace every surface (CLI, MCP) works on.
type Deps = app.Workspace

// Run executes ship with args (without the program name) and returns the
// process exit code.
func Run(ctx context.Context, args []string, streams Streams, lang i18n.Language, build BuildInfo, deps Deps) (code int) {
	global, args := parseGlobalFlags(args)
	journal := streams.Journal
	if journal == nil {
		journal = obs.NewJournal(obs.JournalOptions{})
		journal.Discard()
	}
	if deps.Log == nil {
		deps.Log = journal.Logger
	}
	deps.SettingFlags = append(deps.SettingFlags, global.set...)
	if deps.RunLogs == nil && journal.Dir() != "" {
		deps.RunLogs = journal
	}
	r := runner{ctx: ctx, streams: streams, lang: lang, build: build, deps: deps, journal: journal, global: global}
	r.echoDebug()
	command := commandName(args)
	r.startRun(command)
	defer func() { code = r.finish(command, code, recover()) }()
	r = r.applyRunSettings()
	return r.dispatch(args)
}

func (r runner) dispatch(args []string) int {
	streams, lang := r.streams, r.lang
	// "ship" alone, or with flags only, ships the work in progress.
	if len(args) == 0 || (strings.HasPrefix(args[0], "-") && !isHelpOrVersion(args[0])) {
		return r.ship(args)
	}
	switch args[0] {
	case "version", "--version":
		return r.version(args[1:])
	case "help", "--help", "-h":
		fmt.Fprintln(streams.Out, i18n.Render(lang, "cli.usage", nil))
		return ExitOK
	case "types":
		return r.types(args[1:])
	case "check":
		return r.check(args[1:])
	case "branch":
		return r.branch(args[1:])
	case "plan":
		return r.plan(args[1:])
	case "doctor":
		return r.doctor(args[1:])
	case "checkpoint":
		return r.checkpoint(args[1:])
	case "undo":
		return r.undo(args[1:])
	case "gc":
		return r.gc(args[1:])
	case "publish":
		return r.publish(args[1:])
	case "preview":
		return r.preview(args[1:])
	case "stack":
		return r.stack(args[1:])
	case "config":
		return r.config(args[1:])
	case "init":
		return r.initCommand(args[1:])
	case "sync":
		return r.sync(args[1:])
	case "roadmap":
		return r.roadmap(args[1:])
	case "label":
		return r.label(args[1:])
	case "status":
		return r.statusCommand(args[1:])
	case "tidy":
		return r.tidyCommand(args[1:])
	case "log":
		return r.log(args[1:])
	case "fix":
		return r.fixCommand(args[1:])
	case "mcp":
		return r.mcpCommand(args[1:])
	case "skill":
		return r.skillCommand(args[1:])
	case "uninstall":
		return r.uninstall(args[1:])
	case "trace":
		return r.trace(args[1:])
	case "bug":
		return r.bug(args[1:])
	default:
		r.status(i18n.Render(lang, "cli.error.unknown_command", map[string]string{"command": args[0]}))
		return ExitUsage
	}
}

type runner struct {
	ctx     context.Context
	streams Streams
	lang    i18n.Language
	build   BuildInfo
	deps    Deps
	journal *obs.Journal
	global  globalFlags
	ui      uiPrefs
}

// uiPrefs are the ui settings of the run (applyRunSettings); the zero value
// is the default.
type uiPrefs struct {
	noReviewScreen bool   // ui.reviewScreen false: one confirmation
	accessible     bool   // ui.accessible, as --plain
	color          string // ui.color: "" or app.ColorAuto, app.ColorAlways, app.ColorNever
}

// severityLabels pair a symbol with a word: the meaning never relies on the
// symbol (or a colour) alone.
var severityLabels = map[string]string{"ok": "✓ ok   ", "warning": "⚠ warn ", "blocking": "✗ block"}

func (r runner) doctor(args []string) int {
	fs, asJSON := r.flags("doctor")
	deep := fs.Bool("deep", false, "also check the token's scopes, the rulesets and the project's checks (runs them)")
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	var report app.DoctorReport
	if *deep {
		report = app.CheckUpDeep(r.ctx, r.deps, r.gateProgress(*asJSON))
	} else {
		report = app.CheckUp(r.ctx, r.deps)
	}
	if *asJSON {
		r.writeJSON(report)
	} else {
		for _, f := range report.Findings {
			fmt.Fprintf(r.streams.Out, "%s  %s\n", severityLabels[f.Severity], i18n.Render(r.lang, f.Key, f.Params))
			if f.Remedy != "" {
				fmt.Fprintf(r.streams.Out, "         → %s\n", i18n.Render(r.lang, f.Remedy, f.Params))
			}
		}
		summary := map[string]string{"ok": "cli.doctor.ready", "warning": "cli.doctor.warnings", "blocking": "cli.doctor.blocked"}[report.Worst]
		if report.OutsideRepository() && !report.Blocking() {
			summary = "cli.doctor.not_repository"
		}
		fmt.Fprintf(r.streams.Out, "\n%s\n", i18n.Render(r.lang, summary, nil))
	}
	// Outside a repository ship can do nothing: that blocks too.
	if report.Blocking() || report.OutsideRepository() {
		return ExitError
	}
	return ExitOK
}

// checkpointListLimit is how many checkpoints `ship checkpoint list` shows.
const checkpointListLimit = 20

func (r runner) checkpoint(args []string) int {
	action := "create"
	if len(args) > 0 && (args[0] == "list" || args[0] == "restore") {
		action, args = args[0], args[1:]
	}
	name := "checkpoint " + action
	if action == "create" {
		name = "checkpoint" // ship checkpoint, without a word
	}
	fs, asJSON := r.flags(name)
	var label *string
	if action == "create" {
		label = fs.String("label", "", "label of the checkpoint")
	}
	var dryRun *bool
	if action != "list" {
		dryRun = dryRunFlag(fs)
	}
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	if dryRun != nil {
		r = r.rehearse(*dryRun)
	}
	switch action {
	case "list":
		views, err := app.ListCheckpoints(r.ctx, r.deps.Checkpoints, checkpointListLimit)
		if err != nil {
			return r.fail(err)
		}
		if *asJSON {
			return r.writeJSON(views)
		}
		if len(views) == 0 {
			r.note(i18n.Render(r.lang, "cli.checkpoint.none", nil))
			return ExitOK
		}
		for _, v := range views {
			fmt.Fprintf(r.streams.Out, "%s  %s  %-16s %s\n", v.ShortID, v.CreatedAt.Local().Format("2006-01-02 15:04"), v.Label, v.Branch)
		}
		return ExitOK
	case "restore":
		if fs.NArg() != 1 {
			r.status(i18n.Render(r.lang, "cli.error.missing_argument", map[string]string{"argument": "<checkpoint>"}))
			return ExitUsage
		}
		var result app.RestoreResult
		rehearsal, err := app.Rehearsed(r.deps, func() (err error) {
			result, err = app.RestoreCheckpoint(r.ctx, r.deps.Checkpoints, fs.Arg(0))
			return err
		})
		if err != nil {
			return r.fail(err)
		}
		if *asJSON && *dryRun {
			return r.writeJSON(struct {
				app.RestoreResult
				app.Rehearsal
			}{result, rehearsal})
		}
		if *asJSON {
			return r.writeJSON(result)
		}
		if *dryRun {
			return r.printPlanned(rehearsal)
		}
		r.note(i18n.Render(r.lang, "cli.checkpoint.restored", map[string]string{"id": result.Restored.ShortID, "safety": result.Safety.ShortID}))
		return ExitOK
	default:
		var view app.CheckpointView
		rehearsal, err := app.Rehearsed(r.deps, func() (err error) {
			view, err = app.CreateCheckpoint(r.ctx, r.deps.Checkpoints, *label)
			return err
		})
		if err != nil {
			return r.fail(err)
		}
		if *asJSON && *dryRun {
			return r.writeJSON(struct {
				app.CheckpointView
				app.Rehearsal
			}{view, rehearsal})
		}
		if *asJSON {
			return r.writeJSON(view)
		}
		if *dryRun {
			return r.printPlanned(rehearsal)
		}
		r.note(i18n.Render(r.lang, "cli.checkpoint.created", map[string]string{"id": view.ShortID}))
		return ExitOK
	}
}

func isHelpOrVersion(arg string) bool {
	switch arg {
	case "-h", "--help", "--version":
		return true
	}
	return false
}

// ship is the main flow: plan, human review, local delivery.
func (r runner) ship(args []string) int {
	fs, asJSON := r.flags("ship")
	yes := fs.Bool("yes", false, "accept the plan without asking")
	fs.BoolVar(yes, "y", false, "shorthand for --yes")
	issue := fs.Int("issue", 0, "linked issue number")
	agent := fs.String("agent", "", "agent prefix: ai, claude, codex, copilot, cursor")
	var only pathList
	fs.Var(&only, "only", "deliver only this file or directory, a `path` (repeatable)")
	local := fs.Bool("local", false, "commit only: no push, no pull request")
	skipGates := fs.Bool("skip-gates", false, "push without running lint and tests (said in the pull request)")
	why := fs.String("why", "", "why this change: the commit body and the pull request summary")
	tested := fs.String("tested", "", "how you tested it, recorded as a Tested: trailer")
	dryRun := dryRunFlag(fs)
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	// `ship --yes plan` must not deliver: a word left after the flags is a
	// command written in the wrong place, never something to ignore.
	if fs.NArg() > 0 {
		r.status(i18n.Render(r.lang, "cli.error.unexpected_argument", map[string]string{"argument": fs.Arg(0)}))
		return ExitUsage
	}
	r = r.rehearse(*dryRun)
	// A broken configuration surfaces before anything changes.
	publishes, err := app.PublishesAfterShip(r.ctx, r.deps)
	if err != nil {
		return r.fail(err)
	}
	*local = *local || !publishes
	var tidy app.TidyResult
	var tidyWarning error // said once: the publication may meet it again
	if !*local {
		// Leave a merged branch before planning, so that the work lands on a
		// fresh branch from the up-to-date base. A failure (offline…) is
		// only a warning: shipping goes on. ship.tidy.auto false skips it.
		var ran bool
		tidy, ran, err = app.TidyBeforeShip(r.ctx, r.deps)
		if ran && !*asJSON && (!*dryRun || err != nil) {
			r.printTidy(tidy, err)
			tidyWarning = err
		}
	}
	view, err := app.ProposePlan(r.ctx, r.deps, app.PlanRequest{Issue: *issue, Agent: *agent, Only: only})
	if err != nil {
		return r.fail(err)
	}
	// A dry run asks nothing: it shows the plan as proposed.
	mode := r.reviewModeFor(*yes || *dryRun)
	if mode == reviewConfirm && len(view.TypeChoices) > 0 {
		mode = reviewPlain // a yes/no question cannot pick the type
	}
	if !*asJSON && mode != reviewScreen {
		r.printPlan(view)
	}
	// Refuse before asking: nobody should approve a plan that cannot ship.
	// The review screen lists the secrets instead, where unselecting their
	// files is one key away.
	if len(view.Secrets) > 0 && mode != reviewScreen {
		if *asJSON {
			r.writeJSON(view)
		}
		return r.fail(&app.SecretsFoundError{Findings: view.Secrets})
	}
	request := app.DeliveryRequest{Title: view.Title, Branch: view.Branch, Only: only, Why: *why, Tested: *tested,
		SkipGates: *skipGates, GateObserver: r.gateProgress(*asJSON)}
	var reads app.PlanReads
	if mode == reviewNone {
		// Nothing waits between the plan and the delivery: what the plan
		// read is still the repository. A review reads it again (see
		// DeliveryRequest.Reads).
		reads = view.Reads()
		request.Reads = reads
	}
	cfg := reviewConfig{
		lang: r.lang, plan: view, only: only, why: *why, tested: *tested,
		replan: func(only []string) (app.PlanView, error) {
			return app.ProposePlan(r.ctx, r.deps, app.PlanRequest{Issue: *issue, Agent: *agent, Only: only})
		},
	}
	if mode == reviewScreen || mode == reviewPlain {
		// Unreadable settings keep the built-in conventions: the delivery
		// reports the broken configuration itself.
		cfg.conventions, _ = app.LoadConventions(r.ctx, r.deps, view.Files)
		cfg.checkBranch = app.DeliveryBranchCheck(r.ctx, r.deps, view.Files)
		cfg.branchFor = func(title string) (string, bool) {
			b, err := cfg.conventions.BranchForTitleOf(title, app.BranchRequest{Issue: *issue, Agent: *agent})
			return b, err == nil
		}
	}
	var outcome reviewOutcome
	switch mode {
	case reviewNobody:
		r.status(i18n.Render(r.lang, "cli.ship.needs_human", nil))
		return ExitNeedsHuman
	case reviewConfirm:
		outcome = reviewOutcome{action: reviewCancel}
		if r.confirm(i18n.Render(r.lang, "cli.ship.confirm", nil)) {
			outcome.action = reviewShip
		}
	case reviewScreen:
		var started bool
		if outcome, started = r.runReviewScreen(cfg); !started {
			if len(view.Secrets) > 0 {
				return r.fail(&app.SecretsFoundError{Findings: view.Secrets})
			}
			outcome = r.plainReview(cfg)
		}
	case reviewPlain:
		outcome = r.plainReview(cfg)
	}
	if mode != reviewNone {
		if outcome.action == reviewCancel {
			r.status(i18n.Render(r.lang, "cli.ship.aborted", nil))
			return ExitAborted
		}
		if mode != reviewConfirm {
			request.Title, request.Branch, request.Only = outcome.title, outcome.branch, outcome.only
			request.Why, request.Tested = outcome.why, outcome.tested
		}
		*local = *local || outcome.action == reviewLocal
	}
	result, err := app.DeliverLocally(r.ctx, r.deps, request)
	if err != nil {
		return r.fail(err)
	}
	if !*asJSON && !*dryRun {
		short := result.Commit
		if len(short) > 12 {
			short = short[:12]
		}
		key := "cli.ship.committed"
		if result.CreatedBranch {
			key = "cli.ship.committed_new_branch"
		}
		r.note(i18n.Render(r.lang, key, map[string]string{"commit": short, "branch": result.Branch}))
	}
	shipped := shipResult{Delivery: result}
	if *dryRun && !*local {
		shipped.Tidy = &tidy
	}

	if *local {
		if *dryRun && !*asJSON {
			return r.printPlanned(result.Rehearsal)
		}
		return r.shipLocalOnly(shipped, *asJSON, result.GatesDeferred, app.RemoteProblem(r.ctx, r.deps))
	}
	// The plan was reviewed in a terminal unless --yes: the review then moves
	// to a draft pull request (ADR-0010).
	publication, err := app.Publish(r.ctx, r.deps, app.PublishRequest{
		Issue: *issue, Unattended: *yes, SkipGates: *skipGates, GateObserver: r.gateProgress(*asJSON), Reads: reads,
		GatesPassed: result.Gates,
	})
	if app.IsLocalRepository(err) {
		if *dryRun && !*asJSON {
			return r.printPlanned(tidy.Rehearsal, result.Rehearsal)
		}
		return r.shipLocalOnly(shipped, *asJSON, result.GatesDeferred, err)
	}
	if err != nil {
		code := exitOf(err)
		if !sameFault(err, tidyWarning) {
			code = r.fail(err)
		}
		if !*dryRun {
			r.status(i18n.Render(r.lang, "cli.ship.publish_later", nil))
		}
		return code
	}
	shipped.Publication = &publication
	if *asJSON {
		return r.writeJSON(shipped)
	}
	if *dryRun {
		return r.printPlanned(tidy.Rehearsal, result.Rehearsal, publication.Rehearsal)
	}
	r.printPublication(publication)
	return ExitOK
}

// shipResult is what `ship --json` prints. Tidy is only there on a dry run,
// whose plan starts with it.
type shipResult struct {
	Tidy        *app.TidyResult    `json:"tidy,omitempty"`
	Delivery    app.DeliveryResult `json:"delivery"`
	Publication *app.PublishResult `json:"publication,omitempty"`
}

// shipLocalOnly ends a delivery that stays local. gatesDeferred says the
// checks have not run yet: they run at publication, or now with ship check
// gates. noGitHub is the fault saying the remote is missing or not on
// GitHub (app.RemoteProblem), nil when ship publish can work.
func (r runner) shipLocalOnly(shipped any, asJSON, gatesDeferred bool, noGitHub error) int {
	if asJSON {
		return r.writeJSON(shipped)
	}
	r.note(i18n.Render(r.lang, "cli.ship.undo_hint", nil))
	r.status(r.localOnlyLine(noGitHub, "cli.ship.publish_later"))
	if gatesDeferred {
		r.note(i18n.Render(r.lang, "cli.ship.gates_later", nil))
	}
	return ExitOK
}

// localOnlyLine ends a delivery that stays local: with a remote missing or
// not on GitHub, ship publish cannot work, so the line says that only local
// commits are possible and how to point the remote to GitHub; otherwise it is
// the line of key, which says how to publish later.
func (r runner) localOnlyLine(noGitHub error, key string) string {
	var f *app.Fault
	if errors.As(noGitHub, &f) {
		switch f.Key {
		case "publish.unsupported_remote":
			return i18n.Render(r.lang, "cli.ship.local_only", f.Params)
		case "publish.no_remote":
			return i18n.Render(r.lang, "cli.ship.local_only_no_remote", f.Params)
		}
	}
	return i18n.Render(r.lang, key, nil)
}

func (r runner) printPublication(p app.PublishResult) {
	params := map[string]string{"url": p.PullRequest.URL, "number": strconv.Itoa(p.PullRequest.Number), "base": p.Base}
	switch {
	case !p.Created:
		r.note(i18n.Render(r.lang, "cli.publish.updated", params))
	case p.PullRequest.Draft:
		r.note(i18n.Render(r.lang, "cli.publish.created_draft", params))
	default:
		r.note(i18n.Render(r.lang, "cli.publish.created", params))
	}
	if p.Rebased {
		r.note(i18n.Render(r.lang, "cli.publish.rebased", params))
	}
	if len(p.Labels) > 0 {
		r.note(i18n.Render(r.lang, "cli.publish.labels", map[string]string{"labels": strings.Join(p.Labels, ", ")}))
	}
	if p.ProjectStatus != "" {
		r.note(i18n.Render(r.lang, "cli.publish.project_status", map[string]string{"status": p.ProjectStatus}))
	}
	if p.Assigned != "" {
		r.note(i18n.Render(r.lang, "cli.publish.assigned", map[string]string{"login": p.Assigned}))
	}
	for _, w := range p.Warnings {
		r.status("  ⚠ " + w.Code + "  " + i18n.Render(r.lang, w.Key, w.Params))
	}
	// The URL alone on stdout lets scripts and terminals open it.
	fmt.Fprintln(r.streams.Out, p.PullRequest.URL)
}

func (r runner) statusCommand(args []string) int {
	fs, asJSON := r.flags("status")
	offline := fs.Bool("offline", false, "do not ask GitHub")
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	view, err := app.Status(r.ctx, r.deps, app.StatusRequest{Offline: *offline})
	if err != nil {
		return r.fail(err)
	}
	if *asJSON {
		return r.writeJSON(view)
	}
	out := r.streams.Out
	t := func(key string, params map[string]string) string { return i18n.Render(r.lang, key, params) }
	// Labels are aligned on the longest one in the current language.
	width := 0
	for _, label := range []string{"changes", "upstream", "pull_request", "checks", "review", "overlap", "undo"} {
		width = max(width, utf8.RuneCountInString(t("cli.status.label."+label, nil)))
	}
	line := func(label, value string) {
		text := t("cli.status.label."+label, nil)
		fmt.Fprintf(out, "  %s%s  %s\n", text, strings.Repeat(" ", width-utf8.RuneCountInString(text)), value)
	}
	n := strconv.Itoa

	head := view.Branch
	if view.Detached {
		head = t("cli.status.detached", nil)
	}
	if view.Base != "" && view.Branch != view.Base {
		head += "  " + t("cli.status.versus_base", map[string]string{"ahead": n(view.AheadOfBase), "behind": n(view.BehindBase), "base": view.Base})
	}
	fmt.Fprintln(out, head)

	if view.Uncommitted > 0 {
		line("changes", t("cli.status.uncommitted", map[string]string{"count": n(view.Uncommitted)}))
	} else {
		line("changes", t("cli.status.clean", nil))
	}
	switch {
	case view.Upstream == "":
		line("upstream", t("cli.status.not_pushed", nil))
	case view.Ahead == 0 && view.Behind == 0:
		line("upstream", t("cli.status.in_sync", map[string]string{"upstream": view.Upstream}))
	default:
		line("upstream", t("cli.status.diverged", map[string]string{"upstream": view.Upstream, "ahead": n(view.Ahead), "behind": n(view.Behind)}))
	}
	switch {
	case view.PullRequest != nil && view.PullRequest.Draft:
		line("pull_request", t("cli.status.pull_request_draft", map[string]string{"number": n(view.PullRequest.Number), "url": view.PullRequest.URL}))
	case view.PullRequest != nil:
		line("pull_request", t("cli.status.pull_request", map[string]string{"number": n(view.PullRequest.Number), "url": view.PullRequest.URL}))
	case view.GitHubProblem != nil:
		line("pull_request", view.GitHubProblem.Code+"  "+t(view.GitHubProblem.Key, view.GitHubProblem.Params))
	case view.Base != "" && view.Branch == view.Base:
		// The default branch has no pull request of its own, and status does
		// not ask GitHub for one: "unknown (offline)" would be wrong.
	case view.GitHub == "known":
		line("pull_request", t("cli.status.no_pull_request", nil))
	case view.GitHub == "unknown":
		line("pull_request", t("cli.status.github_unknown", nil))
	}
	if ci := view.CI; ci != nil && ci.State != "none" {
		// The symbol says it without colour (accessibility): ✓ ✗ ⋯
		line("checks", t("cli.status.checks."+string(ci.State), map[string]string{
			"failing": strings.Join(ci.Failing, ", "), "pending": n(ci.Pending), "total": n(ci.Total)}))
	}
	if rv := view.Review; rv != nil && rv.Decision != "none" {
		line("review", t("cli.status.review."+string(rv.Decision), map[string]string{
			"approvers": strings.Join(rv.Approvers, ", "), "requesters": strings.Join(rv.Requesters, ", ")}))
	}
	for _, o := range view.Overlaps {
		line("overlap", t("cli.status.overlap", map[string]string{"number": n(o.Number), "title": o.Title, "files": strings.Join(o.Files, ", ")}))
	}
	if view.Undoable != nil {
		line("undo", t("cli.status.undoable."+view.Undoable.Kind, map[string]string{"title": view.Undoable.Title}))
	}
	fmt.Fprintf(out, "\n→ %s\n", t(view.Next.Key, view.Next.Params))
	return ExitOK
}

func (r runner) publish(args []string) int {
	fs, asJSON := r.flags("publish")
	issue := fs.Int("issue", 0, "linked issue number (default: from the branch name)")
	draft := fs.Bool("draft", false, "open the pull request as a draft")
	skipGates := fs.Bool("skip-gates", false, "push without running lint and tests (said in the pull request)")
	dryRun := dryRunFlag(fs)
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	r = r.rehearse(*dryRun)
	result, err := app.Publish(r.ctx, r.deps, app.PublishRequest{
		Issue: *issue, ForceDraft: *draft, SkipGates: *skipGates, GateObserver: r.gateProgress(*asJSON),
	})
	if err != nil {
		return r.fail(err)
	}
	if *asJSON {
		return r.writeJSON(result)
	}
	if *dryRun {
		return r.printPlanned(result.Rehearsal)
	}
	r.printPublication(result)
	return ExitOK
}

func (r runner) undo(args []string) int {
	fs, asJSON := r.flags("undo")
	yes := fs.Bool("yes", false, "agree to changes on GitHub (close the pull request, delete the remote branch)")
	fs.BoolVar(yes, "y", false, "shorthand for --yes")
	dryRun := dryRunFlag(fs)
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	r = r.rehearse(*dryRun)
	result, err := app.Undo(r.ctx, r.deps, app.UndoRequest{AllowRemote: *yes})
	if *dryRun {
		// A dry run needs no consent: the plan says the real run asks.
		if err != nil {
			return r.fail(err)
		}
		if *asJSON {
			return r.writeJSON(result)
		}
		return r.printPlanned(result.Rehearsal)
	}
	if app.NeedsConsent(err) && !*yes && r.streams.Interactive {
		// The fault itself describes what would change on GitHub.
		var f *app.Fault
		errors.As(err, &f)
		confirmKey := "cli.undo.confirm_remote"
		switch f.Key {
		case "undo.needs_consent_issue":
			confirmKey = "cli.undo.confirm_remote_issue"
		case "undo.needs_consent_stack":
			confirmKey = "cli.undo.confirm_remote_stack"
		case "undo.needs_consent_stack_sync":
			confirmKey = "cli.undo.confirm_remote_stack_sync"
		}
		if !r.confirm(i18n.Render(r.lang, confirmKey, f.Params)) {
			r.status(i18n.Render(r.lang, "cli.ship.aborted", nil))
			return ExitAborted
		}
		result, err = app.Undo(r.ctx, r.deps, app.UndoRequest{AllowRemote: true})
	}
	if err != nil {
		return r.fail(err)
	}
	if *asJSON {
		return r.writeJSON(result)
	}
	key, branches := undoMessage(result)
	r.note(i18n.Render(r.lang, key, map[string]string{
		"title": result.Title, "branch": result.Branch, "branches": strings.Join(branches, ", "),
		"number": strconv.Itoa(result.ClosedPullRequest), "remote_branch": result.DeletedRemoteBranch, "file": result.ConfigFile,
		"count": strconv.Itoa(len(result.ClosedPullRequests)), "layers": strconv.Itoa(len(result.DeletedBranches)),
		"items": strings.Join(result.Restored, ", "), "renamed": result.Renamed,
	}))
	if result.DeletedBranch != "" && result.Kind != "stack" {
		r.note(i18n.Render(r.lang, "cli.undo.branch_deleted", map[string]string{"branch": result.DeletedBranch}))
	}
	if len(result.RestoredRemote) > 0 {
		r.note(i18n.Render(r.lang, "cli.undo.remote_restored", map[string]string{"branches": strings.Join(result.RestoredRemote, ", ")}))
	}
	if len(result.Retargeted) > 0 {
		numbers := make([]string, len(result.Retargeted))
		for i, n := range result.Retargeted {
			numbers[i] = "#" + strconv.Itoa(n)
		}
		r.note(i18n.Render(r.lang, "cli.undo.retargeted", map[string]string{"numbers": strings.Join(numbers, ", ")}))
	}
	if result.RestoredIssueStatus != 0 {
		r.note(i18n.Render(r.lang, "cli.undo.issue_status", map[string]string{"issue": strconv.Itoa(result.RestoredIssueStatus)}))
	}
	if result.Unassigned != "" {
		r.note(i18n.Render(r.lang, "cli.undo.unassigned", map[string]string{"login": result.Unassigned}))
	}
	for _, w := range result.Warnings {
		r.status("  ⚠ " + w.Code + "  " + i18n.Render(r.lang, w.Key, w.Params))
	}
	r.note(i18n.Render(r.lang, "cli.undo.safety", map[string]string{"safety": result.Safety.ShortID}))
	return ExitOK
}

// undoMessage picks the line that says what an undo reverted, by the kind of
// the operation (oplog.Kind): only a delivery leaves changes uncommitted, the
// others put back files, settings, branches or GitHub. branches fills
// {branches}.
func undoMessage(result app.UndoResult) (key string, branches []string) {
	switch result.Kind {
	case "config":
		return "cli.undo.config", nil
	case "fix":
		return "cli.undo.fix", nil
	case "rename":
		return "cli.undo.renamed", nil
	case "roadmap":
		return "cli.undo.roadmap", nil
	case "tidy":
		return "cli.undo.recreated", result.Recreated
	case "stack":
		return "cli.undo.stack", result.DeletedBranches
	case "stack_sync":
		return "cli.undo.stack_sync", append(slices.Clone(result.Recreated), result.Moved...)
	case "stack_publish":
		return "cli.undo.unpublished_stack", nil
	case "publish":
		return "cli.undo.unpublished", nil
	}
	if result.Branch == "" {
		return "cli.undo.done_detached", nil
	}
	return "cli.undo.done", nil
}

func (r runner) gc(args []string) int {
	fs, asJSON := r.flags("gc")
	dryRun := fs.Bool("dry-run", false, "show what would be removed")
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	report, err := app.CollectGarbage(r.ctx, r.deps, app.GarbageRequest{Now: time.Now(), DryRun: *dryRun})
	if err != nil {
		return r.fail(err)
	}
	if *asJSON {
		return r.writeJSON(report)
	}
	key, logsKey := "cli.gc.removed", "cli.gc.logs_removed"
	if *dryRun {
		key, logsKey = "cli.gc.would_remove", "cli.gc.logs_would_remove"
	}
	r.note(i18n.Render(r.lang, key, map[string]string{
		"removed": strconv.Itoa(len(report.Removed)), "kept": strconv.Itoa(report.Kept),
	}))
	if report.Logs != nil {
		r.note(i18n.Render(r.lang, logsKey, map[string]string{
			"runs": strconv.Itoa(report.Logs.RemovedRuns), "size": humanBytes(report.Logs.FreedBytes),
		}))
	}
	return ExitOK
}

// confirm asks a yes/no question; Enter means yes (decision: one-key review).
func (r runner) confirm(question string) bool {
	fmt.Fprintf(r.streams.Err, "%s ", question)
	answer, ok := r.readLine()
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "", "y", "yes", "o", "oui":
		return true
	}
	return false
}

// printSecrets lists findings on stderr, each with the fingerprint that
// accepts it in ship.secrets.allow; values are already masked.
func (r runner) printSecrets(findings []app.SecretView) {
	for _, s := range findings {
		r.status("  " + secretLine(r.lang, s))
		if s.Fingerprint != "" {
			r.status("    " + i18n.Render(r.lang, "cli.secrets.fingerprint", map[string]string{"fingerprint": s.Fingerprint}))
		}
	}
}

func (r runner) printPlan(view app.PlanView) {
	out := r.streams.Out
	fmt.Fprintln(out, view.Title)
	fmt.Fprintf(out, "  %-9s %s\n", i18n.Render(r.lang, "cli.plan.branch", nil), view.Branch)
	fmt.Fprintf(out, "  %-9s %d\n", i18n.Render(r.lang, "cli.plan.files", nil), len(view.Files))
	for i, reason := range view.Reasons {
		label := ""
		if i == 0 {
			label = i18n.Render(r.lang, "cli.plan.why", nil)
		}
		fmt.Fprintf(out, "  %-9s • %s\n", label, i18n.Render(r.lang, reason.Key, r.localizeParams(reason.Params)))
	}
	r.printSize(view.Size)
}

func (r runner) plan(args []string) int {
	fs, asJSON := r.flags("plan")
	issue := fs.Int("issue", 0, "linked issue number")
	agent := fs.String("agent", "", "agent prefix: ai, claude, codex, copilot, cursor")
	var only pathList
	fs.Var(&only, "only", "deliver only this file or directory, a `path` (repeatable)")
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	view, err := app.ProposePlan(r.ctx, r.deps, app.PlanRequest{Issue: *issue, Agent: *agent, Only: only})
	if err != nil {
		return r.fail(err)
	}
	if *asJSON {
		r.writeJSON(view)
	} else {
		r.printPlan(view)
	}
	// The plan changes nothing, but its exit code tells whether it can ship.
	if len(view.Secrets) > 0 {
		if *asJSON {
			return ExitGate
		}
		return r.fail(&app.SecretsFoundError{Findings: view.Secrets})
	}
	return ExitOK
}

// localizeParams translates parameters that are themselves keys (categories).
func (r runner) localizeParams(params map[string]string) map[string]string {
	if category, ok := params["category"]; ok {
		localized := make(map[string]string, len(params))
		for k, v := range params {
			localized[k] = v
		}
		localized["category"] = i18n.Render(r.lang, "plan.category."+category, nil)
		return localized
	}
	return params
}

// fail renders a fault (or an unexpected error) on stderr.
func (r runner) fail(err error) int {
	err = app.Known(err)
	if app.IsUnexpected(err) {
		r.journal.MarkUnexpected()
	}
	var f *app.Fault
	if errors.As(err, &f) {
		r.journal.Logger.Warn("run.fault", "code", f.Code, "key", f.Key)
		r.status(r.renderFault(f))
		if findings, ok := app.SecretsIn(err); ok {
			r.printSecrets(findings)
		}
		if results, ok := app.GatesIn(err); ok {
			r.printRedGate(results[len(results)-1])
		}
		if output, ok := app.CommitlintOutputIn(err); ok {
			r.printRedGate(app.GateResult{Output: output})
		}
		return exitOf(err)
	}
	r.journal.Logger.Error("run.error", "error", err)
	r.status(err.Error())
	return ExitError
}

// exitOf is the exit code of a fault.
func exitOf(err error) int {
	switch {
	case app.IsGateRefusal(err):
		return ExitGate
	case app.IsPolicyRefusal(err):
		return ExitPolicy
	case app.NeedsHuman(err):
		return ExitNeedsHuman
	}
	return ExitError
}

// sameFault reports whether two errors are faults with the same code: the
// second one was already said.
func sameFault(err, said error) bool {
	var a, b *app.Fault
	return errors.As(app.Known(err), &a) && errors.As(app.Known(said), &b) && a.Code == b.Code
}

// status writes a message that matters even with --quiet: errors, warnings,
// what a human must do.
func (r runner) status(msg string) { fmt.Fprintln(r.streams.Err, msg) }

// problem is status for errors, named for readability at call sites.
func (r runner) problem(msg string) { r.status(msg) }

// note writes a message that --quiet silences: progress and success.
func (r runner) note(msg string) {
	if !r.global.quiet {
		r.status(msg)
	}
}

func (r runner) renderFault(f *app.Fault) string {
	return fmt.Sprintf("%s  %s", f.Code, i18n.Render(r.lang, f.Key, f.Params))
}

func (r runner) version(args []string) int {
	fs, asJSON := r.flags("version")
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	if *asJSON {
		return r.writeJSON(map[string]string{
			"version": r.build.Version, "commit": r.build.Commit, "date": r.build.Date, "go": runtime.Version(),
		})
	}
	// A go install (or a local build) records no commit and no date: the
	// placeholders would only be noise, the version alone is the answer.
	if r.build.Commit == "none" && r.build.Date == "unknown" {
		fmt.Fprintf(r.streams.Out, "ship %s\n", r.build.Version)
		return ExitOK
	}
	fmt.Fprintf(r.streams.Out, "ship %s (%s, %s)\n", r.build.Version, r.build.Commit, r.build.Date)
	return ExitOK
}

func (r runner) types(args []string) int {
	fs, asJSON := r.flags("types")
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	conventions, err := app.LoadConventions(r.ctx, r.deps, nil)
	if err != nil {
		return r.fail(err)
	}
	views := conventions.CommitTypes()
	if *asJSON {
		return r.writeJSON(views)
	}
	for _, v := range views {
		fmt.Fprintf(r.streams.Out, "%s %-9s %-6s %s\n", v.Gitmoji, v.Type, v.BranchPrefix+"/", i18n.Render(r.lang, v.DescriptionKey, nil))
	}
	return ExitOK
}

func (r runner) check(args []string) int {
	if r.groupHelp("check", args) {
		return ExitOK
	}
	if len(args) == 0 {
		r.status(i18n.Render(r.lang, "cli.error.missing_argument", map[string]string{"argument": "title|branch|signoff|gates"}))
		return ExitUsage
	}
	if args[0] == "signoff" {
		return r.checkSignoff(args[1:])
	}
	if args[0] == "gates" {
		return r.checkGates(args[1:])
	}
	fs, asJSON := r.flags("check " + args[0])
	if code, ok := r.parse(fs, args[1:]); !ok {
		return code
	}
	value := strings.Join(fs.Args(), " ")
	if value == "" {
		r.status(i18n.Render(r.lang, "cli.error.missing_argument", map[string]string{"argument": args[0]}))
		return ExitUsage
	}
	conventions, err := app.LoadConventions(r.ctx, r.deps, nil)
	if err != nil {
		return r.fail(err)
	}
	switch args[0] {
	case "title":
		result := conventions.CheckTitle(value)
		if *asJSON {
			r.writeJSON(result)
		} else if result.Valid() {
			r.note(i18n.Render(r.lang, "cli.check.title.valid", nil))
		} else {
			for _, f := range result.Faults {
				r.status(r.renderFault(f))
			}
		}
		return exitFor(result.Valid())
	case "branch":
		result := conventions.CheckBranch(value)
		if *asJSON {
			r.writeJSON(result)
		} else if result.Valid() {
			r.note(i18n.Render(r.lang, "cli.check.branch.valid", nil))
		} else {
			r.status(r.renderFault(result.Fault))
		}
		return exitFor(result.Valid())
	default:
		r.status(i18n.Render(r.lang, "cli.error.unknown_command", map[string]string{"command": "check " + args[0]}))
		return ExitUsage
	}
}

func (r runner) branch(args []string) int {
	fs, asJSON := r.flags("branch")
	issue := fs.Int("issue", 0, "linked issue number")
	agent := fs.String("agent", "", "agent prefix: ai, claude, codex, copilot, cursor")
	hotfix := fs.Bool("hotfix", false, "urgent production fix")
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	rest := fs.Args()
	if len(rest) < 2 {
		r.status(i18n.Render(r.lang, "cli.error.missing_argument", map[string]string{"argument": "<type> <subject>"}))
		return ExitUsage
	}
	conventions, err := app.LoadConventions(r.ctx, r.deps, nil)
	if err != nil {
		return r.fail(err)
	}
	name, err := conventions.SuggestBranch(app.BranchRequest{
		CommitType: rest[0], Subject: strings.Join(rest[1:], " "),
		Issue: *issue, Agent: *agent, Hotfix: *hotfix,
	})
	if err != nil {
		return r.fail(err)
	}
	if *asJSON {
		return r.writeJSON(map[string]string{"branch": name})
	}
	fmt.Fprintln(r.streams.Out, name)
	return ExitOK
}

// flags returns a flag set whose errors go to stderr, with the shared --json
// flag. Its usage is printed by r.parse, on stdout for --help.
func (r runner) flags(name string) (*flag.FlagSet, *bool) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(r.streams.Err)
	fs.Usage = func() {}
	return fs, fs.Bool("json", false, "machine-readable output on stdout")
}

func (r runner) writeJSON(v any) int {
	enc := json.NewEncoder(r.streams.Out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return ExitError
	}
	return ExitOK
}

func exitFor(ok bool) int {
	if ok {
		return ExitOK
	}
	return ExitError
}
