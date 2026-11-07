// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package roadmap

import (
	"slices"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/convention"
)

// Marker ends the part of an issue's body that ship-it writes from the roadmap:
// invisible on GitHub, it tells ship-it which issues it may rewrite or close.
// Text below it is written by people (context, pointers, open questions) and
// a sync keeps it as it is.
const Marker = "<!-- ship-it:roadmap -->"

// HandWritten is the part of a managed issue's body below the marker, kept by
// every sync, or "" when there is none.
func HandWritten(body string) string {
	if i := strings.Index(body, Marker); i >= 0 {
		return body[i+len(Marker):]
	}
	return ""
}

// Milestone is a milestone as GitHub has it.
type Milestone struct {
	Number      int
	Title       string
	Description string
	// Due is the due date as YYYY-MM-DD, "" without one.
	Due  string
	Open bool
	// OpenIssues and ClosedIssues are GitHub's counts of the issues in it.
	OpenIssues, ClosedIssues int
}

// Issue is an issue as GitHub has it.
type Issue struct {
	Number int
	// ID is the global id GitHub's sub-issues API needs.
	ID        int64
	Title     string
	Body      string
	Open      bool
	Milestone int // number, 0 without one
	Labels    []string
	// Parent is the number of its parent issue, 0 without one.
	Parent int
	// Assignees are the logins of its assignees.
	Assignees []string
}

// Managed reports whether ship-it created the issue from a roadmap.
func (i Issue) Managed() bool { return strings.Contains(i.Body, Marker) }

// Remote is what the repository holds: its milestones, the issues ship-it
// manages, the issues the roadmap names by number, and its labels.
type Remote struct {
	Milestones []Milestone
	Issues     []Issue
	Labels     []string
}

// Memory is the closed state of each issue at the last sync, kept locally
// (.git/ship/roadmap.json): it tells who changed a state since. Without it
// (a first sync, another clone), GitHub wins and ship-it never reopens.
type Memory map[int]bool

// ChangeKind names a step of the plan.
type ChangeKind string

// Steps, applied in the order of the plan.
const (
	CreateMilestone  ChangeKind = "create_milestone"
	UpdateMilestone  ChangeKind = "update_milestone"
	CreateIssue      ChangeKind = "create_issue"
	UpdateIssue      ChangeKind = "update_issue"
	CloseIssue       ChangeKind = "close_issue"
	ReopenIssue      ChangeKind = "reopen_issue"
	CloseNotPlanned  ChangeKind = "close_not_planned"
	SetParent        ChangeKind = "set_parent"
	TickBox          ChangeKind = "tick_box"
	UntickBox        ChangeKind = "untick_box"
	WriteIssueNumber ChangeKind = "write_number"
)

