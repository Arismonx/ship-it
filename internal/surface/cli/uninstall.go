// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/GregoireF/ship-it/internal/app"
	"github.com/GregoireF/ship-it/internal/i18n"
	"github.com/GregoireF/ship-it/internal/ports"
)

// uninstall removes what ship-it wrote, after listing it (#55): the
// repository's data and agent files, or with --user the user's files. It
// cannot be undone, so the answer defaults to no.
func (r runner) uninstall(args []string) int {
	fs, asJSON := r.flags("uninstall")
	user := fs.Bool("user", false, "remove your own files (configuration, cache, logs) instead of the repository's")
	yes := fs.Bool("yes", false, "remove without asking")
	fs.BoolVar(yes, "y", false, "shorthand for --yes")
	dryRun := dryRunFlag(fs) // lists what would go, removing nothing
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	if fs.NArg() > 0 {
		r.status(i18n.Render(r.lang, "cli.error.unexpected_argument_of", map[string]string{"argument": fs.Arg(0), "command": commandLine(fs.Name())}))
		return ExitUsage
	}
	t := func(key string, params map[string]string) string { return i18n.Render(r.lang, key, params) }
	plan, err := app.Uninstall(r.ctx, r.deps, *user, false)
	if err != nil {
		return r.fail(err)
	}
	if plan.Empty() {
		if *asJSON {
			return r.writeJSON(plan)
		}
		r.listByHand(plan)
		r.note(t("cli.uninstall.nothing", nil))
		return ExitOK
	}
	if !*asJSON {
		r.status(t("cli.uninstall.list", nil))
		for _, l := range plan.Remove {
			r.status("  " + leftoverLine(r.lang, l))
		}
		for _, path := range plan.Edit {
			r.status("  " + t("cli.uninstall.edit", map[string]string{"file": path}))
		}
		r.listByHand(plan)
		if plan.Checkpoints > 0 {
			r.status(t("cli.uninstall.checkpoints", map[string]string{"count": strconv.Itoa(plan.Checkpoints)}))
		}
	}
	if *dryRun {
		if *asJSON {
			return r.writeJSON(plan)
		}
		r.note(t("cli.uninstall.dry_run", nil))
		return ExitOK
	}
	if !*yes {
		if !r.streams.Interactive {
			r.status(t("cli.uninstall.needs_human", nil))
			return ExitNeedsHuman
		}
		if !r.confirmNo(t("cli.uninstall.confirm", nil)) {
			r.status(t("cli.ship.aborted", nil))
			return ExitAborted
		}
	}
	// The run's own journal is in .git/ship/logs or the user's logs: closed
	// first, so that it can go too (Windows keeps open files).
	if r.streams.Journal != nil {
		r.streams.Journal.Close()
	}
	done, err := app.Uninstall(r.ctx, r.deps, *user, true)
	if err != nil {
		return r.fail(err)
	}
	if *asJSON {
		return r.writeJSON(done)
	}
	r.note(t("cli.uninstall.done", nil))
	if !*user {
		r.note(t("cli.uninstall.next_user", nil))
	} else {
		r.note(t("cli.uninstall.next_binary", nil))
	}
	return ExitOK
}

func (r runner) listByHand(plan app.UninstallPlan) {
	for _, path := range plan.ByHand {
		r.status("  " + i18n.Render(r.lang, "cli.uninstall.by_hand", map[string]string{"file": path}))
	}
}

// leftoverLine renders one leftover with its size.
func leftoverLine(lang i18n.Language, l ports.Leftover) string {
	params := map[string]string{"path": l.Path, "size": humanBytes(l.Bytes), "count": strconv.Itoa(l.Files)}
	switch l.Kind {
	case ports.LeftoverRepository:
		return i18n.Render(lang, "cli.uninstall.repository", params)
	case ports.LeftoverAgentFile:
		return i18n.Render(lang, "cli.uninstall.agent_file", params)
	}
	params["holds"] = strings.Join(l.Holds, ", ")
	return i18n.Render(lang, "cli.uninstall.user_dir", params)
}

// confirmNo asks a question whose answer defaults to no: for what cannot be
// undone.
func (r runner) confirmNo(question string) bool {
	fmt.Fprintf(r.streams.Err, "%s ", question)
	answer, ok := r.readLine()
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes", "o", "oui":
		return true
	}
	return false
}
