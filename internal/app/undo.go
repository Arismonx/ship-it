// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/ports"
)

// operationSearchDepth bounds how far back the last undoable operation is
// looked for.
const operationSearchDepth = 200

// UndoResult reports what an undo reverted and where the user stands now.
type UndoResult struct {
	Operation string `json:"operation"`
	Undone    string `json:"undone"`
	// Kind is the kind of the undone operation ("deliver", "tidy").
	Kind string `json:"kind"`
	// Recreated are branches put back by undoing a tidy.
	Recreated []string `json:"recreated,omitempty"`
	Title     string   `json:"title,omitempty"`
	// Branch is the branch HEAD points at now, empty when detached.
	Branch        string `json:"branch,omitempty"`
	DeletedBranch string `json:"deleted_branch,omitempty"`
	// DeletedBranches lists every branch removed (the layers of a stack);
	// Moved the branches put back on their previous commit.
	DeletedBranches []string `json:"deleted_branches,omitempty"`
	Moved           []string `json:"moved,omitempty"`
	// Restored names what undoing a fix put back: files and settings.
	Restored []string `json:"restored,omitempty"`
	// RestoredRemote are the remote branches undoing a stack sync put back,
	// Retargeted the pull requests it pointed at their old base again.
	RestoredRemote []string `json:"restored_remote,omitempty"`
	Retargeted     []int    `json:"retargeted,omitempty"`
	// Renamed is the name a branch had before undoing a rename gave it back
	// its old one (Branch).
	Renamed string `json:"renamed,omitempty"`
	// ClosedPullRequest and DeletedRemoteBranch report remote compensations.
	ClosedPullRequest   int    `json:"closed_pull_request,omitempty"`
	DeletedRemoteBranch string `json:"deleted_remote_branch,omitempty"`
	// ClosedPullRequests lists them all, for a stack; Unstacked is the stack
	// it dissolved.
	ClosedPullRequests []int `json:"closed_pull_requests,omitempty"`
	Unstacked          int   `json:"unstacked,omitempty"`
	// ConfigFile is the configuration file put back by undoing an edit.
	ConfigFile string `json:"config_file,omitempty"`
	// RestoredIssueStatus is the issue whose status in the GitHub Project an
	// undone publication put back; Unassigned the login it removed from it.
	RestoredIssueStatus int    `json:"restored_issue_status,omitempty"`
	Unassigned          string `json:"unassigned,omitempty"`
	// Warnings did not stop the undo: a project step that failed, a status
	// changed by someone since and kept.
	Warnings []ProblemView  `json:"warnings,omitempty"`
	Safety   CheckpointView `json:"safety"`
	Steps    []oplog.Step   `json:"steps"`
	// Rehearsal: on a dry run, Operation and Safety are empty.
	Rehearsal
}

// UndoRequest carries the human's consent: app never asks.
type UndoRequest struct {
	// AllowRemote agrees to change GitHub (close a pull request, delete a
	// remote branch). Without it such an undo stops with UndoNeedsConsent.
	AllowRemote bool
}

// remoteUndoComment explains, on the pull request, why it was closed.
const remoteUndoComment = "Closed with `ship undo`: the author took this work back. The commits are still on their machine."

// Undo reverts the most recent operation that is not undone yet:
//   - a delivery: the commit goes back to uncommitted changes, a branch
//     ship-it created is removed and HEAD returns where it was;
//   - a publication that opened a pull request: the pull request is closed
//     and the remote branch deleted, with the human's explicit consent; the
//     linked issue gets back its status in the GitHub Project and loses the
//     assignee the publication added, unless someone changed them since;
//   - a tidy: the deleted branches are recreated;
//   - a configuration edit: the file is put back as it was, if nobody
//     changed it since.
//
// The files are kept, a safety checkpoint is taken first, and the undo itself
// is appended to the operation log.
func Undo(ctx context.Context, ws Workspace, req UndoRequest) (UndoResult, error) {
	done := ws.begin(ctx, "undo")
	mark := ws.planMark()
	result, err := undo(ctx, ws, req)
	result.Rehearsal = ws.rehearsed(mark)
	done(err, append(operation(result.Operation), slog.String("undone", result.Undone), slog.String("kind", result.Kind))...)
	return result, err
}

