// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/core/rules"
	"github.com/GregoireF/ship-it/internal/ports"
	"github.com/GregoireF/ship-it/internal/testkit"
)

func namePattern(op rules.Operator, pattern string) rules.Rule {
	return rules.Rule{Type: rules.BranchNamePattern, Pattern: &rules.Pattern{Name: "Team prefix", Operator: op, Pattern: pattern}}
}

// deliveredByShip records that ship-it created the branch, as a delivery from
// main does.
func deliveredByShip(log *testkit.FakeOperationLog, branch string) {
	log.Operations = append(log.Operations, oplog.Operation{ID: "op0", Kind: oplog.Deliver, CreatedBranch: true,
		Before: oplog.RepositoryView{Branch: "main", Head: "abc"}, After: oplog.RepositoryView{Branch: branch, Head: "abc"}})
}

func TestPublishRenamesShipsBranchForARuleset(t *testing.T) {
	var calls []string
	forge := &testkit.FakeForge{Rules: map[string][]rules.Rule{
		"feat/42-add-vat":    {namePattern(rules.StartsWith, "feature/")},
		"feature/42-add-vat": {namePattern(rules.StartsWith, "feature/")},
	}}
	ws, log := publishWorkspace(publishableGit(&calls), forge)
	deliveredByShip(log, "feat/42-add-vat")

	result, err := Publish(context.Background(), ws, PublishRequest{})
	if err != nil {
		t.Fatal(err)
	}
	rename := slices.Index(calls, "rename feat/42-add-vat feature/42-add-vat at abc")
	push := slices.Index(calls, "push origin feature/42-add-vat")
	if rename < 0 || push < rename {
		t.Fatalf("rename, then push: %q", calls)
	}
	if result.Branch != "feature/42-add-vat" || forge.Created[0].Head != "feature/42-add-vat" {
		t.Fatalf("the pull request comes from the new name: %+v %+v", result, forge.Created)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Key != "publish.rules.renamed" || result.Warnings[0].Params["rule"] != "Team prefix" {
		t.Fatalf("the user is told: %+v", result.Warnings)
	}
	if !slices.Equal(forge.RulesAsked, []string{"feat/42-add-vat", "feature/42-add-vat"}) {
		t.Fatalf("the new name's own rules are read too: %q", forge.RulesAsked)
	}
	// The rename is logged between the delivery and the publication, so that
	// ship undo reverts the publication, then the name, then the delivery.
	kinds := []oplog.Kind{}
	for _, op := range log.Operations {
		kinds = append(kinds, op.Kind)
	}
	if !slices.Equal(kinds, []oplog.Kind{oplog.Publish, oplog.Rename, oplog.Deliver}) {
		t.Fatalf("operations = %v", kinds)
	}
	renamed := log.Operations[1]
	if renamed.Before.Branch != "feat/42-add-vat" || renamed.After.Branch != "feature/42-add-vat" || renamed.After.Head != "abc" {
		t.Fatalf("rename entry = %+v", renamed)
	}
}

func TestUndoGivesTheOldNameBack(t *testing.T) {
	var calls []string
	git := testkit.HealthyRepository("feature/42-add-vat")
	git.Calls = &calls
	ws, _, log := workspaceOf(git)
	log.Operations = []oplog.Operation{{ID: "op2", Kind: oplog.Rename,
		Before: oplog.RepositoryView{Branch: "feat/42-add-vat", Head: "abc"}, After: oplog.RepositoryView{Branch: "feature/42-add-vat", Head: "abc"}}}
	result, err := Undo(context.Background(), ws, UndoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(calls, "rename feature/42-add-vat feat/42-add-vat at abc") || result.Branch != "feat/42-add-vat" {
		t.Fatalf("calls %q, result %+v", calls, result)
	}
}

func TestPublishRefusesARulesetBranchName(t *testing.T) {
	tests := []struct {
		name    string
		rules   map[string][]rules.Rule
		setup   func(*testkit.FakeGit, *testkit.FakeOperationLog)
		wantKey string
	}{
		{"not created by ship-it", map[string][]rules.Rule{"feat/42-add-vat": {namePattern(rules.StartsWith, "feature/")}},
			func(*testkit.FakeGit, *testkit.FakeOperationLog) {}, "publish.rules.branch_refused"},
		{"already on the remote", map[string][]rules.Rule{"feat/42-add-vat": {namePattern(rules.StartsWith, "feature/")}},
			func(g *testkit.FakeGit, l *testkit.FakeOperationLog) {
				deliveredByShip(l, "feat/42-add-vat")
				g.RemoteBranches = map[string]bool{"origin/feat/42-add-vat": true}
			}, "publish.rules.branch_refused"},
		{"already published", map[string][]rules.Rule{"feat/42-add-vat": {namePattern(rules.StartsWith, "feature/")}},
			func(g *testkit.FakeGit, l *testkit.FakeOperationLog) {
				deliveredByShip(l, "feat/42-add-vat")
				l.Operations = append([]oplog.Operation{{ID: "op1", Kind: oplog.Publish, After: oplog.RepositoryView{Branch: "feat/42-add-vat"}}}, l.Operations...)
			}, "publish.rules.branch_refused"},
		{"new name taken", map[string][]rules.Rule{"feat/42-add-vat": {namePattern(rules.StartsWith, "feature/")}},
			func(g *testkit.FakeGit, l *testkit.FakeOperationLog) {
				deliveredByShip(l, "feat/42-add-vat")
				g.ExistingBranches = map[string]bool{"feature/42-add-vat": true}
			}, "publish.rules.branch_refused"},
		{"nothing fits", map[string][]rules.Rule{"feat/42-add-vat": {namePattern(rules.Contains, "JIRA")}},
			func(g *testkit.FakeGit, l *testkit.FakeOperationLog) { deliveredByShip(l, "feat/42-add-vat") }, "publish.rules.branch_no_name"},
		{"the new name breaks another ruleset", map[string][]rules.Rule{
			"feat/42-add-vat":    {namePattern(rules.StartsWith, "feature/")},
			"feature/42-add-vat": {namePattern(rules.EndsWith, "-x")},
		}, func(g *testkit.FakeGit, l *testkit.FakeOperationLog) { deliveredByShip(l, "feat/42-add-vat") }, "publish.rules.branch_no_name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			git := publishableGit(&calls)
			forge := &testkit.FakeForge{Rules: tt.rules}
			ws, log := publishWorkspace(git, forge)
			tt.setup(&git, log)
			ws.Git = git
			_, err := Publish(context.Background(), ws, PublishRequest{})
			var f *fault.Fault
			if !errors.As(err, &f) || f.Code != fault.BranchNameRefused || f.Key != tt.wantKey {
				t.Fatalf("err = %v", err)
			}
			if f.Params["requirement"] == "" || f.Params["rule"] == "" {
				t.Fatalf("the rule is quoted: %+v", f.Params)
			}
			if tt.wantKey == "publish.rules.branch_refused" && f.Params["command"] != "git branch -m feat/42-add-vat feature/42-add-vat" {
				t.Fatalf("the exact command is given: %+v", f.Params)
			}
			if testkit.Pushed(calls) || slices.ContainsFunc(calls, func(c string) bool { return c[:6] == "rename" }) {
				t.Fatalf("nothing moves: %q", calls)
			}
		})
	}
}

func TestPublishRefusesCommitsARulesetRejects(t *testing.T) {
	commit := ports.CommitRecord{ID: "0123456789abcdef", Message: "✨ feat: add vat", AuthorEmail: "ada@gmail.example", CommitterEmail: "ada@acme.example"}
	pattern := func(t rules.Type, op rules.Operator, p string) rules.Rule {
		return rules.Rule{Type: t, Pattern: &rules.Pattern{Operator: op, Pattern: p}}
	}
	tests := []struct {
		name string
		rule rules.Rule
		code fault.Code
		key  string
	}{
		{"message", pattern(rules.CommitMessagePattern, rules.Regex, `^[A-Z]+-[0-9]+ `), fault.CommitMessageRefused, "publish.rules.commit_message"},
		{"author", pattern(rules.CommitAuthorEmailPattern, rules.EndsWith, "@acme.example"), fault.CommitEmailRefused, "publish.rules.author_email"},
		{"committer", pattern(rules.CommitterEmailPattern, rules.EndsWith, "@corp.example"), fault.CommitEmailRefused, "publish.rules.committer_email"},
		{"signatures", rules.Rule{Type: rules.RequiredSignatures}, fault.UnsignedCommit, "publish.rules.unsigned"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			git := publishableGit(&calls)
			git.RemoteBranches = map[string]bool{"origin/main": true}
			git.Range = []ports.CommitRecord{commit}
			forge := &testkit.FakeForge{Rules: map[string][]rules.Rule{"feat/42-add-vat": {tt.rule}}}
			ws, _ := publishWorkspace(git, forge)
			ws.Runner = &testkit.FakeRunner{}
			_, err := Publish(context.Background(), ws, PublishRequest{})
			var f *fault.Fault
			if !errors.As(err, &f) || f.Code != tt.code || f.Key != tt.key || f.Params["commit"] != "0123456" || f.Params["base"] != "origin/main" {
				t.Fatalf("err = %v", err)
			}
			if testkit.Pushed(calls) {
				t.Fatal("nothing may be pushed")
			}
		})
	}

	t.Run("signed commits pass", func(t *testing.T) {
		git := publishableGit(nil)
		git.RemoteBranches = map[string]bool{"origin/main": true}
		git.Range = []ports.CommitRecord{commit}
		git.Signed = map[string]bool{commit.ID: true}
		forge := &testkit.FakeForge{Rules: map[string][]rules.Rule{"feat/42-add-vat": {{Type: rules.RequiredSignatures}}}}
		ws, _ := publishWorkspace(git, forge)
		if _, err := Publish(context.Background(), ws, PublishRequest{}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestRulesWeCannotReadAreNone(t *testing.T) {
	for _, err := range []error{ports.ErrNotFound, ports.ErrForbidden} {
		forge := &testkit.FakeForge{RulesErr: err}
		ws, _ := publishWorkspace(publishableGit(nil), forge)
		if _, err := Publish(context.Background(), ws, PublishRequest{}); err != nil {
			t.Fatalf("%v: %v", err, err)
		}
	}
	forge := &testkit.FakeForge{RulesErr: ports.ErrRateLimited}
	ws, _ := publishWorkspace(publishableGit(nil), forge)
	if _, err := Publish(context.Background(), ws, PublishRequest{}); !errors.Is(err, &fault.Fault{Code: fault.RateLimited}) {
		t.Fatalf("other errors stop the publication: %v", err)
	}
}
