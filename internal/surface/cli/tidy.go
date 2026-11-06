// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"errors"
	"strconv"
	"strings"

	"github.com/GregoireF/ship-it/internal/app"
	"github.com/GregoireF/ship-it/internal/i18n"
)

// tidyCommand runs the cleanup on demand; `ship` also runs it first.
func (r runner) tidyCommand(args []string) int {
	fs, asJSON := r.flags("tidy")
	dryRun := dryRunFlag(fs)
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	r = r.rehearse(*dryRun)
	result, err := app.Tidy(r.ctx, r.deps)
	if err != nil {
		return r.fail(err)
	}
	if *asJSON {
		return r.writeJSON(result)
	}
	if *dryRun {
		return r.printPlanned(result.Rehearsal)
	}
	r.printTidy(result, nil)
	if result.LeftBranch == "" && len(result.Deleted) == 0 && len(result.Kept) == 0 {
		r.note(i18n.Render(r.lang, "cli.tidy.nothing", nil))
	}
	return ExitOK
}

// printTidy reports what the cleanup did; err is shown as a warning.
func (r runner) printTidy(t app.TidyResult, err error) {
	if err != nil {
		var f *app.Fault
		if errors.As(err, &f) {
			r.status("⚠ " + r.renderFault(f))
		} else {
			r.status("⚠ " + err.Error())
		}
		return
	}
	params := map[string]string{"branch": t.LeftBranch, "number": strconv.Itoa(t.PullRequest), "base": t.Base}
	switch {
	case t.LeftBranch != "" && t.StayedBecauseOfConflicts:
		r.status(i18n.Render(r.lang, "cli.tidy.stayed", params))
	case t.LeftBranch != "":
		r.note(i18n.Render(r.lang, "cli.tidy.left", params))
	}
	if t.BaseDiverged {
		r.status(i18n.Render(r.lang, "cli.tidy.base_diverged", params))
	}
	if len(t.Deleted) > 0 {
		r.note(i18n.Render(r.lang, "cli.tidy.deleted", map[string]string{"branches": strings.Join(t.Deleted, ", ")}))
	}
	if len(t.Kept) > 0 {
		r.status(i18n.Render(r.lang, "cli.tidy.kept", map[string]string{"branches": strings.Join(t.Kept, ", ")}))
	}
}
