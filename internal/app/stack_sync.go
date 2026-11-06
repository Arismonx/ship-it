// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/GregoireF/ship-it/internal/core/config"
	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/core/review"
	"github.com/GregoireF/ship-it/internal/core/stack"
	"github.com/GregoireF/ship-it/internal/ports"
)

// StackLayerView is one layer as ship stack status shows it.
type StackLayerView struct {
	Branch  string `json:"branch"`
	Base    string `json:"base"`
	Current bool   `json:"current,omitempty"`
	// PullRequest is its latest pull request, nil when none or offline.
	PullRequest *ports.PullRequest `json:"pull_request,omitempty"`
	CI          *review.CI         `json:"ci,omitempty"`
}

// StackStatusView is a stack, bottom first.
type StackStatusView struct {
	Base   string           `json:"base"`
	Native bool             `json:"native"`
	Number int              `json:"number,omitempty"`
	Layers []StackLayerView `json:"layers"`
	// GitHubProblem says why GitHub could not be asked; the layers still show.
	GitHubProblem *ProblemView `json:"github_problem,omitempty"`
}

// StackStatus shows the stack of the current branch: its layers, their pull
// requests and checks. Offline reads the operation log and git only.
func StackStatus(ctx context.Context, ws Workspace, offline bool) (StackStatusView, error) {
	status, err := ws.Git.Status(ctx)
	if err != nil {
		return StackStatusView{}, err
	}
	c, err := currentChain(ctx, ws, status.Branch.Name)
	if err != nil {
		return StackStatusView{}, err
	}
	view := StackStatusView{Base: c.Base}
	for i, b := range c.Branches {
		view.Layers = append(view.Layers, StackLayerView{Branch: b, Base: c.Parent(i), Current: b == status.Branch.Name})
	}
	if offline || ws.Forge == nil {
		return view, nil
	}
	repo, err := remoteRepository(ctx, ws)
	if err != nil {
		view.GitHubProblem = problemOf(err)
		return view, nil
	}
	for i := range view.Layers {
		l := &view.Layers[i]
		pr, found, err := ws.Forge.LatestPullRequest(ctx, repo, l.Branch)
		if err != nil {
			view.GitHubProblem = problemOf(forgeFault(err, repo))
			return view, nil
		}
		if !found {
			continue
		}
		l.PullRequest = &pr
		if pr.HeadSHA != "" && !pr.Merged {
			runs, runsErr := ws.Forge.CheckRuns(ctx, repo, pr.HeadSHA)
			statuses, statusErr := ws.Forge.CommitStatuses(ctx, repo, pr.HeadSHA)
			if runsErr == nil && statusErr == nil {
				ci := review.Summarize(runs, statuses)
				l.CI = &ci
			}
		}
	}
	view.Native, _ = stacksSupported(ctx, ws, repo)
	if view.Native {
		for _, l := range view.Layers {
			if l.PullRequest != nil && l.PullRequest.State == "open" {
				if stacks, err := ws.Stacks.StacksOf(ctx, repo, l.PullRequest.Number); err == nil && len(stacks) > 0 {
					view.Number = stacks[0].Number
				}
				break
			}
		}
	}
	return view, nil
}

// StackSyncResult reports a sync.
type StackSyncResult struct {
	Operation string `json:"operation,omitempty"`
	// Mode is ship.stack.rebase as it applied: "local", or "github" on a
	// native stack (a chain is always rebased locally).
	Mode     string           `json:"mode"`
	Deleted  []string         `json:"deleted,omitempty"`
	Rebased  []string         `json:"rebased,omitempty"`
	Followed []string         `json:"followed,omitempty"` // moved to GitHub's rebase
	Retarget []stack.SyncStep `json:"retargeted,omitempty"`
	Branch   string           `json:"branch"`
	Warnings []ProblemView    `json:"warnings,omitempty"`
	Rehearsal
}

