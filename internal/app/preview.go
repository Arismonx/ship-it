// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/codeowners"
	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/core/pullrequest"
	"github.com/GregoireF/ship-it/internal/core/reviewers"
	"github.com/GregoireF/ship-it/internal/core/risk"
	"github.com/GregoireF/ship-it/internal/core/snapshot"
	"github.com/GregoireF/ship-it/internal/ports"
)

// PreviewView is the pull request ship publish would open, as reviewers
// will read it, computed without sending anything (#43, #44).
type PreviewView struct {
	Repository forge.Repository `json:"repository"`
	Head       string           `json:"head"`
	Base       string           `json:"base"`
	Title      string           `json:"title,omitempty"`
	Body       string           `json:"body,omitempty"`
	Draft      bool             `json:"draft,omitempty"`
	// Labels are the kind/ and size/ labels it would get, among the
	// repository's own.
	Labels []string `json:"labels,omitempty"`
	// Existing is the pull request already open for the branch: publishing
	// pushes to it and leaves its title and body as they are.
	Existing *ports.PullRequest `json:"existing,omitempty"`
	// Warnings are what the publication would warn about (overlapping pull
	// requests, rulesets…).
	Warnings []ProblemView `json:"warnings,omitempty"`
	// Risk is how carefully it needs reading, and why (#45); nil when the
	// diff could not be read.
	Risk *risk.Assessment `json:"risk,omitempty"`
	// Reviewers are suggested, never requested (#46).
	Reviewers *ReviewerSuggestion `json:"reviewers,omitempty"`
}

// ReviewerSuggestion is who could review: the CODEOWNERS owners of the
// changed files (GitHub requests them itself once the pull request is
// open), and the people who changed those files most in the last months.
type ReviewerSuggestion struct {
	Owners []string           `json:"owners,omitempty"`
	People []reviewers.Person `json:"people,omitempty"`
}

// Suggestion limits: how far back, how many people, how many accounts
// asked of GitHub.
const (
	reviewHistory = "6 months ago"
	reviewPeople  = 3
	reviewLookups = 5
)

// suggestReviewers reads CODEOWNERS on the base branch, then the history of
// the changed files there; nil when there is nothing to suggest.
func suggestReviewers(ctx context.Context, ws Workspace, repo forge.Repository, baseRef string, paths []string) *ReviewerSuggestion {
	if len(paths) == 0 {
		return nil
	}
	authorEmail, _, _ := ws.Git.ConfigValue(ctx, "user.email")
	var authorLogin string
	if ws.Forge != nil {
		if me, err := ws.Forge.CurrentUser(ctx, repo); err == nil {
			authorLogin = me.Login
		}
	}
	s := &ReviewerSuggestion{}
	for _, location := range codeowners.Locations {
		if content, err := ws.Git.ReadFileAt(ctx, baseRef, location, codeowners.MaxBytes); err == nil {
			s.Owners = reviewers.Owners(codeowners.Parse(content), paths, authorLogin)
			break
		}
	}
	if authors, err := ws.Git.RecentAuthors(ctx, baseRef, reviewHistory, paths); err == nil {
		commits := make([]reviewers.Commit, 0, len(authors))
		for _, a := range authors {
			commits = append(commits, reviewers.Commit{SHA: a.SHA, Name: a.Name, Email: a.Email})
		}
		lookups := 0
		for _, p := range reviewers.Rank(commits, authorEmail, authorLogin) {
			if p.Login == "" && ws.Forge != nil && lookups < reviewLookups {
				lookups++
				login, err := ws.Forge.CommitAuthorLogin(ctx, repo, p.SHA)
				if err != nil {
					lookups = reviewLookups // offline or refused: names and emails only
				}
				p.Login = login
			}
			if authorLogin != "" && strings.EqualFold(p.Login, authorLogin) {
				continue
			}
			s.People = append(s.People, p)
			if len(s.People) == reviewPeople {
				break
			}
		}
	}
	if len(s.Owners) == 0 && len(s.People) == 0 {
		return nil
	}
	return s
}

// Preview rehearses ship publish and returns the pull request it would open:
// the very same code builds the title, the body, the labels and the base, so
// the preview cannot drift from the publication. Nothing is pushed, opened or
// changed; GitHub is only read.
func Preview(ctx context.Context, ws Workspace, req PublishRequest) (PreviewView, error) {
	rehearsed := DryRun(ws)
	result, err := Publish(ctx, rehearsed, req)
	if err != nil {
		return PreviewView{}, err
	}
	view := PreviewView{Repository: result.Repository, Head: result.Branch, Base: result.Base, Labels: result.Labels, Warnings: result.Warnings}
	if result.PullRequestRepository != nil {
		view.Repository = *result.PullRequestRepository
	}
	rehearsed.rehearsal.mu.Lock()
	spec := rehearsed.rehearsal.pullRequest
	rehearsed.rehearsal.mu.Unlock()
	switch {
	case spec != nil:
		view.Title, view.Body, view.Draft = spec.Title, spec.Body, spec.Draft
	case !result.Created:
		existing := result.PullRequest
		view.Existing = &existing
	}
	baseRef := gitRemote(ctx, ws) + "/" + view.Base
	view.Risk = assessRisk(ctx, ws, baseRef)
	if stats, err := ws.Git.DiffStatSince(ctx, baseRef); err == nil {
		paths := make([]string, 0, len(stats))
		for _, s := range stats {
			paths = append(paths, s.Path)
		}
		view.Reviewers = suggestReviewers(ctx, ws, view.Repository, baseRef, paths)
	}
	return view, nil
}

// assessRisk scores the branch's work on baseRef from facts git gives: the
// changed lines (generated files aside), the files and the declarations
// removed. A diff that cannot be read gives no score rather than a wrong one.
func assessRisk(ctx context.Context, ws Workspace, baseRef string) *risk.Assessment {
	diff, err := ws.Git.DiffSince(ctx, baseRef)
	if err != nil {
		return nil
	}
	stats, err := ws.Git.DiffStatSince(ctx, baseRef)
	if err != nil {
		return nil
	}
	changes := pullrequest.ChangesOf(snapshot.ParseUnifiedDiff(diff))
	in := risk.Input{ChangedLines: measure(ctx, ws, stats), RemovedDeclarations: len(changes.Undeclared), SensitivePaths: risk.DefaultSensitivePaths}
	if eff, err := LoadSettings(ctx, ws); err == nil {
		in.SensitivePaths = eff.Settings.Ship.PullRequest.SensitivePaths
	}
	for _, group := range [][]string{changes.Added, changes.Changed, changes.Renamed} {
		for _, p := range group {
			in.Files = append(in.Files, risk.File{Path: p})
		}
	}
	for _, p := range changes.Removed {
		in.Files = append(in.Files, risk.File{Path: p, Removed: true})
	}
	a := risk.Assess(in)
	return &a
}
