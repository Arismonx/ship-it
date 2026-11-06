// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package stack decides how work becomes a stack of pull requests
// (ADR-0006, revision of 2026-11-02): the layers, the chain of branches the
// operation log describes, the order of a sync after a merge and the pull
// request a merge targets.
//
// Allowed imports: standard library without I/O, other core packages.
package stack

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/core/split"
)

// MinLayers is the smallest stack GitHub accepts: two pull requests.
const MinLayers = 2

// Layer is one pull request of a stack: the files it commits.
type Layer struct {
	// Area is the topic directory of the layer ("src/api/"), or the areas it
	// groups, joined with ", ".
	Area  string   `json:"area"`
	Lines int      `json:"lines"`
	Files []string `json:"files"`
}

// Layers splits the work into layers by area, as the split suggestion does
// (split.Areas, biggest first), the first one at the bottom. Beyond maxLayers
// the smaller areas are grouped in the top layer. Work of a single area
// cannot be stacked: StackNeedsLayers.
func Layers(files []split.File, maxLayers int) ([]Layer, error) {
	maxLayers = max(maxLayers, MinLayers)
	areas := split.Areas(files)
	if len(areas) < MinLayers {
		return nil, fault.New(fault.StackNeedsLayers, "stack.one_area")
	}
	layers := make([]Layer, 0, min(len(areas), maxLayers))
	for i, a := range areas {
		if i < maxLayers {
			layers = append(layers, Layer{Area: a.Path, Lines: a.Lines, Files: slices.Clone(a.Files)})
			continue
		}
		top := &layers[maxLayers-1]
		top.Area += ", " + a.Path
		top.Lines += a.Lines
		top.Files = append(top.Files, a.Files...)
	}
	return layers, nil
}

// Choose builds the layers the human chose: each selection (files, or
// directories with everything under them, as --only takes them) is one
// layer, bottom first. A file goes to the first selection that matches it;
// the files no selection matches stay uncommitted.
func Choose(files []split.File, selections [][]string) ([]Layer, error) {
	if len(selections) < MinLayers {
		return nil, fault.New(fault.StackNeedsLayers, "stack.too_few_layers", "count", fmt.Sprint(len(selections)))
	}
	layers := make([]Layer, len(selections))
	for i, sel := range selections {
		layers[i].Area = strings.Join(sel, ", ")
	}
	for _, f := range files {
		for i, sel := range selections {
			if split.Matches(f.Path, sel) {
				layers[i].Files = append(layers[i].Files, f.Path)
				layers[i].Lines += f.Lines
				break
			}
		}
	}
	for _, l := range layers {
		if len(l.Files) == 0 {
			return nil, fault.New(fault.StackNeedsLayers, "stack.empty_layer", "layer", l.Area)
		}
	}
	return layers, nil
}

// Chain is a stack as ship-it built it: branches from the bottom up, the
// bottom one based on Base.
type Chain struct {
	Base     string   `json:"base"`
	Branches []string `json:"branches"`
}

// Top is the branch at the top of the chain.
func (c Chain) Top() string { return c.Branches[len(c.Branches)-1] }

// Index returns the position of branch in the chain, -1 when absent.
func (c Chain) Index(branch string) int { return slices.Index(c.Branches, branch) }

// Parent is the branch a layer is based on: the layer below, or Base.
func (c Chain) Parent(i int) string {
	if i == 0 {
		return c.Base
	}
	return c.Branches[i-1]
}

// Chains rebuilds the stacks from the operation log (newest first): each
// Stack operation not undone starts one, each delivery that added a layer on
// top of one (StackParent) extends it, and the renames a GitHub ruleset
// asked for (a StackPublish's Renames, a Rename) follow the branches.
func Chains(operations []oplog.Operation) []Chain {
	live := oplog.Undoable(operations)
	var chains []Chain
	for i := len(live) - 1; i >= 0; i-- {
		op := live[i]
		switch {
		case op.Kind == oplog.Stack && op.Stack != nil && len(op.Stack.Layers) > 0:
			c := Chain{Base: op.Stack.Base}
			for _, l := range op.Stack.Layers {
				c.Branches = append(c.Branches, l.Branch)
			}
			chains = append(chains, c)
		case op.Kind == oplog.StackPublish && op.Stack != nil && len(op.Stack.Renames) > 0:
			// Layers renamed for a GitHub ruleset keep their place.
			for _, r := range op.Stack.Renames {
				for j := range chains {
					if k := chains[j].Index(r.From); k >= 0 {
						chains[j].Branches[k] = r.To
					}
				}
			}
		case op.Kind == oplog.Rename:
			// A layer added on top, renamed when ship published it.
			for j := range chains {
				if k := chains[j].Index(op.Before.Branch); k >= 0 {
					chains[j].Branches[k] = op.After.Branch
				}
			}
		case op.Kind == oplog.Deliver && op.StackParent != "" && op.CreatedBranch:
			for j := range chains {
				if chains[j].Top() == op.StackParent {
					chains[j].Branches = append(chains[j].Branches, op.After.Branch)
					break
				}
			}
		}
	}
	return chains
}

