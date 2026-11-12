// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/GregoireF/ship-it/internal/app"
	"github.com/GregoireF/ship-it/internal/i18n"
)

// stack runs ship stack and its subcommands (docs/reference/cli.md).
func (r runner) stack(args []string) int {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "status":
			return r.stackStatus(args[1:])
		case "sync":
			return r.stackSync(args[1:])
		case "merge":
			return r.stackMerge(args[1:])
		case "publish":
			return r.stackPublish(args[1:])
		default:
			r.status(i18n.Render(r.lang, "cli.error.unknown_command", map[string]string{"command": "stack " + args[0]}))
			return ExitUsage
		}
	}
	return r.stackCreate(args)
}

// layerList collects --layer values: each one a layer, its paths separated
// by commas.
type layerList [][]string

func (l *layerList) String() string { return fmt.Sprint(*l) }
func (l *layerList) Set(v string) error {
	var paths []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			paths = append(paths, p)
		}
	}
	*l = append(*l, paths)
	return nil
}

// stackShipped is what `ship stack --json` prints.
type stackShipped struct {
	Delivery    app.StackDeliveryResult `json:"delivery"`
	Publication *app.StackPublishResult `json:"publication,omitempty"`
}

func (r runner) stackCreate(args []string) int {
	fs, asJSON := r.flags("stack")
	yes := fs.Bool("yes", false, "accept the layers without asking")
	fs.BoolVar(yes, "y", false, "shorthand for --yes")
	var layers layerList
	fs.Var(&layers, "layer", "one layer, bottom first: `paths` of files or directories separated by commas (repeatable)")
	agent := fs.String("agent", "", "agent prefix: ai, claude, codex, copilot, cursor")
	issue := fs.Int("issue", 0, "linked issue number: closed by the bottom layer, referred to by the others")
	local := fs.Bool("local", false, "commit the layers only: no push, no pull request")
	draft := fs.Bool("draft", false, "open the pull requests as drafts")
	skipGates := fs.Bool("skip-gates", false, "push without running lint and tests (said in the pull requests)")
	dryRun := dryRunFlag(fs)
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	r = r.rehearse(*dryRun)
	publishes, err := app.PublishesAfterShip(r.ctx, r.deps)
	if err != nil {
		return r.fail(err)
	}
	*local = *local || !publishes
	view, err := app.ProposeStack(r.ctx, r.deps, app.StackPlanRequest{Layers: layers, Agent: *agent})
	if err != nil {
		return r.fail(err)
	}
	// As for ship: --yes and a dry run ask nothing, the screen when it can.
	mode := r.reviewModeFor(*yes || *dryRun)
	if !*asJSON && mode != reviewScreen {
		r.printStackPlan(view)
	}
	if len(view.Secrets) > 0 {
		return r.fail(&app.SecretsFoundError{Findings: view.Secrets})
	}
	req := app.StackDeliveryRequest{}
	for _, l := range view.Layers {
		req.Layers = append(req.Layers, app.StackLayerSpec{Title: l.Title, Branch: l.Branch, Files: l.Files})
	}
	if mode != reviewNone {
		outcome, code, ok := r.reviewStack(mode, view, *agent)
		if !ok {
			return code
		}
		if mode != reviewConfirm {
			req.Layers = outcome.layers
		}
		*local = *local || outcome.action == reviewLocal
	}
	delivery, err := app.DeliverStack(r.ctx, r.deps, req)
	if err != nil {
		return r.fail(err)
	}
	shipped := stackShipped{Delivery: delivery}
	if !*asJSON && !*dryRun {
		for i, l := range delivery.Layers {
			r.note(i18n.Render(r.lang, "cli.stack.committed", map[string]string{
				"position": strconv.Itoa(i + 1), "commit": shortCommit(l.Commit), "branch": l.Branch}))
		}
	}
	if *local {
		if *asJSON {
			return r.writeJSON(shipped)
		}
		if *dryRun {
			return r.printPlanned(delivery.Rehearsal)
		}
		r.note(i18n.Render(r.lang, "cli.ship.undo_hint", nil))
		r.status(r.localOnlyLine(app.RemoteProblem(r.ctx, r.deps), "cli.stack.publish_later"))
		return ExitOK
	}
	publication, err := app.PublishStack(r.ctx, r.deps, app.StackPublishRequest{
		Issue: *issue, Unattended: *yes, ForceDraft: *draft, SkipGates: *skipGates, GateObserver: r.gateProgress(*asJSON),
	})
	if app.IsLocalRepository(err) {
		if *asJSON {
			return r.writeJSON(shipped)
		}
		if *dryRun {
			return r.printPlanned(delivery.Rehearsal)
		}
		r.status(r.localOnlyLine(err, "cli.stack.publish_later"))
		return ExitOK
	}
	if err != nil {
		code := r.fail(err)
		if !*dryRun {
			r.status(i18n.Render(r.lang, "cli.stack.publish_later", nil))
		}
		return code
	}
	shipped.Publication = &publication
	if *asJSON {
		return r.writeJSON(shipped)
	}
	if *dryRun {
		return r.printPlanned(delivery.Rehearsal, publication.Rehearsal)
	}
	r.printStackPublication(publication)
	return ExitOK
}

