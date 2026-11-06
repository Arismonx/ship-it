// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/GregoireF/ship-it/internal/app"
	"github.com/GregoireF/ship-it/internal/i18n"
)

// mcpNext is the step a client asks of the user after the install.
var mcpNext = map[string]string{"claude": "cli.mcp.next.claude", "codex": "cli.mcp.next.codex"}

// mcpCommand handles `ship mcp install`; `ship mcp` itself (the server) is
// started by cmd/ship, which owns stdin and stdout.
func (r runner) mcpCommand(args []string) int {
	if r.groupHelp("mcp", args) {
		return ExitOK
	}
	if len(args) == 0 || args[0] != "install" {
		r.status(i18n.Render(r.lang, "cli.error.unknown_command", map[string]string{"command": strings.TrimSpace("mcp " + firstOf(args))}))
		return ExitUsage
	}
	fs, asJSON := r.flags("mcp install")
	yes := fs.Bool("yes", false, "write without asking")
	client := fs.String("client", "claude", "the AI agent to configure: "+strings.Join(app.MCPClients(), ", "))
	printOnly := fs.Bool("print", false, "print what declares ship-it for the client, writing nothing")
	dryRun := dryRunFlag(fs)
	if code, ok := r.parse(fs, args[1:]); !ok {
		return code
	}
	if !slices.Contains(app.MCPClients(), *client) {
		r.status(i18n.Render(r.lang, "cli.mcp.unknown_client", map[string]string{"client": *client, "clients": strings.Join(app.MCPClients(), ", ")}))
		return ExitUsage
	}
	if *printOnly {
		_, snippet, err := app.MCPSnippet(*client)
		if err != nil {
			return r.fail(err)
		}
		fmt.Fprint(r.streams.Out, snippet)
		return ExitOK
	}
	r = r.rehearse(*dryRun)
	plan, err := app.InstallMCP(r.ctx, r.deps, *client, false)
	if err != nil {
		return r.failByHand(err, *client)
	}
	if !plan.Changed {
		if *asJSON {
			return r.writeJSON(plan)
		}
		key := "cli.mcp.already"
		if plan.Kept {
			key = "cli.mcp.kept"
		}
		r.note(i18n.Render(r.lang, key, map[string]string{"file": plan.Path}))
		return ExitOK
	}
	if !*asJSON {
		r.status(i18n.Render(r.lang, "cli.mcp.preview", map[string]string{"file": plan.Path}))
		fmt.Fprint(r.streams.Out, plan.After)
	}
	if !*yes && !*dryRun {
		if !r.streams.Interactive {
			r.status(i18n.Render(r.lang, "cli.mcp.needs_human", nil))
			return ExitNeedsHuman
		}
		if !r.confirm(i18n.Render(r.lang, "cli.mcp.confirm", map[string]string{"file": plan.Path})) {
			r.status(i18n.Render(r.lang, "cli.ship.aborted", nil))
			return ExitAborted
		}
	}
	done, err := app.InstallMCP(r.ctx, r.deps, *client, true)
	if err != nil {
		return r.fail(err)
	}
	if *asJSON {
		return r.writeJSON(done)
	}
	if *dryRun {
		return r.printPlanned(done.Rehearsal)
	}
	r.note(i18n.Render(r.lang, "cli.mcp.done", map[string]string{"file": done.Path}))
	if key, ok := mcpNext[done.Client]; ok {
		r.note(i18n.Render(r.lang, key, nil))
	}
	return ExitOK
}

// failByHand reports a file ship-it may not change, then prints on stdout
// what the user adds to it by hand.
func (r runner) failByHand(err error, client string) int {
	code := r.fail(err)
	var f *app.Fault
	if !errors.As(err, &f) || (f.Key != "mcp.manual_edit" && f.Key != "config.invalid_json") {
		return code
	}
	path, snippet, serr := app.MCPSnippet(client)
	if serr == nil {
		r.status(i18n.Render(r.lang, "cli.mcp.by_hand", map[string]string{"file": path}))
		fmt.Fprint(r.streams.Out, snippet)
	}
	return code
}
