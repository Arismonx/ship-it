// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/core/stack"
)

// chainOf returns the stack a branch belongs to, from the operation log.
func chainOf(ctx context.Context, ws Workspace, branch string) (stack.Chain, bool) {
	if ws.Operations == nil || branch == "" {
		return stack.Chain{}, false
	}
	ops, err := ws.Operations.Recent(ctx, operationSearchDepth)
	if err != nil {
		return stack.Chain{}, false
	}
	return stack.Find(stack.Chains(ops), branch)
}

// onStackTop reports whether branch is the top layer of a stack: `ship` then
// adds a layer on top (a new branch from it) instead of committing on it.
func onStackTop(ctx context.Context, ws Workspace, branch string) bool {
	c, ok := chainOf(ctx, ws, branch)
	return ok && c.Top() == branch
}

// layerTitle is the title ship stack gave the commit of a layer.
func layerTitle(ctx context.Context, ws Workspace, branch string) string {
	ops, err := ws.Operations.Recent(ctx, operationSearchDepth)
	if err != nil {
		return branch
	}
	for _, op := range oplog.Undoable(ops) {
		if op.Kind == oplog.Stack && op.Stack != nil {
			for _, l := range op.Stack.Layers {
				if l.Branch == branch && l.Title != "" {
					return l.Title
				}
			}
		}
	}
	return branch
}

// stackLayer is where a branch of a stack publishes: its parent layer and
// how GitHub groups them.
type stackLayer struct {
	parent   string
	position int // 1 for the bottom layer
	size     int
	native   bool
	// chain is the stack, the new layer on top.
	chain stack.Chain
}

// stackLayerOf returns the layer branch is in a stack, above the bottom
// one, nil otherwise: a bottom layer publishes against the base as any
// branch does. The parent must be on GitHub already.
func stackLayerOf(ctx context.Context, ws Workspace, repo forge.Repository, branch string) (*stackLayer, error) {
	c, ok := chainOf(ctx, ws, branch)
	if !ok {
		return nil, nil
	}
	c = existing(ctx, ws, c)
	i := c.Index(branch)
	if i <= 0 {
		return nil, nil
	}
	l := &stackLayer{parent: c.Parent(i), position: i + 1, size: len(c.Branches), chain: c}
	if exists, err := ws.Git.RemoteBranchExists(ctx, gitRemote(ctx, ws), l.parent); err != nil || !exists {
		return nil, fault.New(fault.StackParentUnpublished, "stack.parent_unpublished", "branch", branch, "parent", l.parent)
	}
	native, err := stacksSupported(ctx, ws, repo)
	if err != nil {
		return nil, err
	}
	l.native = native
	return l, nil
}

// chainNote is the body line of a layer of a classic chain, "" otherwise.
func (l *stackLayer) chainNote(ctx context.Context, ws Workspace, repo forge.Repository) string {
	if l == nil || l.native {
		return ""
	}
	after := 0
	if pr, found, err := ws.Forge.FindOpenPullRequest(ctx, repo, l.parent); err == nil && found {
		after = pr.Number
	}
	return "\n" + stack.ChainNote(l.position, l.size, after) + "\n"
}

// renumberChain writes, in the body of every open pull request of a
// classic chain (branches bottom first), the line that says where it sits
// now: after a layer is added on top or merged and removed. A pull request
// that already says so is left alone; a failure only warns. It returns what
// it rewrote.
func renumberChain(ctx context.Context, ws Workspace, repo forge.Repository, branches []string, warnings *[]ProblemView) []oplog.ChainNoteChange {
	if ws.Stacks == nil {
		return nil
	}
	numbers := make([]int, len(branches))
	for i, b := range branches {
		if pr, found, err := ws.Forge.FindOpenPullRequest(ctx, repo, b); err == nil && found {
			numbers[i] = pr.Number
		}
	}
	var changes []oplog.ChainNoteChange
	for i, n := range numbers {
		if n == 0 {
			continue
		}
		after := 0
		if i > 0 {
			after = numbers[i-1]
		}
		note := stack.ChainNote(i+1, len(branches), after)
		pr, err := ws.Stacks.PullRequest(ctx, repo, n)
		if err != nil {
			*warnings = append(*warnings, *problemOf(forgeFault(err, repo)))
			continue
		}
		body, old := stack.WithChainNote(pr.Body, note)
		if old == note {
			continue
		}
		if err := ws.Stacks.SetPullRequestBody(ctx, repo, n, body); err != nil {
			*warnings = append(*warnings, *problemOf(forgeFault(err, repo)))
			continue
		}
		changes = append(changes, oplog.ChainNoteChange{Number: n, Before: old, After: note})
	}
	return changes
}

// join puts the new pull request of a layer on top of its parent's stack:
// added to the stack the parent's pull request is in, or a new stack of the
// two. A refusal only warns: the pull request is open. In a classic chain,
// the layers below learn that the chain grew.
func (l *stackLayer) join(ctx context.Context, ws Workspace, repo forge.Repository, number int, result *PublishResult) {
	if l == nil {
		return
	}
	if !l.native {
		renumberChain(ctx, ws, repo, l.chain.Branches, &result.Warnings)
		return
	}
	parent, found, err := ws.Forge.FindOpenPullRequest(ctx, repo, l.parent)
	if err != nil || !found {
		result.Warnings = append(result.Warnings, ProblemView{Code: string(fault.StackRefused), Key: "stack.parent_not_open",
			Params: map[string]string{"parent": l.parent}})
		return
	}
	result.Stack = groupInStack(ctx, ws, repo, []int{parent.Number, number}, &result.Warnings)
}
