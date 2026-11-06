// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package testkit

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/core/roadmap"
	"github.com/GregoireF/ship-it/internal/ports"
)

// FakeTracker is an in-memory ports.Tracker: milestones and issues numbered
// from 1, sub-issue links by parent number, and every call recorded.
type FakeTracker struct {
	MilestoneList []roadmap.Milestone
	IssueList     []roadmap.Issue
	// Children maps a parent issue number to its sub-issue numbers.
	Children map[int][]int
	Comments map[int][]string
	Calls    []string
}

var _ ports.Tracker = (*FakeTracker)(nil)

func (f *FakeTracker) record(format string, args ...any) {
	f.Calls = append(f.Calls, fmt.Sprintf(format, args...))
}

// Milestones implements ports.Tracker, with the issue counts of each.
func (f *FakeTracker) Milestones(context.Context, forge.Repository) ([]roadmap.Milestone, error) {
	out := slices.Clone(f.MilestoneList)
	for i := range out {
		out[i].OpenIssues, out[i].ClosedIssues = 0, 0
		for _, is := range f.IssueList {
			if is.Milestone == out[i].Number {
				if is.Open {
					out[i].OpenIssues++
				} else {
					out[i].ClosedIssues++
				}
			}
		}
	}
	return out, nil
}

// CreateMilestone implements ports.Tracker.
func (f *FakeTracker) CreateMilestone(_ context.Context, _ forge.Repository, m roadmap.Milestone) (int, error) {
	m.Number, m.Open = len(f.MilestoneList)+1, true
	f.MilestoneList = append(f.MilestoneList, m)
	f.record("create milestone %d %s", m.Number, m.Title)
	return m.Number, nil
}

// UpdateMilestone implements ports.Tracker.
func (f *FakeTracker) UpdateMilestone(_ context.Context, _ forge.Repository, m roadmap.Milestone) error {
	for i := range f.MilestoneList {
		if f.MilestoneList[i].Number == m.Number {
			f.MilestoneList[i].Description, f.MilestoneList[i].Due = m.Description, m.Due
		}
	}
	f.record("update milestone %d", m.Number)
	return nil
}

// DeleteMilestone implements ports.Tracker.
func (f *FakeTracker) DeleteMilestone(_ context.Context, _ forge.Repository, number int) error {
	f.MilestoneList = slices.DeleteFunc(f.MilestoneList, func(m roadmap.Milestone) bool { return m.Number == number })
	f.record("delete milestone %d", number)
	return nil
}

// TrackedIssues implements ports.Tracker.
func (f *FakeTracker) TrackedIssues(_ context.Context, _ forge.Repository, marker string, numbers []int) ([]roadmap.Issue, error) {
	var out []roadmap.Issue
	for _, is := range f.IssueList {
		if strings.Contains(is.Body, marker) || slices.Contains(numbers, is.Number) {
			out = append(out, is)
		}
	}
	return out, nil
}

// SubIssues implements ports.Tracker.
func (f *FakeTracker) SubIssues(_ context.Context, _ forge.Repository, parent int) ([]int, error) {
	return f.Children[parent], nil
}

// CreateIssue implements ports.Tracker; ids are 1000 + number.
func (f *FakeTracker) CreateIssue(_ context.Context, _ forge.Repository, title, body string, milestone int, labels []string) (roadmap.Issue, error) {
	is := roadmap.Issue{Number: len(f.IssueList) + 1, Title: title, Body: body, Open: true, Milestone: milestone, Labels: labels}
	is.ID = int64(1000 + is.Number)
	f.IssueList = append(f.IssueList, is)
	f.record("create issue %d %s", is.Number, title)
	return is, nil
}

// UpdateIssue implements ports.Tracker.
func (f *FakeTracker) UpdateIssue(_ context.Context, _ forge.Repository, number int, u ports.IssueUpdate) error {
	for i := range f.IssueList {
		is := &f.IssueList[i]
		if is.Number != number {
			continue
		}
		if u.Title != nil {
			is.Title = *u.Title
		}
		if u.Body != nil {
			is.Body = *u.Body
		}
		if u.Milestone != nil {
			is.Milestone = *u.Milestone
		}
		if u.State != "" {
			is.Open = u.State == "open"
		}
	}
	f.record("update issue %d %s %s", number, u.State, u.StateReason)
	return nil
}

// CommentIssue implements ports.Tracker.
func (f *FakeTracker) CommentIssue(_ context.Context, _ forge.Repository, number int, comment string) error {
	if f.Comments == nil {
		f.Comments = map[int][]string{}
	}
	f.Comments[number] = append(f.Comments[number], comment)
	return nil
}

// Issue implements ports.Tracker.
func (f *FakeTracker) Issue(_ context.Context, _ forge.Repository, number int) (roadmap.Issue, error) {
	for _, is := range f.IssueList {
		if is.Number == number {
			return is, nil
		}
	}
	return roadmap.Issue{}, ports.ErrNotFound
}

// AddAssignees implements ports.Tracker.
func (f *FakeTracker) AddAssignees(_ context.Context, _ forge.Repository, number int, logins []string) error {
	for i := range f.IssueList {
		if f.IssueList[i].Number == number {
			f.IssueList[i].Assignees = append(f.IssueList[i].Assignees, logins...)
		}
	}
	f.record("assign %d %s", number, strings.Join(logins, ","))
	return nil
}

// RemoveAssignees implements ports.Tracker.
func (f *FakeTracker) RemoveAssignees(_ context.Context, _ forge.Repository, number int, logins []string) error {
	for i := range f.IssueList {
		if f.IssueList[i].Number == number {
			f.IssueList[i].Assignees = slices.DeleteFunc(f.IssueList[i].Assignees, func(l string) bool { return slices.Contains(logins, l) })
		}
	}
	f.record("unassign %d %s", number, strings.Join(logins, ","))
	return nil
}

// SetParent implements ports.Tracker.
func (f *FakeTracker) SetParent(_ context.Context, _ forge.Repository, parent int, child int64) error {
	if f.Children == nil {
		f.Children = map[int][]int{}
	}
	f.Children[parent] = append(f.Children[parent], int(child-1000))
	f.record("parent %d of %d", parent, child-1000)
	return nil
}

// RemoveParent implements ports.Tracker.
func (f *FakeTracker) RemoveParent(_ context.Context, _ forge.Repository, parent int, child int64) error {
	f.Children[parent] = slices.DeleteFunc(f.Children[parent], func(n int) bool { return n == int(child-1000) })
	f.record("unparent %d of %d", parent, child-1000)
	return nil
}

// FakeRoadmapMemory is an in-memory ports.RoadmapMemory.
type FakeRoadmapMemory struct{ Memory roadmap.Memory }

var _ ports.RoadmapMemory = (*FakeRoadmapMemory)(nil)

// Load implements ports.RoadmapMemory.
func (m *FakeRoadmapMemory) Load(context.Context) (roadmap.Memory, error) {
	out := roadmap.Memory{}
	for k, v := range m.Memory {
		out[k] = v
	}
	return out, nil
}

// Save implements ports.RoadmapMemory.
func (m *FakeRoadmapMemory) Save(_ context.Context, memory roadmap.Memory) error {
	m.Memory = memory
	return nil
}
