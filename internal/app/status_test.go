// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/core/review"
	"github.com/GregoireF/ship-it/internal/ports"
	"github.com/GregoireF/ship-it/internal/testkit"
)

func featureBranchGit() testkit.FakeGit {
	git := testkit.HealthyRepository("feat/42-add-vat")
	git.WorkingTree.Branch.Upstream = "origin/feat/42-add-vat"
	git.DefaultBranch = "main"
	git.RemoteBranches = map[string]bool{"origin/main": true}
	git.Comparisons = map[string][2]int{"origin/main": {2, 5}}
	return git
}

func TestStatusOfAPullRequestInReview(t *testing.T) {
	pr := ports.PullRequest{Number: 7, URL: "https://github.com/acme/shop/pull/7"}
	ws, _, log := workspaceOf(featureBranchGit())
	ws.Forge = &testkit.FakeForge{Open: map[string]ports.PullRequest{"feat/42-add-vat": pr}}
	log.Operations = []oplog.Operation{{ID: "op1", Kind: oplog.Publish, Title: "✨ feat: add vat"}}

	view, err := Status(context.Background(), ws, StatusRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if view.GitHub != "known" || view.PullRequest == nil || view.PullRequest.Number != 7 {
		t.Fatalf("view = %+v", view)
	}
	if view.Base != "main" || view.AheadOfBase != 2 || view.BehindBase != 5 {
		t.Errorf("comparison with the base = %+v", view)
	}
	if view.Next.Key != "status.next.await_review" || view.Undoable == nil || view.Undoable.Kind != "publish" {
		t.Errorf("next = %+v, undoable = %+v", view.Next, view.Undoable)
	}
}

func TestStatusOfflineAndWithoutToken(t *testing.T) {
	ws, _, _ := workspaceOf(featureBranchGit())
	ws.Forge = &testkit.FakeForge{Err: ports.ErrNotAuthenticated}

	offline, _ := Status(context.Background(), ws, StatusRequest{Offline: true})
	if offline.GitHub != "unknown" || offline.GitHubProblem != nil {
		t.Errorf("offline = %+v", offline)
	}
	noToken, _ := Status(context.Background(), ws, StatusRequest{})
	if noToken.GitHubProblem == nil || noToken.GitHubProblem.Code != "SHIP-E0603" {
		t.Errorf("no token = %+v", noToken.GitHubProblem)
	}
}

func TestStatusSuggestsShippingUncommittedWork(t *testing.T) {
	ws, _, _ := workspaceOf(testkit.HealthyRepository("main", "a.go", "b.go"))
	ws.Forge = &testkit.FakeForge{Err: ports.ErrNotAuthenticated} // never called on the trunk
	view, err := Status(context.Background(), ws, StatusRequest{})
	if err != nil || view.Next.Key != "status.next.ship" || view.Next.Params["count"] != "2" || view.GitHubProblem != nil {
		t.Fatalf("view = %+v, %v", view, err)
	}
}

func TestStatusReadsChecksAndReviews(t *testing.T) {
	git := testkit.HealthyRepository("feat/vat")
	git.WorkingTree.Branch.Upstream = "origin/feat/vat"
	git.DefaultBranch = "main"
	git.RemoteBranches = map[string]bool{"origin/main": true}
	git.Comparisons = map[string][2]int{"origin/main": {1, 0}}
	forge := &testkit.FakeForge{
		Open:       map[string]ports.PullRequest{"feat/vat": {Number: 7, URL: "https://github.com/acme/shop/pull/7", HeadSHA: "c0ffee"}},
		Runs:       []review.CheckRun{{Name: "test", Status: "completed", Conclusion: "failure"}},
		ReviewList: []review.Review{{Author: "ada", State: "APPROVED"}},
	}
	view, err := Status(context.Background(), Workspace{Git: git, Forge: forge}, StatusRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if view.CI == nil || view.CI.State != review.CIFailing || view.Review == nil || view.Review.Decision != review.Approved {
		t.Fatalf("view = %+v", view)
	}
	if view.Next.Key != "status.next.fix_ci" {
		t.Errorf("next = %s: a red check comes before the approval", view.Next.Key)
	}
}
