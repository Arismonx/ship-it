// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"strconv"

	"github.com/GregoireF/ship-it/internal/app"
	"github.com/GregoireF/ship-it/internal/i18n"
)

// checkSignoff runs `ship check signoff [--from REV] [--to REV]`: every
// commit of the range must carry a DCO sign-off matching its author.
func (r runner) checkSignoff(args []string) int {
	fs, asJSON := r.flags("check signoff")
	from := fs.String("from", "", "first excluded revision (default: origin's default branch)")
	to := fs.String("to", "", "last revision (default: HEAD)")
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	if fs.NArg() > 0 {
		return ExitUsage
	}
	result, err := app.CheckSignoffs(r.ctx, r.deps, *from, *to)
	if err != nil {
		return r.fail(err)
	}
	if *asJSON {
		r.writeJSON(result)
		return exitFor(result.Valid())
	}
	if result.Valid() {
		r.note(i18n.Render(r.lang, "cli.check.signoff.valid", map[string]string{
			"count": strconv.Itoa(result.Checked - result.Exempt), "from": result.From, "to": result.To,
		}))
		return ExitOK
	}
	for _, f := range result.Failures {
		r.status("✗ " + abbreviate(f.Commit) + " " + f.Subject)
		r.status("  " + r.renderFault(f.Fault))
	}
	r.status(i18n.Render(r.lang, "cli.check.signoff.fix", map[string]string{"base": result.From}))
	return ExitError
}

// abbreviate shortens a commit id the way git log --oneline shows it.
func abbreviate(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}
