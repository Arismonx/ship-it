// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"log/slog"

	"github.com/GregoireF/ship-it/internal/core/config"
	"github.com/GregoireF/ship-it/internal/core/convention"
	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/ports"
)

// trunkBranches never receive work directly: ship-it moves the work to a
// dedicated branch first (trunk-based model, decision NB3).
var trunkBranches = map[string]bool{"main": true, "master": true, "develop": true, "trunk": true}

// createsBranch reports whether a delivery from this branch moves the work to
// a dedicated branch: from a trunk branch or a detached HEAD, except for the
// very first commit of a repository, which lands on the trunk (there is no
// base yet to branch from).
func createsBranch(b ports.BranchState, defaultBranch string) bool {
	return b.Commit != "" && (b.Detached || isTrunk(b.Name, defaultBranch))
}

// isTrunk reports whether a branch never receives work directly: a usual
// trunk name, or the remote's default branch whatever its name. Every use
// case asks this one question, so that ship, publish, status, tidy and fix
// agree on a repository whose default branch is "production".
func isTrunk(name, defaultBranch string) bool {
	return trunkBranches[name] || (defaultBranch != "" && name == defaultBranch)
}

// defaultBranchOf is the remote's default branch as git knows it locally
// (origin/HEAD), "" when unknown: a trunk even when named "production".
func defaultBranchOf(ctx context.Context, ws Workspace) string {
	name, _ := ws.Git.RemoteDefaultBranch(ctx, gitRemote(ctx, ws))
	return name
}

// DeliveryRequest is an accepted plan, possibly edited by the human.
type DeliveryRequest struct {
	Title  string
	Branch string
	// Only delivers part of the work (see PlanRequest.Only); the rest stays
	// uncommitted, untouched.
	Only []string
	// AIAssistant names who wrote Title and Branch when it was not the
	// deterministic planner (ADR-0004: the pull request mentions it).
	AIAssistant string
	// Why is the motivation, in the developer's words: the commit body, then
	// the pull request summary. Tested says how they checked it, recorded as
	// a "Tested:" trailer. Both are optional; nothing is written when empty.
	Why, Tested string
	// Reads are the plan's reads (PlanView.Reads) when the delivery follows
	// the plan at once (--yes, an agent): status, diff, default branch and
	// history are not read again. Leave it empty after a human review: the
	// files may have changed meanwhile, and the delivery must read again
	// what it scans and commits.
	Reads PlanReads
	// SkipGates and GateObserver are those of the publication, for the gates
	// that ship.gates.beforeCommit runs before the commit.
	SkipGates    bool
	GateObserver GateObserver
}

// DeliveryResult reports what changed in the repository.
type DeliveryResult struct {
	Operation     string         `json:"operation,omitempty"`
	Checkpoint    CheckpointView `json:"checkpoint"`
	Branch        string         `json:"branch"`
	CreatedBranch bool           `json:"created_branch"`
	Commit        string         `json:"commit"`
	Title         string         `json:"title"`
	// Gates ran before the commit (ship.gates.beforeCommit).
	Gates []GateResult `json:"gates,omitempty"`
	// GatesDeferred: the gates are on and did not run before the commit;
	// they run at publication (ship check gates runs them now).
	GatesDeferred bool `json:"gates_deferred,omitempty"`
	// Rehearsal: on a dry run, Commit is empty and Planned lists the steps.
	Rehearsal
}

