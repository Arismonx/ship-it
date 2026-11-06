// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"slices"

	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/ports"
)

// mergedPullRequestScan bounds how many merged pull requests the cleanup reads.
const mergedPullRequestScan = 50

// TidyResult reports what the tidy did; an empty result means nothing to do.
type TidyResult struct {
	Operation string `json:"operation,omitempty"`
	Base      string `json:"base,omitempty"`
	// LeftBranch is the branch whose pull request was merged; the user is now
	// on Base, with the work in progress carried over.
	LeftBranch  string `json:"left_branch,omitempty"`
	PullRequest int    `json:"pull_request,omitempty"`
	// StayedBecauseOfConflicts: the work would conflict with the new base, so
	// nothing moved (all or nothing).
	StayedBecauseOfConflicts bool `json:"stayed_because_of_conflicts,omitempty"`
	// BaseDiverged: the local base has commits origin lacks; not updated.
	BaseDiverged bool `json:"base_diverged,omitempty"`
	// Deleted branches had nothing more than their merged pull request.
	Deleted []string `json:"deleted,omitempty"`
	// Kept branches have commits added after the merge.
	Kept []string `json:"kept,omitempty"`
	Rehearsal
}

// Tidy brings the repository back in line after pull requests were merged
// (docs/reference/error-playbook.md, "The pull request of the current branch is already merged", heritage of the draft):
//
//  1. on a branch whose pull request is merged, it goes back to the
//     up-to-date base and carries the work in progress over, but only when
//     that work applies without conflict (checked before anything moves);
//  2. it deletes local branches that hold nothing more than their merged
//     pull request (squash merges defeat `git branch -d`).
//
// Every deletion is a compare-and-swap and is logged: `ship undo` recreates
// the branches. Without a GitHub remote or a token, it does nothing.
//
// ship.tidy.deleteMerged false keeps every branch.
func Tidy(ctx context.Context, ws Workspace) (TidyResult, error) {
	done := ws.begin(ctx, "tidy")
	mark := ws.planMark()
	result, err := tidy(ctx, ws)
	result.Rehearsal = ws.rehearsed(mark)
	done(err, operation(result.Operation)...)
	return result, err
}

// TidyBeforeShip is the tidy that starts `ship`, unless ship.tidy.auto is
// false; ran reports whether it ran.
func TidyBeforeShip(ctx context.Context, ws Workspace) (result TidyResult, ran bool, err error) {
	if eff, err := LoadSettings(ctx, ws); err == nil && !eff.Settings.Ship.Tidy.Auto {
		return TidyResult{}, false, nil
	}
	result, err = Tidy(ctx, ws)
	return result, true, err
}

