// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/core/roadmap"
	"github.com/GregoireF/ship-it/internal/ports"
	"github.com/GregoireF/ship-it/internal/testkit"
)

const projectConfig = `{"github": {"project": {"url": "https://github.com/orgs/acme/projects/7", "dateField": "Target date", "phaseField": "Phase"}}}`

func fakeProject() *testkit.FakeProjects {
	return &testkit.FakeProjects{Fields: []roadmap.ProjectField{
		{ID: 1, Name: "Title", Type: "title"},
		{ID: 2, Name: "Status", Type: roadmap.SingleSelectField, Options: []roadmap.FieldOption{{ID: "t", Name: "Todo"}, {ID: "p", Name: "In Progress"}, {ID: "d", Name: "Done"}}},
		{ID: 3, Name: "Target date", Type: roadmap.DateField},
		{ID: 4, Name: "Phase", Type: roadmap.SingleSelectField, Options: []roadmap.FieldOption{{ID: "f", Name: "Phase 1 — Foundations"}}},
	}}
}

func TestRoadmapSyncFillsTheProjectAndUndoes(t *testing.T) {
	ws, _, _, _ := roadmapWorkspace(t)
	project := fakeProject()
	ws.Projects, ws.Config = project, &testkit.FakeConfigFiles{Repository: projectConfig}
	ctx := context.Background()
	plan, err := PlanRoadmap(ctx, ws, "")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Project == nil || plan.Project.Kind != "orgs" || len(plan.ProjectProblems) != 0 {
		t.Fatalf("project = %+v, problems %+v", plan.Project, plan.ProjectProblems)
	}
	if _, err := ApplyRoadmap(ctx, ws, plan); err != nil {
		t.Fatal(err)
	}
	// The three created issues are in the project, with status, date and phase.
	for issue, status := range map[int]string{1: "t", 2: "t", 3: "d"} {
		got := map[int64]string{}
		for _, field := range []int64{2, 3, 4} {
			got[field], _ = project.Value(issue, field)
		}
		if want := map[int64]string{2: status, 3: "2026-12-15", 4: "f"}; !reflect.DeepEqual(got, want) {
			t.Errorf("#%d: values %v, want %v", issue, got, want)
		}
	}
	if again, err := PlanRoadmap(ctx, ws, ""); err != nil || !again.Empty() {
		t.Fatalf("second plan = %+v, %v", again.Changes, err)
	}

	// By hand: #1 is in progress (kept), its date moved (the file wins).
	project.Items[0].Values[2], project.Items[0].Values[3] = "p", "2027-01-01"
	plan, err = PlanRoadmap(ctx, ws, "")
	if err != nil || len(plan.Changes) != 1 || plan.Changes[0].Kind != roadmap.SetProjectField || plan.Changes[0].ValueID != "2026-12-15" {
		t.Fatalf("plan = %+v, %v", plan.Changes, err)
	}
	if _, err := ApplyRoadmap(ctx, ws, plan); err != nil {
		t.Fatal(err)
	}
	if _, err := Undo(ctx, ws, UndoRequest{AllowRemote: true}); err != nil {
		t.Fatal(err)
	}
	if v, _ := project.Value(1, 3); v != "2027-01-01" {
		t.Errorf("undo did not restore the date: %q", v)
	}
	// Undoing the first sync removes the items it added.
	if _, err := Undo(ctx, ws, UndoRequest{AllowRemote: true}); err != nil {
		t.Fatal(err)
	}
	if len(project.Items) != 0 {
		t.Errorf("items left after undo: %+v", project.Items)
	}
}

func TestRoadmapSyncReportsWhatTheProjectLacks(t *testing.T) {
	ws, _, _, _ := roadmapWorkspace(t)
	project := fakeProject()
	project.Fields[3].Options = nil // no option for the phase
	ws.Projects, ws.Config = project, &testkit.FakeConfigFiles{Repository: projectConfig}
	plan, err := PlanRoadmap(context.Background(), ws, "")
	if err != nil {
		t.Fatal(err)
	}
	want := []ProblemView{{Code: string(fault.ProjectIncomplete), Key: "project.problem.option_missing",
		Params: map[string]string{"project": "acme/7", "field": "Phase", "value": "Phase 1 — Foundations", "type": ""}}}
	if !reflect.DeepEqual(plan.ProjectProblems, want) {
		t.Fatalf("problems = %+v", plan.ProjectProblems)
	}
}