// DeliverLocally records the accepted plan: checkpoint, dedicated branch when
// the work sits on a trunk branch, then one commit with every change.
// Publishing (push, pull request) is a separate step.
//
// Whatever it changed, even when interrupted halfway, is written to the
// operation log so that `ship undo` can revert it.
func DeliverLocally(ctx context.Context, ws Workspace, req DeliveryRequest) (result DeliveryResult, err error) {
	// Deferred first so that it runs last, once the operation is logged.
	done := ws.begin(ctx, "deliver")
	mark := ws.planMark()
	defer func() {
		if ws.IsDryRun() {
			result.Commit, result.Rehearsal = "", ws.rehearsed(mark)
		}
		done(err, append(operation(result.Operation), slog.Bool("new_branch", result.CreatedBranch))...)
	}()
	// One `git status` serves the doctor and the delivery.
	reads, readErr := req.Reads.r, error(nil)
	if reads == nil {
		reads, readErr = readStatus(ctx, ws.Git)
	}
	var known *ports.WorkingTreeStatus
	if readErr == nil {
		known = &reads.status
	}
	if report := doctorWith(ctx, ws.Git, ws.OperatingSystem, gitRemote(ctx, ws), known); report.Blocking() {
		return DeliveryResult{}, fault.New(fault.DoctorBlocking, "deliver.doctor_blocking")
	}
	// The style rules wait for the project's own (commitlint), and the types
	// for the repository's table (ship.commits.types), below.
	if faults := withoutStyleFaults(CheckTitle(req.Title).Faults); len(faults) > 0 && faults[0].Code != fault.UnknownCommitType {
		return DeliveryResult{}, faults[0]
	}
	if readErr != nil {
		return DeliveryResult{}, readErr
	}
	status := reads.status
	if status.Clean() {
		return DeliveryResult{}, fault.New(fault.NothingToShip, "plan.nothing_to_ship")
	}
	// Enforced here too, not only in the plan: every surface (CLI, MCP,
	// daemon) goes through this gate.
	diffs, err := reads.selectedWork(ctx, ws.Git, req.Only)
	if err != nil && (len(req.Only) > 0 || !errors.Is(err, &fault.Fault{Code: fault.NothingToShip})) {
		return DeliveryResult{}, err
	}
	found, err := scanSecrets(ctx, ws, diffs)
	if err != nil {
		return DeliveryResult{}, err
	}
	if len(found) > 0 {
		return DeliveryResult{}, &SecretsFoundError{Findings: found}
	}
	// The message follows the repository's gitmoji habit (ship.commits.gitmoji).
	eff, err := LoadSettingsFor(ctx, ws, diffPaths(diffs))
	if err != nil {
		return DeliveryResult{}, err
	}
	keys := eff.Settings.Ship.Issues.TrackerKeys
	ticket := ""
	trunk := reads.defaultBranch(ctx, ws)
	// On the top layer of a stack, the work becomes a new layer above it.
	stackParent := ""
	if !createsBranch(status.Branch, trunk) && onStackTop(ctx, ws, status.Branch.Name) {
		stackParent = status.Branch.Name
	}
	if !createsBranch(status.Branch, trunk) {
		ticket = convention.FindTicket(status.Branch.Name, keys)
	}
	placement := placementWith(ctx, ws, eff, reads)
	rules := eff.Settings.Conventions()
	check := checkTitleAs(rules, req.Title, placement, ticket)
	// Rule zero: the project's commitlint, when installed, decides the style
	// and validates the very message, right before anything changes.
	lint := projectCommitlint(ctx, ws)
	if lint != nil {
		check.Faults = withoutFaults(check.Faults, givingWayTo(lint, eff))
		rules.BodyWidth = bodyWidthWith(lint, rules)
	}
	if !check.Valid() {
		return DeliveryResult{}, check.Faults[0]
	}
	if lint != nil {
		header, err := rules.ParseHeader(check.Canonical)
		if err != nil {
			return DeliveryResult{}, asFault(err)
		}
		full := func(title string) string { return rules.CommitMessage(title, req.Why, req.Tested) }
		if check.Canonical, _, err = followCommitlint(ctx, ws, lint, rules, header, placement, full); err != nil {
			return DeliveryResult{}, err
		}
	}

	// Sign-off and signature (ship.commits.signoff, sign): a signature
	// without a key stops here, before anything changes.
	commitOpts, err := commitOptions(ctx, ws, eff)
	if err != nil {
		return DeliveryResult{}, err
	}

	// ship.gates.beforeCommit: a red gate refuses the commit itself, before
	// anything changes.
	gateResults, err := gatesBeforeCommit(ctx, ws, eff, trunk, req)
	if err != nil {
		return DeliveryResult{Gates: gateResults}, err
	}

	checkpoint, err := ws.Checkpoints.Create(ctx, "deliver")
	if err != nil {
		return DeliveryResult{}, err
	}
	result = DeliveryResult{Checkpoint: viewOf(checkpoint), Branch: status.Branch.Name, Title: check.Canonical}
	result.Gates = gateResults
	result.GatesDeferred = eff.Settings.Ship.Gates.Enabled && ws.Runner != nil && len(gateResults) == 0
	before := repositoryViewOf(status.Branch)
	defer func() {
		if !result.CreatedBranch && result.Commit == "" {
			return // nothing to undo
		}
		after := oplog.RepositoryView{Branch: result.Branch, Head: before.Head}
		if result.Commit != "" {
			after.Head = result.Commit
		}
		op, logErr := ws.Operations.Append(ctx, oplog.Operation{
			Kind: oplog.Deliver, Checkpoint: checkpoint.ID, Before: before, After: after,
			CreatedBranch: result.CreatedBranch, Commit: result.Commit, Title: result.Title, Only: req.Only,
			AIAssistant: req.AIAssistant, StackParent: stackParent,
		})
		result.Operation = op.ID
		err = errors.Join(err, logErr)
	}()

	if createsBranch(status.Branch, trunk) || stackParent != "" {
		if err := rules.ValidateBranch(req.Branch, keys); err != nil {
			return result, err
		}
		branch := convention.UniqueBranchName(req.Branch, func(name string) bool {
			exists, err := ws.Git.LocalBranchExists(ctx, name)
			return err == nil && exists
		})
		if err := ws.Git.SwitchToNewBranch(ctx, branch); err != nil {
			return result, err
		}
		result.Branch, result.CreatedBranch = branch, true
	}

	var commit string
	if len(req.Only) > 0 {
		commit, err = ws.Git.CommitPaths(ctx, rules.CommitMessage(check.Canonical, req.Why, req.Tested), req.Only, commitOpts)
	} else {
		if err := ws.Git.StageAll(ctx); err != nil {
			return result, err
		}
		commit, err = ws.Git.Commit(ctx, rules.CommitMessage(check.Canonical, req.Why, req.Tested), commitOpts)
	}
	if err != nil {
		return result, err
	}
	result.Commit = commit
	return result, nil
}

