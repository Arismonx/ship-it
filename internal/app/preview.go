// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"

	"github.com/GregoireF/ship-it/internal/core/forge"
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
	return view, nil
}
