// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/pullrequest"
	"github.com/GregoireF/ship-it/internal/testkit"
)

func TestPullRequestDraftSetting(t *testing.T) {
	tests := []struct {
		name, setting     string
		unattended, agent bool
		want              bool
	}{
		{"unattended, reviewed", "unattended", false, false, false},
		{"unattended, --yes", "unattended", true, false, true},
		{"always", "always", false, false, true},
		{"never, --yes", "never", true, false, false},
		{"never, AI agent", "never", true, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			forge := &testkit.FakeForge{}
			ws, _ := publishWorkspace(publishableGit(nil), forge)
			ws.Agent = tt.agent
			ws.Config = &testkit.FakeConfigFiles{Repository: `{"ship": {"pullRequest": {"draft": "` + tt.setting + `"}}}`}
			if _, err := Publish(context.Background(), ws, PublishRequest{Unattended: tt.unattended}); err != nil {
				t.Fatal(err)
			}
			if got := forge.Created[0].Draft; got != tt.want {
				t.Errorf("draft = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPullRequestLabelPrefixesAndSizes(t *testing.T) {
	forge := &testkit.FakeForge{RepositoryLabels: testkit.LabelNames("type: feature", "kind/feature", "small", "size/M")}
	ws, _ := publishWorkspace(publishableGit(nil), forge)
	ws.Config = &testkit.FakeConfigFiles{Repository: `{"ship": {"pullRequest": {
		"labelPrefixes": {"kind": "type: ", "size": ""}, "sizes": {"small": 50, "big": 0}}}}`}
	result, err := Publish(context.Background(), ws, PublishRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Labels, []string{"type: feature", "small"}) {
		t.Errorf("labels = %v, want the configured prefixes and sizes", result.Labels)
	}
}

func TestPullRequestSections(t *testing.T) {
	git := publishableGit(nil)
	git.CommitMessages = []string{"✨ feat: add vat\n\nThe EU changed its VAT rules.\n\nTested: by hand"}
	git.BranchDiff = "diff --git a/vat.go b/vat.go\nnew file mode 100644\n--- /dev/null\n+++ b/vat.go\n@@ -0,0 +1 @@\n+func WithVAT() {}\n"
	forge := &testkit.FakeForge{}
	ws, _ := publishWorkspace(git, forge)
	ws.Config = &testkit.FakeConfigFiles{Repository: `{"ship": {"pullRequest": {"sections": {"why": false, "testing": false, "issue": false, "changes": false}}}}`}
	if _, err := Publish(context.Background(), ws, PublishRequest{}); err != nil {
		t.Fatal(err)
	}
	if body := forge.Created[0].Body; body != pullrequest.Marker+"\n" {
		t.Errorf("body = %q: every section is off", body)
	}

	// On by default: the facts of the branch's diff close the summary.
	forge = &testkit.FakeForge{}
	ws, _ = publishWorkspace(git, forge)
	if _, err := Publish(context.Background(), ws, PublishRequest{}); err != nil {
		t.Fatal(err)
	}
	if body := forge.Created[0].Body; !strings.Contains(body, "**Changes:** 1 file: 1 added (`vat.go`). Added `WithVAT`.") {
		t.Errorf("body = %q: want the summary of the diff", body)
	}
}

func TestPublishToTheConfiguredRemote(t *testing.T) {
	var calls []string
	git := publishableGit(&calls)
	git.RemoteNames = []string{"origin", "mine"}
	git.RemoteURLs = map[string]string{"origin": "git@github.com:acme/shop.git", "mine": "git@github.com:acme/shop.git"}
	forge := &testkit.FakeForge{}
	ws, log := publishWorkspace(git, forge)
	ws.Config = &testkit.FakeConfigFiles{Repository: `{"ship": {"git": {"remote": "mine"}}}`}
	if _, err := Publish(context.Background(), ws, PublishRequest{}); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(calls, "fetch mine") || !slices.Contains(calls, "push mine feat/42-add-vat") || slices.Contains(calls, "fetch origin") {
		t.Errorf("calls = %q", calls)
	}
	if op := log.Operations[0]; op.Remote != "mine" {
		t.Errorf("operation remote = %q", op.Remote)
	}
}

func TestTidySettings(t *testing.T) {
	t.Run("auto off", func(t *testing.T) {
		var calls []string
		git, forge := afterMerge(&calls)
		ws, _, _ := workspaceOf(git)
		ws.Forge = forge
		ws.Config = &testkit.FakeConfigFiles{Repository: `{"ship": {"tidy": {"auto": false}}}`}
		if _, ran, err := TidyBeforeShip(context.Background(), ws); ran || err != nil || len(calls) != 0 {
			t.Fatalf("ran = %v, %v, calls %q", ran, err, calls)
		}
		// ship tidy itself still runs.
		if _, err := Tidy(context.Background(), ws); err != nil || len(calls) == 0 {
			t.Fatalf("ship tidy: %v, calls %q", err, calls)
		}
	})
	t.Run("keep merged branches", func(t *testing.T) {
		var calls []string
		git, forge := afterMerge(&calls)
		ws, _, _ := workspaceOf(git)
		ws.Forge = forge
		ws.Config = &testkit.FakeConfigFiles{Repository: `{"ship": {"tidy": {"deleteMerged": false}}}`}
		result, ran, err := TidyBeforeShip(context.Background(), ws)
		if !ran || err != nil {
			t.Fatal(err)
		}
		if len(result.Deleted) != 0 || result.LeftBranch != "feat/vat" || !reflect.DeepEqual(result.Kept, []string{"feat/vat"}) {
			t.Errorf("result = %+v", result)
		}
		for _, c := range calls {
			if strings.HasPrefix(c, "delete ") {
				t.Errorf("deleted a branch: %q", calls)
			}
		}
	})
}
