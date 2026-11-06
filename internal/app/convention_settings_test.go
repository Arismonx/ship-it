// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/ports"
	"github.com/GregoireF/ship-it/internal/testkit"
)

// conventionsFile is a repository that changes every convention setting.
const conventionsFile = `{"ship": {
	"commits": {"subjectMaxLength": 30, "bodyWidth": 40, "scopes": ["api", "release"],
		"types": {"deps": {"emoji": "⬆️", "branchPrefix": "deps", "label": "dependencies"}, "wip": false,
			"feat": {"emoji": "🚀"}}},
	"branches": {"maxWords": 2, "issue": "none"}}}`

func TestLoadConventions(t *testing.T) {
	ws := Workspace{Git: testkit.HealthyRepository("main"), Config: &testkit.FakeConfigFiles{Repository: conventionsFile}}
	c, err := LoadConventions(context.Background(), ws, nil)
	if err != nil {
		t.Fatal(err)
	}
	var types []string
	for _, v := range c.CommitTypes() {
		types = append(types, v.Type)
	}
	if strings.Contains(strings.Join(types, " "), "wip") || types[len(types)-1] != "deps" {
		t.Errorf("types = %v", types)
	}
	if v := c.CommitTypes()[0]; v.Gitmoji != "🚀" || v.Label != "feature" {
		t.Errorf("feat = %+v", v)
	}
	if !c.CheckTitle("⬆️ deps(api): bump yaml").Valid() {
		t.Errorf("an added type must pass: %+v", c.CheckTitle("⬆️ deps(api): bump yaml"))
	}
	for title, want := range map[string]fault.Code{
		"🚧 wip: try":            fault.UnknownCommitType,
		"✨ feat(db): add index": fault.ScopeNotAllowed,
		"✨ feat: add a subject longer than thirty chars": fault.SubjectTooLong,
	} {
		if check := c.CheckTitle(title); check.Valid() || check.Faults[0].Code != want {
			t.Errorf("%s: %+v, want %s", title, check.Faults, want)
		}
	}
	if CheckTitle("⬆️ deps: bump yaml").Valid() {
		t.Error("the built-in check knows no deps type")
	}
	if b, err := c.SuggestBranch(BranchRequest{CommitType: "deps", Subject: "bump yaml parser version", Issue: 4}); err != nil || b != "deps/bump-yaml" {
		t.Errorf("SuggestBranch = %q, %v", b, err)
	}
	if !c.CheckBranch("deps/bump-yaml").Valid() || CheckBranch("deps/bump-yaml").Valid() {
		t.Error("deps/ is a prefix of this repository only")
	}
	if title, ok := c.RetypeTitle("🔧 chore: bump yaml", "deps"); !ok || title != "🔧 deps: bump yaml" {
		t.Errorf("RetypeTitle = %q, %v", title, ok)
	}
	if c.MaxTitleLength() != 72 || (Conventions{}).MaxTitleLength() != 72 {
		t.Error("the header length keeps its default")
	}
}

func TestLoadConventionsReportsABrokenFile(t *testing.T) {
	ws := Workspace{Config: &testkit.FakeConfigFiles{Repository: `{"ship": {"commits": {"types": {"feat": {"branchPrefix": "Feat"}}}}}`}}
	if _, err := LoadConventions(context.Background(), ws, nil); !errors.Is(err, &fault.Fault{Code: fault.ConfigInvalidValue}) {
		t.Fatalf("error = %v", err)
	}
}

func TestProposePlanFollowsTheConventions(t *testing.T) {
	git := testkit.FakeGit{
		WorkingTree: ports.WorkingTreeStatus{
			Branch: ports.BranchState{Name: "main", Commit: "abc"},
			Files:  []ports.FileChange{{Path: "internal/core/release/notes.go", State: ports.Untracked}},
		},
		WorkingFiles: map[string]string{"internal/core/release/notes.go": "package release\n\nfunc Notes() {}\n"},
	}
	ws := Workspace{Git: git, Config: &testkit.FakeConfigFiles{Repository: conventionsFile}}
	view, err := ProposePlan(context.Background(), ws, PlanRequest{Issue: 9})
	if err != nil {
		t.Fatal(err)
	}
	if view.Title != "🚀 feat(release): add notes" || view.Branch != "feat/add-notes" {
		t.Fatalf("plan = %q %q", view.Title, view.Branch)
	}

	ws.Config = &testkit.FakeConfigFiles{Repository: `{"ship": {"commits": {"scopeFrom": "none", "types": {"feat": false}}}}`}
	view, err = ProposePlan(context.Background(), ws, PlanRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if view.Title != "🐛 fix: add notes" || view.Branch != "fix/add-notes" {
		t.Fatalf("plan = %q %q", view.Title, view.Branch)
	}
	if !slicesContainKey(view.Reasons, "plan.reason.type_removed") {
		t.Errorf("reasons = %+v", view.Reasons)
	}
}

func slicesContainKey(reasons []ReasonView, key string) bool {
	for _, r := range reasons {
		if r.Key == key {
			return true
		}
	}
	return false
}

func TestDeliverFollowsTheConventions(t *testing.T) {
	var calls []string
	ws, _, _ := workspaceOf(deliverableGit("main", &calls))
	ws.Config = &testkit.FakeConfigFiles{Repository: conventionsFile}
	why := "the parser of the previous version reads anchors wrongly in nested lists"
	result, err := DeliverLocally(context.Background(), ws,
		DeliveryRequest{Title: "deps(api): bump yaml", Branch: "deps/bump-yaml", Why: why})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"switch deps/bump-yaml", "stage",
		"commit ⬆️ deps(api): bump yaml\n\nthe parser of the previous version reads\nanchors wrongly in nested lists\n"}
	if !reflect.DeepEqual(calls, want) || result.Title != "⬆️ deps(api): bump yaml" {
		t.Fatalf("calls = %q, result = %+v", calls, result)
	}
}

func TestDeliverRefusesWhatTheConventionsRefuse(t *testing.T) {
	for title, code := range map[string]fault.Code{
		"🚧 wip: try":            fault.UnknownCommitType,
		"✨ feat(db): add index": fault.ScopeNotAllowed,
	} {
		var calls []string
		ws, store, _ := workspaceOf(deliverableGit("main", &calls))
		ws.Config = &testkit.FakeConfigFiles{Repository: conventionsFile}
		_, err := DeliverLocally(context.Background(), ws, DeliveryRequest{Title: title, Branch: "chore/x"})
		if !errors.Is(err, &fault.Fault{Code: code}) || len(calls) != 0 || len(store.Checkpoints) != 0 {
			t.Errorf("%s: error = %v, calls = %q", title, err, calls)
		}
	}
}

func TestDeliveryBranchCheckAcceptsThePrefixesOfTheTable(t *testing.T) {
	ws := Workspace{Git: testkit.HealthyRepository("main"), Config: &testkit.FakeConfigFiles{Repository: conventionsFile}}
	check := DeliveryBranchCheck(context.Background(), ws, nil)
	if !check("deps/bump-yaml").Valid() || check("nope/bump-yaml").Valid() {
		t.Error("deps/ is accepted, nope/ is not")
	}
}
