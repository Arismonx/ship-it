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

// roadmap runs `ship roadmap sync [--yes] [--dry-run] [--file PATH] [--json]`:
// the plan is shown, then applied after a confirmation, as `ship sync` does.
// A dry run stops after the plan, which already says every change in words.
func (r runner) roadmap(args []string) int {
	if r.groupHelp("roadmap", args) {
		return ExitOK
	}
	if len(args) == 0 || args[0] != "sync" {
		r.status(i18n.Render(r.lang, "cli.error.unknown_command", map[string]string{"command": strings.TrimSpace("roadmap " + firstOf(args))}))
		return ExitUsage
	}
	fs, asJSON := r.flags("roadmap sync")
	yes := fs.Bool("yes", false, "apply without asking")
	dryRun := dryRunFlag(fs)
	file := fs.String("file", app.RoadmapFile, "roadmap file, relative to the repository root")
	if code, ok := r.parse(fs, args[1:]); !ok {
		return code
	}
	plan, err := app.PlanRoadmap(r.ctx, r.deps, *file)
	if err != nil {
		return r.fail(err)
	}
	if !*asJSON {
		r.printRoadmapPlan(plan)
	}
	if plan.Empty() || *dryRun {
		if *asJSON {
			return r.writeJSON(struct {
				app.RoadmapPlan
				DryRun bool `json:"dry_run,omitempty"`
			}{plan, *dryRun})
		}
		if *dryRun && !plan.Empty() {
			r.note(i18n.Render(r.lang, "cli.dryrun.unchanged", nil))
		}
		return ExitOK
	}
	if !*yes {
		if !r.streams.Interactive {
			r.status(i18n.Render(r.lang, "cli.roadmap.needs_human", nil))
			return ExitNeedsHuman
		}
		if !r.confirm(i18n.Render(r.lang, "cli.roadmap.confirm", map[string]string{"count": strconv.Itoa(len(plan.Changes))})) {
			r.status(i18n.Render(r.lang, "cli.ship.aborted", nil))
			return ExitAborted
		}
	}
	result, err := app.ApplyRoadmap(r.ctx, r.deps, plan)
	if *asJSON {
		r.writeJSON(struct {
			Plan   app.RoadmapPlan   `json:"plan"`
			Result app.RoadmapResult `json:"result"`
		}{plan, result})
	}
	if err != nil {
		r.status(i18n.Render(r.lang, "cli.roadmap.partial", map[string]string{"applied": strconv.Itoa(result.Applied), "total": strconv.Itoa(len(plan.Changes))}))
		return r.fail(err)
	}
	if !*asJSON {
		r.note(i18n.Render(r.lang, "cli.roadmap.done", map[string]string{"applied": strconv.Itoa(result.Applied)}))
		if result.FileChanged {
			r.note(i18n.Render(r.lang, "cli.roadmap.file_changed", map[string]string{"file": plan.Path}))
		}
	}
	return ExitOK
}

// printRoadmapPlan lists the steps; each says in words what it does.
func (r runner) printRoadmapPlan(plan app.RoadmapPlan) {
	out := r.streams.Out
	params := map[string]string{"repository": plan.Repository.Slug(), "file": plan.Path}
	defer func() {
		// What the project lacks is never created: said after the plan.
		for _, w := range plan.ProjectProblems {
			r.status("  ⚠ " + w.Code + "  " + i18n.Render(r.lang, w.Key, w.Params))
		}
	}()
	if plan.Empty() {
		fmt.Fprintln(out, i18n.Render(r.lang, "cli.roadmap.in_sync", params))
		return
	}
	fmt.Fprintln(out, i18n.Render(r.lang, "cli.roadmap.plan", params))
	for _, c := range plan.Changes {
		line := map[string]string{"phase": c.Phase, "title": c.Title, "number": strconv.Itoa(c.Number), "line": strconv.Itoa(c.Line),
			"field": c.Field, "value": c.Value}
		if plan.Project != nil {
			line["project"] = plan.Project.String()
		}
		key := "cli.roadmap.step." + string(c.Kind)
		// The issue an earlier step creates is named by its line.
		switch {
		case c.Number == 0 && c.Kind == "add_to_project":
			key = "cli.roadmap.step.project_add_line"
		case c.Number == 0 && c.Kind == "set_project_field":
			key = "cli.roadmap.step.project_set_line"
		}
		fmt.Fprintln(out, "  "+i18n.Render(r.lang, key, line))
	}
}

func firstOf(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}