func undo(ctx context.Context, ws Workspace, req UndoRequest) (UndoResult, error) {
	ops, err := ws.Operations.Recent(ctx, operationSearchDepth)
	if err != nil {
		return UndoResult{}, err
	}
	target, ok := oplog.LastUndoable(ops)
	if !ok {
		return UndoResult{}, fault.New(fault.NothingToUndo, "undo.nothing")
	}
	status, err := ws.Git.Status(ctx)
	if err != nil {
		return UndoResult{}, err
	}
	current := repositoryViewOf(status.Branch)
	if target.Checkpoint != "" {
		// A checkpoint removed by `ship gc` only makes the undo skip the
		// defensive restore; refs and index are still put back.
		if _, err := findCheckpoint(ctx, ws.Checkpoints, target.Checkpoint); err != nil {
			target.Checkpoint = ""
		}
	}
	steps, err := oplog.PlanUndo(target, current)
	if err != nil {
		return UndoResult{}, err
	}
	if oplog.NeedsCleanTree(target) && !status.Clean() {
		return UndoResult{}, fault.New(fault.UndoNeedsCleanTree, "undo.needs_clean_tree")
	}
	steps, err = checkRemoteSteps(ctx, ws, target, steps, req)
	if err != nil {
		return UndoResult{}, err
	}

	safety, err := ws.Checkpoints.Create(ctx, "before-undo")
	if err != nil {
		return UndoResult{}, err
	}
	result := UndoResult{Undone: target.ID, Kind: string(target.Kind), Title: target.Title, Branch: target.After.Branch, Safety: viewOf(safety), Steps: steps}
	for _, step := range steps {
		if isLinkedIssueStep(target, step) {
			// Like publish, the project side only warns: the pull request
			// is closed already.
			changed, err := undoLinkedIssueStep(ctx, ws, step)
			switch {
			case err != nil:
				result.Warnings = append(result.Warnings, *problemOf(err))
			case changed && step.Kind == oplog.UnassignIssue:
				result.Unassigned = step.Assignee
			case changed:
				result.RestoredIssueStatus = step.Number
			}
			continue
		}
		if isStackSyncStep(step) {
			if err := undoStackSyncStep(ctx, ws, step); err != nil {
				result.Warnings = append(result.Warnings, *problemOf(err))
				continue
			}
			switch step.Kind {
			case oplog.RestoreRemoteBranch:
				result.RestoredRemote = append(result.RestoredRemote, step.Branch)
			case oplog.RetargetPullRequest:
				result.Retargeted = append(result.Retargeted, step.Number)
			}
			continue
		}
		if err := applyUndoStep(ctx, ws, step); err != nil {
			return result, fmt.Errorf("undo step %s: %w", step.Kind, err)
		}
		switch step.Kind {
		case oplog.AttachHead:
			result.Branch = step.Branch
		case oplog.DetachHead:
			result.Branch = ""
		case oplog.DeleteBranch:
			if step.Branch != result.Branch {
				result.DeletedBranch = step.Branch
			}
			result.DeletedBranches = append(result.DeletedBranches, step.Branch)
		case oplog.MoveBranch:
			result.Moved = append(result.Moved, step.Branch)
		case oplog.RestoreFile:
			result.Restored = append(result.Restored, step.File.Path)
		case oplog.RestoreConfig:
			result.Restored = append(result.Restored, step.Config.Key)
		case oplog.TrackFiles:
			result.Restored = append(result.Restored, step.Paths...)
		case oplog.RecreateBranch:
			result.Recreated = append(result.Recreated, step.Branch)
		case oplog.ClosePullRequest:
			result.ClosedPullRequest = step.Number
			result.ClosedPullRequests = append(result.ClosedPullRequests, step.Number)
		case oplog.DeleteRemoteBranch:
			result.DeletedRemoteBranch = step.Branch
		case oplog.RestoreConfigFile:
			result.ConfigFile = step.ConfigFile.Path
		case oplog.RenameBranch:
			if step.Branch == result.Branch { // a layer below the top is not checked out
				result.Branch = step.Target
			}
			result.Renamed = step.Branch
		case oplog.Unstack:
			result.Unstacked = step.Number
		}
	}

	op, err := ws.Operations.Append(ctx, oplog.Operation{
		Kind: oplog.Undo, Checkpoint: safety.ID, Before: current, After: target.Before, Undoes: target.ID,
	})
	result.Operation = op.ID
	return result, err
}