func TestRoadmapSyncWithoutTheProjectScope(t *testing.T) {
	ws, _, _, _ := roadmapWorkspace(t)
	ws.Projects = &testkit.FakeProjects{Err: ports.ErrProjectScope}
	ws.Config = &testkit.FakeConfigFiles{Repository: projectConfig}
	_, err := PlanRoadmap(context.Background(), ws, "")
	if f := asFault(err); f.Code != fault.ProjectAccessDenied || f.Key != "project.scope_missing" {
		t.Fatalf("err = %v", err)
	}
}

func projectPublishWorkspace(project *testkit.FakeProjects, assignees ...string) (Workspace, *testkit.FakeForge, *testkit.FakeTracker) {
	forge := &testkit.FakeForge{User: ports.User{Login: "grace"}}
	ws, _ := publishWorkspace(publishableGit(nil), forge)
	tracker := &testkit.FakeTracker{IssueList: []roadmap.Issue{{Number: 42, ID: 1042, Open: true, Assignees: assignees}}}
	ws.Tracker, ws.Projects = tracker, project
	ws.Config = &testkit.FakeConfigFiles{Repository: `{"github": {"project": {"url": "acme/7"}}}`}
	return ws, forge, tracker
}

func TestPublishMovesTheLinkedIssueInProgress(t *testing.T) {
	project := fakeProject()
	project.Items = []roadmap.ProjectItem{{ID: 500, Issue: 42, Values: map[int64]string{2: "t"}}}
	ws, _, tracker := projectPublishWorkspace(project)
	result, err := Publish(context.Background(), ws, PublishRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := project.Value(42, 2); v != "p" || result.ProjectStatus != "In Progress" {
		t.Errorf("status = %q, result %q", v, result.ProjectStatus)
	}
	if got := tracker.IssueList[0].Assignees; !reflect.DeepEqual(got, []string{"grace"}) || result.Assigned != "grace" {
		t.Errorf("assignees = %v, result %q", got, result.Assigned)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("warnings = %+v", result.Warnings)
	}
}

func TestPublishLeavesIssuesOutsideTheProject(t *testing.T) {
	// Not in the project: neither moved nor assigned.
	ws, _, tracker := projectPublishWorkspace(fakeProject())
	result, err := Publish(context.Background(), ws, PublishRequest{})
	if err != nil || result.ProjectStatus != "" || result.Assigned != "" || len(tracker.IssueList[0].Assignees) != 0 {
		t.Fatalf("result = %+v, %v", result, err)
	}

	// Already assigned: kept as it is.
	project := fakeProject()
	project.Items = []roadmap.ProjectItem{{ID: 500, Issue: 42, Values: map[int64]string{}}}
	ws, _, tracker = projectPublishWorkspace(project, "ada")
	if result, err = Publish(context.Background(), ws, PublishRequest{}); err != nil || result.Assigned != "" ||
		!reflect.DeepEqual(tracker.IssueList[0].Assignees, []string{"ada"}) {
		t.Fatalf("result = %+v, %v, assignees %v", result, err, tracker.IssueList[0].Assignees)
	}
}

func TestPublishWarnsWhenTheProjectIsOutOfReach(t *testing.T) {
	ws, forge, _ := projectPublishWorkspace(&testkit.FakeProjects{Err: ports.ErrProjectScope})
	result, err := Publish(context.Background(), ws, PublishRequest{})
	if err != nil || len(forge.Created) != 1 {
		t.Fatalf("the pull request must open anyway: %v", err)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Key != "project.scope_missing" {
		t.Fatalf("warnings = %+v", result.Warnings)
	}
}

// publishedInProject publishes with the linked issue #42 in the project,
// holding status (empty: no status), and returns the workspace to undo it.
func publishedInProject(t *testing.T, status string, assignees ...string) (Workspace, *testkit.FakeProjects, *testkit.FakeTracker, *testkit.FakeForge) {
	t.Helper()
	project := fakeProject()
	values := map[int64]string{}
	if status != "" {
		values[2] = status
	}
	project.Items = []roadmap.ProjectItem{{ID: 500, Issue: 42, Values: values}}
	ws, forge, tracker := projectPublishWorkspace(project, assignees...)
	ws.Config = &testkit.FakeConfigFiles{Repository: `{"github": {"project": {"url": "acme/7"}}}`}
	if _, err := Publish(context.Background(), ws, PublishRequest{}); err != nil {
		t.Fatal(err)
	}
	return ws, project, tracker, forge
}

func TestUndoOfAPublicationPutsTheProjectBack(t *testing.T) {
	for name, before := range map[string]string{"a status": "t", "no status": ""} {
		t.Run(name, func(t *testing.T) {
			ws, project, tracker, forge := publishedInProject(t, before)
			op := ws.Operations.(*testkit.FakeOperationLog).Operations[0]
			if op.LinkedIssue == nil || op.LinkedIssue.Project.String() != "acme/7" || op.LinkedIssue.Project.Kind == "" {
				t.Fatalf("recorded %+v", op.LinkedIssue)
			}
			want := oplog.LinkedIssueChange{Issue: 42, Project: op.LinkedIssue.Project, Item: 500, Field: 2, Before: before, After: "p", Assigned: "grace"}
			if *op.LinkedIssue != want {
				t.Fatalf("recorded %+v, want %+v", *op.LinkedIssue, want)
			}
			result, err := Undo(context.Background(), ws, UndoRequest{AllowRemote: true})
			if err != nil {
				t.Fatal(err)
			}
			if v, _ := project.Value(42, 2); v != before {
				t.Errorf("status = %q, want %q", v, before)
			}
			if got := tracker.IssueList[0].Assignees; len(got) != 0 {
				t.Errorf("assignees = %v", got)
			}
			if result.RestoredIssueStatus != 42 || result.Unassigned != "grace" || len(result.Warnings) != 0 || len(forge.Closed) != 1 {
				t.Errorf("result = %+v", result)
			}
		})
	}
}

func TestUndoOfAPublicationKeepsAHumanChange(t *testing.T) {
	ws, project, tracker, _ := publishedInProject(t, "t")
	// Since: someone moved the issue to Done, and unassigned grace for ada.
	project.Items[0].Values[2] = "d"
	tracker.IssueList[0].Assignees = []string{"ada"}
	result, err := Undo(context.Background(), ws, UndoRequest{AllowRemote: true})
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := project.Value(42, 2); v != "d" {
		t.Errorf("a human's status was overwritten: %q", v)
	}
	if !reflect.DeepEqual(tracker.IssueList[0].Assignees, []string{"ada"}) {
		t.Errorf("assignees = %v", tracker.IssueList[0].Assignees)
	}
	if slices.ContainsFunc(project.Calls, func(c string) bool { return strings.HasPrefix(c, "set") && c != "set 500 2 p" }) ||
		slices.ContainsFunc(tracker.Calls, func(c string) bool { return strings.HasPrefix(c, "unassign") }) {
		t.Errorf("writes: %q, %q", project.Calls, tracker.Calls)
	}
	want := []ProblemView{{Code: string(fault.ProjectStatusChanged), Key: "undo.project_status_changed",
		Params: map[string]string{"issue": "42", "project": "acme/7"}}}
	if !reflect.DeepEqual(result.Warnings, want) || result.RestoredIssueStatus != 0 || result.Unassigned != "" {
		t.Errorf("result = %+v", result)
	}
}

func TestUndoOfAPublicationRemovesOnlyTheAssigneeItAdded(t *testing.T) {
	// Assigned before: publish added nobody, undo removes nobody.
	ws, _, tracker, _ := publishedInProject(t, "t", "ada")
	if op := ws.Operations.(*testkit.FakeOperationLog).Operations[0]; op.LinkedIssue == nil || op.LinkedIssue.Assigned != "" {
		t.Fatalf("recorded %+v", op.LinkedIssue)
	}
	if result, err := Undo(context.Background(), ws, UndoRequest{AllowRemote: true}); err != nil || result.Unassigned != "" {
		t.Fatalf("result = %+v, %v", result, err)
	}
	if !reflect.DeepEqual(tracker.IssueList[0].Assignees, []string{"ada"}) {
		t.Errorf("assignees = %v", tracker.IssueList[0].Assignees)
	}

	// Added by publish, then ada joined: only grace leaves.
	ws, _, tracker, _ = publishedInProject(t, "t")
	tracker.IssueList[0].Assignees = append(tracker.IssueList[0].Assignees, "ada")
	if result, err := Undo(context.Background(), ws, UndoRequest{AllowRemote: true}); err != nil || result.Unassigned != "grace" {
		t.Fatalf("result = %+v, %v", result, err)
	}
	if !reflect.DeepEqual(tracker.IssueList[0].Assignees, []string{"ada"}) {
		t.Errorf("assignees = %v", tracker.IssueList[0].Assignees)
	}
}

func TestUndoOfAPublicationWarnsWhenTheProjectIsOutOfReach(t *testing.T) {
	ws, project, _, forge := publishedInProject(t, "t")
	project.Err = ports.ErrProjectScope
	result, err := Undo(context.Background(), ws, UndoRequest{AllowRemote: true})
	if err != nil || len(forge.Closed) != 1 {
		t.Fatalf("the undo must go on: %v", err)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Key != "project.scope_missing" || result.Unassigned != "grace" {
		t.Fatalf("result = %+v", result)
	}
}

func TestUndoOfAnOldPublicationLeavesTheProjectAlone(t *testing.T) {
	ws, project, tracker, forge := publishedInProject(t, "t")
	log := ws.Operations.(*testkit.FakeOperationLog)
	log.Operations[0].LinkedIssue = nil // logged before ship-it recorded it
	project.Calls, tracker.Calls = nil, nil
	result, err := Undo(context.Background(), ws, UndoRequest{AllowRemote: true})
	if err != nil || len(forge.Closed) != 1 || len(result.Warnings) != 0 {
		t.Fatalf("result = %+v, %v", result, err)
	}
	if v, _ := project.Value(42, 2); v != "p" || len(project.Calls) != 0 || len(tracker.Calls) != 0 {
		t.Errorf("status %q, calls %q %q", v, project.Calls, tracker.Calls)
	}
	// Its consent names no issue.
	log.Operations = log.Operations[1:]
	if _, err := Undo(context.Background(), ws, UndoRequest{}); asFault(err).Key != "undo.needs_consent" {
		t.Errorf("err = %v", err)
	}
}

func TestDryRunOfUndoingAPublicationPlansTheProjectSteps(t *testing.T) {
	ws, project, tracker, forge := publishedInProject(t, "t")
	if _, err := Undo(context.Background(), ws, UndoRequest{}); asFault(err).Key != "undo.needs_consent_issue" ||
		asFault(err).Params["issue"] != "42" {
		t.Fatalf("err = %v", err)
	}
	result, err := Undo(context.Background(), rehearsedWorkspace(t, ws), UndoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	wantPlan(t, result.Rehearsal, "needs_consent", "checkpoint", "close_pull_request", "delete_remote_branch",
		"unassign_issue", "set_project_field", "log")
	if v, _ := project.Value(42, 2); v != "p" || len(tracker.IssueList[0].Assignees) != 1 || len(forge.Closed) != 0 {
		t.Errorf("a dry run changed GitHub: status %q, assignees %v, closed %v", v, tracker.IssueList[0].Assignees, forge.Closed)
	}
}

func TestDryRunOfPublishLeavesTheProjectAlone(t *testing.T) {
	project := fakeProject()
	project.Items = []roadmap.ProjectItem{{ID: 500, Issue: 42, Values: map[int64]string{2: "t"}}}
	ws, _, tracker := projectPublishWorkspace(project)
	result, err := Publish(context.Background(), rehearsedWorkspace(t, ws), PublishRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if keys := plannedKeys(result.Rehearsal); !slices.Contains(keys, "set_project_field") {
		t.Errorf("planned = %q", keys)
	}
	if v, _ := project.Value(42, 2); v != "t" || len(tracker.IssueList[0].Assignees) != 0 {
		t.Errorf("a dry run changed the project (%q) or the assignees %v", v, tracker.IssueList[0].Assignees)
	}
}
