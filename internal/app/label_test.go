// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"reflect"
	"testing"

	"github.com/GregoireF/ship-it/internal/ports"
	"github.com/GregoireF/ship-it/internal/testkit"
)

func labelForge(current ...string) *testkit.FakeForge {
	return &testkit.FakeForge{
		RepositoryLabels: testkit.LabelNames("kind/feature", "kind/bug", "size/XS", "size/S", "size/XXL"),
		PullRequests: map[int]ports.PullRequestContent{12: {
			PullRequest:    ports.PullRequest{Number: 12, Labels: current},
			CommitMessages: []string{"✨ feat: add vat", "🐛 fix: round vat\n\nSigned-off-by: A <a@x.org>"},
			Files: []ports.FileStat{
				{Path: "billing/vat.go", Added: 20, Deleted: 5},
				{Path: "go.sum", Added: 400},              // lockfile: not counted
				{Path: "api/generated.pb.go", Added: 900}, // linguist-generated: not counted
			},
		}},
	}
}

func TestLabelPullRequestFirstTime(t *testing.T) {
	git := testkit.HealthyRepository("main")
	git.Generated = map[string]bool{"api/generated.pb.go": true}
	forge := labelForge("help wanted")
	ws, _ := publishWorkspace(git, forge)
	result, err := LabelPullRequest(context.Background(), ws, 12)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"kind/feature", "kind/bug", "size/S"}
	if !reflect.DeepEqual(result.Added, want) || !reflect.DeepEqual(forge.Added[12], want) || len(result.Removed) != 0 {
		t.Fatalf("result = %+v, added %v", result, forge.Added[12])
	}
}

func TestLabelPullRequestRefreshesTheSizeOnly(t *testing.T) {
	git := testkit.HealthyRepository("main")
	git.Generated = map[string]bool{"api/generated.pb.go": true}
	// A reviewer removed kind/bug: it is not put back.
	forge := labelForge("kind/feature", "size/XS")
	ws, _ := publishWorkspace(git, forge)
	result, err := LabelPullRequest(context.Background(), ws, 12)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Added, []string{"size/S"}) || !reflect.DeepEqual(result.Removed, []string{"size/XS"}) {
		t.Fatalf("result = %+v", result)
	}
	if !reflect.DeepEqual(forge.LabelCalls, []string{"remove size/XS from #12"}) {
		t.Errorf("calls = %q", forge.LabelCalls)
	}
	// Up to date: nothing changes.
	forge = labelForge("kind/feature", "size/S")
	ws, _ = publishWorkspace(git, forge)
	if result, err := LabelPullRequest(context.Background(), ws, 12); err != nil || len(result.Added)+len(result.Removed) != 0 {
		t.Fatalf("up to date: %+v, %v", result, err)
	}
}

func TestLabelPullRequestUnknown(t *testing.T) {
	ws, _ := publishWorkspace(testkit.HealthyRepository("main"), labelForge())
	if _, err := LabelPullRequest(context.Background(), ws, 99); err == nil {
		t.Fatal("an unknown pull request must fail")
	}
}