// SyncStack follows a merge of the lower layers: it fetches, deletes the
// merged layers (local and remote branches), rebases the others on the
// layer below them that stays (ship.stack.rebase = local, or a classic
// chain) and pushes them with a lease, or moves them to GitHub's own rebase
// (github), then retargets the pull requests of a chain. A checkpoint comes
// first; ship undo puts the local branches back and, with consent, what the
// sync changed on GitHub.
func SyncStack(ctx context.Context, ws Workspace) (StackSyncResult, error) {
	done := ws.begin(ctx, "stack/sync")
	mark := ws.planMark()
	result, err := syncStack(ctx, ws)
	result.Rehearsal = ws.rehearsed(mark)
	done(err, operation(result.Operation)...)
	return result, err
}

func syncStack(ctx context.Context, ws Workspace) (result StackSyncResult, err error) {
	status, err := ws.Git.Status(ctx)
	if err != nil {
		return result, err
	}
	if !status.Clean() {
		return result, fault.New(fault.StackNeedsCleanTree, "stack.needs_clean_tree")
	}
	c, err := currentChain(ctx, ws, status.Branch.Name)
	if err != nil {
		return result, err
	}
	c = existing(ctx, ws, c)
	repo, err := remoteRepository(ctx, ws)
	if err != nil {
		return result, err
	}
	eff, err := LoadSettings(ctx, ws)
	if err != nil {
		return result, err
	}
	native, err := stacksSupported(ctx, ws, repo)
	if err != nil {
		return result, err
	}
	// The layers are replayed signed as ship.commits.sign says.
	commitOpts, err := commitOptions(ctx, ws, eff)
	if err != nil {
		return result, err
	}
	result.Mode = config.StackRebaseLocal
	if native && eff.Settings.Ship.Stack.Rebase == config.StackRebaseGitHub {
		result.Mode = config.StackRebaseGitHub
	}
	// The commits of each layer, here and on GitHub, before anything moves.
	oldTip, oldRemote := map[string]string{}, map[string]string{}
	for _, b := range c.Branches {
		if oldTip[b], err = ws.Git.CommitOf(ctx, "refs/heads/"+b); err != nil {
			return result, err
		}
		oldRemote[b], _ = ws.Git.CommitOf(ctx, "refs/remotes/"+gitRemote(ctx, ws)+"/"+b)
	}
	fetched := ws.begin(ctx, "stack/fetch")
	err = ws.Git.Fetch(ctx, gitRemote(ctx, ws))
	fetched(err)
	if err != nil {
		return result, fault.New(fault.FetchFailed, "tidy.fetch_failed", "remote", gitRemote(ctx, ws), "detail", lastLine(err.Error()))
	}
	states := make([]stack.LayerState, len(c.Branches))
	prs := map[string]ports.PullRequest{}
	for i, b := range c.Branches {
		states[i].Branch = b
		pr, found, err := ws.Forge.LatestPullRequest(ctx, repo, b)
		if err != nil {
			return result, forgeFault(err, repo)
		}
		if found {
			prs[b], states[i].Merged = pr, pr.Merged
		}
	}
	plan := stack.PlanSync(c, states)
	if len(plan.Merged) == 0 {
		return result, fault.New(fault.NothingToSync, "stack.nothing_to_sync")
	}

	checkpoint, err := ws.Checkpoints.Create(ctx, "before-stack-sync")
	if err != nil {
		return result, err
	}
	before := repositoryViewOf(status.Branch)
	record := &oplog.StackRecord{Base: c.Base, Native: native}
	moved := false
	defer func() {
		if !moved {
			return
		}
		now, statusErr := ws.Git.Status(ctx)
		after := oplog.RepositoryView{Branch: result.Branch}
		if statusErr == nil {
			after = repositoryViewOf(now.Branch)
		}
		op, logErr := ws.Operations.Append(ctx, oplog.Operation{
			Kind: oplog.StackSync, Checkpoint: checkpoint.ID, Before: before, After: after, Title: c.Base, Stack: record,
		})
		result.Operation = op.ID
		err = errors.Join(err, statusErr, logErr)
	}()
	// Off the layers first: they move, and the merged ones go.
	baseRef := gitRemote(ctx, ws) + "/" + c.Base
	if err := ws.Git.SwitchBranch(ctx, c.Base); err != nil {
		return result, err
	}
	moved = true
	result.Branch = c.Base

	newTip := map[string]string{}
	stopped := false
	for _, step := range plan.Remaining {
		b := step.Branch
		layer := oplog.StackLayer{Branch: b, Previous: oldTip[b], Commit: oldTip[b]}
		switch {
		case stopped:
		case result.Mode == config.StackRebaseGitHub:
			remote, _ := ws.Git.CommitOf(ctx, "refs/remotes/"+gitRemote(ctx, ws)+"/"+b)
			if oldTip[b] != oldRemote[b] {
				result.Warnings = append(result.Warnings, ProblemView{Code: string(fault.StackLayerUnpushed), Key: "stack.layer_unpushed",
					Params: map[string]string{"branch": b}})
				break
			}
			if remote != "" && remote != oldTip[b] {
				if err := ws.Git.MoveBranch(ctx, b, remote, oldTip[b]); err != nil {
					return result, err
				}
				layer.Commit = remote
				result.Followed = append(result.Followed, b)
			}
		default:
			onto, oldParent := baseRef, oldTip[step.OldParent]
			if step.Parent != c.Base {
				onto = newTip[step.Parent]
			}
			if step.OldParent == c.Base {
				oldParent = baseRef
			}
			upstream, err := ws.Git.MergeBase(ctx, oldTip[b], oldParent)
			if err != nil {
				return result, err
			}
			ontoCommit := onto
			if onto == baseRef {
				ontoCommit, _ = ws.Git.CommitOf(ctx, "refs/remotes/"+baseRef)
			}
			if upstream == ontoCommit && step.Parent == step.OldParent && !result.rebased(step.OldParent) {
				break // already on its parent
			}
			rebased := ws.begin(ctx, "stack/rebase")
			conflicted, err := ws.Git.RebaseOnto(ctx, b, onto, upstream, commitOpts)
			rebased(err, slog.Bool("conflict", conflicted))
			if err != nil {
				return result, err
			}
			if conflicted {
				result.Warnings = append(result.Warnings, ProblemView{Code: string(fault.StackRebaseConflict), Key: "stack.rebase_conflict",
					Params: map[string]string{"branch": b, "onto": step.Parent}})
				stopped = true
				break
			}
			tip, err := ws.Git.CommitOf(ctx, "refs/heads/"+b)
			if err != nil {
				return result, err
			}
			layer.Commit = tip
			result.Rebased = append(result.Rebased, b)
			if remote, _ := ws.Git.CommitOf(ctx, "refs/remotes/"+gitRemote(ctx, ws)+"/"+b); remote != "" && remote != tip {
				pushed := ws.begin(ctx, "stack/push")
				err := ws.Git.Push(ctx, gitRemote(ctx, ws), b, remote)
				pushed(err, slog.Bool("lease", true))
				if err != nil {
					return result, fault.New(fault.PushRejected, "publish.push_rejected", "branch", b, "detail", lastLine(err.Error()))
				}
				layer.RemotePrevious, layer.Pushed = remote, true
			}
		}
		newTip[b] = layer.Commit
		if newTip[b] == "" {
			newTip[b] = oldTip[b]
		}
		// A chain's pull request targets the merged branch: GitHub does not
		// retarget it; a native stack's is retargeted by GitHub.
		if pr, ok := prs[b]; ok && step.Retarget && !native && pr.State == "open" && ws.Stacks != nil {
			if err := ws.Stacks.SetPullRequestBase(ctx, repo, pr.Number, step.Parent); err != nil {
				result.Warnings = append(result.Warnings, *problemOf(forgeFault(err, repo)))
			} else {
				result.Retarget = append(result.Retarget, step)
				layer.PullRequest, layer.PreviousBase, layer.Base = pr.Number, step.OldParent, step.Parent
			}
		}
		record.Layers = append(record.Layers, layer)
	}
	for _, b := range plan.Merged {
		if err := ws.Git.DeleteBranch(ctx, b, oldTip[b]); err != nil {
			return result, err
		}
		layer := oplog.StackLayer{Branch: b, Previous: oldTip[b], Deleted: true}
		result.Deleted = append(result.Deleted, b)
		if remote, _ := ws.Git.CommitOf(ctx, "refs/remotes/"+gitRemote(ctx, ws)+"/"+b); remote != "" {
			// GitHub may have deleted it already; a moved one is kept.
			if ws.Git.DeleteRemoteBranch(ctx, gitRemote(ctx, ws), b, remote) == nil {
				layer.RemotePrevious, layer.RemoteDeleted = remote, true
			}
		}
		record.Layers = append(record.Layers, layer)
	}
	if !native {
		// The layers left say where they sit now: "Part 1 of 2."
		var left []string
		for _, s := range plan.Remaining {
			left = append(left, s.Branch)
		}
		record.Notes = renumberChain(ctx, ws, repo, left, &result.Warnings)
	}
	// Back where the work was, or the lowest layer left.
	target := status.Branch.Name
	if contains(plan.Merged, target) || target == "" {
		target = c.Base
		if len(plan.Remaining) > 0 {
			target = plan.Remaining[0].Branch
		}
	}
	if err := ws.Git.SwitchBranch(ctx, target); err != nil {
		return result, err
	}
	result.Branch = target
	return result, nil
}