// checkRemoteSteps asks for consent and reads the pull request as it is now:
// a merged one cannot be undone, a closed one needs no closing.
func checkRemoteSteps(ctx context.Context, ws Workspace, target oplog.Operation, steps []oplog.Step, req UndoRequest) ([]oplog.Step, error) {
	if !slices.ContainsFunc(steps, oplog.Step.Remote) {
		return steps, nil
	}
	switch target.Kind {
	case oplog.StackPublish:
		return checkStackUndo(ctx, ws, target, steps, req)
	case oplog.StackSync:
		return checkStackSyncUndo(ws, steps, req)
	}
	if !req.AllowRemote {
		if target.Kind == oplog.Roadmap {
			created, milestones, items := 0, 0, 0
			if rs := target.RoadmapSync; rs != nil {
				created, milestones = len(rs.CreatedIssues), len(rs.CreatedMilestones)
				if rs.Project != nil {
					items = len(rs.Project.AddedItems)
				}
			}
			if !ws.IsDryRun() {
				return nil, fault.New(fault.UndoNeedsConsent, "undo.needs_consent_roadmap",
					"issues", strconv.Itoa(created), "milestones", strconv.Itoa(milestones), "items", strconv.Itoa(items))
			}
			ws.planStep("app.dryrun.needs_consent_roadmap", "issues", strconv.Itoa(created), "milestones", strconv.Itoa(milestones))
		} else {
			if !ws.IsDryRun() {
				if slices.ContainsFunc(steps, func(s oplog.Step) bool { return isLinkedIssueStep(target, s) }) {
					return nil, fault.New(fault.UndoNeedsConsent, "undo.needs_consent_issue",
						"number", strconv.Itoa(target.PullRequest), "branch", target.After.Branch, "url", target.PullRequestURL,
						"issue", strconv.Itoa(target.LinkedIssue.Issue))
				}
				return nil, fault.New(fault.UndoNeedsConsent, "undo.needs_consent",
					"number", strconv.Itoa(target.PullRequest), "branch", target.After.Branch, "url", target.PullRequestURL)
			}
			// Nothing changes, so nothing needs consent yet: the plan says the
			// real run will ask.
			ws.planStep("app.dryrun.needs_consent", "number", strconv.Itoa(target.PullRequest), "branch", target.After.Branch)
		}
	}
	if target.Kind == oplog.Roadmap {
		return steps, nil // nothing to check on a pull request
	}
	remote := target.Remote
	if remote == "" {
		remote = gitRemote(ctx, ws)
	}
	if ws.Forge == nil {
		return nil, fault.New(fault.NoGitHubRemote, "publish.no_remote", "remote", remote)
	}
	own, err := repositoryOf(ctx, ws, remote)
	if err != nil {
		return nil, err
	}
	// A fork's pull request lives on the parent, its head on the fork.
	repo, head := pullRequestRepository(own, target.PullRequestRepository), target.After.Branch
	if repo != own {
		head = own.Owner + ":" + head
	}
	pr, found, err := ws.Forge.LatestPullRequest(ctx, repo, head)
	if err != nil {
		return nil, forgeFault(err, repo)
	}
	if found && pr.Number == target.PullRequest {
		if pr.Merged {
			return nil, fault.New(fault.UndoOfMergedWork, "undo.merged", "url", target.PullRequestURL)
		}
		if pr.State == "closed" {
			steps = slices.DeleteFunc(steps, func(s oplog.Step) bool { return s.Kind == oplog.ClosePullRequest })
		}
	}
	return steps, nil
}

