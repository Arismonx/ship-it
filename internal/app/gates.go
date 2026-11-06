// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/gates"
	"github.com/GregoireF/ship-it/internal/ports"
)

// gateTimeout bounds one gate: a hanging test suite must not hang ship.
const gateTimeout = 15 * time.Minute

// packageJSONLimit bounds the package.json read for detection.
const packageJSONLimit = 1 << 20

// GateStatus is how a gate ended.
type GateStatus string

// Gate outcomes.
const (
	GatePassed GateStatus = "passed"
	GateFailed GateStatus = "failed"
	// GateSkipped: the project defines no such script.
	GateSkipped GateStatus = "skipped"
	// GateUnavailable: the tool (pnpm, yarn…) is not installed.
	GateUnavailable GateStatus = "unavailable"
)

// GateResult is one gate as surfaces show it.
type GateResult struct {
	Name     string     `json:"name"`
	Command  string     `json:"command,omitempty"`
	Status   GateStatus `json:"status"`
	Duration int64      `json:"duration_ms,omitempty"`
	// Output is the end of the output of a gate that did not pass.
	Output string `json:"output,omitempty"`
}

// GateObserver follows the gates as they run; surfaces show progress with it.
type GateObserver interface {
	GateStarted(name, command string)
	GateFinished(result GateResult)
}

// GateMoment says when the gates run: it decides which files they see and
// what the red-gate message advises.
type GateMoment int

// The moments gates run at.
const (
	// GatesAtPush: before a push; the gates see the commits since the base.
	GatesAtPush GateMoment = iota
	// GatesOnDemand: ship check gates; the gates also see the working tree.
	GatesOnDemand
	// GatesBeforeCommit: ship.gates.beforeCommit; the gates also see the
	// working tree, which the commit is about to take.
	GatesBeforeCommit
)

// GatesFailedError stops a publication on a red gate. It wraps SHIP-E0902.
type GatesFailedError struct {
	Results []GateResult
	// Moment says what the red gate stopped: a push, a commit or nothing.
	Moment GateMoment
}

func (e *GatesFailedError) Error() string { return e.Unwrap().Error() }

// Unwrap exposes the fault naming the gate that failed.
func (e *GatesFailedError) Unwrap() error {
	last := e.Results[len(e.Results)-1]
	// The keys are written out: docs/reference/errors.md is generated from them.
	if last.Status == GateUnavailable {
		tool := strings.Fields(last.Command)[0]
		switch e.Moment {
		case GatesOnDemand:
			return fault.New(fault.GateToolMissing, "gates.tool_missing_check", "gate", last.Name, "tool", tool)
		case GatesBeforeCommit:
			return fault.New(fault.GateToolMissing, "gates.tool_missing_commit", "gate", last.Name, "tool", tool)
		}
		return fault.New(fault.GateToolMissing, "gates.tool_missing", "gate", last.Name, "tool", tool)
	}
	switch e.Moment {
	case GatesOnDemand:
		return fault.New(fault.GateFailed, "gates.failed_check", "gate", last.Name, "command", last.Command)
	case GatesBeforeCommit:
		return fault.New(fault.GateFailed, "gates.failed_commit", "gate", last.Name, "command", last.Command)
	}
	return fault.New(fault.GateFailed, "gates.failed", "gate", last.Name, "command", last.Command)
}

// logGate records a gate: its command (a project script such as
// "pnpm run lint", never its output), status and duration.
func logGate(ctx context.Context, ws Workspace, g GateResult, err error) {
	level := slog.LevelInfo
	attrs := []slog.Attr{
		slog.String(attrStep, "gate/"+g.Name), slog.Int64(attrDuration, g.Duration),
		slog.String("gate", g.Name), slog.String("cmd", g.Command), slog.String("status", string(g.Status)),
	}
	if err != nil {
		level = slog.LevelWarn
		attrs = append(attrs, slog.String("error", err.Error()))
	} else if g.Status != GatePassed {
		level = slog.LevelWarn
	}
	ws.logger().LogAttrs(ctx, level, "gate.done", attrs...)
}

// GatesIn returns the gate results an error carries, if it is a red gate.
func GatesIn(err error) ([]GateResult, bool) {
	var failed *GatesFailedError
	if errors.As(err, &failed) {
		return failed.Results, true
	}
	return nil, false
}

// RunGates runs the project's checks, cheapest first, and stops at the first
// red one. base is the ref the work is compared to ("origin/main"), used to
// run only the affected tests; "" runs them all.
func RunGates(ctx context.Context, ws Workspace, base string, observer GateObserver) ([]GateResult, error) {
	return runGatesAt(ctx, ws, base, observer, GatesAtPush)
}

