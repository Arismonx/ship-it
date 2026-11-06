// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"slices"

	"github.com/GregoireF/ship-it/internal/core/forge"
)

// LabelResult says what `ship label --pr N` changed.
type LabelResult struct {
	Repository forge.Repository `json:"repository"`
	Number     int              `json:"number"`
	Added      []string         `json:"added,omitempty"`
	Removed    []string         `json:"removed,omitempty"`
	// Disabled is true when ship.pullRequest.labels is off.
	Disabled bool `json:"disabled,omitempty"`
	Rehearsal
}

// LabelPullRequest puts on pull request number of origin's repository the
// kind/ and size/ labels ship-it would have set had it published it, from
// what GitHub knows (its commits and files): so a pull request opened
// without ship-it, from a fork for instance, is labelled the same way. Only
// labels the repository has are used. A pull request without a size/ label
// gets every label; later runs only refresh the size, as Publish does, so a
// kind/ label a reviewer removed stays removed.
func LabelPullRequest(ctx context.Context, ws Workspace, number int) (result LabelResult, err error) {
	mark := ws.planMark()
	defer func() { result.Rehearsal = ws.rehearsed(mark) }()
	repo, err := remoteRepository(ctx, ws)
	if err != nil {
		return LabelResult{}, err
	}
	result = LabelResult{Repository: repo, Number: number}
	eff, err := LoadSettings(ctx, ws)
	if err != nil {
		return result, err
	}
	if !eff.Settings.Ship.PullRequest.Labels {
		result.Disabled = true
		return result, nil
	}
	content, err := ws.Forge.PullRequestContent(ctx, repo, number)
	if err != nil {
		return result, forgeFault(err, repo)
	}
	existing, err := ws.Forge.Labels(ctx, repo)
	if err != nil {
		return result, forgeFault(err, repo)
	}
	rules := eff.Settings.Conventions()
	_, headers := branchCommits(rules, content.CommitMessages)
	tax := eff.Settings.PullRequestLabels()
	wanted := tax.Labels(rules, headers, measure(ctx, ws, content.Files), labelNames(existing))

	current := content.PullRequest.Labels
	if tax.HasSizeLabel(current) {
		result.Added, result.Removed = tax.RefreshLabels(current, wanted)
	} else {
		for _, l := range wanted {
			if !slices.Contains(current, l) {
				result.Added = append(result.Added, l)
			}
		}
	}
	for _, l := range result.Removed {
		if err := ws.Forge.RemoveLabel(ctx, repo, number, l); err != nil {
			return result, forgeFault(err, repo)
		}
	}
	if len(result.Added) > 0 {
		if err := ws.Forge.AddLabels(ctx, repo, number, result.Added); err != nil {
			return result, forgeFault(err, repo)
		}
	}
	return result, nil
}
