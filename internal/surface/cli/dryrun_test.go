// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/app"
	"github.com/GregoireF/ship-it/internal/i18n"
)

// writingCommands are the flag sets of the commands that change something:
// each must take --dry-run (docs/reference/cli.md, "Dry runs"); the others, which
// only read, must not.
var writingCommands = map[string]bool{
	"ship": true, "publish": true, "undo": true, "tidy": true, "fix": true, "sync": true, "init": true,
	"config ": true, "mcp install": true, "label": true, "gc": true, "roadmap sync": true,
	"stack": true, "stack publish": true, "stack sync": true, "stack merge": true,
	"check gates":       true, // lists the gates: they may write build outputs
	"checkpoint create": true, "checkpoint restore": true,
	"uninstall": true,
}

func TestEveryWritingCommandTakesDryRun(t *testing.T) {
	dryRun := regexp.MustCompile(`(?m)^\s+--dry-run\b`)
	for name, args := range helpArgs {
		_, stdout, _ := run(args...)
		if got, want := dryRun.MatchString(stdout), writingCommands[name]; got != want {
			t.Errorf("%s: --dry-run defined %v, want %v", name, got, want)
		}
	}
}

func runnerForTest() (runner, *bytes.Buffer) {
	var out, errOut bytes.Buffer
	return runner{lang: i18n.English, streams: Streams{Out: &out, Err: &errOut}}, &out
}

func TestPrintPlannedRendersEveryStep(t *testing.T) {
	r, out := runnerForTest()
	r.printPlanned(app.Rehearsal{DryRun: true, Planned: []app.PlannedStep{
		{Key: "app.dryrun.push", Params: map[string]string{"branch": "feat/vat", "remote": "origin"}, Remote: true},
	}})
	if got := out.String(); !strings.Contains(got, "Dry run: nothing was changed") || !strings.Contains(got, "• push feat/vat to origin") {
		t.Errorf("output:\n%s", got)
	}

	r, out = runnerForTest()
	r.lang = i18n.French
	r.printPlanned(app.Rehearsal{DryRun: true})
	if got := out.String(); !strings.Contains(got, "Simulation : rien n'a été modifié") {
		t.Errorf("output:\n%s", got)
	}
}
