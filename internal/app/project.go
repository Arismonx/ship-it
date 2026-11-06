// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/config"
	"github.com/GregoireF/ship-it/internal/core/convention"
	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/core/roadmap"
	"github.com/GregoireF/ship-it/internal/ports"
)

// configuredProject reads github.project.url: on is false when it is empty.
func configuredProject(s config.ProjectSettings, repo forge.Repository) (project forge.Project, on bool, err error) {
	if strings.TrimSpace(s.URL) == "" {
		return forge.Project{}, false, nil
	}
	project, ok := forge.ParseProject(s.URL)
	if !ok { // the settings validate the shape: only a direct caller gets here
		return project, true, fault.New(fault.ConfigInvalidValue, "config.invalid_value",
			"file", "settings", "key", "github.project.url", "value", s.URL, "allowed", "a project URL or owner/number")
	}
	if project.Host != "" && !strings.EqualFold(project.Host, repo.Host) {
		return project, true, fault.New(fault.ProjectOtherHost, "project.other_host",
			"project", s.URL, "host", project.Host, "origin", repo.Host)
	}
	return project, true, nil
}

// projectFault turns the errors of a project call into faults with their
// remedy: the scope or permission it lacks, or a project it cannot see.
func projectFault(err error, repo forge.Repository, project forge.Project) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ports.ErrProjectScope):
		return fault.New(fault.ProjectAccessDenied, "project.scope_missing", "project", project.String(), "host", repo.Host)
	case errors.Is(err, ports.ErrForbidden):
		return fault.New(fault.ProjectAccessDenied, "project.forbidden", "project", project.String(), "host", repo.Host)
	case errors.Is(err, ports.ErrNotFound):
		return fault.New(fault.ProjectNotFound, "project.not_found", "project", project.String(), "host", repo.Host)
	}
	return forgeFault(err, repo)
}

// projectProblem renders what the project lacks, for the plan and the
// warnings of ship publish.
func projectProblem(p roadmap.ProjectProblem, project forge.Project) ProblemView {
	return ProblemView{Code: string(fault.ProjectIncomplete), Key: "project.problem." + string(p.Kind),
		Params: map[string]string{"project": project.String(), "field": p.Field, "value": p.Value, "type": p.Type}}
}

func projectOptions(s config.ProjectSettings) roadmap.ProjectOptions {
	return roadmap.ProjectOptions{StatusField: s.StatusField, Todo: s.Todo, Done: s.Done, DateField: s.DateField, PhaseField: s.PhaseField}
}

// wantedFields are the ids of the configured fields the project has: the
// items are listed with their values.
func wantedFields(fields []roadmap.ProjectField, opts roadmap.ProjectOptions) []int64 {
	var ids []int64
	for _, want := range [][2]string{
		{opts.StatusField, roadmap.SingleSelectField}, {opts.DateField, roadmap.DateField}, {opts.PhaseField, roadmap.SingleSelectField},
	} {
		if want[0] == "" {
			continue
		}
		if f, problem := roadmap.FindField(fields, want[0], want[1]); problem == nil && !slices.Contains(ids, f.ID) {
			ids = append(ids, f.ID)
		}
	}
	return ids
}

// planProject adds the project steps to a roadmap plan, when github.project
// is set.
func planProject(ctx context.Context, ws Workspace, plan *RoadmapPlan) error {
	if ws.Projects == nil {
		return nil
	}
	eff, err := LoadSettings(ctx, ws)
	if err != nil {
		return err
	}
	s := eff.Settings.GitHub.Project
	project, on, err := configuredProject(s, plan.Repository)
	if !on || err != nil {
		return err
	}
	project, fields, err := ws.Projects.ProjectFields(ctx, plan.Repository, project)
	if err != nil {
		return projectFault(err, plan.Repository, project)
	}
	opts := projectOptions(s)
	items, err := ws.Projects.ProjectItems(ctx, plan.Repository, project, wantedFields(fields, opts))
	if err != nil {
		return projectFault(err, plan.Repository, project)
	}
	steps, problems := roadmap.PlanProject(plan.parsed, plan.remote, plan.Changes, roadmap.Project{Fields: fields, Items: items}, opts)
	plan.Changes = append(plan.Changes, steps...)
	plan.Project = &project
	for _, p := range problems {
		plan.ProjectProblems = append(plan.ProjectProblems, projectProblem(p, project))
	}
	plan.projectItems = items
	return nil
}

