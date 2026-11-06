// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/ports"
	"github.com/GregoireF/ship-it/internal/testkit"
)

func TestProposePlanWithUntrackedFile(t *testing.T) {
	git := testkit.FakeGit{
		WorkingTree: ports.WorkingTreeStatus{
			Branch: ports.BranchState{Name: "main", Commit: "abc"},
			Files:  []ports.FileChange{{Path: "internal/core/release/notes.go", State: ports.Untracked}},
		},
		WorkingFiles: map[string]string{"internal/core/release/notes.go": "package release\n\nfunc Notes() {}\n"},
	}
	view, err := ProposePlan(context.Background(), Workspace{Git: git}, PlanRequest{Issue: 9})
	if err != nil {
		t.Fatal(err)
	}
	if view.Title != "✨ feat(release): add notes" || view.Branch != "feat/9-add-notes" {
		t.Fatalf("plan = %+v", view)
	}
	if !view.CreatesBranch || view.CurrentBranch != "main" {
		t.Errorf("from the trunk the delivery creates its branch: %+v", view)
	}
}

func TestProposePlanOnAFeatureBranchKeepsIt(t *testing.T) {
	git := testkit.FakeGit{
		WorkingTree: ports.WorkingTreeStatus{
			Branch: ports.BranchState{Name: "fix/crash", Commit: "abc"},
			Files:  []ports.FileChange{{Path: "a.go", State: ports.Untracked}},
		},
		WorkingFiles: map[string]string{"a.go": "package a\n"},
	}
	view, err := ProposePlan(context.Background(), Workspace{Git: git}, PlanRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if view.CreatesBranch || view.CurrentBranch != "fix/crash" {
		t.Fatalf("view = %+v", view)
	}
}

func TestProposePlanNothingToShip(t *testing.T) {
	_, err := ProposePlan(context.Background(), Workspace{Git: testkit.HealthyRepository("main")}, PlanRequest{})
	if !errors.Is(err, &fault.Fault{Code: fault.NothingToShip}) {
		t.Fatalf("error = %v", err)
	}
}

func TestProposePlanBeforeFirstCommit(t *testing.T) {
	git := testkit.FakeGit{
		WorkingTree: ports.WorkingTreeStatus{
			Branch: ports.BranchState{Name: "main"},
			Files:  []ports.FileChange{{Path: "README.md", State: ports.Tracked, Staged: ports.Added}},
		},
		WorkingFiles: map[string]string{"README.md": "# hello\n"},
	}
	view, err := ProposePlan(context.Background(), Workspace{Git: git}, PlanRequest{})
	if err != nil {
		t.Fatal(err)
	}
	// The first commit of a repository is its initial commit, on the trunk.
	if view.Title != "🎉 chore: initial commit" || view.Branch != "main" || view.Files[0] != "README.md" {
		t.Fatalf("view = %+v", view)
	}
}