func repositoryViewOf(b ports.BranchState) oplog.RepositoryView {
	return oplog.RepositoryView{Branch: b.Name, Head: b.Commit, Detached: b.Detached}
}

// gatesBeforeCommit runs the gates when ship.gates.beforeCommit asks for
// it (and the gates are on), with the same --skip-gates rule as a push.
func gatesBeforeCommit(ctx context.Context, ws Workspace, eff config.Effective, trunk string, req DeliveryRequest) ([]GateResult, error) {
	settings := eff.Settings.Ship.Gates
	if !settings.BeforeCommit || !settings.Enabled || ws.Runner == nil {
		return nil, nil
	}
	if req.SkipGates {
		if !settings.AllowSkip {
			return nil, fault.New(fault.GateSkipRefused, "publish.skip_refused", "source", eff.Source("ship.gates.allowSkip"))
		}
		return nil, nil
	}
	return runGatesAt(ctx, ws, gatesBase(ctx, ws, trunk), req.GateObserver, GatesBeforeCommit)
}

// gatesBase is the ref the gates compare the work to: the remote's default
// branch, or none (every test runs).
func gatesBase(ctx context.Context, ws Workspace, trunk string) string {
	if trunk == "" {
		return ""
	}
	return gitRemote(ctx, ws) + "/" + trunk
}
