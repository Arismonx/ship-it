// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/convention"
	"github.com/GregoireF/ship-it/internal/core/fault"
)

// SignoffCheck is the result of checking the DCO sign-off of a range of
// commits (`ship check signoff`).
type SignoffCheck struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Checked int    `json:"checked"`
	// Exempt counts merges and bot commits, which need no sign-off.
	Exempt   int              `json:"exempt"`
	Failures []SignoffFailure `json:"failures,omitempty"`
}

// SignoffFailure is one commit without a valid sign-off.
type SignoffFailure struct {
	Commit  string `json:"commit"`
	Subject string `json:"subject"`
	Fault   *Fault `json:"fault"`
}

// Valid reports whether every commit of the range is signed off.
func (c SignoffCheck) Valid() bool { return len(c.Failures) == 0 }

// CheckSignoffs checks every commit of from..to. An empty from is the
// remote-tracking default branch of the remote (ship.git.remote), or of the
// "upstream" remote of a fork when it names the parent: the commits a pull
// request would bring. An empty to is HEAD.
func CheckSignoffs(ctx context.Context, ws Workspace, from, to string) (SignoffCheck, error) {
	git := ws.Git
	if to == "" {
		to = "HEAD"
	}
	if from == "" {
		publishRemote := forkBaseRemote(ctx, ws)
		branch, err := git.RemoteDefaultBranch(ctx, publishRemote)
		if err != nil {
			return SignoffCheck{}, err
		}
		if branch == "" {
			return SignoffCheck{}, fault.New(fault.SignoffNoBase, "convention.signoff.no_base", "remote", publishRemote)
		}
		from = publishRemote + "/" + branch
	}
	commits, err := git.CommitsBetween(ctx, from, to)
	if err != nil {
		return SignoffCheck{}, err
	}
	result := SignoffCheck{From: from, To: to, Checked: len(commits)}
	for _, c := range commits {
		commit := convention.SignedCommit{
			Merge:      len(c.Parents) > 1,
			AuthorName: c.AuthorName, AuthorEmail: c.AuthorEmail,
			CommitterName: c.CommitterName, CommitterEmail: c.CommitterEmail,
			Message: c.Message,
		}
		if commit.Merge || convention.IsBotEmail(c.AuthorEmail) {
			result.Exempt++
			continue
		}
		if f := convention.CheckSignoff(commit); f != nil {
			subject, _, _ := strings.Cut(c.Message, "\n")
			result.Failures = append(result.Failures, SignoffFailure{Commit: c.ID, Subject: subject, Fault: f})
		}
	}
	return result, nil
}

// forkBaseRemote is the remote whose default branch is the base of pull
// requests, read locally: "upstream" when it names another repository of the
// same server than the remote (a fork's parent) and knows its default
// branch, else the remote.
func forkBaseRemote(ctx context.Context, ws Workspace) string {
	remote := gitRemote(ctx, ws)
	if remote == upstreamRemote {
		return remote
	}
	own, err := repositoryOf(ctx, ws, remote)
	if err != nil {
		return remote
	}
	up, err := repositoryOf(ctx, ws, upstreamRemote)
	if err != nil || !strings.EqualFold(up.Host, own.Host) || sameRepository(up, own) {
		return remote
	}
	if branch, err := ws.Git.RemoteDefaultBranch(ctx, upstreamRemote); err != nil || branch == "" {
		return remote
	}
	return upstreamRemote
}