func tidy(ctx context.Context, ws Workspace) (TidyResult, error) {
	var result TidyResult
	if ws.Forge == nil {
		return result, nil
	}
	eff, err := LoadSettings(ctx, ws)
	if err != nil {
		return result, err
	}
	publishRemote := remoteOf(eff)
	deleteMerged := eff.Settings.Ship.Tidy.DeleteMerged
	repo, err := repositoryOf(ctx, ws, publishRemote)
	if err != nil {
		return result, nil // a local repository has nothing to tidy
	}
	if inProgress, _ := ws.Git.OperationInProgress(ctx); inProgress != "" {
		return result, nil // ship doctor reports it
	}
	info, err := ws.Forge.Repository(ctx, repo)
	if err != nil {
		return result, forgeFault(err, repo)
	}
	// A fork's pull requests are on its parent, and so is its base.
	target, err := resolvePublication(ctx, ws, eff, repo, info)
	if err != nil {
		return result, err
	}
	baseRef, err := fetchBase(ctx, ws, target)
	if err != nil {
		return result, err
	}
	base := target.base
	if baseRef == "" {
		baseRef = publishRemote + "/" + base
	}
	repo = target.prRepo
	result.Base = base
	status, err := ws.Git.Status(ctx)
	if err != nil {
		return result, err
	}
	before := repositoryViewOf(status.Branch)
	var deleted []oplog.BranchRef
	moved := false

	current := status.Branch.Name
	if current != "" && !isTrunk(current, base) {
		pr, found, err := ws.Forge.LatestPullRequest(ctx, repo, target.head(current))
		if err != nil {
			return result, forgeFault(err, repo)
		}
		if found && pr.Merged {
			result.LeftBranch, result.PullRequest = current, pr.Number
			moved, err = leaveMergedBranch(ctx, ws, baseRef, base, !status.Clean(), &result)
			if err != nil {
				return result, err
			}
			if moved && !deleteMerged {
				result.Kept = append(result.Kept, current)
			} else if moved {
				if ref, ok := deleteIfMerged(ctx, ws, current, pr); ok {
					deleted = append(deleted, ref)
				} else {
					result.Kept = append(result.Kept, current)
				}
			}
		}
	}

	now, _ := ws.Git.Status(ctx)
	var merged []ports.PullRequest
	var locals map[string]string
	if deleteMerged {
		if merged, err = ws.Forge.MergedPullRequests(ctx, repo, mergedPullRequestScan); err != nil {
			return result, forgeFault(err, repo)
		}
		if locals, err = ws.Git.LocalBranches(ctx); err != nil {
			return result, err
		}
	}
	for _, pr := range merged {
		name := pr.HeadBranch
		if _, local := locals[name]; !local || name == base || name == now.Branch.Name || isTrunk(name, base) ||
			slices.ContainsFunc(deleted, func(b oplog.BranchRef) bool { return b.Name == name }) {
			continue
		}
		if ref, ok := deleteIfMerged(ctx, ws, name, pr); ok {
			deleted = append(deleted, ref)
		}
	}

	for _, d := range deleted {
		result.Deleted = append(result.Deleted, d.Name)
	}
	if moved || len(deleted) > 0 {
		op, err := ws.Operations.Append(ctx, oplog.Operation{
			Kind: oplog.Tidy, Before: before, After: repositoryViewOf(now.Branch), DeletedBranches: deleted,
			PullRequest: result.PullRequest, Remote: publishRemote,
		})
		result.Operation = op.ID
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

// leaveMergedBranch moves to the up-to-date base with the work in progress,
// all or nothing. It reports whether HEAD moved.
// target is the remote-tracking base, such as origin/main.
func leaveMergedBranch(ctx context.Context, ws Workspace, target, base string, dirty bool, result *TidyResult) (bool, error) {
	if _, err := ws.Checkpoints.Create(ctx, "before-tidy"); err != nil {
		return false, err
	}
	if dirty {
		if err := ws.Git.Stash(ctx, "ship-it: leaving "+result.LeftBranch); err != nil {
			return false, err
		}
		clean, err := ws.Git.StashAppliesCleanlyOn(ctx, target)
		if err != nil || !clean {
			// Put everything back exactly where it was: the stash comes from
			// this very commit, so it applies without conflict.
			if popErr := ws.Git.StashPop(ctx); popErr != nil {
				return false, popErr
			}
			result.StayedBecauseOfConflicts = err == nil
			return false, err
		}
	}
	if err := ws.Git.SwitchBranch(ctx, base); err != nil {
		if dirty {
			_ = ws.Git.StashPop(ctx)
		}
		return false, err
	}
	if err := ws.Git.FastForward(ctx, target); err != nil {
		result.BaseDiverged = true
	}
	if dirty {
		if err := ws.Git.StashPop(ctx); err != nil {
			// Checked above, so unexpected; the stash and the checkpoint
			// still hold the work.
			return true, err
		}
	}
	return true, nil
}

// deleteIfMerged deletes a local branch whose tip is the merged head of its
// pull request, or an ancestor of it: nothing of the user would be lost.
func deleteIfMerged(ctx context.Context, ws Workspace, branch string, pr ports.PullRequest) (oplog.BranchRef, bool) {
	tip, err := ws.Git.CommitOf(ctx, "refs/heads/"+branch)
	if err != nil || tip == "" || pr.HeadSHA == "" {
		return oplog.BranchRef{}, false
	}
	if tip != pr.HeadSHA {
		if ancestor, err := ws.Git.IsAncestor(ctx, tip, pr.HeadSHA); err != nil || !ancestor {
			return oplog.BranchRef{}, false
		}
	}
	if err := ws.Git.DeleteBranch(ctx, branch, tip); err != nil {
		return oplog.BranchRef{}, false
	}
	return oplog.BranchRef{Name: branch, Commit: tip}, true
}
