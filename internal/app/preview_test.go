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
