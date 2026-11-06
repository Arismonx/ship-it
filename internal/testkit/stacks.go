// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package testkit

import (
	"context"
	"slices"
	"strconv"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/ports"
)

// FakeStacks is an in-memory ports.Stacks.
type FakeStacks struct {
	// Unavailable answers ErrStacksUnavailable, as GHES does.
	Unavailable bool
	// Stacks are the stacks of the repository; CreateStack numbers them 1, 2…
	Stacks []ports.Stack
	// Err fails the writes (CreateStack, AddToStack).
	Err error
	// PullRequests answer PullRequest by number.
	PullRequests map[int]ports.StackPullRequest
	// Merges are the statuses MergeResult returns, one per poll; the last one
	// repeats. MergeAsync answers pending with uuid "u-1".
	Merges []ports.MergeStatus
	// Calls records every write, in order.
	Calls []string
	polls int
}

var _ ports.Stacks = (*FakeStacks)(nil)

// StacksOf implements ports.Stacks.
func (f *FakeStacks) StacksOf(_ context.Context, _ forge.Repository, number int) ([]ports.Stack, error) {
	if f.Unavailable {
		return nil, ports.ErrStacksUnavailable
	}
	if number == 0 {
		return f.Stacks, nil
	}
	var found []ports.Stack
	for _, s := range f.Stacks {
		if slices.ContainsFunc(s.PullRequests, func(p ports.StackPullRequest) bool { return p.Number == number }) {
			found = append(found, s)
		}
	}
	return found, nil
}

func pullRequestsOf(numbers []int) []ports.StackPullRequest {
	prs := make([]ports.StackPullRequest, len(numbers))
	for i, n := range numbers {
		prs[i] = ports.StackPullRequest{Number: n, State: "open"}
	}
	return prs
}

// CreateStack implements ports.Stacks.
func (f *FakeStacks) CreateStack(_ context.Context, _ forge.Repository, numbers []int) (ports.Stack, error) {
	f.Calls = append(f.Calls, "create "+join(numbers))
	if f.Unavailable {
		return ports.Stack{}, ports.ErrStacksUnavailable
	}
	if f.Err != nil {
		return ports.Stack{}, f.Err
	}
	s := ports.Stack{Number: len(f.Stacks) + 1, Open: true, Base: "main", PullRequests: pullRequestsOf(numbers)}
	f.Stacks = append(f.Stacks, s)
	return s, nil
}

// AddToStack implements ports.Stacks.
func (f *FakeStacks) AddToStack(_ context.Context, _ forge.Repository, number int, numbers []int) (ports.Stack, error) {
	f.Calls = append(f.Calls, "add "+strconv.Itoa(number)+" "+join(numbers))
	if f.Err != nil {
		return ports.Stack{}, f.Err
	}
	for i := range f.Stacks {
		if f.Stacks[i].Number == number {
			f.Stacks[i].PullRequests = append(f.Stacks[i].PullRequests, pullRequestsOf(numbers)...)
			return f.Stacks[i], nil
		}
	}
	return ports.Stack{}, ports.ErrNotFound
}

// Unstack implements ports.Stacks.
func (f *FakeStacks) Unstack(_ context.Context, _ forge.Repository, number int) (bool, error) {
	f.Calls = append(f.Calls, "unstack "+strconv.Itoa(number))
	f.Stacks = slices.DeleteFunc(f.Stacks, func(s ports.Stack) bool { return s.Number == number })
	return true, nil
}

// PullRequest implements ports.Stacks.
func (f *FakeStacks) PullRequest(_ context.Context, _ forge.Repository, number int) (ports.StackPullRequest, error) {
	if pr, ok := f.PullRequests[number]; ok {
		return pr, nil
	}
	return ports.StackPullRequest{}, ports.ErrNotFound
}

// SetPullRequestBase implements ports.Stacks.
func (f *FakeStacks) SetPullRequestBase(_ context.Context, _ forge.Repository, number int, base string) error {
	f.Calls = append(f.Calls, "retarget "+strconv.Itoa(number)+" "+base)
	return nil
}

// SetPullRequestBody implements ports.Stacks: PullRequests[number] gets it.
func (f *FakeStacks) SetPullRequestBody(_ context.Context, _ forge.Repository, number int, body string) error {
	f.Calls = append(f.Calls, "body "+strconv.Itoa(number))
	if f.PullRequests == nil {
		f.PullRequests = map[int]ports.StackPullRequest{}
	}
	pr := f.PullRequests[number]
	pr.Number, pr.Body = number, body
	f.PullRequests[number] = pr
	return nil
}

// MergeAsync implements ports.Stacks.
func (f *FakeStacks) MergeAsync(_ context.Context, _ forge.Repository, number int, sha string) (ports.MergeStatus, error) {
	f.Calls = append(f.Calls, "merge "+strconv.Itoa(number)+" at "+sha)
	return ports.MergeStatus{Status: "pending", UUID: "u-1"}, nil
}

// MergeResult implements ports.Stacks.
func (f *FakeStacks) MergeResult(context.Context, forge.Repository, int, string) (ports.MergeStatus, error) {
	if len(f.Merges) == 0 {
		return ports.MergeStatus{Status: "merged", SHA: "m0"}, nil
	}
	s := f.Merges[min(f.polls, len(f.Merges)-1)]
	f.polls++
	return s, nil
}

func join(numbers []int) string {
	s := ""
	for i, n := range numbers {
		if i > 0 {
			s += ","
		}
		s += strconv.Itoa(n)
	}
	return s
}

// GuardStacks fails t on every write of s.
func GuardStacks(t testing.TB, s ports.Stacks) ports.Stacks { return guardStacks{Stacks: s, t: t} }

type guardStacks struct {
	ports.Stacks
	t testing.TB
}

func (g guardStacks) CreateStack(context.Context, forge.Repository, []int) (ports.Stack, error) {
	wrote(g.t, "CreateStack")
	return ports.Stack{}, nil
}
func (g guardStacks) AddToStack(context.Context, forge.Repository, int, []int) (ports.Stack, error) {
	wrote(g.t, "AddToStack")
	return ports.Stack{}, nil
}
func (g guardStacks) Unstack(context.Context, forge.Repository, int) (bool, error) {
	wrote(g.t, "Unstack")
	return false, nil
}
func (g guardStacks) SetPullRequestBase(context.Context, forge.Repository, int, string) error {
	wrote(g.t, "SetPullRequestBase")
	return nil
}
func (g guardStacks) SetPullRequestBody(context.Context, forge.Repository, int, string) error {
	wrote(g.t, "SetPullRequestBody")
	return nil
}
func (g guardStacks) MergeAsync(context.Context, forge.Repository, int, string) (ports.MergeStatus, error) {
	wrote(g.t, "MergeAsync")
	return ports.MergeStatus{}, nil
}
