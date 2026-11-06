// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package ports

import (
	"context"

	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/core/roadmap"
)

// IssueUpdate is a change to an issue; nil fields are left as they are.
type IssueUpdate struct {
	Title     *string
	Body      *string
	Milestone *int // 0 removes the milestone
	// State is "open" or "closed"; StateReason "completed", "not_planned"
	// or "reopened" (GitHub's values).
	State       string
	StateReason string
}

// Tracker is GitHub's planning side: milestones, issues and sub-issues, for
// `ship roadmap sync` (ADR-0016).
type Tracker interface {
	// Milestones returns every milestone, open and closed.
	Milestones(ctx context.Context, repo forge.Repository) ([]roadmap.Milestone, error)
	CreateMilestone(ctx context.Context, repo forge.Repository, m roadmap.Milestone) (int, error)
	UpdateMilestone(ctx context.Context, repo forge.Repository, m roadmap.Milestone) error
	// DeleteMilestone removes a milestone (undo of one ship-it created).
	DeleteMilestone(ctx context.Context, repo forge.Repository, number int) error
	// TrackedIssues returns the issues whose body holds marker, and the
	// issues numbers names, open and closed, pull requests excluded.
	TrackedIssues(ctx context.Context, repo forge.Repository, marker string, numbers []int) ([]roadmap.Issue, error)
	// SubIssues returns the numbers of the sub-issues of parent.
	SubIssues(ctx context.Context, repo forge.Repository, parent int) ([]int, error)
	CreateIssue(ctx context.Context, repo forge.Repository, title, body string, milestone int, labels []string) (roadmap.Issue, error)
	UpdateIssue(ctx context.Context, repo forge.Repository, number int, update IssueUpdate) error
	CommentIssue(ctx context.Context, repo forge.Repository, number int, comment string) error
	// Issue returns one issue, its assignees included.
	Issue(ctx context.Context, repo forge.Repository, number int) (roadmap.Issue, error)
	// AddAssignees assigns logins to an issue, keeping its other assignees.
	AddAssignees(ctx context.Context, repo forge.Repository, number int, logins []string) error
	// RemoveAssignees unassigns logins from an issue, keeping the others
	// (undo of a publication that assigned the issue).
	RemoveAssignees(ctx context.Context, repo forge.Repository, number int, logins []string) error
	// SetParent makes child (by its global id) a sub-issue of parent,
	// replacing any other parent.
	SetParent(ctx context.Context, repo forge.Repository, parent int, child int64) error
	// RemoveParent detaches child from parent (undo).
	RemoveParent(ctx context.Context, repo forge.Repository, parent int, child int64) error
}

// RoadmapMemory keeps the closed state of each roadmap issue at the last
// sync, locally (.git/ship/roadmap.json), so that the next sync knows who
// changed a state since (roadmap.Memory).
type RoadmapMemory interface {
	// Load returns an empty memory when there is none yet.
	Load(ctx context.Context) (roadmap.Memory, error)
	Save(ctx context.Context, m roadmap.Memory) error
}
