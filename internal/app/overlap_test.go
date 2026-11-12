// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"reflect"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/ports"
	"github.com/GregoireF/ship-it/internal/testkit"
)

var shopRepo = forge.Repository{Host: "github.com", Owner: "acme", Name: "shop"}

func TestOpenOverlaps(t *testing.T) {
	git := testkit.FakeGit{BranchDiff: "diff --git a/shop.go b/shop.go\n--- a/shop.go\n+++ b/shop.go\n@@ -12,2 +12,3 @@\n-a\n+b\n" +
		"diff --git a/cart.go b/cart.go\n--- a/cart.go\n+++ b/cart.go\n@@ -40 +40 @@\n-x\n+y\n"}
	pr := func(n int, head string, files ...ports.FilePatch) ports.PullRequestPatches {
		return ports.PullRequestPatches{PullRequest: ports.PullRequest{Number: n, URL: "u"}, Head: head, Title: "t", Files: files}
	}
	forge := &testkit.FakeForge{OpenPatches: []ports.PullRequestPatches{
		pr(9, "feat/mine", ports.FilePatch{Path: "shop.go", Status: "modified", Patch: "@@ -12 +12 @@"}),
		pr(8, "feat/vat", ports.FilePatch{Path: "shop.go", Status: "modified", Patch: "@@ -13,4 +13,4 @@"}),
		pr(7, "feat/far", ports.FilePatch{Path: "shop.go", Status: "modified", Patch: "@@ -90 +90 @@"}),
		pr(6, "feat/logo", ports.FilePatch{Path: "cart.go", Status: "modified"}),
		pr(5, "feat/move", ports.FilePatch{Path: "basket.go", PreviousPath: "cart.go", Status: "renamed"}),
		pr(4, "feat/new", ports.FilePatch{Path: "shop.go", Status: "added", Patch: "@@ -0,0 +1 @@"}),
	}}
	ws := Workspace{Git: git, Forge: forge}
	got, err := OpenOverlaps(context.Background(), ws, shopRepo, "origin/main", "main", "feat/mine")
	if err != nil {
		t.Fatal(err)
	}
	want := []OverlapView{
		{Number: 8, Title: "t", URL: "u", Files: []string{"shop.go"}},
		{Number: 6, Title: "t", URL: "u", Files: []string{"cart.go"}},
		{Number: 5, Title: "t", URL: "u", Files: []string{"cart.go"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("OpenOverlaps =\n%+v\nwant\n%+v\n(its own pull request, far lines and new files aside; no patch or a rename: the whole file)", got, want)
	}

	// Only new files on the branch: nothing to compare, GitHub is not asked.
	ws.Git = testkit.FakeGit{BranchDiff: "diff --git a/vat.go b/vat.go\nnew file mode 100644\n@@ -0,0 +1 @@\n"}
	ws.Forge = &testkit.FakeForge{Err: context.Canceled}
	if got, err := OpenOverlaps(context.Background(), ws, shopRepo, "origin/main", "main", "feat/mine"); err != nil || got != nil {
		t.Fatalf("only new files: %v, %v", got, err)
	}
}
