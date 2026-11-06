// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"strconv"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/ports"
)

// dryStacks forwards the reads of ports.Stacks and plans its writes.
type dryStacks struct {
	stacks ports.Stacks
	r      *rehearsal
}

var _ ports.Stacks = (*dryStacks)(nil)

func numbersOf(numbers []int) string {
	parts := make([]string, len(numbers))
	for i, n := range numbers {
		if n == 0 {
			parts[i] = "#?" // a pull request the dry run did not open
			continue
		}
		parts[i] = "#" + strconv.Itoa(n)
	}
	return strings.Join(parts, ", ")
}

func (d *dryStacks) StacksOf(ctx context.Context, repo forge.Repository, number int) ([]ports.Stack, error) {
	return d.stacks.StacksOf(ctx, repo, number)
}

// CreateStack returns a stack without a number: the real run would get one.
func (d *dryStacks) CreateStack(_ context.Context, _ forge.Repository, numbers []int) (ports.Stack, error) {
	d.r.plan(true, "app.dryrun.create_stack", "pull_requests", numbersOf(numbers))
	return ports.Stack{Open: true}, nil
}
func (d *dryStacks) AddToStack(_ context.Context, _ forge.Repository, number int, numbers []int) (ports.Stack, error) {
	d.r.plan(true, "app.dryrun.add_to_stack", "number", strconv.Itoa(number), "pull_requests", numbersOf(numbers))
	return ports.Stack{Number: number, Open: true}, nil
}
func (d *dryStacks) Unstack(_ context.Context, _ forge.Repository, number int) (bool, error) {
	d.r.plan(true, "app.dryrun.unstack", "number", strconv.Itoa(number))
	return true, nil
}
func (d *dryStacks) PullRequest(ctx context.Context, repo forge.Repository, number int) (ports.StackPullRequest, error) {
	return d.stacks.PullRequest(ctx, repo, number)
}
func (d *dryStacks) SetPullRequestBase(_ context.Context, _ forge.Repository, number int, base string) error {
	d.r.plan(true, "app.dryrun.retarget", "number", strconv.Itoa(number), "base", base)
	return nil
}

func (d *dryStacks) SetPullRequestBody(_ context.Context, _ forge.Repository, number int, _ string) error {
	d.r.plan(true, "app.dryrun.chain_note", "number", strconv.Itoa(number))
	return nil
}

// MergeAsync plans the merge; the result says nothing was asked.
func (d *dryStacks) MergeAsync(_ context.Context, _ forge.Repository, number int, _ string) (ports.MergeStatus, error) {
	d.r.plan(true, "app.dryrun.merge", "number", strconv.Itoa(number))
	return ports.MergeStatus{Status: "pending"}, nil
}
func (d *dryStacks) MergeResult(ctx context.Context, repo forge.Repository, number int, uuid string) (ports.MergeStatus, error) {
	return d.stacks.MergeResult(ctx, repo, number, uuid)
}
