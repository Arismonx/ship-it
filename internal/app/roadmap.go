// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"slices"

	"github.com/GregoireF/ship-it/internal/core/convention"
	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/core/roadmap"
	"github.com/GregoireF/ship-it/internal/ports"
)

// RoadmapFile is the roadmap of a repository, relative to its root.
const RoadmapFile = "ROADMAP.md"

// roadmapReadLimit bounds the roadmap file.
const roadmapReadLimit = 1 << 20

// RoadmapPlan is what `ship roadmap sync` would do; it changes nothing.
type RoadmapPlan struct {
	Repository forge.Repository `json:"repository"`
	Path       string           `json:"path"`
	Changes    []roadmap.Change `json:"changes"`
	// Project is the GitHub Project of github.project, nil when none is set;
	// ProjectProblems are the configured fields and options it lacks, which
	// the sync leaves alone.
	Project         *forge.Project `json:"project,omitempty"`
	ProjectProblems []ProblemView  `json:"project_problems,omitempty"`

	// What the plan was computed from, for ApplyRoadmap.
	parsed       roadmap.Roadmap
	remote       roadmap.Remote
	content      []byte
	projectItems []roadmap.ProjectItem
}

// Empty reports whether GitHub and the file already agree.
func (p RoadmapPlan) Empty() bool { return len(p.Changes) == 0 }

// PlanRoadmap compares the roadmap file with the repository's milestones and
// issues. It only reads.
func PlanRoadmap(ctx context.Context, ws Workspace, path string) (RoadmapPlan, error) {
	if path == "" {
		path = RoadmapFile
	}
	content, err := ws.Git.ReadWorkingFile(ctx, path, roadmapReadLimit)
	if err != nil {
		return RoadmapPlan{}, fault.New(fault.RoadmapMissing, "roadmap.missing", "file", path)
	}
	eff, err := LoadSettings(ctx, ws)
	if err != nil {
		return RoadmapPlan{}, err
	}
	rules := eff.Settings.Conventions() // kinds follow ship.commits.types
	parsed := roadmap.ParseWith(content, rules)
	if len(parsed.Phases) == 0 {
		return RoadmapPlan{}, fault.New(fault.RoadmapMissing, "roadmap.empty", "file", path)
	}
	if ws.Tracker == nil || ws.Forge == nil {
		return RoadmapPlan{}, fault.New(fault.NoGitHubRemote, "publish.no_remote", "remote", gitRemote(ctx, ws))
	}
	repo, err := remoteRepository(ctx, ws)
	if err != nil {
		return RoadmapPlan{}, err
	}
	info, err := ws.Forge.Repository(ctx, repo)
	if err != nil {
		return RoadmapPlan{}, forgeFault(err, repo)
	}
	remote, err := readRoadmapRemote(ctx, ws, repo, parsed)
	if err != nil {
		return RoadmapPlan{}, err
	}
	memory := roadmap.Memory{}
	if ws.RoadmapMemory != nil {
		if memory, err = ws.RoadmapMemory.Load(ctx); err != nil {
			return RoadmapPlan{}, err
		}
	}
	source := "https://" + repo.Host + "/" + repo.Slug() + "/blob/" + info.DefaultBranch + "/" + path
	changes := roadmap.Plan(parsed, remote, memory, roadmap.Options{
		Source: source,
		KindLabel: func(t convention.CommitType) string {
			return eff.Settings.PullRequestLabels().KindLabel(rules, t, remote.Labels)
		},
	})
	plan := RoadmapPlan{Repository: repo, Path: path, Changes: changes, parsed: parsed, remote: remote, content: content}
	if err := planProject(ctx, ws, &plan); err != nil {
		return RoadmapPlan{}, err
	}
	return plan, nil
}

// readRoadmapRemote reads the milestones, the managed issues and those the
// roadmap names, the labels, and the sub-issues of the items that have some.
func readRoadmapRemote(ctx context.Context, ws Workspace, repo forge.Repository, r roadmap.Roadmap) (roadmap.Remote, error) {
	var remote roadmap.Remote
	var err error
	if remote.Milestones, err = ws.Tracker.Milestones(ctx, repo); err != nil {
		return remote, forgeFault(err, repo)
	}
	var numbers []int
	for _, p := range r.Phases {
		for _, it := range p.Items {
			numbers = appendNumber(numbers, it.Number)
			for _, child := range it.Children {
				numbers = appendNumber(numbers, child.Number)
			}
		}
	}
	if remote.Issues, err = ws.Tracker.TrackedIssues(ctx, repo, roadmap.Marker, numbers); err != nil {
		return remote, forgeFault(err, repo)
	}
	if labels, err := ws.Forge.Labels(ctx, repo); err == nil {
		remote.Labels = labelNames(labels)
	}
	// Parents: only the items with children are asked for theirs.
	parentOf := map[int]int{}
	for _, p := range r.Phases {
		for _, it := range p.Items {
			if it.Number == 0 || len(it.Children) == 0 {
				continue
			}
			children, err := ws.Tracker.SubIssues(ctx, repo, it.Number)
			if err != nil {
				return remote, forgeFault(err, repo)
			}
			for _, c := range children {
				parentOf[c] = it.Number
			}
		}
	}
	for i := range remote.Issues {
		remote.Issues[i].Parent = parentOf[remote.Issues[i].Number]
	}
	return remote, nil
}