// Change is one step. Items are named by their line in ROADMAP.md; an item
// created by an earlier step of the same plan has Number 0 and is resolved
// by the caller through that line.
type Change struct {
	Kind ChangeKind `json:"kind"`
	// Phase is the milestone's title.
	Phase string `json:"phase,omitempty"`
	// Line is the item's line in ROADMAP.md, 0 for a milestone change or an
	// issue no item names any more.
	Line   int    `json:"line,omitempty"`
	Number int    `json:"number,omitempty"`
	Title  string `json:"title,omitempty"`
	// Body, Labels, Description and Due are the content to write.
	Body        string   `json:"body,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	Description string   `json:"description,omitempty"`
	Due         string   `json:"due,omitempty"`
	// ParentLine is the parent item of a SetParent (and of a created
	// sub-issue); ParentNumber its issue when already known.
	ParentLine   int `json:"parent_line,omitempty"`
	ParentNumber int `json:"parent_number,omitempty"`
	// Item is the project item of a SetProjectField, 0 when an earlier step
	// adds it; Field, FieldID, Value and ValueID say what it sets.
	Item    int64  `json:"item,omitempty"`
	Field   string `json:"field,omitempty"`
	FieldID int64  `json:"field_id,omitempty"`
	Value   string `json:"value,omitempty"`
	ValueID string `json:"value_id,omitempty"`
}

// Options give the plan what the roadmap cannot know.
type Options struct {
	// Source is the URL of ROADMAP.md on the default branch, linked from
	// every managed issue.
	Source string
	// KindLabel maps a commit type to the repository's label for it ("" when
	// the repository has none): labels are only those that exist.
	KindLabel func(convention.CommitType) string
}

// Plan compares the roadmap with the repository and the memory of the last
// sync, and returns the steps that bring GitHub and ROADMAP.md in line.
// Content flows from the file (titles, descriptions, milestones) for the
// issues ship-it manages; states follow the rules of Memory; nothing is ever
// deleted: an issue no item names any more is closed as not planned.
func Plan(r Roadmap, remote Remote, memory Memory, opts Options) []Change {
	var plan []Change
	byNumber := map[int]Issue{}
	for _, is := range remote.Issues {
		byNumber[is.Number] = is
	}
	named := map[int]bool{}

	for _, phase := range r.Phases {
		ms, found := milestoneByTitle(remote.Milestones, phase.Title)
		switch {
		case !found:
			plan = append(plan, Change{Kind: CreateMilestone, Phase: phase.Title, Description: phase.Exit, Due: phase.Due})
		case ms.Description != phase.Exit || ms.Due != phase.Due:
			plan = append(plan, Change{Kind: UpdateMilestone, Phase: phase.Title, Number: ms.Number, Description: phase.Exit, Due: phase.Due})
		}
		for _, item := range phase.Items {
			plan = append(plan, planItem(item, phase, ms.Number, 0, 0, byNumber, remote, memory, opts, named)...)
			parent := item.Number
			if parent == 0 {
				if is, ok := matchManaged(remote.Issues, item, ms.Number); ok {
					parent = is.Number
				}
			}
			for _, child := range item.Children {
				plan = append(plan, planItem(child, phase, ms.Number, item.Line, parent, byNumber, remote, memory, opts, named)...)
			}
		}
	}

	// Managed issues no item names any more: closed, never deleted.
	for _, is := range remote.Issues {
		if is.Managed() && is.Open && !named[is.Number] {
			plan = append(plan, Change{Kind: CloseNotPlanned, Number: is.Number, Title: is.Title})
		}
	}
	return plan
}

func planItem(item Item, phase Phase, milestone, parentLine, parentNumber int, byNumber map[int]Issue,
	remote Remote, memory Memory, opts Options, named map[int]bool) []Change {
	var plan []Change
	is, found := byNumber[item.Number]
	if item.Number == 0 {
		// Before its number was written back (a sync interrupted, a file not
		// delivered yet), a managed issue of the same title is the item's.
		is, found = matchManaged(remote.Issues, item, milestone)
		if found {
			plan = append(plan, Change{Kind: WriteIssueNumber, Line: item.Line, Number: is.Number})
		}
	}
	body := issueBody(item, phase, opts.Source)
	var labels []string
	if item.Kind != "" && opts.KindLabel != nil {
		if l := opts.KindLabel(item.Kind); l != "" && slices.Contains(remote.Labels, l) {
			labels = []string{l}
		}
	}

	if !found {
		plan = append(plan, Change{Kind: CreateIssue, Phase: phase.Title, Line: item.Line, Title: item.Title(), Body: body,
			Labels: labels, ParentLine: parentLine, ParentNumber: parentNumber})
		if item.Done {
			plan = append(plan, Change{Kind: CloseIssue, Line: item.Line, Title: item.Title()})
		}
		return plan
	}
	named[is.Number] = true

	// Content: the file wins for the issues ship-it manages; an issue written
	// by hand and linked by number only gets its milestone.
	update := Change{Kind: UpdateIssue, Phase: phase.Title, Line: item.Line, Number: is.Number}
	changed := is.Milestone != milestone || milestone == 0
	if is.Managed() {
		// The text people wrote below the marker stays.
		full := body + HandWritten(is.Body)
		update.Title, update.Body = item.Title(), full
		changed = changed || is.Title != item.Title() || is.Body != full
	}
	if changed {
		plan = append(plan, update)
	}

	// State.
	closed := !is.Open
	last, remembered := memory[is.Number]
	switch {
	case closed == item.Done:
	case remembered && last != closed:
		// GitHub changed it since the last sync: the box follows.
		kind := TickBox
		if !closed {
			kind = UntickBox
		}
		plan = append(plan, Change{Kind: kind, Line: item.Line, Number: is.Number})
	case remembered:
		// The box changed since the last sync: GitHub follows.
		kind := CloseIssue
		if !item.Done {
			kind = ReopenIssue
		}
		plan = append(plan, Change{Kind: kind, Line: item.Line, Number: is.Number})
	case closed:
		plan = append(plan, Change{Kind: TickBox, Line: item.Line, Number: is.Number}) // GitHub wins
	default:
		plan = append(plan, Change{Kind: CloseIssue, Line: item.Line, Number: is.Number}) // a ticked box closes
	}

	if parentNumber != 0 && is.Parent != parentNumber {
		plan = append(plan, Change{Kind: SetParent, Line: item.Line, Number: is.Number, ParentLine: parentLine, ParentNumber: parentNumber})
	} else if parentNumber == 0 && parentLine != 0 {
		plan = append(plan, Change{Kind: SetParent, Line: item.Line, Number: is.Number, ParentLine: parentLine})
	}
	return plan
}

// issueBody is the description of a managed issue: the item's own lines (or
// its full text when the title had to be shortened), then where it is
// tracked, then the marker.
func issueBody(item Item, phase Phase, source string) string {
	var parts []string
	if full := item.textWithoutKind(); item.Title() != full {
		parts = append(parts, full)
	}
	if len(item.Description) > 0 {
		parts = append(parts, strings.Join(item.Description, "\n"))
	}
	tracked := "Tracked in ROADMAP.md · " + phase.Title
	if source != "" {
		tracked = "Tracked in [ROADMAP.md](" + source + ") · " + phase.Title
	}
	parts = append(parts, tracked, Marker)
	return strings.Join(parts, "\n\n")
}

func milestoneByTitle(milestones []Milestone, title string) (Milestone, bool) {
	for _, m := range milestones {
		if m.Title == title {
			return m, true
		}
	}
	return Milestone{}, false
}

// matchManaged finds the managed issue of an item that has no number yet:
// same title, and same milestone when the milestone exists.
func matchManaged(issues []Issue, item Item, milestone int) (Issue, bool) {
	for _, is := range issues {
		if is.Managed() && is.Title == item.Title() && (milestone == 0 || is.Milestone == milestone) {
			return is, true
		}
	}
	return Issue{}, false
}
