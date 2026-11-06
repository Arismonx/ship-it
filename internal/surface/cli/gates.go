// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"strconv"
	"strings"

	"github.com/GregoireF/ship-it/internal/app"
	"github.com/GregoireF/ship-it/internal/i18n"
)

// gateProgress shows each gate on stderr as it runs; nothing in JSON mode,
// where the results come with the final document.
func (r runner) gateProgress(asJSON bool) app.GateObserver {
	if asJSON {
		return nil
	}
	return progress{r}
}

type progress struct{ r runner }

func (p progress) GateStarted(name, command string) {
	p.r.note(i18n.Render(p.r.lang, "cli.gates.started", map[string]string{"gate": name, "command": command}))
}

func (p progress) GateFinished(g app.GateResult) {
	seconds := strconv.FormatFloat(float64(g.Duration)/1000, 'f', 1, 64)
	p.r.note(i18n.Render(p.r.lang, "cli.gates."+string(g.Status), map[string]string{"gate": g.Name, "seconds": seconds}))
}

// redGateLines bounds the output shown for a red gate: the end is where the
// error is; the full run is in the gate's own tool.
const redGateLines = 40

func (r runner) printRedGate(g app.GateResult) {
	lines := strings.Split(strings.TrimRight(g.Output, "\n"), "\n")
	if len(lines) > redGateLines {
		lines = lines[len(lines)-redGateLines:]
	}
	for _, line := range lines {
		if line != "" {
			r.status("  │ " + line)
		}
	}
}

// checkGates runs `ship check gates [--dry-run]`: the project's checks now,
// without committing or pushing. A red gate exits 3, as for a publication.
func (r runner) checkGates(args []string) int {
	fs, asJSON := r.flags("check gates")
	dryRun := dryRunFlag(fs)
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	r = r.rehearse(*dryRun)
	result, err := app.CheckGates(r.ctx, r.deps, r.gateProgress(*asJSON))
	if err != nil {
		if *asJSON && len(result.Gates) > 0 {
			r.writeJSON(result)
		}
		return r.fail(err)
	}
	if *asJSON {
		return r.writeJSON(result)
	}
	if *dryRun {
		return r.printPlanned(result.Rehearsal)
	}
	key := "cli.check.gates.none" // no gate found, or none with a script
	for _, g := range result.Gates {
		if g.Status == app.GatePassed {
			key = "cli.check.gates.passed"
		}
	}
	r.note(i18n.Render(r.lang, key, nil))
	return ExitOK
}