func (r StackSyncResult) rebased(branch string) bool { return contains(r.Rebased, branch) }

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// StackMergeRequest carries the human's confirmation: app never asks.
type StackMergeRequest struct {
	// Confirmed: a human confirmed the plan in a terminal, or passed --yes.
	Confirmed bool
	// Agent: an AI agent asks. ship stack merge is never for agents.
	Agent bool
	// Poll and Timeout pace the wait for GitHub's answer; Sleep waits (tests
	// pass one that returns at once). Zero values: 3 s, 10 min, a timer.
	Poll    time.Duration
	Timeout time.Duration
	Sleep   func(context.Context, time.Duration) error
}

// StackMergePlan is what a merge would do, shown before it is confirmed.
type StackMergePlan struct {
	Base   string `json:"base"`
	Native bool   `json:"native"`
	// Target is the pull request the merge is asked on; Merges lists every
	// one it merges, bottom first.
	Target ports.PullRequest   `json:"target"`
	Merges []ports.PullRequest `json:"merges"`
}

// StackMergeResult reports a merge.
type StackMergeResult struct {
	Operation string         `json:"operation,omitempty"`
	Plan      StackMergePlan `json:"plan"`
	// Status is GitHub's last answer: merged, enqueued or pending.
	Status   ports.MergeStatus `json:"status"`
	Warnings []ProblemView     `json:"warnings,omitempty"`
	Rehearsal
}

