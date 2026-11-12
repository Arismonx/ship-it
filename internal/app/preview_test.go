// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/pullrequest"
	"github.com/GregoireF/ship-it/internal/ports"
	"github.com/GregoireF/ship-it/internal/testkit"
)

// TestPreviewShowsThePullRequestAndSendsNothing: #44. The preview is the
// publication rehearsed: the same title, body and labels, nothing pushed or
// opened.
func TestPreviewShowsThePullRequestAndSendsNothing(t *testing.T) {
	var calls []string
	git := publishableGit(&calls)
	git.CommitMessages = []string{"✨ feat: add vat\n\nThe EU changed its VAT rules."}
	forge := &testkit.FakeForge{}
	ws, _ := publishWorkspace(git, forge)
	view, err := Preview(context.Background(), ws, PublishRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(forge.Created) != 0 || slices.ContainsFunc(calls, func(c string) bool { return strings.HasPrefix(c, "push") }) {
		t.Fatalf("a preview sent something: created %d, git calls %v", len(forge.Created), calls)
	}
	if view.Title != "✨ feat: add vat" || !strings.Contains(view.Body, "The EU changed its VAT rules.") ||
		!strings.HasSuffix(strings.TrimSpace(view.Body), pullrequest.Marker) || view.Head != "feat/42-add-vat" || view.Existing != nil {
		t.Fatalf("preview = %+v", view)
	}

	// The same as the publication would open.
	published := &testkit.FakeForge{}
	ws, _ = publishWorkspace(publishableGit(nil), published)
	ws.Git = git
	if _, err := Publish(context.Background(), ws, PublishRequest{}); err != nil {
		t.Fatal(err)
	}
	if spec := published.Created[0]; spec.Title != view.Title || spec.Body != view.Body || spec.Base != view.Base {
		t.Fatalf("preview %q/%q differs from the publication %q/%q", view.Title, view.Base, spec.Title, spec.Base)
	}
}

// TestPreviewScoresTheRisk: #45. Code without a test and a migration give
// four points, each with its reason.
func TestPreviewScoresTheRisk(t *testing.T) {
	git := publishableGit(nil)
	git.BranchDiff = "diff --git a/vat.go b/vat.go\n--- a/vat.go\n+++ b/vat.go\n@@ -1 +1 @@\n-a\n+b\n" +
		"diff --git a/db/migrations/002_vat.sql b/db/migrations/002_vat.sql\n--- a/db/migrations/002_vat.sql\n+++ b/db/migrations/002_vat.sql\n@@ -1 +1 @@\n-a\n+b\n"
	git.DiffStats = []ports.FileStat{{Path: "vat.go", Added: 1, Deleted: 1}, {Path: "db/migrations/002_vat.sql", Added: 1, Deleted: 1}}
	ws, _ := publishWorkspace(git, &testkit.FakeForge{})
	view, err := Preview(context.Background(), ws, PublishRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if view.Risk == nil || view.Risk.Level != "high" || view.Risk.Points != 4 || len(view.Risk.Factors) != 2 ||
		view.Risk.Factors[0].Key != "risk.untested" || view.Risk.Factors[1].Params["files"] != "db/migrations/002_vat.sql" {
		t.Fatalf("risk = %+v", view.Risk)
	}
}

func TestPreviewOfABranchWithAnOpenPullRequest(t *testing.T) {
	pr := ports.PullRequest{Number: 7, URL: "https://github.com/acme/shop/pull/7"}
	forge := &testkit.FakeForge{Open: map[string]ports.PullRequest{"feat/42-add-vat": pr}}
	ws, _ := publishWorkspace(publishableGit(nil), forge)
	view, err := Preview(context.Background(), ws, PublishRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if view.Existing == nil || view.Existing.Number != 7 || view.Title != "" {
		t.Fatalf("preview = %+v, want the open pull request #7 and no new title", view)
	}
}
