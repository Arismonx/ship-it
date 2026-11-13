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

// rewindFiles is how many files a list line names before "(+N)".
const rewindFiles = 4

// rewind lists the recent checkpoints with what changed in each, then puts
// the files back as in the one chosen (#39). It takes a checkpoint first, so
// ship undo reverts it.
func (r runner) rewind(args []string) int {
	fs, asJSON := r.flags("rewind")
	limit := fs.Int("limit", 10, "how many checkpoints to list")
	yes := fs.Bool("yes", false, "rewind without asking (with a checkpoint given)")
	fs.BoolVar(yes, "y", false, "shorthand for --yes")
	dryRun := dryRunFlag(fs)
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	t := func(key string, params map[string]string) string { return i18n.Render(r.lang, key, params) }
	choice := fs.Arg(0)
	if choice == "" {
		entries, err := app.RewindList(r.ctx, r.deps, max(*limit, 1))
		if err != nil {
			return r.fail(err)
		}
		if *asJSON && (*dryRun || !r.streams.Interactive) {
			return r.writeJSON(entries)
		}
		if len(entries) == 0 {
			r.note(t("cli.rewind.none", nil))
			return ExitOK
		}
		for i, e := range entries {
			branch := ""
			if e.Branch != "" {
				branch = "  (" + e.Branch + ")"
			}
			fmt.Fprintln(r.streams.Out, t("cli.rewind.entry", map[string]string{"n": strconv.Itoa(i + 1), "id": e.ShortID,
				"age": e.CreatedAt.Local().Format("2006-01-02 15:04"), "label": e.Label, "branch": branch}))
			if len(e.Changes) == 0 {
				fmt.Fprintln(r.streams.Out, t("cli.rewind.no_change", nil))
			} else {
				fmt.Fprintln(r.streams.Out, t("cli.rewind.files", map[string]string{"files": changeList(e.Changes)}))
			}
		}
		if !r.streams.Interactive || *dryRun {
			if !*dryRun {
				r.status(t("cli.rewind.needs_human", nil))
				return ExitNeedsHuman
			}
			return ExitOK
		}
		fmt.Fprintf(r.streams.Err, "%s ", t("cli.rewind.ask", map[string]string{"count": strconv.Itoa(len(entries))}))
		answer, ok := r.readLine()
		if choice = strings.TrimSpace(answer); !ok || choice == "" {
			r.status(t("cli.ship.aborted", nil))
			return ExitAborted
		}
	}
	r = r.rehearse(*dryRun)
	plan, err := app.PrepareRewind(r.ctx, r.deps, choice)
	if err != nil {
		return r.fail(err)
	}
	if *dryRun {
		if *asJSON {
			return r.writeJSON(plan)
		}
		return r.printPlanned(plan.Rehearsal)
	}
	if len(plan.Rewrites) == 0 {
		_ = app.CancelRewind(r.ctx, r.deps, plan)
		r.note(t("cli.rewind.same", map[string]string{"id": plan.Target.ShortID}))
		return ExitOK
	}
	if !*asJSON {
		r.status(t("cli.rewind.rewrites", map[string]string{"id": plan.Target.ShortID, "count": strconv.Itoa(len(plan.Rewrites))}))
		for _, c := range plan.Rewrites {
			r.status("  " + c.Path)
		}
		if len(plan.Kept) > 0 {
			r.status(t("cli.rewind.kept", map[string]string{"count": strconv.Itoa(len(plan.Kept)), "files": strings.Join(plan.Kept, ", ")}))
		}
	}
	if !*yes {
		if !r.streams.Interactive {
			_ = app.CancelRewind(r.ctx, r.deps, plan)
			r.status(t("cli.rewind.needs_human", nil))
			return ExitNeedsHuman
		}
		if !r.confirm(t("cli.rewind.confirm", nil)) {
			_ = app.CancelRewind(r.ctx, r.deps, plan)
			r.status(t("cli.ship.aborted", nil))
			return ExitAborted
		}
	}
	done, err := app.ApplyRewind(r.ctx, r.deps, plan)
	if err != nil {
		return r.fail(err)
	}
	if *asJSON {
		return r.writeJSON(done)
	}
	r.note(t("cli.rewind.done", map[string]string{"id": plan.Target.ShortID}))
	return ExitOK
}

// changeList names the first changed files of a checkpoint, with a sign:
// + added, - deleted, nothing for modified.
func changeList(changes []ports.CheckpointChange) string {
	var names []string
	for i, c := range changes {
		if i == rewindFiles {
			names = append(names, "(+"+strconv.Itoa(len(changes)-rewindFiles)+")")
			break
		}
		switch c.Status {
		case ports.ChangeAdded:
			names = append(names, "+"+c.Path)
		case ports.ChangeDeleted:
			names = append(names, "-"+c.Path)
		default:
			names = append(names, c.Path)
		}
	}
	return strings.Join(names, ", ")
}
