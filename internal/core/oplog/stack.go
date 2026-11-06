// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package oplog

import "github.com/GregoireF/ship-it/internal/core/fault"

// Operation kinds of ship stack (ADR-0006, revision of 2026-11-02).
const (
	// Stack created one branch and one commit per layer, each branch based on
	// the previous one: Stack.Layers bottom to top, Before where the work was.
	// ship undo puts the work back uncommitted and removes the branches.
	Stack Kind = "stack"
	// StackPublish pushed the layers, opened their pull requests and, on
	// github.com, grouped them in a native stack. ship undo closes the
	// unmerged pull requests, dissolves the stack and deletes the remote
	// branches, with consent.
	StackPublish Kind = "stack_publish"
	// StackSync followed a merge: it deleted the merged layers and rebased
	// the others. ship undo puts the local branches back and, with consent,
	// GitHub: the remote branches where nobody pushed since, the bases and
	// chain lines of the pull requests.
	StackSync Kind = "stack_sync"
	// StackMerge asked GitHub to merge the stack. Recorded for ship log;
	// a merge is never undone.
	StackMerge Kind = "stack_merge"
)

// The compensations of a stack on GitHub. Unstack dissolves stack Number
// (its pull requests stay open). RestoreRemoteBranch pushes Commit to the
// remote Branch only if it still points at Expected ("": only if it does
// not exist); RetargetPullRequest makes pull request Number target Target
// again, only if it still targets Expected; RestoreChainNote puts Value
// back in place of the chain line Expected in its body. Where someone
// changed them since, theirs is kept.
const (
	Unstack             StepKind = "unstack"
	RestoreRemoteBranch StepKind = "restore_remote_branch"
	RetargetPullRequest StepKind = "retarget_pull_request"
	RestoreChainNote    StepKind = "restore_chain_note"
)

// StackRecord is what a stack operation did.
type StackRecord struct {
	// Base is the branch the bottom layer targets.
	Base string `json:"base,omitempty"`
	// Native: the layers form a GitHub stack, Number; else a classic chain of
	// pull requests, each based on the previous branch.
	Native bool         `json:"native,omitempty"`
	Number int          `json:"number,omitempty"`
	Layers []StackLayer `json:"layers"`
	// Renames are the layer branches StackPublish renamed before pushing
	// them, as a GitHub ruleset requires (only branches ship-it created and
	// never pushed). ship undo gives them their old names back.
	Renames []StackRename `json:"renames,omitempty"`
	// Notes are the chain lines StackSync rewrote.
	Notes []ChainNoteChange `json:"notes,omitempty"`
}