func applyUndoStep(ctx context.Context, ws Workspace, step oplog.Step) error {
	switch step.Kind {
	case oplog.CloseCreatedIssue, oplog.RestoreIssueState, oplog.RemoveSubIssue, oplog.DeleteMilestone,
		oplog.RestoreProjectField, oplog.RemoveProjectItem:
		return undoRoadmapStep(ctx, ws, step)
	case oplog.UnassignIssue:
		_, err := undoLinkedIssueStep(ctx, ws, step)
		return err
	case oplog.Unstack:
		return unstackStep(ctx, ws, step)
	case oplog.ClosePullRequest:
		own, err := remoteRepository(ctx, ws)
		if err != nil {
			return err
		}
		repo := pullRequestRepository(own, step.Repository)
		if step.ClearMilestone && ws.Tracker != nil {
			// A closed pull request would count as done in the milestone.
			none := 0
			if err := ws.Tracker.UpdateIssue(ctx, repo, step.Number, ports.IssueUpdate{Milestone: &none}); err != nil {
				return forgeFault(err, repo)
			}
		}
		comment := remoteUndoComment
		if step.Branch != "" { // a layer of a stack
			comment = stackUndoComment
		}
		return forgeFault(ws.Forge.ClosePullRequest(ctx, repo, step.Number, comment), repo)
	case oplog.DeleteRemoteBranch:
		remote := step.RemoteName
		if remote == "" {
			remote = gitRemote(ctx, ws)
		}
		if err := ws.Git.DeleteRemoteBranch(ctx, remote, step.Branch, step.Expected); err != nil {
			return fault.New(fault.RemoteBranchMoved, "undo.remote_branch_moved", "branch", step.Branch)
		}
		return nil
	case oplog.AttachHead:
		return ws.Git.AttachHead(ctx, step.Branch)
	case oplog.DetachHead:
		return ws.Git.DetachHead(ctx, step.Commit)
	case oplog.MoveBranch:
		return ws.Git.MoveBranch(ctx, step.Branch, step.Commit, step.Expected)
	case oplog.DeleteBranch:
		return ws.Git.DeleteBranch(ctx, step.Branch, step.Expected)
	case oplog.RenameBranch:
		return ws.Git.RenameBranch(ctx, step.Branch, step.Target, step.Expected)
	case oplog.ResetIndex:
		return ws.Git.ResetIndex(ctx, step.Commit)
	case oplog.RestoreCheckpoint:
		return ws.Checkpoints.RestoreFiles(ctx, step.Checkpoint)
	case oplog.RecreateBranch:
		return ws.Git.RecreateBranch(ctx, step.Branch, step.Commit)
	case oplog.RestoreConfig:
		if step.Config.HadOld {
			return ws.Git.SetConfig(ctx, step.Config.Key, step.Config.Old, step.Config.Global)
		}
		return ws.Git.UnsetConfig(ctx, step.Config.Key, step.Config.Global)
	case oplog.TrackFiles:
		return ws.Git.Track(ctx, step.Paths)
	case oplog.RestoreConfigFile:
		return restoreConfigFile(ctx, ws, *step.ConfigFile)
	case oplog.RestoreFile:
		if step.File.Existed {
			return ws.Git.WriteWorkingFile(ctx, step.File.Path, []byte(step.File.Content))
		}
		return ws.Git.RemoveWorkingFile(ctx, step.File.Path)
	}
	return fmt.Errorf("unknown undo step %q", step.Kind)
}

// restoreConfigFile puts a configuration file back as it was before an edit,
// only if it still holds what the edit wrote: a later change by hand is never
// overwritten.
func restoreConfigFile(ctx context.Context, ws Workspace, edit oplog.ConfigFileEdit) error {
	if ws.Config == nil {
		return errors.New("app: no configuration files")
	}
	_, err := ws.Config.ReplaceConfigFile(ctx, ports.ConfigFileChange{
		Scope: ports.ConfigScope(edit.Scope), Old: []byte(edit.After), OldExists: true,
		New: []byte(edit.Before), NewExists: edit.BeforeExisted,
	})
	if errors.Is(err, ports.ErrConfigChanged) {
		return fault.New(fault.ConfigFileChanged, "undo.config_changed", "file", edit.Path, "key", edit.Key)
	}
	return err
}

// isLinkedIssueStep reports whether step puts back what a publication changed
// on its linked issue: the project status, the assignee.
func isLinkedIssueStep(target oplog.Operation, step oplog.Step) bool {
	return (target.Kind == oplog.Publish || target.Kind == oplog.StackPublish) && (step.Kind == oplog.RestoreProjectField || step.Kind == oplog.UnassignIssue)
}

