// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"slices"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/snapshot"
	"github.com/GregoireF/ship-it/internal/core/split"
	"github.com/GregoireF/ship-it/internal/ports"
)

// PlanReads is the repository as a plan read it: the status, the work in
// progress, the remote's default branch and the recent history. A delivery
// (then a publication) that follows the plan at once reuses them instead of
// asking git again: each read is a git process, about 20 ms on Windows.
//
// They go stale as soon as anything can change the files: after a human
// review, the delivery must read them again (zero value) and scan what it
// commits. Use cases run one after another: a PlanReads is not shared by
// concurrent deliveries.
type PlanReads struct{ r *repositoryReads }

// Reads returns what the plan read, for the delivery that follows at once.
func (v PlanView) Reads() PlanReads { return PlanReads{r: v.reads} }

// repositoryReads reads each fact once, when first needed, and keeps it.
type repositoryReads struct {
	status ports.WorkingTreeStatus

	workRead bool
	work     []snapshot.FileDiff // every change, before the --only selection
	workErr  error

	trunkRead bool
	trunk     string

	subjectsRead bool
	subjects     []string
	subjectsErr  error
}

// readStatus starts the reads of a use case with `git status`.
func readStatus(ctx context.Context, git ports.Git) (*repositoryReads, error) {
	status, err := git.Status(ctx)
	if err != nil {
		return nil, err
	}
	return &repositoryReads{status: status}, nil
}

// selectedWork is the work in progress restricted to the --only selection
// (see selectedWorkIn); the diff is read once whatever the selection.
func (r *repositoryReads) selectedWork(ctx context.Context, git ports.Git, only []string) ([]snapshot.FileDiff, error) {
	if !r.workRead {
		r.work, r.workErr = workInProgress(ctx, git, r.status)
		r.workRead = true
	}
	if r.workErr != nil || len(only) == 0 {
		return r.work, r.workErr
	}
	// A copy: the full list stays for another selection.
	selected := slices.DeleteFunc(slices.Clone(r.work), func(d snapshot.FileDiff) bool { return !split.Matches(d.Path, only) })
	if len(selected) == 0 {
		return nil, fault.New(fault.NothingToShip, "plan.nothing_selected", "only", strings.Join(only, " "))
	}
	return selected, nil
}

// defaultBranch is defaultBranchOf, read once.
func (r *repositoryReads) defaultBranch(ctx context.Context, ws Workspace) string {
	if !r.trunkRead {
		r.trunk, r.trunkRead = defaultBranchOf(ctx, ws), true
	}
	return r.trunk
}

// recentSubjects are the subjects of the last commits, read once: the habit
// of commitPlacement.
func (r *repositoryReads) recentSubjects(ctx context.Context, ws Workspace) ([]string, error) {
	if !r.subjectsRead {
		r.subjects, r.subjectsErr = ws.Git.RecentSubjects(ctx, habitDepth)
		r.subjectsRead = true
	}
	return r.subjects, r.subjectsErr
}

// defaultBranch is the plan's read of the remote's default branch when there
// is one, else a new read.
func (p PlanReads) defaultBranch(ctx context.Context, ws Workspace) string {
	if p.r == nil {
		return defaultBranchOf(ctx, ws)
	}
	return p.r.defaultBranch(ctx, ws)
}