// reviewStack asks the human to review the layers as mode says. ok is false
// when nothing ships: code is then the exit code.
func (r runner) reviewStack(mode reviewMode, view app.StackPlanView, agent string) (outcome stackReviewOutcome, code int, ok bool) {
	var files []string
	for _, l := range view.Layers {
		files = append(files, l.Files...)
	}
	cfg := stackReviewConfig{lang: r.lang, plan: view}
	if mode == reviewScreen || mode == reviewPlain {
		// Unreadable settings keep the built-in conventions: the delivery
		// reports the broken configuration itself.
		cfg.conventions, _ = app.LoadConventions(r.ctx, r.deps, files)
		cfg.checkBranch = app.DeliveryBranchCheck(r.ctx, r.deps, files)
		cfg.branchFor = func(title string) (string, bool) {
			b, err := cfg.conventions.BranchForTitleOf(title, app.BranchRequest{Agent: agent})
			return b, err == nil
		}
		cfg.replan = func(layers [][]string) (app.StackPlanView, error) {
			return app.ProposeStack(r.ctx, r.deps, app.StackPlanRequest{Layers: layers, Agent: agent})
		}
	}
	switch mode {
	case reviewNobody:
		r.status(i18n.Render(r.lang, "cli.stack.needs_human", nil))
		return outcome, ExitNeedsHuman, false
	case reviewConfirm:
		outcome.action = reviewCancel
		if r.confirm(i18n.Render(r.lang, "cli.stack.confirm", map[string]string{"count": strconv.Itoa(len(view.Layers))})) {
			outcome.action = reviewShip
		}
	case reviewScreen:
		var started bool
		if outcome, started = r.runStackReviewScreen(cfg); !started {
			r.printStackPlan(view)
			outcome = r.plainStackReview(cfg)
		}
	case reviewPlain:
		outcome = r.plainStackReview(cfg)
	}
	if outcome.action == reviewCancel {
		r.status(i18n.Render(r.lang, "cli.ship.aborted", nil))
		return outcome, ExitAborted, false
	}
	return outcome, ExitOK, true
}

func shortCommit(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}

func (r runner) printStackPlan(view app.StackPlanView) {
	out := r.streams.Out
	fmt.Fprintln(out, i18n.Render(r.lang, "cli.stack.plan", map[string]string{"count": strconv.Itoa(len(view.Layers)), "base": view.Base}))
	for i, l := range view.Layers {
		fmt.Fprintf(out, "  %d. %s\n", i+1, l.Title)
		fmt.Fprintf(out, "     %s\n", i18n.Render(r.lang, "cli.stack.layer", map[string]string{
			"branch": l.Branch, "files": strconv.Itoa(len(l.Files)), "lines": strconv.Itoa(l.Lines), "area": l.Area}))
	}
	if len(view.Left) > 0 {
		fmt.Fprintln(out, "  "+i18n.Render(r.lang, "cli.stack.left", map[string]string{"files": strings.Join(view.Left, ", ")}))
	}
}

