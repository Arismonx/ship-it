// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"strconv"
	"strings"

	"github.com/GregoireF/ship-it/internal/app"
	"github.com/GregoireF/ship-it/internal/i18n"
)

// label runs `ship label --pr N`: the kind/ and size/ labels of a pull
// request, computed from what GitHub knows of it (triage.yml).
func (r runner) label(args []string) int {
	fs, asJSON := r.flags("label")
	number := fs.Int("pr", 0, "number of the pull request to label")
	dryRun := dryRunFlag(fs)
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	if fs.NArg() > 0 {
		return ExitUsage
	}
	r = r.rehearse(*dryRun)
	if *number <= 0 {
		r.status(i18n.Render(r.lang, "cli.error.missing_argument", map[string]string{"argument": "--pr <number>"}))
		return ExitUsage
	}
	result, err := app.LabelPullRequest(r.ctx, r.deps, *number)
	if err != nil {
		return r.fail(err)
	}
	if *asJSON {
		return r.writeJSON(result)
	}
	params := map[string]string{"number": strconv.Itoa(result.Number)}
	switch {
	case result.Disabled:
		r.note(i18n.Render(r.lang, "cli.label.disabled", nil))
	case len(result.Added)+len(result.Removed) == 0:
		r.note(i18n.Render(r.lang, "cli.label.unchanged", params))
	case *dryRun:
		return r.printPlanned(result.Rehearsal)
	default:
		params["added"] = strings.Join(result.Added, ", ")
		params["removed"] = strings.Join(result.Removed, ", ")
		if params["removed"] == "" {
			params["removed"] = "-"
		}
		if params["added"] == "" {
			params["added"] = "-"
		}
		r.note(i18n.Render(r.lang, "cli.label.changed", params))
	}
	return ExitOK
}