// ChainNoteChange is a "Part 2 of 3, after #41." line StackSync rewrote in
// the body of pull request Number of a classic chain: Before is the line it
// held ("" for none), After the one written ("" when removed).
type ChainNoteChange struct {
	Number int    `json:"number"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}

// StackRename is a layer branch renamed from From to To at Commit.
type StackRename struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Commit string `json:"commit"`
}

// StackLayer is one layer of a stack operation. Only the fields its
// operation documents are set.
type StackLayer struct {
	Branch string `json:"branch"`
	// Commit is the commit Stack made, the commit StackPublish pushed, or the
	// commit StackSync left the branch at.
	Commit string `json:"commit,omitempty"`
	// Title is the title of the layer's commit (Stack).
	Title string `json:"title,omitempty"`
	// PullRequest and URL name the layer's pull request; Created is set when
	// StackPublish opened it.
	PullRequest int    `json:"pull_request,omitempty"`
	URL         string `json:"url,omitempty"`
	Created     bool   `json:"created,omitempty"`
	// Previous is the commit of the branch before StackSync rebased or
	// deleted it; Deleted is set when it was merged and removed.
	Previous string `json:"previous,omitempty"`
	Deleted  bool   `json:"deleted,omitempty"`
	// RemotePrevious is the commit of the remote branch right before StackSync
	// changed it: Pushed is set when it pushed Commit there under a lease,
	// RemoteDeleted when it deleted the branch of a merged layer.
	RemotePrevious string `json:"remote_previous,omitempty"`
	Pushed         bool   `json:"pushed,omitempty"`
	RemoteDeleted  bool   `json:"remote_deleted,omitempty"`
	// PreviousBase is the branch PullRequest targeted before StackSync
	// retargeted it to Base (a classic chain).
	PreviousBase string `json:"previous_base,omitempty"`
	Base         string `json:"base,omitempty"`
}

// planStackUndo reverts the stack operations.
func planStackUndo(op Operation, current RepositoryView) ([]Step, error) {
	if op.Stack == nil || (len(op.Stack.Layers) == 0 && len(op.Stack.Renames) == 0) {
		return nil, fault.New(fault.NothingToUndo, "undo.nothing")
	}
	layers := op.Stack.Layers
	switch op.Kind {
	case StackPublish:
		// GitHub first: the stack, then each pull request it opened and its
		// branch, from the top down.
		var steps []Step
		if op.Stack.Native && op.Stack.Number != 0 {
			steps = append(steps, Step{Kind: Unstack, Number: op.Stack.Number})
		}
		for i := len(layers) - 1; i >= 0; i-- {
			l := layers[i]
			if !l.Created {
				continue // updated, not opened: not ours to close
			}
			steps = append(steps,
				Step{Kind: ClosePullRequest, Number: l.PullRequest, Branch: l.Branch},
				Step{Kind: DeleteRemoteBranch, Branch: l.Branch, Expected: l.Commit})
		}
		// What the bottom layer changed on its linked issue, as for a Publish.
		steps = append(steps, linkedIssueSteps(op.LinkedIssue)...)
		// The old names last, once the remote branches under the new ones
		// are gone: from the top down, as they were renamed bottom up.
		for i := len(op.Stack.Renames) - 1; i >= 0; i-- {
			r := op.Stack.Renames[i]
			steps = append(steps, Step{Kind: RenameBranch, Branch: r.To, Target: r.From, Expected: r.Commit})
		}
		if len(steps) == 0 {
			url := ""
			if len(layers) > 0 {
				url = layers[0].URL
			}
			return nil, fault.New(fault.UndoOfPublishedWork, "undo.published", "url", url)
		}
		return steps, nil
	case Stack:
		if current.Head != op.After.Head || current.Branch != op.After.Branch || current.Detached {
			return nil, fault.New(fault.RepositoryMovedSinceOperation, "undo.repository_moved",
				"branch", op.After.Branch, "current", current.Branch)
		}
		steps := []Step{headBack(op.Before)}
		steps = append(steps, Step{Kind: ResetIndex, Commit: op.Before.Head})
		if op.Checkpoint != "" {
			steps = append(steps, Step{Kind: RestoreCheckpoint, Checkpoint: op.Checkpoint})
		}
		for i := len(layers) - 1; i >= 0; i-- {
			steps = append(steps, Step{Kind: DeleteBranch, Branch: layers[i].Branch, Expected: layers[i].Commit})
		}
		return steps, nil
	default: // StackSync
		if current.Head != op.After.Head || current.Branch != op.After.Branch || current.Detached {
			return nil, fault.New(fault.RepositoryMovedSinceOperation, "undo.repository_moved",
				"branch", op.After.Branch, "current", current.Branch)
		}
		var steps []Step
		for _, l := range layers {
			switch {
			case l.Deleted:
				steps = append(steps, Step{Kind: RecreateBranch, Branch: l.Branch, Commit: l.Previous})
			case l.Previous != "" && l.Previous != l.Commit:
				steps = append(steps, Step{Kind: MoveBranch, Branch: l.Branch, Commit: l.Previous, Expected: l.Commit})
			}
		}
		steps = append(steps, headBack(op.Before), Step{Kind: ResetIndex, Commit: op.Before.Head})
		if op.Checkpoint != "" {
			steps = append(steps, Step{Kind: RestoreCheckpoint, Checkpoint: op.Checkpoint})
		}
		return append(steps, syncRemoteSteps(op.Stack)...), nil
	}
}

// syncRemoteSteps put GitHub back as a sync found it: the remote branches
// first (a pull request is retargeted to a branch that exists), then the
// bases of the pull requests, then the chain lines, newest first.
func syncRemoteSteps(record *StackRecord) []Step {
	var steps []Step
	for _, l := range record.Layers {
		switch {
		case l.RemotePrevious == "":
		case l.Deleted && l.RemoteDeleted:
			steps = append(steps, Step{Kind: RestoreRemoteBranch, Branch: l.Branch, Commit: l.RemotePrevious})
		case !l.Deleted && l.Pushed:
			steps = append(steps, Step{Kind: RestoreRemoteBranch, Branch: l.Branch, Commit: l.RemotePrevious, Expected: l.Commit})
		}
	}
	for _, l := range record.Layers {
		if l.PullRequest != 0 && l.PreviousBase != "" {
			steps = append(steps, Step{Kind: RetargetPullRequest, Number: l.PullRequest, Branch: l.Branch, Target: l.PreviousBase, Expected: l.Base})
		}
	}
	for i := len(record.Notes) - 1; i >= 0; i-- {
		n := record.Notes[i]
		steps = append(steps, Step{Kind: RestoreChainNote, Number: n.Number, Value: n.Before, Expected: n.After})
	}
	return steps
}

// headBack points HEAD where it was, files untouched.
func headBack(before RepositoryView) Step {
	if before.Detached {
		return Step{Kind: DetachHead, Commit: before.Head}
	}
	return Step{Kind: AttachHead, Branch: before.Branch}
}