func (r runner) printStackPublication(p app.StackPublishResult) {
	key := "cli.stack.published_chain"
	if p.Native && p.Number != 0 {
		key = "cli.stack.published"
	}
	r.note(i18n.Render(r.lang, key, map[string]string{"count": strconv.Itoa(len(p.Layers)), "number": strconv.Itoa(p.Number), "base": p.Base}))
	for i, l := range p.Layers {
		lineKey := "cli.stack.pull_request"
		if !l.Created {
			lineKey = "cli.stack.pull_request_updated"
		}
		if l.Created && l.PullRequest.Draft {
			lineKey = "cli.stack.pull_request_draft"
		}
		r.note("  " + i18n.Render(r.lang, lineKey, map[string]string{
			"position": strconv.Itoa(i + 1), "number": strconv.Itoa(l.PullRequest.Number), "base": l.Base, "url": l.PullRequest.URL}))
	}
	r.printWarnings(p.Warnings)
	for _, l := range p.Layers {
		fmt.Fprintln(r.streams.Out, l.PullRequest.URL)
	}
}

func (r runner) printWarnings(warnings []app.ProblemView) {
	for _, w := range warnings {
		r.status("  ⚠ " + w.Code + "  " + i18n.Render(r.lang, w.Key, w.Params))
	}
}

func (r runner) stackPublish(args []string) int {
	fs, asJSON := r.flags("stack publish")
	yes := fs.Bool("yes", false, "accept without asking")
	fs.BoolVar(yes, "y", false, "shorthand for --yes")
	issue := fs.Int("issue", 0, "linked issue number: closed by the bottom layer, referred to by the others")
	draft := fs.Bool("draft", false, "open the pull requests as drafts")
	skipGates := fs.Bool("skip-gates", false, "push without running lint and tests (said in the pull requests)")
	dryRun := dryRunFlag(fs)
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	_ = yes
	r = r.rehearse(*dryRun)
	result, err := app.PublishStack(r.ctx, r.deps, app.StackPublishRequest{Issue: *issue, ForceDraft: *draft, SkipGates: *skipGates, GateObserver: r.gateProgress(*asJSON)})
	if err != nil {
		return r.fail(err)
	}
	if *asJSON {
		return r.writeJSON(result)
	}
	if *dryRun {
		return r.printPlanned(result.Rehearsal)
	}
	r.printStackPublication(result)
	return ExitOK
}

func (r runner) stackStatus(args []string) int {
	fs, asJSON := r.flags("stack status")
	offline := fs.Bool("offline", false, "do not ask GitHub")
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	view, err := app.StackStatus(r.ctx, r.deps, *offline)
	if err != nil {
		return r.fail(err)
	}
	if *asJSON {
		return r.writeJSON(view)
	}
	out := r.streams.Out
	t := func(key string, params map[string]string) string { return i18n.Render(r.lang, key, params) }
	header := t("cli.stack.status_chain", map[string]string{"base": view.Base})
	if view.Native && view.Number != 0 {
		header = t("cli.stack.status_native", map[string]string{"number": strconv.Itoa(view.Number), "base": view.Base})
	}
	fmt.Fprintln(out, header)
	// Top first, as GitHub draws a stack.
	for i := len(view.Layers) - 1; i >= 0; i-- {
		l := view.Layers[i]
		marker := " "
		if l.Current {
			marker = "▸"
		}
		pr := t("cli.stack.status_no_pull_request", nil)
		if p := l.PullRequest; p != nil {
			state := "open"
			switch {
			case p.Merged:
				state = "merged"
			case p.State == "closed":
				state = "closed"
			case p.Draft:
				state = "draft"
			}
			pr = t("cli.stack.status_pull_request."+state, map[string]string{"number": strconv.Itoa(p.Number)})
		}
		checks := ""
		if ci := l.CI; ci != nil && ci.State != "none" {
			checks = "  " + t("cli.status.checks."+string(ci.State), map[string]string{
				"failing": strings.Join(ci.Failing, ", "), "pending": strconv.Itoa(ci.Pending), "total": strconv.Itoa(ci.Total)})
		}
		fmt.Fprintf(out, "%s %d. %s  %s%s\n", marker, i+1, l.Branch, pr, checks)
	}
	if p := view.GitHubProblem; p != nil {
		r.status("  ⚠ " + p.Code + "  " + t(p.Key, p.Params))
	}
	return ExitOK
}

