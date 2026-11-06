// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/core/stack"
	"github.com/GregoireF/ship-it/internal/ports"
)

// stackUndoComment explains, on each pull request of an undone stack, why it
// was closed.
const stackUndoComment = "Closed with `ship undo`: the author took this stack back. The commits are still on their machine."

// checkStackUndo asks for consent to undo a published stack and reads each
// pull request as it is now: a merged layer is kept (its pull request and
// branch), a closed one needs no closing. When every layer is merged there
// is nothing left to undo.
func checkStackUndo(ctx context.Context, ws Workspace, target oplog.Operation, steps []oplog.Step, req UndoRequest) ([]oplog.Step, error) {
	layers := target.Stack.Layers
	if !req.AllowRemote {
		params := []string{"count", strconv.Itoa(len(layers)), "branch", layers[0].Branch, "top", layers[len(layers)-1].Branch}
		if !ws.IsDryRun() {
			return nil, fault.New(fault.UndoNeedsConsent, "undo.needs_consent_stack", params...)
		}
		ws.planStep("app.dryrun.needs_consent_stack", params...)
	}
	if ws.Forge == nil {
		return nil, fault.New(fault.NoGitHubRemote, "publish.no_remote", "remote", gitRemote(ctx, ws))
	}
	repo, err := remoteRepository(ctx, ws)
	if err != nil {
		return nil, err
	}
	keep := map[string]bool{} // merged: nothing to undo for the layer
	closed := map[int]bool{}  // closed already
	for _, l := range layers {
		pr, found, err := ws.Forge.LatestPullRequest(ctx, repo, l.Branch)
		if err != nil {
			return nil, forgeFault(err, repo)
		}
		if !found || pr.Number != l.PullRequest {
			continue
		}
		switch {
		case pr.Merged:
			keep[l.Branch] = true
		case pr.State == "closed":
			closed[pr.Number] = true
		}
	}
	if len(keep) == len(layers) {
		return nil, fault.New(fault.UndoOfMergedWork, "undo.merged", "url", layers[0].URL)
	}
	// A sync since then pushed rebased layers: the remote branches hold the
	// commits it recorded, ship-it's own (an undone sync put them back).
	pushed := pushedSince(ctx, ws, target.ID)
	for i, s := range steps {
		if commit, ok := pushed[s.Branch]; ok && s.Kind == oplog.DeleteRemoteBranch {
			steps[i].Expected = commit
		}
	}
	return slices.DeleteFunc(steps, func(s oplog.Step) bool {
		switch s.Kind {
		case oplog.ClosePullRequest:
			return keep[s.Branch] || closed[s.Number]
		case oplog.DeleteRemoteBranch:
			return keep[s.Branch]
		}
		return false
	}), nil
}

// checkStackSyncUndo asks for consent before undoing a sync changes GitHub:
// the remote branches it pushed or deleted, the pull requests it retargeted.
func checkStackSyncUndo(ws Workspace, steps []oplog.Step, req UndoRequest) ([]oplog.Step, error) {
	if req.AllowRemote {
		return steps, nil
	}
	var branches []string
	pulls := 0
	for _, s := range steps {
		switch s.Kind {
		case oplog.RestoreRemoteBranch:
			branches = append(branches, s.Branch)
		case oplog.RetargetPullRequest, oplog.RestoreChainNote:
			pulls++
		}
	}
	params := []string{"branches", strings.Join(branches, ", "), "count", strconv.Itoa(len(branches)), "pull_requests", strconv.Itoa(pulls)}
	if !ws.IsDryRun() {
		return nil, fault.New(fault.UndoNeedsConsent, "undo.needs_consent_stack_sync", params...)
	}
	ws.planStep("app.dryrun.needs_consent_stack_sync", params...)
	return steps, nil
}

// isStackSyncStep reports whether step puts GitHub back after a sync: done
// where nobody changed it since, a warning otherwise.
func isStackSyncStep(step oplog.Step) bool {
	switch step.Kind {
	case oplog.RestoreRemoteBranch, oplog.RetargetPullRequest, oplog.RestoreChainNote:
		return true
	}
	return false
}

// undoStackSyncStep applies one of them, compare-and-swap: a remote branch
// someone pushed to, a pull request someone retargeted or whose chain line
// someone edited is kept as they left it (StackChangedSince).
func undoStackSyncStep(ctx context.Context, ws Workspace, step oplog.Step) error {
	if step.Kind == oplog.RestoreRemoteBranch {
		pushed := ws.begin(ctx, "undo/push_back")
		err := ws.Git.PushCommit(ctx, gitRemote(ctx, ws), step.Branch, step.Commit, step.Expected)
		pushed(err, slog.Bool("lease", true))
		if err != nil {
			return fault.New(fault.StackChangedSince, "undo.stack_remote_kept", "branch", step.Branch, "detail", lastLine(err.Error()))
		}
		return nil
	}
	if ws.Stacks == nil {
		return fault.New(fault.NoGitHubRemote, "publish.no_remote", "remote", gitRemote(ctx, ws))
	}
	repo, err := remoteRepository(ctx, ws)
	if err != nil {
		return err
	}
	pr, err := ws.Stacks.PullRequest(ctx, repo, step.Number)
	if err != nil {
		return forgeFault(err, repo)
	}
	number := strconv.Itoa(step.Number)
	if step.Kind == oplog.RetargetPullRequest {
		if pr.State != "open" || pr.BaseBranch != step.Expected {
			return fault.New(fault.StackChangedSince, "undo.stack_base_kept", "number", number, "base", pr.BaseBranch)
		}
		return forgeFault(ws.Stacks.SetPullRequestBase(ctx, repo, step.Number, step.Target), repo)
	}
	body, current := stack.WithChainNote(pr.Body, step.Value)
	if current != step.Expected {
		return fault.New(fault.StackChangedSince, "undo.stack_note_kept", "number", number)
	}
	return forgeFault(ws.Stacks.SetPullRequestBody(ctx, repo, step.Number, body), repo)
}

// pushedSince maps each layer a stack sync rebased and pushed after the
// operation id to the newest commit it pushed.
func pushedSince(ctx context.Context, ws Workspace, id string) map[string]string {
	pushed := map[string]string{}
	ops, err := ws.Operations.Recent(ctx, operationSearchDepth)
	if err != nil {
		return pushed
	}
	undone := map[string]bool{}
	for _, op := range ops {
		if op.Kind == oplog.Undo {
			undone[op.Undoes] = true
		}
	}
	for _, op := range ops { // newest first
		if op.ID == id {
			break
		}
		// An undone sync put the remote branches back where it found them.
		if op.Kind != oplog.StackSync || op.Stack == nil || undone[op.ID] {
			continue
		}
		for _, l := range op.Stack.Layers {
			if _, newer := pushed[l.Branch]; !newer && !l.Deleted && l.Commit != "" && l.Commit != l.Previous {
				pushed[l.Branch] = l.Commit
			}
		}
	}
	return pushed
}

// unstackStep dissolves the stack; one GitHub removed already is fine.
func unstackStep(ctx context.Context, ws Workspace, step oplog.Step) error {
	if ws.Stacks == nil {
		return fault.New(fault.NoGitHubRemote, "publish.no_remote", "remote", gitRemote(ctx, ws))
	}
	repo, err := remoteRepository(ctx, ws)
	if err != nil {
		return err
	}
	if _, err := ws.Stacks.Unstack(ctx, repo, step.Number); err != nil && !errors.Is(err, ports.ErrNotFound) {
		return forgeFault(err, repo)
	}
	return nil
}