// PlanStackMerge reads what ship stack merge would merge. It changes nothing.
func PlanStackMerge(ctx context.Context, ws Workspace) (StackMergePlan, error) {
	status, err := ws.Git.Status(ctx)
	if err != nil {
		return StackMergePlan{}, err
	}
	c, err := currentChain(ctx, ws, status.Branch.Name)
	if err != nil {
		return StackMergePlan{}, err
	}
	c = existing(ctx, ws, c)
	repo, err := remoteRepository(ctx, ws)
	if err != nil {
		return StackMergePlan{}, err
	}
	plan := StackMergePlan{Base: c.Base}
	if plan.Native, err = stacksSupported(ctx, ws, repo); err != nil {
		return StackMergePlan{}, err
	}
	var open []ports.PullRequest
	var numbers []int
	for _, b := range c.Branches {
		pr, found, err := ws.Forge.FindOpenPullRequest(ctx, repo, b)
		if err != nil {
			return StackMergePlan{}, forgeFault(err, repo)
		}
		if found {
			open = append(open, pr)
			numbers = append(numbers, pr.Number)
		}
	}
	if len(open) == 0 {
		return StackMergePlan{}, fault.New(fault.NothingToPublish, "stack.nothing_to_merge")
	}
	target := stack.MergeTarget(plan.Native, numbers)
	for _, pr := range open {
		plan.Merges = append(plan.Merges, pr)
		if pr.Number == target {
			plan.Target = pr
			break
		}
	}
	return plan, nil
}

