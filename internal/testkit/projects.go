// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package testkit

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/core/roadmap"
	"github.com/GregoireF/ship-it/internal/ports"
)

// FakeProjects is an in-memory ports.Projects holding one project: items
// are numbered from 500, an issue's global id being 1000 + its number (as
// FakeTracker gives them).
type FakeProjects struct {
	// Kind is the project's owner kind, found when asked for "owner/n".
	Kind   string
	Fields []roadmap.ProjectField
	Items  []roadmap.ProjectItem
	// Err, when set, is returned by every call.
	Err   error
	Calls []string
}

var _ ports.Projects = (*FakeProjects)(nil)

func (f *FakeProjects) record(format string, args ...any) {
	f.Calls = append(f.Calls, fmt.Sprintf(format, args...))
}

// ProjectFields implements ports.Projects.
func (f *FakeProjects) ProjectFields(_ context.Context, _ forge.Repository, project forge.Project) (forge.Project, []roadmap.ProjectField, error) {
	if project.Kind == "" {
		project.Kind = f.Kind
		if project.Kind == "" {
			project.Kind = forge.ProjectOfOrganization
		}
	}
	return project, f.Fields, f.Err
}

// ProjectItems implements ports.Projects, copies of the items.
func (f *FakeProjects) ProjectItems(context.Context, forge.Repository, forge.Project, []int64) ([]roadmap.ProjectItem, error) {
	out := make([]roadmap.ProjectItem, len(f.Items))
	for i, it := range f.Items {
		it.Values = maps.Clone(it.Values)
		out[i] = it
	}
	return out, f.Err
}

// AddProjectItem implements ports.Projects.
func (f *FakeProjects) AddProjectItem(_ context.Context, _ forge.Repository, _ forge.Project, issueID int64) (int64, error) {
	if f.Err != nil {
		return 0, f.Err
	}
	item := roadmap.ProjectItem{ID: int64(500 + len(f.Items)), Issue: int(issueID - 1000), Values: map[int64]string{}}
	f.Items = append(f.Items, item)
	f.record("add %d", item.Issue)
	return item.ID, nil
}

// SetProjectField implements ports.Projects.
func (f *FakeProjects) SetProjectField(_ context.Context, _ forge.Repository, _ forge.Project, item, field int64, value string) error {
	if f.Err != nil {
		return f.Err
	}
	for i := range f.Items {
		if f.Items[i].ID == item {
			if value == "" {
				delete(f.Items[i].Values, field)
			} else {
				f.Items[i].Values[field] = value
			}
		}
	}
	f.record("set %d %d %s", item, field, value)
	return nil
}

// RemoveProjectItem implements ports.Projects.
func (f *FakeProjects) RemoveProjectItem(_ context.Context, _ forge.Repository, _ forge.Project, item int64) error {
	f.Items = slices.DeleteFunc(f.Items, func(it roadmap.ProjectItem) bool { return it.ID == item })
	f.record("remove %d", item)
	return f.Err
}

// Value returns the value of a field of the item of an issue, "" if none.
func (f *FakeProjects) Value(issue int, field int64) (string, bool) {
	for _, it := range f.Items {
		if it.Issue == issue {
			return it.Values[field], true
		}
	}
	return "", false
}