// undoLinkedIssueStep puts back what a publication changed on its linked
// issue, compare-and-swap: a status someone changed since is kept (a
// warning), an assignee already removed needs nothing. changed reports
// whether it wrote.
func undoLinkedIssueStep(ctx context.Context, ws Workspace, step oplog.Step) (changed bool, err error) {
	if ws.Tracker == nil || (step.Kind == oplog.RestoreProjectField && (ws.Projects == nil || step.Project == nil)) {
		return false, fault.New(fault.NoGitHubRemote, "publish.no_remote", "remote", gitRemote(ctx, ws))
	}
	repo, err := remoteRepository(ctx, ws)
	if err != nil {
		return false, err
	}
	if step.Kind == oplog.UnassignIssue {
		is, err := ws.Tracker.Issue(ctx, repo, step.Number)
		if err != nil {
			return false, forgeFault(err, repo)
		}
		if !slices.ContainsFunc(is.Assignees, func(login string) bool { return strings.EqualFold(login, step.Assignee) }) {
			return false, nil // removed since: nothing to do
		}
		if err := ws.Tracker.RemoveAssignees(ctx, repo, step.Number, []string{step.Assignee}); err != nil {
			return false, forgeFault(err, repo)
		}
		return true, nil
	}
	project := *step.Project
	items, err := ws.Projects.ProjectItems(ctx, repo, project, []int64{step.Field})
	if err != nil {
		return false, projectFault(err, repo, project)
	}
	current, found := "", false
	for _, it := range items {
		if it.ID == step.Item {
			current, found = it.Values[step.Field], true
		}
	}
	if !found || current != step.Expected {
		return false, fault.New(fault.ProjectStatusChanged, "undo.project_status_changed",
			"issue", strconv.Itoa(step.Number), "project", project.String())
	}
	if err := ws.Projects.SetProjectField(ctx, repo, project, step.Item, step.Field, step.Value); err != nil {
		return false, projectFault(err, repo, project)
	}
	return true, nil
}

// roadmapUndoComment explains, on an issue closed by an undo, why.
const roadmapUndoComment = "Closed by `ship undo`: the roadmap sync that created this issue was undone."

// undoRoadmapStep applies one compensation of a roadmap sync on GitHub.
func undoRoadmapStep(ctx context.Context, ws Workspace, step oplog.Step) error {
	if ws.Tracker == nil {
		return fault.New(fault.NoGitHubRemote, "publish.no_remote", "remote", gitRemote(ctx, ws))
	}
	repo, err := remoteRepository(ctx, ws)
	if err != nil {
		return err
	}
	switch step.Kind {
	case oplog.RestoreProjectField, oplog.RemoveProjectItem:
		if ws.Projects == nil || step.Project == nil {
			return fault.New(fault.NoGitHubRemote, "publish.no_remote", "remote", gitRemote(ctx, ws))
		}
		if step.Kind == oplog.RemoveProjectItem {
			return projectFault(ws.Projects.RemoveProjectItem(ctx, repo, *step.Project, step.Item), repo, *step.Project)
		}
		return projectFault(ws.Projects.SetProjectField(ctx, repo, *step.Project, step.Item, step.Field, step.Value), repo, *step.Project)
	case oplog.CloseCreatedIssue:
		if err := ws.Tracker.CommentIssue(ctx, repo, step.Number, roadmapUndoComment); err != nil {
			return forgeFault(err, repo)
		}
		return forgeFault(ws.Tracker.UpdateIssue(ctx, repo, step.Number, ports.IssueUpdate{State: "closed", StateReason: "not_planned"}), repo)
	case oplog.RestoreIssueState:
		update := ports.IssueUpdate{State: "closed", StateReason: "completed"}
		if step.WasOpen {
			update = ports.IssueUpdate{State: "open", StateReason: "reopened"}
		}
		return forgeFault(ws.Tracker.UpdateIssue(ctx, repo, step.Number, update), repo)
	case oplog.RemoveSubIssue:
		return forgeFault(ws.Tracker.RemoveParent(ctx, repo, step.Number, step.Child), repo)
	default: // DeleteMilestone, only when it holds nothing but what the sync created
		milestones, err := ws.Tracker.Milestones(ctx, repo)
		if err != nil {
			return forgeFault(err, repo)
		}
		for _, m := range milestones {
			if m.Number == step.Number {
				if m.OpenIssues == 0 && m.ClosedIssues <= step.Issues {
					return forgeFault(ws.Tracker.DeleteMilestone(ctx, repo, m.Number), repo)
				}
				return nil // in use since: kept
			}
		}
		return nil
	}
}

// pullRequestRepository is the repository a publication's pull request was
// opened on: slug ("owner/name", the parent of a fork) on the host of own,
// else own.
func pullRequestRepository(own forge.Repository, slug string) forge.Repository {
	owner, name, ok := strings.Cut(slug, "/")
	if !ok || owner == "" || name == "" {
		return own
	}
	return forge.Repository{Host: own.Host, Owner: owner, Name: name}
}
