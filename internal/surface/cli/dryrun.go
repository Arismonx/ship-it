// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"flag"
	"fmt"

	"github.com/GregoireF/ship-it/internal/app"
	"github.com/GregoireF/ship-it/internal/i18n"
)

// dryRunFlag adds --dry-run to a command that writes. It is a flag of each
// such command, not a global one: on a command that only reads it would be
// a promise with nothing behind it (docs/reference/cli.md, "Dry runs").
func dryRunFlag(fs *flag.FlagSet) *bool {
	return fs.Bool("dry-run", false, "run every check and show what would change, without changing anything")
}

// rehearse returns the runner whose workspace changes nothing, on a dry run.
func (r runner) rehearse(dryRun bool) runner {
	if dryRun {
		r.deps = app.DryRun(r.deps)
	}
	return r
}

// printPlanned shows, on stdout (it is the result of a dry run), every step
// the real run would take, in order.
func (r runner) printPlanned(rehearsals ...app.Rehearsal) int {
	var steps []app.PlannedStep
	for _, rh := range rehearsals {
		steps = append(steps, rh.Planned...)
	}
	out := r.streams.Out
	if len(steps) == 0 {
		fmt.Fprintln(out, i18n.Render(r.lang, "cli.dryrun.nothing", nil))
		return ExitOK
	}
	fmt.Fprintln(out, i18n.Render(r.lang, "cli.dryrun.header", nil))
	for _, s := range steps {
		fmt.Fprintln(out, "  • "+i18n.Render(r.lang, s.Key, s.Params))
	}
	r.note(i18n.Render(r.lang, "cli.dryrun.unchanged", nil))
	return ExitOK
}
