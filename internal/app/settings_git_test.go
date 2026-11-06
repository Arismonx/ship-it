// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/testkit"
)

func TestDeliverSignsOffAndSigns(t *testing.T) {
	signed := "✨ feat: x\n\nSigned-off-by: Ada <ada@example.invalid>"
	tests := []struct {
		name     string
		config   string
		change   func(*testkit.FakeGit)
		wantNote string
	}{
		{"defaults, no sign of a DCO", "", nil, ""},
		{"signoff always", `{"ship": {"commits": {"signoff": "always"}}}`, nil, " [signoff]"},
		{"signoff auto, history signed off", "", func(g *testkit.FakeGit) { g.Messages = []string{signed, signed, "🐛 fix: y"} }, " [signoff]"},
		{"signoff auto, contributing guide", "", func(g *testkit.FakeGit) {
			g.WorkingFiles = map[string]string{".github/CONTRIBUTING.md": "Sign off your commits (DCO)."}
		}, " [signoff]"},
		{"signoff auto, DCO app", "", func(g *testkit.FakeGit) { g.WorkingFiles = map[string]string{".github/dco.yml": "require:\n"} }, " [signoff]"},
		{"signoff never, history signed off", `{"ship": {"commits": {"signoff": "never"}}}`, func(g *testkit.FakeGit) { g.Messages = []string{signed} }, ""},
		{"sign never", `{"ship": {"commits": {"sign": "never"}}}`, nil, " [sign never]"},
		{"sign always, with a key", `{"ship": {"commits": {"sign": "always"}}}`, func(g *testkit.FakeGit) { g.ConfigValues["user.signingkey"] = "ABCD" }, " [sign always]"},
		{"sign always, SSH key command", `{"ship": {"commits": {"sign": "always"}}}`, func(g *testkit.FakeGit) {
			g.ConfigValues["gpg.format"], g.ConfigValues["gpg.ssh.defaultkeycommand"] = "ssh", "ssh-add -L"
		}, " [sign always]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			git := deliverableGit("fix/crash", &calls)
			if tt.change != nil {
				tt.change(&git)
			}
			ws, _, _ := workspaceOf(git)
			ws.Config = &testkit.FakeConfigFiles{Repository: tt.config}
			if _, err := DeliverLocally(context.Background(), ws, DeliveryRequest{Title: "🐛 fix: handle nil config"}); err != nil {
				t.Fatal(err)
			}
			if want := "commit 🐛 fix: handle nil config\n" + tt.wantNote; calls[len(calls)-1] != want {
				t.Errorf("commit = %q, want %q", calls[len(calls)-1], want)
			}
		})
	}
}

func TestSignAlwaysWithoutAKeyChangesNothing(t *testing.T) {
	var calls []string
	ws, store, _ := workspaceOf(deliverableGit("fix/crash", &calls))
	ws.Config = &testkit.FakeConfigFiles{Repository: `{"ship": {"commits": {"sign": "always"}}}`}
	_, err := DeliverLocally(context.Background(), ws, DeliveryRequest{Title: "🐛 fix: handle nil config"})
	if !errors.Is(err, &fault.Fault{Code: fault.SigningKeyMissing}) || len(calls) != 0 || len(store.Checkpoints) != 0 {
		t.Fatalf("error = %v, calls %q, checkpoints %d", err, calls, len(store.Checkpoints))
	}
}

func behindTheBase(calls *[]string) testkit.FakeGit {
	git := publishableGit(calls)
	git.RemoteBranches = map[string]bool{"origin/main": true}
	git.Comparisons = map[string][2]int{"origin/main": {1, 3}}
	git.WorkingTree.Branch.Commit = "c1"
	git.Rebased = "c1m"
	git.Refs = map[string]string{"refs/remotes/origin/feat/42-add-vat": "c1"} // pushed before
	return git
}

func TestPublishUpdateMerge(t *testing.T) {
	var calls []string
	ws, log := publishWorkspace(behindTheBase(&calls), &testkit.FakeForge{})
	ws.Config = &testkit.FakeConfigFiles{Repository: `{"ship": {"publish": {"update": "merge"}, "commits": {"sign": "never"}}}`}
	result, err := Publish(context.Background(), ws, PublishRequest{})
	if err != nil || !result.Merged || result.Rebased {
		t.Fatalf("result = %+v, %v", result, err)
	}
	// No rewrite: a normal push, without a lease.
	want := []string{"fetch origin", "merge origin/main [sign never]", "push origin feat/42-add-vat"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %q", calls)
	}
	if log.Operations[1].Kind != oplog.Merge || log.Operations[1].Title != "origin/main" || log.Operations[0].Commit != "c1m" {
		t.Errorf("operations = %+v", log.Operations)
	}
}

func TestPublishUpdateMergeConflict(t *testing.T) {
	var calls []string
	git := behindTheBase(&calls)
	git.MergeConflicts = true
	ws, _ := publishWorkspace(git, &testkit.FakeForge{})
	ws.Config = &testkit.FakeConfigFiles{Repository: `{"ship": {"publish": {"update": "merge"}}}`}
	result, err := Publish(context.Background(), ws, PublishRequest{})
	if err != nil || result.Merged || len(result.Warnings) != 1 || result.Warnings[0].Code != string(fault.MergeConflict) {
		t.Fatalf("result = %+v, %v", result, err)
	}
	if calls[len(calls)-1] != "push origin feat/42-add-vat" {
		t.Errorf("published as it is: calls %q", calls)
	}
}

func TestPublishUpdateNone(t *testing.T) {
	var calls []string
	ws, _ := publishWorkspace(behindTheBase(&calls), &testkit.FakeForge{})
	ws.Config = &testkit.FakeConfigFiles{Repository: `{"ship": {"publish": {"update": "none"}}}`}
	result, err := Publish(context.Background(), ws, PublishRequest{})
	if err != nil || result.Merged || result.Rebased {
		t.Fatalf("result = %+v, %v", result, err)
	}
	if !reflect.DeepEqual(calls, []string{"fetch origin", "push origin feat/42-add-vat"}) {
		t.Fatalf("calls = %q", calls)
	}
}

func TestPublishRebaseSignsAsConfigured(t *testing.T) {
	var calls []string
	git := behindTheBase(&calls)
	git.ConfigValues["user.signingkey"] = "ABCD"
	ws, _ := publishWorkspace(git, &testkit.FakeForge{})
	ws.Config = &testkit.FakeConfigFiles{Repository: `{"ship": {"commits": {"sign": "always"}}}`}
	if _, err := Publish(context.Background(), ws, PublishRequest{}); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(calls, "rebase-sign always") {
		t.Errorf("calls = %q", calls)
	}
}

func TestMergeIsRehearsed(t *testing.T) {
	git := behindTheBase(nil)
	ws, _ := publishWorkspace(git, &testkit.FakeForge{})
	ws.Git = testkit.GuardGit(t, git)
	ws.Forge = testkit.GuardForge(t, ws.Forge)
	ws.Config = &testkit.FakeConfigFiles{Repository: `{"ship": {"publish": {"update": "merge"}}}`}
	result, err := Publish(context.Background(), DryRun(ws), PublishRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(result.Planned, func(s PlannedStep) bool { return s.Key == "app.dryrun.merge" && s.Params["ref"] == "origin/main" }) {
		t.Errorf("planned = %+v", result.Planned)
	}
}