func appendNumber(numbers []int, n int) []int {
	if n > 0 && !slices.Contains(numbers, n) {
		return append(numbers, n)
	}
	return numbers
}

// RoadmapResult reports what a sync applied.
type RoadmapResult struct {
	Operation string `json:"operation,omitempty"`
	Applied   int    `json:"applied"`
	// FileChanged: issue numbers or boxes were written into the file, to be
	// delivered like any other change (decision of 2026-11-01).
	FileChanged bool `json:"file_changed"`
}

// ApplyRoadmap applies a plan the human reviewed, in order, and stops at the
// first failure. Whatever it did is recorded — the file it rewrote, the
// issues and milestones it created, the states and links it changed — so
// that `ship undo` can compensate it, even after a failure.
func ApplyRoadmap(ctx context.Context, ws Workspace, plan RoadmapPlan) (result RoadmapResult, err error) {
	repo := plan.Repository
	record := &oplog.RoadmapSync{}
	numberOf := map[int]int{}       // item line → issue number
	idOf := map[int]int64{}         // issue number → global id
	open := map[int]bool{}          // issue number → open
	milestoneOf := map[string]int{} // phase title → milestone number
	for _, m := range plan.remote.Milestones {
		milestoneOf[m.Title] = m.Number
	}
	for _, is := range plan.remote.Issues {
		idOf[is.Number], open[is.Number] = is.ID, is.Open
	}
	for _, p := range plan.parsed.Phases {
		for _, it := range p.Items {
			numberOf[it.Line] = it.Number
			for _, c := range it.Children {
				numberOf[c.Line] = c.Number
			}
		}
	}
	var edits []roadmap.Edit

	defer func() {
		if result.Applied == 0 && len(edits) == 0 {
			return
		}
		var before []oplog.FileBefore
		if len(edits) > 0 {
			updated := roadmap.Annotate(plan.content, edits)
			if werr := ws.Git.WriteWorkingFile(ctx, plan.Path, updated); werr != nil {
				err = errors.Join(err, werr)
			} else {
				result.FileChanged = true
				before = []oplog.FileBefore{{Path: plan.Path, Content: string(plan.content), Existed: true}}
			}
		}
		if ws.RoadmapMemory != nil {
			err = errors.Join(err, ws.RoadmapMemory.Save(ctx, finalMemory(plan, numberOf, edits)))
		}
		op, lerr := ws.Operations.Append(ctx, oplog.Operation{
			Kind: oplog.Roadmap, Title: plan.Path, Remote: gitRemote(ctx, ws), FilesBefore: before, RoadmapSync: record,
		})
		result.Operation = op.ID
		err = errors.Join(err, lerr)
	}()

	number := func(c roadmap.Change) int {
		if c.Number != 0 {
			return c.Number
		}
		return numberOf[c.Line]
	}
	// The project: items by issue, the values they had, what the sync adds.
	itemOf := map[int]int64{}
	valuesOf := map[int64]map[int64]string{}
	for _, it := range plan.projectItems {
		itemOf[it.Issue], valuesOf[it.ID] = it.ID, it.Values
	}
	added := map[int64]bool{}
	projectRecord := func() *oplog.ProjectSync {
		if record.Project == nil {
			record.Project = &oplog.ProjectSync{Project: *plan.Project}
		}
		return record.Project
	}
	setState := func(n int, update ports.IssueUpdate) error {
		record.States = append(record.States, oplog.IssueState{Number: n, WasOpen: open[n]})
		open[n] = update.State == "open"
		return ws.Tracker.UpdateIssue(ctx, repo, n, update)
	}
	for _, c := range plan.Changes {
		var stepErr error
		switch c.Kind {
		case roadmap.CreateMilestone:
			var n int
			n, stepErr = ws.Tracker.CreateMilestone(ctx, repo, roadmap.Milestone{Title: c.Phase, Description: c.Description, Due: c.Due})
			if stepErr == nil {
				milestoneOf[c.Phase] = n
				record.CreatedMilestones = append(record.CreatedMilestones, n)
			}
		case roadmap.UpdateMilestone:
			stepErr = ws.Tracker.UpdateMilestone(ctx, repo, roadmap.Milestone{Number: c.Number, Title: c.Phase, Description: c.Description, Due: c.Due})
		case roadmap.CreateIssue:
			var is roadmap.Issue
			is, stepErr = ws.Tracker.CreateIssue(ctx, repo, c.Title, c.Body, milestoneOf[c.Phase], c.Labels)
			if stepErr == nil {
				numberOf[c.Line], idOf[is.Number], open[is.Number] = is.Number, is.ID, true
				record.CreatedIssues = append(record.CreatedIssues, oplog.CreatedIssue{Number: is.Number, Milestone: milestoneOf[c.Phase]})
				edits = append(edits, roadmap.Edit{Line: c.Line, Number: is.Number})
				if parent := c.ParentNumber; parent != 0 || c.ParentLine != 0 {
					if parent == 0 {
						parent = numberOf[c.ParentLine]
					}
					if parent != 0 {
						stepErr = ws.Tracker.SetParent(ctx, repo, parent, is.ID)
						record.Parents = append(record.Parents, oplog.ParentLink{Parent: parent, Child: is.ID})
					}
				}
			}
		case roadmap.UpdateIssue:
			milestone := milestoneOf[c.Phase]
			update := ports.IssueUpdate{Milestone: &milestone}
			if c.Title != "" {
				update.Title, update.Body = &c.Title, &c.Body
			}
			stepErr = ws.Tracker.UpdateIssue(ctx, repo, c.Number, update)
		case roadmap.CloseIssue:
			stepErr = setState(number(c), ports.IssueUpdate{State: "closed", StateReason: "completed"})
		case roadmap.ReopenIssue:
			stepErr = setState(number(c), ports.IssueUpdate{State: "open", StateReason: "reopened"})
		case roadmap.CloseNotPlanned:
			if stepErr = ws.Tracker.CommentIssue(ctx, repo, c.Number, "Removed from ROADMAP.md: closed as not planned by `ship roadmap sync`."); stepErr == nil {
				stepErr = setState(c.Number, ports.IssueUpdate{State: "closed", StateReason: "not_planned"})
			}
		case roadmap.SetParent:
			parent := c.ParentNumber
			if parent == 0 {
				parent = numberOf[c.ParentLine]
			}
			if child := idOf[c.Number]; parent != 0 && child != 0 {
				stepErr = ws.Tracker.SetParent(ctx, repo, parent, child)
				record.Parents = append(record.Parents, oplog.ParentLink{Parent: parent, Child: child})
			}
		case roadmap.TickBox:
			edits = append(edits, roadmap.Edit{Line: c.Line, Check: true})
		case roadmap.UntickBox:
			edits = append(edits, roadmap.Edit{Line: c.Line, Uncheck: true})
		case roadmap.WriteIssueNumber:
			numberOf[c.Line] = c.Number
			edits = append(edits, roadmap.Edit{Line: c.Line, Number: c.Number})
		case roadmap.AddToProject:
			n := number(c)
			var item int64
			if item, stepErr = ws.Projects.AddProjectItem(ctx, repo, *plan.Project, idOf[n]); stepErr == nil {
				itemOf[n], added[item] = item, true
				projectRecord().AddedItems = append(projectRecord().AddedItems, item)
			}
		case roadmap.SetProjectField:
			item := c.Item
			if item == 0 {
				item = itemOf[number(c)]
			}
			if stepErr = ws.Projects.SetProjectField(ctx, repo, *plan.Project, item, c.FieldID, c.ValueID); stepErr == nil && !added[item] {
				// An item the sync added goes away whole on undo.
				projectRecord().Values = append(projectRecord().Values, oplog.FieldValue{Item: item, Field: c.FieldID, Before: valuesOf[item][c.FieldID]})
			}
		}
		if stepErr != nil {
			if plan.Project != nil && (c.Kind == roadmap.AddToProject || c.Kind == roadmap.SetProjectField) {
				return result, projectFault(stepErr, repo, *plan.Project)
			}
			return result, forgeFault(stepErr, repo)
		}
		result.Applied++
	}
	return result, nil
}

// finalMemory is the closed state of every item after the sync, where the
// file and GitHub agree: the box, unless the sync ticked or unticked it.
func finalMemory(plan RoadmapPlan, numberOf map[int]int, edits []roadmap.Edit) roadmap.Memory {
	ticked := map[int]bool{}
	for _, e := range edits {
		switch {
		case e.Check:
			ticked[e.Line] = true
		case e.Uncheck:
			ticked[e.Line] = false
		}
	}
	memory := roadmap.Memory{}
	note := func(it roadmap.Item) {
		n := numberOf[it.Line]
		if n == 0 {
			return
		}
		closed := it.Done
		if v, ok := ticked[it.Line]; ok {
			closed = v
		}
		memory[n] = closed
	}
	for _, p := range plan.parsed.Phases {
		for _, it := range p.Items {
			note(it)
			for _, c := range it.Children {
				note(c)
			}
		}
	}
	return memory
}
