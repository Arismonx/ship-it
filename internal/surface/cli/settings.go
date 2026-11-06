// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/GregoireF/ship-it/internal/app"
	"github.com/GregoireF/ship-it/internal/i18n"
)

// config shows every effective setting and the layer it comes from.
func (r runner) config(args []string) int {
	if code, ok := r.configSubcommand(args); ok {
		return code
	}
	fs, asJSON := r.flags("config")
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	view, err := app.ShowConfig(r.ctx, r.deps)
	if err != nil {
		return r.fail(err)
	}
	if *asJSON {
		return r.writeJSON(view)
	}
	width := 0
	for _, s := range view.Settings {
		width = max(width, utf8.RuneCountInString(s.Key))
	}
	for _, s := range view.Settings {
		source := ""
		switch s.Source {
		case "default":
		case "policy":
			source = "  " + i18n.Render(r.lang, "cli.config.locked", map[string]string{"organization": view.Policy.Organization})
		default:
			source = "  ← " + s.Source
		}
		fmt.Fprintf(r.streams.Out, "%s%s  %s%s\n", s.Key, strings.Repeat(" ", width-utf8.RuneCountInString(s.Key)), s.Value, source)
	}
	if len(view.Files) == 0 {
		r.status(i18n.Render(r.lang, "cli.config.no_file", nil))
	}
	for _, f := range view.Files {
		r.status(i18n.Render(r.lang, "cli.config.file", map[string]string{"file": f}))
	}
	if p := view.Policy; p != nil {
		r.status(i18n.Render(r.lang, "cli.config.policy", map[string]string{
			"organization": p.Organization, "count": strconv.Itoa(len(p.Locked)), "contact": p.Contact,
		}))
	}
	return ExitOK
}

// initCommand writes .github/ship.json; GitHub is changed by `ship sync`.
func (r runner) initCommand(args []string) int {
	fs, asJSON := r.flags("init")
	dryRun := dryRunFlag(fs)
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	r = r.rehearse(*dryRun)
	result, err := app.Init(r.ctx, r.deps)
	if err != nil {
		return r.fail(err)
	}
	if *asJSON {
		return r.writeJSON(result)
	}
	if *dryRun {
		if len(result.Areas) > 0 {
			r.note(i18n.Render(r.lang, "cli.init.areas", map[string]string{"areas": strings.Join(result.Areas, ", ")}))
		}
		return r.printPlanned(result.Rehearsal)
	}
	r.note(i18n.Render(r.lang, "cli.init.created", map[string]string{"file": result.Path}))
	if len(result.Areas) > 0 {
		r.note(i18n.Render(r.lang, "cli.init.areas", map[string]string{"areas": strings.Join(result.Areas, ", ")}))
	}
	r.note(i18n.Render(r.lang, "cli.init.next", nil))
	return ExitOK
}

// sync shows what would change on GitHub, then applies it once a human
// agreed (ADR-0014: plan, review, apply).
func (r runner) sync(args []string) int {
	fs, asJSON := r.flags("sync")
	yes := fs.Bool("yes", false, "apply without asking")
	dryRun := dryRunFlag(fs)
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	r = r.rehearse(*dryRun)
	plan, err := app.PlanSync(r.ctx, r.deps)
	if err != nil {
		return r.fail(err)
	}
	if !*asJSON {
		r.printSyncPlan(plan)
	}
	if plan.Empty() || plan.ReadOnly {
		if *asJSON {
			return r.writeJSON(plan)
		}
		return ExitOK
	}
	if !*yes && !*dryRun {
		if !r.streams.Interactive {
			r.status(i18n.Render(r.lang, "cli.sync.needs_human", nil))
			return ExitNeedsHuman
		}
		if !r.confirm(i18n.Render(r.lang, "cli.sync.confirm", map[string]string{"count": strconv.Itoa(len(plan.Labels))})) {
			r.status(i18n.Render(r.lang, "cli.ship.aborted", nil))
			return ExitAborted
		}
	}
	result, err := app.ApplySync(r.ctx, r.deps, plan)
	if *asJSON {
		r.writeJSON(struct {
			Plan   app.SyncPlan   `json:"plan"`
			Result app.SyncResult `json:"result"`
		}{plan, result})
	}
	if err != nil {
		r.status(i18n.Render(r.lang, "cli.sync.partial", map[string]string{"applied": strconv.Itoa(result.Applied), "total": strconv.Itoa(len(plan.Labels))}))
		return r.fail(err)
	}
	if *dryRun && !*asJSON {
		return r.printPlanned(result.Rehearsal)
	}
	if !*asJSON {
		r.note(i18n.Render(r.lang, "cli.sync.done", map[string]string{"applied": strconv.Itoa(result.Applied)}))
	}
	return ExitOK
}

func (r runner) printSyncPlan(plan app.SyncPlan) {
	out := r.streams.Out
	repository := map[string]string{"repository": plan.Repository.Slug(), "tool": plan.ManagedBy}
	switch {
	case plan.Empty():
		fmt.Fprintln(out, i18n.Render(r.lang, "cli.sync.in_sync", repository))
		return
	case plan.ReadOnly:
		fmt.Fprintln(out, i18n.Render(r.lang, "cli.sync.read_only", repository))
	default:
		fmt.Fprintln(out, i18n.Render(r.lang, "cli.sync.plan", repository))
	}
	// Symbols come with words: the meaning never relies on the symbol alone.
	for _, c := range plan.Labels {
		params := map[string]string{"name": c.Label.Name, "from": c.From, "color": c.Label.Color, "description": c.Label.Description}
		fmt.Fprintln(out, "  "+i18n.Render(r.lang, "cli.sync.label."+string(c.Kind), params))
	}
}
