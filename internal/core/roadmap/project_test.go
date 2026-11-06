// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package roadmap

import (
	"reflect"
	"testing"
)

var projectFields = []ProjectField{
	{ID: 1, Name: "Title", Type: "title"},
	{ID: 2, Name: "Status", Type: SingleSelectField, Options: []FieldOption{{"t", "Todo"}, {"p", "In Progress"}, {"d", "Done"}}},
	{ID: 3, Name: "Target date", Type: DateField},
	{ID: 4, Name: "Phase", Type: SingleSelectField, Options: []FieldOption{{"p1", "Phase 1"}}},
	{ID: 5, Name: "Notes", Type: "text"},
}

var allOptions = ProjectOptions{StatusField: "Status", Todo: "Todo", Done: "done", DateField: "Target date", PhaseField: "phase"}

func TestPlanProject(t *testing.T) {
	body := func(title string) string { return issueBody(Item{Text: title}, Phase{Title: "Phase 1"}, "") }
	remote := Remote{
		Milestones: []Milestone{{Number: 1, Title: "Phase 1", Due: "2026-12-15", Open: true}},
		Issues: []Issue{
			{Number: 12, Title: "CLI", Body: body("CLI"), Open: true, Milestone: 1},
			{Number: 13, Title: "git layer", Body: body("git layer"), Open: false, Milestone: 1, Parent: 12},
			{Number: 14, Title: "docs", Body: body("docs"), Open: true, Milestone: 1},
		},
	}
	tests := []struct {
		name    string
		roadmap string
		items   []ProjectItem
		opts    ProjectOptions
		want    []Change
	}{
		{
			name:    "existing issues are added, with status, date and phase",
			roadmap: "## Phase 1\n**Due:** 2026-12-15\n- [ ] CLI #12\n  - [x] git layer #13\n",
			opts:    allOptions,
			want: []Change{
				{Kind: AddToProject, Phase: "Phase 1", Line: 3, Number: 12, Title: "CLI"},
				{Kind: SetProjectField, Phase: "Phase 1", Line: 3, Number: 12, Field: "Status", FieldID: 2, Value: "Todo", ValueID: "t"},
				{Kind: SetProjectField, Phase: "Phase 1", Line: 3, Number: 12, Field: "Target date", FieldID: 3, Value: "2026-12-15", ValueID: "2026-12-15"},
				{Kind: SetProjectField, Phase: "Phase 1", Line: 3, Number: 12, Field: "Phase", FieldID: 4, Value: "Phase 1", ValueID: "p1"},
				{Kind: AddToProject, Phase: "Phase 1", Line: 4, Number: 13, Title: "git layer"},
				{Kind: SetProjectField, Phase: "Phase 1", Line: 4, Number: 13, Field: "Status", FieldID: 2, Value: "Done", ValueID: "d"},
				{Kind: SetProjectField, Phase: "Phase 1", Line: 4, Number: 13, Field: "Target date", FieldID: 3, Value: "2026-12-15", ValueID: "2026-12-15"},
				{Kind: SetProjectField, Phase: "Phase 1", Line: 4, Number: 13, Field: "Phase", FieldID: 4, Value: "Phase 1", ValueID: "p1"},
			},
		},
		{
			name:    "a status set by hand on an open issue is kept; a closed one becomes done",
			roadmap: "## Phase 1\n- [ ] CLI #12\n  - [x] git layer #13\n",
			items: []ProjectItem{
				{ID: 100, Issue: 12, Values: map[int64]string{2: "p"}},
				{ID: 101, Issue: 13, Values: map[int64]string{2: "p"}},
			},
			opts: ProjectOptions{StatusField: "Status", Todo: "Todo", Done: "Done"},
			want: []Change{
				{Kind: SetProjectField, Phase: "Phase 1", Line: 3, Number: 13, Item: 101, Field: "Status", FieldID: 2, Value: "Done", ValueID: "d"},
			},
		},
		{
			name:    "an issue in line with the project plans nothing",
			roadmap: "## Phase 1\n**Due:** 2026-12-15\n- [ ] CLI #12\n",
			items:   []ProjectItem{{ID: 100, Issue: 12, Values: map[int64]string{2: "t", 3: "2026-12-15", 4: "p1"}}},
			opts:    allOptions,
		},
		{
			name:    "an issue the plan creates is added by its line, an issue the plan closes is done",
			roadmap: "## Phase 1\n- [ ] new one\n- [x] docs #14\n",
			items:   []ProjectItem{{ID: 102, Issue: 14, Values: map[int64]string{2: "t"}}},
			opts:    ProjectOptions{StatusField: "Status", Todo: "Todo", Done: "Done"},
			want: []Change{
				{Kind: AddToProject, Phase: "Phase 1", Line: 2, Title: "new one"},
				{Kind: SetProjectField, Phase: "Phase 1", Line: 2, Field: "Status", FieldID: 2, Value: "Todo", ValueID: "t"},
				{Kind: SetProjectField, Phase: "Phase 1", Line: 3, Number: 14, Item: 102, Field: "Status", FieldID: 2, Value: "Done", ValueID: "d"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Parse([]byte(tt.roadmap))
			plan := Plan(r, remote, Memory{}, Options{})
			got, problems := PlanProject(r, remote, plan, Project{Fields: projectFields, Items: tt.items}, tt.opts)
			if len(problems) > 0 {
				t.Errorf("problems = %+v", problems)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("steps =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestPlanProjectReportsWhatTheProjectLacks(t *testing.T) {
	r := Parse([]byte("## Phase 2\n**Due:** 2027-01-31\n- [ ] CLI\n"))
	opts := ProjectOptions{StatusField: "Status", Todo: "Backlog", Done: "Done", DateField: "Notes", PhaseField: "Phase"}
	steps, problems := PlanProject(r, Remote{}, Plan(r, Remote{}, nil, Options{}), Project{Fields: projectFields}, opts)
	want := []ProjectProblem{
		{Kind: OptionMissing, Field: "Status", Value: "Backlog"},
		{Kind: FieldWrongType, Field: "Notes", Type: DateField},
		{Kind: OptionMissing, Field: "Phase", Value: "Phase 2"},
	}
	if !reflect.DeepEqual(problems, want) {
		t.Errorf("problems = %+v", problems)
	}
	// The item is still added; nothing is invented.
	if !reflect.DeepEqual(kinds(steps), []ChangeKind{AddToProject}) {
		t.Errorf("steps = %+v", steps)
	}
	if _, problem := FindField(projectFields, "Size", SingleSelectField); problem == nil || problem.Kind != FieldMissing {
		t.Errorf("missing field: %+v", problem)
	}
}