// runGatesAt is RunGates at a given moment (see GateMoment).
func runGatesAt(ctx context.Context, ws Workspace, base string, observer GateObserver, moment GateMoment) ([]GateResult, error) {
	detection, err := gates.Detect(gateProject(ctx, ws, base, moment != GatesAtPush))
	if err != nil {
		return nil, err
	}

	if ws.IsDryRun() {
		// Listed, not run: gates may take minutes and write build outputs.
		for _, gate := range detection.Gates {
			ws.planStep("app.dryrun.gate", "gate", gate.Name, "command", strings.Join(gate.Command, " "))
		}
		return nil, nil
	}
	var results []GateResult
	for _, name := range detection.Missing {
		results = append(results, GateResult{Name: name, Status: GateSkipped})
	}
	for _, gate := range detection.Gates {
		command := strings.Join(gate.Command, " ")
		if observer != nil {
			observer.GateStarted(gate.Name, command)
		}
		gateCtx, cancel := context.WithTimeout(ctx, gateTimeout)
		run, err := ws.Runner.Run(gateCtx, gate.Command)
		cancel()
		result := GateResult{Name: gate.Name, Command: command, Duration: run.Duration.Milliseconds(), Status: GatePassed}
		switch {
		case errors.Is(err, ports.ErrCommandNotFound):
			result.Status = GateUnavailable
		case err != nil:
			logGate(ctx, ws, result, err)
			return results, err
		case run.ExitCode != 0:
			result.Status, result.Output = GateFailed, string(run.Output)
		case gate.FailOnOutput && strings.TrimSpace(string(run.Output)) != "":
			// gofmt -l lists the files it would change and exits 0.
			result.Status, result.Output = GateFailed, string(run.Output)
		}
		logGate(ctx, ws, result, err)
		results = append(results, result)
		if observer != nil {
			observer.GateFinished(result)
		}
		if result.Status != GatePassed {
			return results, &GatesFailedError{Results: results, Moment: moment}
		}
	}
	return results, nil
}

// gateProject reads what the gates are detected from: root files, marker
// files, the root entries (for .NET solutions) and the changed files.
// workingTree adds the uncommitted changes, untracked files included but not
// ignored ones: what the next commit takes.
func gateProject(ctx context.Context, ws Workspace, base string, workingTree bool) gates.Project {
	project := gates.Project{Base: base, Present: map[string]bool{}, Files: map[string][]byte{}, Windows: ws.OperatingSystem == "windows"}
	exists := func(name string) bool {
		// A limit of 0 reads nothing: "too large" proves the file exists.
		_, err := ws.Git.ReadWorkingFile(ctx, name, 0)
		return err == nil || errors.Is(err, ports.ErrFileTooLarge)
	}
	if content, err := ws.Git.ReadWorkingFile(ctx, "package.json", packageJSONLimit); err == nil {
		project.PackageJSON = content
	}
	for _, name := range gates.RootFiles {
		if content, err := ws.Git.ReadWorkingFile(ctx, name, packageJSONLimit); err == nil {
			project.Files[name] = content
		}
	}
	for _, name := range append(gates.Lockfiles(), gates.MarkerFiles...) {
		if exists(name) {
			project.Present[name] = true
		}
	}
	// Solutions and projects have free names: list the root of HEAD.
	if entries, err := ws.Git.FilesAt(ctx, "HEAD", ""); err == nil {
		for _, name := range entries {
			project.Present[name] = true
		}
	}
	seen := map[string]bool{}
	changed := func(path string) {
		if !seen[path] && exists(path) { // a deleted file has nothing to format
			seen[path] = true
			project.Changed = append(project.Changed, path)
		}
	}
	if stats, err := ws.Git.DiffStatSince(ctx, base); err == nil {
		for _, s := range stats {
			changed(s.Path)
		}
	}
	if workingTree {
		// git status lists untracked files one by one, without ignored ones.
		if status, err := ws.Git.Status(ctx); err == nil {
			for _, f := range status.Files {
				if f.State != ports.Ignored {
					changed(f.Path)
				}
			}
		}
	}
	return project
}

// GatesCheck is what ship check gates reports.
type GatesCheck struct {
	Gates []GateResult `json:"gates"`
	// Rehearsal: on a dry run, Planned lists the gates instead of running them.
	Rehearsal
}

// CheckGates runs the project's gates now, on the working tree as it is,
// without committing or pushing anything: what publication would run. A red
// gate is a GatesFailedError, as for a publication.
func CheckGates(ctx context.Context, ws Workspace, observer GateObserver) (result GatesCheck, err error) {
	mark := ws.planMark()
	defer func() {
		if ws.IsDryRun() {
			result.Rehearsal = ws.rehearsed(mark)
		}
	}()
	if _, err := ws.Git.TopLevel(ctx); err != nil {
		return result, err
	}
	if ws.Runner == nil {
		return result, nil
	}
	result.Gates, err = runGatesAt(ctx, ws, gatesBase(ctx, ws, defaultBranchOf(ctx, ws)), observer, GatesOnDemand)
	return result, err
}