func (r runner) stackSync(args []string) int {
	fs, asJSON := r.flags("stack sync")
	dryRun := dryRunFlag(fs)
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	r = r.rehearse(*dryRun)
	result, err := app.SyncStack(r.ctx, r.deps)
	if err != nil {
		return r.fail(err)
	}
	if *asJSON {
		return r.writeJSON(result)
	}
	if *dryRun {
		return r.printPlanned(result.Rehearsal)
	}
	params := map[string]string{
		"deleted": strings.Join(result.Deleted, ", "), "rebased": strings.Join(result.Rebased, ", "),
		"followed": strings.Join(result.Followed, ", "), "branch": result.Branch,
	}
	r.note(i18n.Render(r.lang, "cli.stack.synced", params))
	if len(result.Rebased) > 0 {
		r.note(i18n.Render(r.lang, "cli.stack.rebased", params))
	}
	if len(result.Followed) > 0 {
		r.note(i18n.Render(r.lang, "cli.stack.followed", params))
	}
	r.printWarnings(result.Warnings)
	r.note(i18n.Render(r.lang, "cli.ship.undo_hint", nil))
	return ExitOK
}

func (r runner) stackMerge(args []string) int {
	fs, asJSON := r.flags("stack merge")
	yes := fs.Bool("yes", false, "merge without asking (GitHub's rules still apply)")
	fs.BoolVar(yes, "y", false, "shorthand for --yes")
	dryRun := dryRunFlag(fs)
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	r = r.rehearse(*dryRun)
	plan, err := app.PlanStackMerge(r.ctx, r.deps)
	if err != nil {
		return r.fail(err)
	}
	numbers := make([]string, len(plan.Merges))
	for i, pr := range plan.Merges {
		numbers[i] = "#" + strconv.Itoa(pr.Number)
	}
	params := map[string]string{"numbers": strings.Join(numbers, ", "), "base": plan.Base, "target": strconv.Itoa(plan.Target.Number)}
	if !*asJSON {
		key := "cli.stack.merge_plan_chain"
		if plan.Native {
			key = "cli.stack.merge_plan"
		}
		fmt.Fprintln(r.streams.Out, i18n.Render(r.lang, key, params))
	}
	// Merging is a human's gesture: a terminal confirmation, or --yes typed
	// by that human (decision of 2026-11-02).
	confirmed := *yes
	if !confirmed && !*dryRun {
		if !r.streams.Interactive {
			_, err := app.MergeStack(r.ctx, r.deps, app.StackMergeRequest{}) // refused: nobody confirmed
			return r.fail(err)
		}
		if !r.confirm(i18n.Render(r.lang, "cli.stack.merge_confirm", params)) {
			r.status(i18n.Render(r.lang, "cli.ship.aborted", nil))
			return ExitAborted
		}
		confirmed = true
	}
	if !*asJSON && !*dryRun {
		r.note(i18n.Render(r.lang, "cli.stack.merge_waiting", params))
	}
	result, err := app.MergeStack(r.ctx, r.deps, app.StackMergeRequest{Confirmed: confirmed})
	if err != nil {
		return r.fail(err)
	}
	if *asJSON {
		return r.writeJSON(result)
	}
	if *dryRun {
		return r.printPlanned(result.Rehearsal)
	}
	if result.Status.Status == "merged" {
		r.note(i18n.Render(r.lang, "cli.stack.merged", params))
	}
	r.printWarnings(result.Warnings)
	return ExitOK
}
