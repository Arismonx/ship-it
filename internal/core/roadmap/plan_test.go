// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package roadmap

import (
	"reflect"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/convention"
)

func kinds(plan []Change) []ChangeKind {
	var out []ChangeKind
	for _, c := range plan {
		out = append(out, c.Kind)
	}
	return out
}

func kindLabel(t convention.CommitType) string {
	if t == convention.Feature {
		return "kind/feature"
	}
	return ""
}

func TestFirstSyncCreatesEverything(t *testing.T) {
	r := Parse([]byte("## Phase 1\n**Due:** 2026-12-15\n- [ ] feat: CLI\n  - [ ] git layer\n- [x] ADRs\n**Exit:** done.\n"))
	plan := Plan(r, Remote{Labels: []string{"kind/feature"}}, nil, Options{Source: "https://github.com/acme/shop/blob/main/ROADMAP.md", KindLabel: kindLabel})
	want := []ChangeKind{CreateMilestone, CreateIssue, CreateIssue, CreateIssue, CloseIssue}
	if got := kinds(plan); !reflect.DeepEqual(got, want) {
		t.Fatalf("plan = %v, want %v", got, want)
	}
	if plan[0].Description != "done." || plan[0].Due != "2026-12-15" {
		t.Errorf("milestone = %+v", plan[0])
	}
	if c := plan[1]; c.Title != "CLI" || !reflect.DeepEqual(c.Labels, []string{"kind/feature"}) ||
		c.Body != "Tracked in [ROADMAP.md](https://github.com/acme/shop/blob/main/ROADMAP.md) · Phase 1\n\n"+Marker {
		t.Errorf("issue = %+v", c)
	}
	if c := plan[2]; c.Title != "git layer" || c.ParentLine != 3 || c.ParentNumber != 0 {
		t.Errorf("sub-issue = %+v", c)
	}
}

func TestAnUpToDateRoadmapPlansNothing(t *testing.T) {
	r := Parse([]byte("## Phase 1\n- [ ] CLI #12\n  - [x] git layer #13\n"))
	body := func(title string) string { return issueBody(Item{Text: title}, Phase{Title: "Phase 1"}, "") }
	remote := Remote{
		Milestones: []Milestone{{Number: 1, Title: "Phase 1", Open: true}},
		Issues: []Issue{
			{Number: 12, Title: "CLI", Body: body("CLI"), Open: true, Milestone: 1},
			{Number: 13, Title: "git layer", Body: body("git layer"), Milestone: 1, Parent: 12},
		},
	}
	if plan := Plan(r, remote, nil, Options{}); len(plan) != 0 {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestStatesFollowWhoChanged(t *testing.T) {
	r := Parse([]byte("## P\n- [ ] a #1\n- [x] b #2\n"))
	remote := func(aOpen, bOpen bool) Remote {
		body := func(title string) string { return issueBody(Item{Text: title}, Phase{Title: "P"}, "") }
		return Remote{
			Milestones: []Milestone{{Number: 5, Title: "P", Open: true}},
			Issues:     []Issue{{Number: 1, Title: "a", Body: body("a"), Open: aOpen, Milestone: 5}, {Number: 2, Title: "b", Body: body("b"), Open: bOpen, Milestone: 5}},
		}
	}
	for _, tt := range []struct {
		name   string
		remote Remote
		memory Memory
		want   []ChangeKind
	}{
		// a closed on GitHub, b reopened on GitHub since the last sync.
		{"GitHub changed", remote(false, true), Memory{1: false, 2: true}, []ChangeKind{TickBox, UntickBox}},
		// a ticked? No: a unticked while closed in memory → reopen; b ticked → close.
		{"the file changed", remote(false, true), Memory{1: true, 2: false}, []ChangeKind{ReopenIssue, CloseIssue}},
		// No memory: GitHub's closed state ticks the box, a ticked box closes, nothing reopens.
		{"no memory", remote(false, true), nil, []ChangeKind{TickBox, CloseIssue}},
	} {
		if got := kinds(Plan(r, tt.remote, tt.memory, Options{})); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestRemovedItemsAreClosedNotPlanned(t *testing.T) {
	r := Parse([]byte("## P\n- [ ] kept #1\n"))
	body := issueBody(Item{Text: "kept"}, Phase{Title: "P"}, "")
	remote := Remote{
		Milestones: []Milestone{{Number: 5, Title: "P", Open: true}},
		Issues: []Issue{
			{Number: 1, Title: "kept", Body: body, Open: true, Milestone: 5},
			{Number: 2, Title: "gone", Body: "x\n\n" + Marker, Open: true, Milestone: 5},
			{Number: 3, Title: "written by hand", Body: "no marker", Open: true},
		},
	}
	plan := Plan(r, remote, nil, Options{})
	if len(plan) != 1 || plan[0].Kind != CloseNotPlanned || plan[0].Number != 2 {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestAnInterruptedSyncIsResumedWithoutDuplicates(t *testing.T) {
	r := Parse([]byte("## P\n- [ ] CLI\n"))
	body := issueBody(Item{Text: "CLI"}, Phase{Title: "P"}, "")
	remote := Remote{Milestones: []Milestone{{Number: 5, Title: "P", Open: true}}, Issues: []Issue{{Number: 9, Title: "CLI", Body: body, Open: true, Milestone: 5}}}
	plan := Plan(r, remote, nil, Options{})
	if len(plan) != 1 || plan[0].Kind != WriteIssueNumber || plan[0].Number != 9 || plan[0].Line != 2 {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestHandWrittenIssuesKeepTheirContent(t *testing.T) {
	r := Parse([]byte("## P\n- [ ] CLI skeleton #4\n"))
	remote := Remote{Milestones: []Milestone{{Number: 5, Title: "P", Open: true}}, Issues: []Issue{{Number: 4, Title: "Old title", Body: "written by a human", Open: true}}}
	plan := Plan(r, remote, nil, Options{})
	if len(plan) != 1 || plan[0].Kind != UpdateIssue || plan[0].Title != "" || plan[0].Body != "" {
		t.Fatalf("only the milestone is set on a hand-written issue: %+v", plan)
	}
}
