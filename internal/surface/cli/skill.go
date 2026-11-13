// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"strings"

	"github.com/GregoireF/ship-it/internal/app"
	"github.com/GregoireF/ship-it/internal/i18n"
)

// skillClients are the agents ship skill install knows, as --client takes
// them.
var skillClients = []string{"claude", "vscode", "cursor", "codex"}

// skillCommand handles `ship skill install` (#50).
func (r runner) skillCommand(args []string) int {
	if r.groupHelp("skill", args) {
		return ExitOK
	}
	if len(args) == 0 || args[0] != "install" {
		r.status(i18n.Render(r.lang, "cli.error.unknown_command", map[string]string{"command": strings.TrimSpace("skill " + firstOf(args))}))
		return ExitUsage
	}
	fs, asJSON := r.flags("skill install")
	yes := fs.Bool("yes", false, "write without asking")
	fs.BoolVar(yes, "y", false, "shorthand for --yes")
	client := fs.String("client", "", "one AI agent only: "+strings.Join(skillClients, ", ")+" (default: every agent)")
	printOnly := fs.Bool("print", false, "print the skill, writing nothing")
	dryRun := dryRunFlag(fs)
	if code, ok := r.parse(fs, args[1:]); !ok {
		return code
	}
	if *client != "" && !containsString(skillClients, *client) {
		r.status(i18n.Render(r.lang, "cli.mcp.unknown_client", map[string]string{"client": *client, "clients": strings.Join(skillClients, ", ")}))
		return ExitUsage
	}
	if *printOnly {
		_, _ = r.streams.Out.Write(app.SkillContent())
		return ExitOK
	}
	r = r.rehearse(*dryRun)
	plan, err := app.InstallSkill(r.ctx, r.deps, *client, false)
	if err != nil {
		return r.fail(err)
	}
	if !plan.Changed {
		if *asJSON {
			return r.writeJSON(plan)
		}
		r.skillNotes(&plan)
		r.note(i18n.Render(r.lang, "cli.skill.already", nil))
		return ExitOK
	}
	if !*asJSON {
		r.skillPreview(&plan)
	}
	if !*yes && !*dryRun {
		if !r.streams.Interactive {
			r.status(i18n.Render(r.lang, "cli.skill.needs_human", nil))
			return ExitNeedsHuman
		}
		if !r.confirm(i18n.Render(r.lang, "cli.mcp.confirm", map[string]string{"file": strings.Join(changedSkillFiles(&plan), ", ")})) {
			r.status(i18n.Render(r.lang, "cli.ship.aborted", nil))
			return ExitAborted
		}
	}
	done, err := app.InstallSkill(r.ctx, r.deps, *client, true)
	if err != nil {
		return r.fail(err)
	}
	if *asJSON {
		return r.writeJSON(done)
	}
	if *dryRun {
		return r.printPlanned(done.Rehearsal)
	}
	r.note(i18n.Render(r.lang, "cli.skill.done", map[string]string{"files": strings.Join(changedSkillFiles(&done), ", ")}))
	r.skillNotes(&done)
	return ExitOK
}

// changedSkillFiles are the files an installation writes.
func changedSkillFiles(s *app.SkillInstall) []string {
	var paths []string
	if s == nil {
		return paths
	}
	for _, f := range s.Files {
		if f.Created || f.Updated {
			paths = append(paths, f.Path)
		}
	}
	return paths
}

// skillPreview lists what an installation would write.
func (r runner) skillPreview(s *app.SkillInstall) {
	for _, f := range s.Files {
		switch {
		case f.Created:
			r.status(i18n.Render(r.lang, "cli.skill.create", map[string]string{"file": f.Path}))
		case f.Updated:
			r.status(i18n.Render(r.lang, "cli.skill.update", map[string]string{"file": f.Path}))
		}
	}
	r.skillNotes(s)
}

// skillNotes says which SKILL.md files are the user's and stay as they are.
func (r runner) skillNotes(s *app.SkillInstall) {
	if s == nil {
		return
	}
	for _, f := range s.Files {
		if f.Kept {
			r.note(i18n.Render(r.lang, "cli.skill.kept", map[string]string{"file": f.Path}))
		}
	}
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