// MergeStack asks GitHub to merge the stack through the asynchronous merge
// API (the only one that merges stacks), then waits for the answer. Only
// for a confirmed human: an agent is refused, and GitHub's rules always
// apply.
func MergeStack(ctx context.Context, ws Workspace, req StackMergeRequest) (StackMergeResult, error) {
	done := ws.begin(ctx, "stack/merge")
	mark := ws.planMark()
	result, err := mergeStack(ctx, ws, req)
	result.Rehearsal = ws.rehearsed(mark)
	done(err, operation(result.Operation)...)
	return result, err
}

func mergeStack(ctx context.Context, ws Workspace, req StackMergeRequest) (StackMergeResult, error) {
	if req.Agent || (!req.Confirmed && !ws.IsDryRun()) {
		return StackMergeResult{}, fault.New(fault.MergeNeedsConfirmation, "stack.merge_needs_human")
	}
	plan, err := PlanStackMerge(ctx, ws)
	if err != nil {
		return StackMergeResult{}, err
	}
	result := StackMergeResult{Plan: plan}
	if ws.Stacks == nil {
		return result, fault.New(fault.NoGitHubRemote, "publish.no_remote", "remote", gitRemote(ctx, ws))
	}
	repo, err := remoteRepository(ctx, ws)
	if err != nil {
		return result, err
	}
	target := plan.Target
	st, err := ws.Stacks.MergeAsync(ctx, repo, target.Number, target.HeadSHA)
	if err != nil {
		return result, mergeFault(err, repo, target.Number)
	}
	result.Status = st
	if ws.IsDryRun() {
		return result, nil
	}
	poll, timeout, sleep := req.Poll, req.Timeout, req.Sleep
	if poll <= 0 {
		poll = 3 * time.Second
	}
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	if sleep == nil {
		sleep = sleepContext
	}
	for waited := time.Duration(0); st.Status == "pending" && st.UUID != ""; waited += poll {
		if waited >= timeout {
			result.Warnings = append(result.Warnings, ProblemView{Code: string(fault.MergeStillRunning), Key: "stack.merge_running",
				Params: map[string]string{"url": target.URL}})
			break
		}
		if err := sleep(ctx, poll); err != nil {
			return result, err
		}
		if st, err = ws.Stacks.MergeResult(ctx, repo, target.Number, st.UUID); err != nil {
			return result, mergeFault(err, repo, target.Number)
		}
		result.Status = st
	}
	numbers := make([]string, len(plan.Merges))
	for i, pr := range plan.Merges {
		numbers[i] = "#" + strconv.Itoa(pr.Number)
	}
	op, logErr := ws.Operations.Append(ctx, oplog.Operation{Kind: oplog.StackMerge, PullRequest: target.Number,
		PullRequestURL: target.URL, Title: st.Status})
	result.Operation = op.ID
	switch st.Status {
	case "failed":
		return result, errors.Join(fault.New(fault.MergeFailed, "stack.merge_failed", "number", strconv.Itoa(target.Number),
			"detail", st.Message), logErr)
	case "enqueued":
		result.Warnings = append(result.Warnings, ProblemView{Code: string(fault.MergeQueued), Key: "stack.merge_queued",
			Params: map[string]string{"url": target.URL}})
	}
	return result, logErr
}

// mergeFault explains a refusal of the merge request.
func mergeFault(err error, repo forge.Repository, number int) error {
	switch {
	case errors.Is(err, ports.ErrConflict), errors.Is(err, ports.ErrInvalid), errors.Is(err, ports.ErrForbidden):
		return fault.New(fault.MergeFailed, "stack.merge_failed", "number", strconv.Itoa(number), "detail", lastLine(err.Error()))
	}
	return forgeFault(err, repo)
}

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
