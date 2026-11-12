// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"strconv"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/core/overlap"
	"github.com/GregoireF/ship-it/internal/ports"
)

// OverlapView is an open pull request that changes some of the lines the
// branch changes too: a likely conflict for whoever merges second (#40).
type OverlapView struct {
	Number int      `json:"number"`
	Title  string   `json:"title"`
	URL    string   `json:"url"`
	Files  []string `json:"files"`
}

// openPullRequestLimit bounds how many open pull requests are read: one API
// call each for their files, newest first.
const openPullRequestLimit = 30

// OpenOverlaps returns the open pull requests into baseBranch that change the
// same or adjacent lines as the branch's work (baseRef...HEAD), the branch's
// own pull request aside, newest first.
func OpenOverlaps(ctx context.Context, ws Workspace, repo forge.Repository, baseRef, baseBranch, branch string) ([]OverlapView, error) {
	diff, err := ws.Git.DiffSince(ctx, baseRef)
	if err != nil {
		return nil, err
	}
	mine := overlap.ParseDiff(diff)
	if len(mine) == 0 {
		return nil, nil // only new files: nothing another change can touch
	}
	open, err := ws.Forge.OpenPullRequestPatches(ctx, repo, baseBranch, openPullRequestLimit)
	if err != nil {
		return nil, err
	}
	var overlaps []OverlapView
	for _, pr := range open {
		if pr.Head == branch {
			continue
		}
		if paths := overlap.Overlaps(mine, filesOf(pr.Files)); len(paths) > 0 {
			overlaps = append(overlaps, OverlapView{Number: pr.PullRequest.Number, Title: pr.Title, URL: pr.PullRequest.URL, Files: paths})
		}
	}
	return overlaps, nil
}

// overlapWarning says which open pull request changes the same lines.
func overlapWarning(o OverlapView) ProblemView {
	return ProblemView{Code: string(fault.OverlappingPullRequest), Key: "publish.overlap", Params: map[string]string{
		"number": strconv.Itoa(o.Number), "title": o.Title, "files": strings.Join(o.Files, ", "), "url": o.URL,
	}}
}

// filesOf turns a pull request's files into what it does to the base: a new
// file touches nothing there; a removed or renamed file, or one GitHub gives
// no patch for (binary, too large), is changed as a whole.
func filesOf(patches []ports.FilePatch) []overlap.File {
	var files []overlap.File
	for _, f := range patches {
		switch {
		case f.Status == "added":
		case f.Status == "removed":
			files = append(files, overlap.File{Path: f.Path, Whole: true})
		case f.Status == "renamed" && f.PreviousPath != "":
			files = append(files, overlap.File{Path: f.PreviousPath, Whole: true})
		case f.Patch == "":
			files = append(files, overlap.File{Path: f.Path, Whole: true})
		default:
			files = append(files, overlap.File{Path: f.Path, Ranges: overlap.ParsePatch(f.Patch)})
		}
	}
	return files
}
