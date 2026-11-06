// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package roadmap

import (
	"slices"
	"strings"
)

// Field types of a GitHub Project (v2), as the REST API names them
// (data_type).
const (
	SingleSelectField = "single_select"
	DateField         = "date"
)

// ProjectField is a field of a project.
type ProjectField struct {
	ID   int64
	Name string
	// Type is the API's data_type: "single_select", "date", "text"…
	Type    string
	Options []FieldOption
}

// FieldOption is an option of a single-select field.
type FieldOption struct {
	ID   string
	Name string
}

// ProjectItem is an issue of the repository in the project.
type ProjectItem struct {
	ID    int64
	Issue int // its number
	// Values are the raw values of the fields asked for, by field id: the
	// option id of a single select, YYYY-MM-DD for a date; absent or "" when
	// empty.
	Values map[int64]string
}

// Project is what the sync knows of the project: its fields, and the items
// that are issues of the repository.
type Project struct {
	Fields []ProjectField
	Items  []ProjectItem
}

// ProjectOptions are the github.project settings: field and option names,
// "" for a field that is off.
type ProjectOptions struct {
	StatusField, Todo, Done string
	DateField, PhaseField   string
}

// Project steps, after every issue step of the plan.
const (
	// AddToProject adds the issue (Number, or the issue created for Line).
	AddToProject ChangeKind = "add_to_project"
	// SetProjectField sets Field (FieldID) of the issue's item to Value,
	// ValueID being what the API takes (option id, date).
	SetProjectField ChangeKind = "set_project_field"
)

// ProblemKind names what the project lacks.
type ProblemKind string

// Problems, reported in the plan: ship-it never creates fields or options.
const (
	FieldMissing ProblemKind = "field_missing"
	// FieldWrongType: the field exists with another type than Type.
	FieldWrongType ProblemKind = "field_wrong_type"
	// OptionMissing: the single-select field has no option named Value (the
	// REST API cannot add one to an existing field).
	OptionMissing ProblemKind = "option_missing"
)

// ProjectProblem is a configured field or value the project does not have.
type ProjectProblem struct {
	Kind  ProblemKind `json:"kind"`
	Field string      `json:"field"`
	// Value is the missing option; Type the type the field should have.
	Value string `json:"value,omitempty"`
	Type  string `json:"type,omitempty"`
}

// FindField returns the field named name (without regard to case) when it
// has type typ, else the problem.
func FindField(fields []ProjectField, name, typ string) (ProjectField, *ProjectProblem) {
	for _, f := range fields {
		if strings.EqualFold(f.Name, name) {
			if f.Type != typ {
				return ProjectField{}, &ProjectProblem{Kind: FieldWrongType, Field: name, Type: typ}
			}
			return f, nil
		}
	}
	return ProjectField{}, &ProjectProblem{Kind: FieldMissing, Field: name, Type: typ}
}

// FindOption returns the option named name (without regard to case) of a
// single-select field, else the problem.
func FindOption(field ProjectField, name string) (FieldOption, *ProjectProblem) {
	for _, o := range field.Options {
		if strings.EqualFold(o.Name, name) {
			return o, nil
		}
	}
	return FieldOption{}, &ProjectProblem{Kind: OptionMissing, Field: field.Name, Value: name}
}

// PlanProject returns the project steps that follow plan (the issue steps of
// Plan for the same roadmap and remote): every roadmap issue is in the
// project — the existing ones too, as GitHub's auto-add ignores them —, a
// closed issue has the done status, an open one without status gets todo (a
// status set by hand on an open issue is kept), the date field holds the
// milestone's due date and the phase field the milestone's title. A field or
// an option the project lacks is reported once, never invented.
func PlanProject(r Roadmap, remote Remote, plan []Change, project Project, opts ProjectOptions) ([]Change, []ProjectProblem) {
	var problems []ProjectProblem
	report := func(p *ProjectProblem) {
		if p != nil && !slices.Contains(problems, *p) {
			problems = append(problems, *p)
		}
	}
	var status, date, phase *ProjectField
	var todo, done *FieldOption
	if opts.StatusField != "" {
		f, problem := FindField(project.Fields, opts.StatusField, SingleSelectField)
		report(problem)
		if problem == nil {
			status = &f
			if opts.Todo != "" {
				o, problem := FindOption(f, opts.Todo)
				report(problem)
				if problem == nil {
					todo = &o
				}
			}
			if opts.Done != "" {
				o, problem := FindOption(f, opts.Done)
				report(problem)
				if problem == nil {
					done = &o
				}
			}
		}
	}
	if opts.DateField != "" {
		f, problem := FindField(project.Fields, opts.DateField, DateField)
		report(problem)
		if problem == nil {
			date = &f
		}
	}
	if opts.PhaseField != "" {
		f, problem := FindField(project.Fields, opts.PhaseField, SingleSelectField)
		report(problem)
		if problem == nil {
			phase = &f
		}
	}

	open := map[int]bool{}
	for _, is := range remote.Issues {
		open[is.Number] = is.Open
	}
	created := map[int]bool{}     // line → created by the plan
	numberOf := map[int]int{}     // line → number written back
	closedAfter := map[int]bool{} // line → state the plan gives it
	for _, c := range plan {
		switch c.Kind {
		case CreateIssue:
			created[c.Line] = true
		case WriteIssueNumber:
			numberOf[c.Line] = c.Number
		case CloseIssue:
			closedAfter[c.Line] = true
		case ReopenIssue:
			closedAfter[c.Line] = false
		}
	}
	items := map[int]ProjectItem{}
	for _, it := range project.Items {
		items[it.Issue] = it
	}

	var steps []Change
	visit := func(item Item, ph Phase) {
		number := item.Number
		if created[item.Line] {
			number = 0
		} else if number == 0 {
			number = numberOf[item.Line]
		}
		if number == 0 && !created[item.Line] {
			return // not resolved: Plan creates it
		}
		closed := !open[number]
		if created[item.Line] {
			closed = false
		}
		if v, ok := closedAfter[item.Line]; ok {
			closed = v
		}
		pi, inProject := items[number]
		if number == 0 || !inProject {
			steps = append(steps, Change{Kind: AddToProject, Phase: ph.Title, Line: item.Line, Number: number, Title: item.Title()})
		}
		set := func(f *ProjectField, value, valueID string) {
			if pi.Values[f.ID] == valueID {
				return
			}
			steps = append(steps, Change{Kind: SetProjectField, Phase: ph.Title, Line: item.Line, Number: number, Item: pi.ID,
				Field: f.Name, FieldID: f.ID, Value: value, ValueID: valueID})
		}
		if status != nil {
			switch {
			case closed && done != nil:
				set(status, done.Name, done.ID)
			case !closed && todo != nil && pi.Values[status.ID] == "":
				set(status, todo.Name, todo.ID)
			}
		}
		if date != nil && ph.Due != "" {
			set(date, ph.Due, ph.Due)
		}
		if phase != nil {
			o, problem := FindOption(*phase, ph.Title)
			report(problem)
			if problem == nil {
				set(phase, o.Name, o.ID)
			}
		}
	}
	for _, ph := range r.Phases {
		for _, item := range ph.Items {
			visit(item, ph)
			for _, child := range item.Children {
				visit(child, ph)
			}
		}
	}
	return steps, problems
}
