// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"fmt"
	"strings"

	"github.com/GregoireF/ship-it/internal/app"
	"github.com/GregoireF/ship-it/internal/i18n"
)

// logLimit is how many operations `ship log` shows by default.
const logLimit = 20

// log shows what ship-it did, newest first, and what ship undo reverts next.
func (r runner) log(args []string) int {
	fs, asJSON := r.flags("log")
	limit := fs.Int("n", logLimit, "number of operations")
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	entries, err := app.Log(r.ctx, r.deps, max(*limit, 1))
	if err != nil {
		return r.fail(err)
	}
	if *asJSON {
		return r.writeJSON(entries)
	}
	if len(entries) == 0 {
		r.note(i18n.Render(r.lang, "cli.log.empty", nil))
		return ExitOK
	}
	anyNext := false
	for _, e := range entries {
		marker := "  "
		if e.Next {
			marker = "↶ "
			anyNext = true
		}
		what := i18n.Render(r.lang, "cli.log.kind."+e.Kind, map[string]string{
			"title": e.Title, "branch": e.Branch, "url": e.URL, "branches": strings.Join(e.Deleted, ", "), "undoes": shortID(e.Undoes),
		})
		line := fmt.Sprintf("%s%s  %s  %s", marker, e.StartedAt.Local().Format("2006-01-02 15:04"), shortID(e.ID), what)
		if e.UndoneBy != "" {
			line += "  " + i18n.Render(r.lang, "cli.log.undone", nil)
		}
		fmt.Fprintln(r.streams.Out, line)
	}
	// The legend explains the mark; without one it would only puzzle.
	if anyNext {
		r.note(i18n.Render(r.lang, "cli.log.legend", nil))
	}
	return ExitOK
}

// shortID abbreviates an operation id for humans; JSON keeps it whole. Ids
// start with their timestamp, so the random end tells them apart.
func shortID(id string) string {
	if len(id) > 8 {
		return id[len(id)-8:]
	}
	return id
}
