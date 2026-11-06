// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/ports"
	"github.com/GregoireF/ship-it/internal/testkit"
)

var acmeShop = &ports.ParentRepository{
	Repository:    forge.Repository{Host: "github.com", Owner: "acme", Name: "shop"},
	DefaultBranch: "main", CloneURL: "https://github.com/acme/shop.git", SSHURL: "git@github.com:acme/shop.git",
}

// forkGit is ada's fork of acme/shop, cloned over SSH, without an upstream
// remote.
func forkGit(calls *[]string) testkit.FakeGit {
	git := publishableGit(calls)
	git.RemoteURLs = map[string]string{"origin": "git@github.com:ada/shop.git"}
	git.Refs = map[string]string{"refs/ship/parent/main": "p1"}
	return git
}

func TestPublishFromAForkFetchesTheParentByURL(t *testing.T) {
	var calls []string
	forge := &testkit.FakeForge{Parent: acmeShop, User: ports.User{Login: "ada"}}
	ws, log := publishWorkspace(forkGit(&calls), forge)
	result, err := Publish(context.Background(), ws, PublishRequest{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"fetch origin", "fetch-url git@github.com:acme/shop.git main into refs/ship/parent/main", "push origin feat/42-add-vat"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %q", calls)
	}
	spec := forge.Created[0]
	if forge.CreatedOn[0] != "acme/shop" || spec.Head != "ada:feat/42-add-vat" || spec.Base != "main" || !strings.HasPrefix(spec.Body, "Closes #42") {
		t.Errorf("pull request on %s: %+v", forge.CreatedOn[0], spec)
	}
	if result.Repository.Slug() != "ada/shop" || result.PullRequestRepository == nil || result.PullRequestRepository.Slug() != "acme/shop" {
		t.Errorf("result = %+v", result)
	}
	if op := log.Operations[0]; op.PullRequestRepository != "acme/shop" || op.Remote != "origin" {
		t.Errorf("operation = %+v", op)
	}

	// Undo closes the pull request on the parent, deletes the branch on the fork.
	calls = nil
	undone, err := Undo(context.Background(), ws, UndoRequest{AllowRemote: true})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(forge.ClosedOn, []string{"acme/shop"}) || undone.ClosedPullRequest != 1 ||
		!slices.ContainsFunc(calls, func(c string) bool { return strings.HasPrefix(c, "delete-remote origin feat/42-add-vat ") }) {
		t.Errorf("undo: closed on %v, calls %q", forge.ClosedOn, calls)
	}
}

func TestPublishFromAForkWithAnUpstreamRemote(t *testing.T) {
	var calls []string
	git := forkGit(&calls)
	git.RemoteURLs["upstream"] = "https://github.com/acme/shop.git"
	git.RemoteBranches = map[string]bool{"upstream/main": true}
	git.Comparisons = map[string][2]int{"upstream/main": {1, 2}}
	git.Rebased = "c1r"
	forge := &testkit.FakeForge{Parent: acmeShop}
	ws, _ := publishWorkspace(git, forge)
	if _, err := Publish(context.Background(), ws, PublishRequest{}); err != nil {
		t.Fatal(err)
	}
	want := []string{"fetch origin", "fetch upstream", "rebase upstream/main", "push origin feat/42-add-vat"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %q", calls)
	}
}

// An upstream remote alone makes a fork, for a repository GitHub does not
// report as one.
func TestAnUpstreamRemoteNamesTheParent(t *testing.T) {
	git := forkGit(nil)
	git.RemoteURLs["upstream"] = "git@github.com:acme/shop.git"
	forge := &testkit.FakeForge{DefaultBranches: map[string]string{"acme/shop": "trunk"}}
	ws, _ := publishWorkspace(git, forge)
	if _, err := Publish(context.Background(), ws, PublishRequest{}); err != nil {
		t.Fatal(err)
	}
	if forge.CreatedOn[0] != "acme/shop" || forge.Created[0].Base != "trunk" || forge.Created[0].Head != "ada:feat/42-add-vat" {
		t.Errorf("pull request on %v: %+v", forge.CreatedOn, forge.Created[0])
	}
}

func TestPullRequestBaseSetting(t *testing.T) {
	var calls []string
	git := publishableGit(&calls)
	git.RemoteBranches = map[string]bool{"origin/develop": true}
	git.Comparisons = map[string][2]int{"origin/develop": {1, 1}}
	forge := &testkit.FakeForge{}
	ws, _ := publishWorkspace(git, forge)
	ws.Config = &testkit.FakeConfigFiles{Repository: `{"ship": {"pullRequest": {"base": "develop"}}}`}
	result, err := Publish(context.Background(), ws, PublishRequest{})
	if err != nil {
		t.Fatal(err)
	}
	spec := forge.Created[0]
	// Closing keywords only work on the default branch: the issue is linked.
	if spec.Base != "develop" || result.Base != "develop" || !strings.HasPrefix(spec.Body, "Refs #42") {
		t.Errorf("spec = %+v", spec)
	}
	if !slices.Contains(calls, "rebase origin/develop") {
		t.Errorf("calls = %q", calls)
	}
}

func TestTidyInAFork(t *testing.T) {
	var calls []string
	git, forge := afterMerge(&calls)
	git.RemoteURLs = map[string]string{"origin": "https://github.com/ada/shop.git", "upstream": "https://github.com/acme/shop.git"}
	git.RemoteBranches = map[string]bool{"upstream/main": true}
	forge.Parent = acmeShop
	forge.Latest = map[string]ports.PullRequest{"ada:feat/vat": forge.Latest["feat/vat"]}
	ws, _, _ := workspaceOf(git)
	ws.Forge = forge
	result, err := Tidy(context.Background(), ws)
	if err != nil {
		t.Fatal(err)
	}
	if result.LeftBranch != "feat/vat" || !slices.Contains(calls, "fetch upstream") || !slices.Contains(calls, "fast-forward upstream/main") {
		t.Errorf("result = %+v, calls %q", result, calls)
	}
}

func TestSignoffCheckOfAForkStartsAtTheParent(t *testing.T) {
	var asked string
	git := forkGit(nil)
	git.DefaultBranch = "main"
	git.RangeAsked = &asked
	ws, _ := publishWorkspace(git, &testkit.FakeForge{})
	if _, err := CheckSignoffs(context.Background(), ws, "", ""); err != nil || asked != "origin/main..HEAD" {
		t.Fatalf("without upstream: range %q, %v", asked, err)
	}
	git.RemoteURLs["upstream"] = "https://github.com/acme/shop.git"
	ws.Git = git
	if _, err := CheckSignoffs(context.Background(), ws, "", ""); err != nil || asked != "upstream/main..HEAD" {
		t.Fatalf("with upstream: range %q, %v", asked, err)
	}
}
