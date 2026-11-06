// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"fmt"

	"github.com/GregoireF/ship-it/internal/app"
	"github.com/GregoireF/ship-it/internal/i18n"
)

// fixCommand lists every remedy, then applies them all after one Enter
// (decision of 2026-10-23), or right away with --yes.
func (r runner) fixCommand(args []string) int {
	fs, asJSON := r.flags("fix")
	yes := fs.Bool("yes", false, "apply the remedies without asking")
	dryRun := dryRunFlag(fs)
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	r = r.rehearse(*dryRun)
	report, err := app.Fix(r.ctx, r.deps, app.FixRequest{})
	if err != nil {
		return r.fail(err)
	}
	if !*asJSON {
		r.printRemedies(report)
	}
	if len(report.Remedies) == 0 {
		if *asJSON {
			return r.writeJSON(report)
		}
		if len(report.Manual) == 0 {
			r.note(i18n.Render(r.lang, "cli.fix.nothing", nil))
		}
		return exitFor(len(report.Manual) == 0)
	}
	if !*yes && !*dryRun {
		if !r.streams.Interactive {
			r.status(i18n.Render(r.lang, "cli.fix.needs_human", nil))
			return ExitNeedsHuman
		}
		if !r.confirm(i18n.Render(r.lang, "cli.fix.confirm", nil)) {
			r.status(i18n.Render(r.lang, "cli.ship.aborted", nil))
			return ExitAborted
		}
	}
	report, err = app.Fix(r.ctx, r.deps, app.FixRequest{Apply: true})
	if *asJSON {
		r.writeJSON(report)
	}
	if err != nil {
		return r.fail(err)
	}
	if *dryRun && !*asJSON {
		return r.printPlanned(report.Rehearsal)
	}
	if !*asJSON {
		if report.Tidy != nil {
			r.printTidy(*report.Tidy, nil)
		}
		r.note(i18n.Render(r.lang, "cli.fix.done", nil))
	}
	return ExitOK
}

func (r runner) printRemedies(report app.FixReport) {
	out := r.streams.Out
	for _, rem := range report.Remedies {
		fmt.Fprintf(out, "  ✚ %s\n", i18n.Render(r.lang, rem.Key, rem.Params))
	}
	for _, f := range report.Manual {
		fmt.Fprintf(out, "  %s  %s\n", severityLabels[f.Severity], i18n.Render(r.lang, f.Key, f.Params))
		if f.Remedy != "" {
			fmt.Fprintf(out, "           → %s\n", i18n.Render(r.lang, f.Remedy, f.Params))
		}
	}
}
