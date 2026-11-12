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
	var agent agentFlag
	fs.Var(&agent, "agent", "only what AI agents did; --agent claude for one agent")
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	// --agent takes its name as the next word too: ship log --agent claude.
	if agent.name == app.AllAgents && fs.NArg() == 1 {
		agent.name = fs.Arg(0)
	} else if fs.NArg() > 0 {
		r.status(i18n.Render(r.lang, "cli.error.unexpected_argument_of", map[string]string{"argument": fs.Arg(0), "command": commandLine(fs.Name())}))
		return ExitUsage
	}
	entries, err := app.Log(r.ctx, r.deps, max(*limit, 1))
	if err != nil {
		return r.fail(err)
	}
	if agent.name != "" {
		entries = app.FilterByAgent(entries, agent.name)
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
		if e.Agent != "" {
			line += "  " + i18n.Render(r.lang, "cli.log.agent", map[string]string{"agent": e.Agent})
		}
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

// agentFlag is --agent: alone, every agent (app.AllAgents); --agent=claude,
// one of them.
type agentFlag struct{ name string }

func (f *agentFlag) String() string { return f.name }

func (f *agentFlag) Set(v string) error {
	if v == "true" {
		v = app.AllAgents
	}
	f.name = strings.ToLower(strings.TrimSpace(v))
	return nil
}

// IsBoolFlag lets --agent stand alone, like a boolean flag.
func (f *agentFlag) IsBoolFlag() bool { return true }