// moveLinkedIssue puts the issue a new pull request closes "in progress" in
// the project, and assigns it to the pull request's author when nobody has
// it, if the issue is in the project (decision of 2026-11-01). The pull
// request exists already: every problem is a warning.
//
// It returns what it changed, for the operation log, so that an undo of the
// publication puts it back; nil when it changed nothing.
func moveLinkedIssue(ctx context.Context, ws Workspace, repo forge.Repository, eff config.Effective, issue int, pr ports.PullRequest, result *PublishResult) *oplog.LinkedIssueChange {
	if issue == 0 || ws.Projects == nil || ws.Tracker == nil ||
		!convention.IssueKeyword(eff.Settings.Ship.Issues.Keyword).ClosesOnMerge() {
		return nil
	}
	s := eff.Settings.GitHub.Project
	warn := func(err error) { result.Warnings = append(result.Warnings, *problemOf(err)) }
	project, on, err := configuredProject(s, repo)
	if !on {
		return nil
	}
	if err != nil {
		warn(err)
		return nil
	}
	project, fields, err := ws.Projects.ProjectFields(ctx, repo, project)
	if err != nil {
		warn(projectFault(err, repo, project))
		return nil
	}
	var status roadmap.ProjectField
	var progress roadmap.FieldOption
	var problem *roadmap.ProjectProblem
	var ids []int64
	if s.StatusField != "" && s.InProgress != "" {
		if status, problem = roadmap.FindField(fields, s.StatusField, roadmap.SingleSelectField); problem == nil {
			ids = []int64{status.ID}
			progress, problem = roadmap.FindOption(status, s.InProgress)
		}
	}
	items, err := ws.Projects.ProjectItems(ctx, repo, project, ids)
	if err != nil {
		warn(projectFault(err, repo, project))
		return nil
	}
	var item *roadmap.ProjectItem
	for i := range items {
		if items[i].Issue == issue {
			item = &items[i]
		}
	}
	if item == nil {
		return nil // not in the project: nothing to move
	}
	change := oplog.LinkedIssueChange{Issue: issue, Project: project, Item: item.ID}
	switch {
	case problem != nil:
		result.Warnings = append(result.Warnings, projectProblem(*problem, project))
	case s.StatusField == "" || s.InProgress == "":
	case item.Values[status.ID] != progress.ID:
		if err := ws.Projects.SetProjectField(ctx, repo, project, item.ID, status.ID, progress.ID); err != nil {
			warn(projectFault(err, repo, project))
		} else {
			result.ProjectStatus = progress.Name
			change.Field, change.Before, change.After = status.ID, item.Values[status.ID], progress.ID
		}
	}
	if assigned := assignAuthor(ctx, ws, repo, issue, pr, result); assigned != "" {
		change.Assigned = assigned
	}
	if change.Field == 0 && change.Assigned == "" {
		return nil
	}
	return &change
}

// assignAuthor assigns the linked issue to the pull request's author when
// nobody has it; it returns the login it added.
func assignAuthor(ctx context.Context, ws Workspace, repo forge.Repository, issue int, pr ports.PullRequest, result *PublishResult) string {
	if pr.Author == "" {
		return ""
	}
	is, err := ws.Tracker.Issue(ctx, repo, issue)
	if err != nil {
		result.Warnings = append(result.Warnings, *problemOf(forgeFault(err, repo)))
		return ""
	}
	if len(is.Assignees) > 0 {
		return ""
	}
	if err := ws.Tracker.AddAssignees(ctx, repo, issue, []string{pr.Author}); err != nil {
		result.Warnings = append(result.Warnings, *problemOf(forgeFault(err, repo)))
		return ""
	}
	result.Assigned = pr.Author
	return pr.Author
}