// Find returns the chain branch belongs to.
func Find(chains []Chain, branch string) (Chain, bool) {
	// The newest chain wins: a branch name reused after an undo.
	for i := len(chains) - 1; i >= 0; i-- {
		if chains[i].Index(branch) >= 0 {
			return chains[i], true
		}
	}
	return Chain{}, false
}

// Without returns the chain without the branches keep refuses (merged and
// deleted since): the lowest one left is based on Base.
func (c Chain) Without(gone func(branch string) bool) Chain {
	out := Chain{Base: c.Base}
	for _, b := range c.Branches {
		if !gone(b) {
			out.Branches = append(out.Branches, b)
		}
	}
	return out
}

// ChainNote is the line a pull request of a classic chain carries in its
// body, where GitHub shows no stack (GHES): "Part 2 of 3, after #41". Pull
// request bodies are written in English, like the rest of them.
func ChainNote(position, size, after int) string {
	if after == 0 {
		return fmt.Sprintf("Part %d of %d.", position, size)
	}
	return fmt.Sprintf("Part %d of %d, after #%d.", position, size, after)
}

// chainNoteLine matches the line ChainNote writes.
var chainNoteLine = regexp.MustCompile(`(?m)^Part [0-9]+ of [0-9]+(, after #[0-9]+)?\.$`)

// WithChainNote puts note in a pull request body: in place of the chain
// line it holds (old), or at its end. An empty note removes the line.
func WithChainNote(body, note string) (updated, old string) {
	if loc := chainNoteLine.FindStringIndex(body); loc != nil {
		old = body[loc[0]:loc[1]]
		if note == "" {
			end := loc[1]
			if end < len(body) && body[end] == '\n' {
				end++
			}
			return body[:loc[0]] + body[end:], old
		}
		return body[:loc[0]] + note + body[loc[1]:], old
	}
	if note == "" {
		return body, ""
	}
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return body + "\n" + note + "\n", ""
}

// LayerState is a layer as a sync finds it.
type LayerState struct {
	Branch string
	// Merged: its pull request was merged.
	Merged bool
}

// SyncStep is what a sync does to one layer that stays.
type SyncStep struct {
	Branch string `json:"branch"`
	// Parent is the branch it is based on after the sync: the layer below
	// that stays, or the chain's base.
	Parent string `json:"parent"`
	// OldParent is the branch it was based on; Retarget is set when its pull
	// request must target Parent instead.
	OldParent string `json:"old_parent"`
	Retarget  bool   `json:"retarget,omitempty"`
}

// SyncPlan is the order of a sync, bottom up.
type SyncPlan struct {
	// Merged are the layers to delete, Remaining the others with their new
	// parent.
	Merged    []string   `json:"merged,omitempty"`
	Remaining []SyncStep `json:"remaining,omitempty"`
}

// PlanSync decides a sync: merged layers go, every other one is rebased on
// the layer below it that stays (or the base), bottom up. GitHub merges a
// stack from the bottom, so merged layers are below the others; one merged
// above an open one is dropped all the same.
func PlanSync(c Chain, layers []LayerState) SyncPlan {
	merged := map[string]bool{}
	var plan SyncPlan
	for _, l := range layers {
		if l.Merged {
			merged[l.Branch] = true
			plan.Merged = append(plan.Merged, l.Branch)
		}
	}
	parent := c.Base
	for i, b := range c.Branches {
		if merged[b] {
			continue
		}
		old := c.Parent(i)
		plan.Remaining = append(plan.Remaining, SyncStep{Branch: b, Parent: parent, OldParent: old, Retarget: old != parent})
		parent = b
	}
	return plan
}

// MergeTarget is the pull request a merge of the stack is asked on. Merging
// a layer of a native stack merges every layer below it, so the top one
// merges the stack ("every pull request in the stack up to and including the
// one you request", GitHub Docs, "Stacked pull requests APIs and webhooks",
// read 2026-11-05). A classic chain is merged one layer at a time, from the
// bottom: the pull requests above target branches, not the base.
func MergeTarget(native bool, numbers []int) int {
	if len(numbers) == 0 {
		return 0
	}
	if native {
		return numbers[len(numbers)-1]
	}
	return numbers[0]
}
