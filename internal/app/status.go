// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/core/review"
	"github.com/GregoireF/ship-it/internal/core/situation"
	"github.com/GregoireF/ship-it/internal/ports"
)

// statusGitHubTimeout bounds the single API call of `ship status`, which
// must stay quick even when GitHub is slow.
const statusGitHubTimeout = 5 * time.Second

// StatusRequest configures `ship status`.
type StatusRequest struct {
	// Offline skips GitHub entirely.
	Offline bool
}

// ProblemView is a fault as JSON surfaces show it.
type ProblemView struct {
	Code   string            `json:"code"`
	Key    string            `json:"key"`
	Params map[string]string `json:"params,omitempty"`
}

// OperationView is an entry of the operation log.
type OperationView struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title,omitempty"`
}

// ActionView is the suggested next step.
type ActionView struct {
	Key    string            `json:"key"`
	Params map[string]string `json:"params,omitempty"`
}

// StatusView is where the user stands and what to do next.
type StatusView struct {
	Branch      string `json:"branch,omitempty"`
	Detached    bool   `json:"detached,omitempty"`
	Uncommitted int    `json:"uncommitted"`
	Upstream    string `json:"upstream,omitempty"`
	Ahead       int    `json:"ahead"`
	Behind      int    `json:"behind"`
	Base        string `json:"base,omitempty"`
	AheadOfBase int    `json:"ahead_of_base"`
	BehindBase  int    `json:"behind_base"`

	Repository *forge.Repository `json:"repository,omitempty"`
	// GitHub is "not_used", "unknown" (offline or failed: see GitHubProblem)
	// or "known".
	GitHub        string             `json:"github"`
	GitHubProblem *ProblemView       `json:"github_problem,omitempty"`
	PullRequest   *ports.PullRequest `json:"pull_request,omitempty"`
	// CI and Review describe the open pull request, when GitHub told.
	CI     *review.CI      `json:"ci,omitempty"`
	Review *review.Verdict `json:"review,omitempty"`

	// Undoable is what `ship undo` would revert.
	Undoable *OperationView `json:"undoable,omitempty"`
	Next     ActionView     `json:"next"`
}

var gitHubKnowledgeNames = map[situation.GitHubKnowledge]string{
	situation.GitHubNotUsed: "not_used", situation.GitHubUnknown: "unknown", situation.GitHubKnown: "known",
}

// Status reads the repository and, within one short timeout, what GitHub
// knows of the pull request (checks, reviews), and suggests the
// next action. It never changes anything.
func Status(ctx context.Context, ws Workspace, req StatusRequest) (StatusView, error) {
	// Independent reads run at once: each is a git process (about 20 ms on
	// Windows), and status is meant to answer in well under a second.
	var (
		status             ports.WorkingTreeStatus
		err                error
		inProgress         string
		remoteURL, base    string
		remoteErr, baseErr error
	)
	// The remote comes from the settings, which only read files once the run
	// read them (the git reads they need are cached).
	publishRemote := gitRemote(ctx, ws)
	parallel(
		func() { status, err = ws.Git.Status(ctx) },
		func() { inProgress, _ = ws.Git.OperationInProgress(ctx) },
		func() { remoteURL, remoteErr = ws.Git.RemoteURL(ctx, publishRemote) },
		func() { base, baseErr = ws.Git.RemoteDefaultBranch(ctx, publishRemote) },
	)
	if err != nil {
		return StatusView{}, err
	}
	b := status.Branch
	facts := situation.Facts{
		Branch: b.Name, Detached: b.Detached, OnTrunk: isTrunk(b.Name, base), Unborn: b.Commit == "",
		Upstream: b.Upstream, Ahead: b.Ahead, Behind: b.Behind,
	}
	for _, f := range status.Files {
		if f.State != ports.Ignored {
			facts.Uncommitted++
		}
	}
	facts.OperationInProgress = inProgress
	view := StatusView{}

	var repo *forge.Repository
	facts.Remote = publishRemote
	if remoteErr == nil {
		if r, ok := forge.ParseRemoteURL(remoteURL); ok {
			repo = &r
			facts.GitHub = situation.GitHubUnknown
			view.Repository = repo
		} else {
			facts.ForeignRemote = true
		}
	} else {
		facts.NoRemote = true
	}

	if baseErr == nil {
		facts.Base = base
	}
	online := repo != nil && !req.Offline && ws.Forge != nil
	if online && !facts.Unborn && !facts.Detached && !facts.OnTrunk {
		lookupCtx, cancel := context.WithTimeout(ctx, statusGitHubTimeout)
		// A fork with an "upstream" remote: its pull request is there.
		prRepo, head := *repo, b.Name
		if publishRemote != upstreamRemote {
			if up, err := repositoryOf(ctx, ws, upstreamRemote); err == nil && strings.EqualFold(up.Host, repo.Host) && !sameRepository(up, *repo) {
				prRepo, head = up, repo.Owner+":"+b.Name
			}
		}
		pr, found, err := ws.Forge.FindOpenPullRequest(lookupCtx, prRepo, head)
		if err == nil && facts.Base == "" {
			info, infoErr := ws.Forge.Repository(lookupCtx, prRepo)
			facts.Base, err = info.DefaultBranch, infoErr
		}
		if err != nil {
			view.GitHubProblem = problemOf(forgeFault(err, *repo))
		} else {
			facts.GitHub = situation.GitHubKnown
			if found {
				view.PullRequest = &pr
				facts.PullRequest = &situation.PullRequest{Number: pr.Number, URL: pr.URL, Draft: pr.Draft}
				pullRequestState(lookupCtx, ws, prRepo, pr, b.Commit, facts.PullRequest, &view)
			}
		}
		cancel()
	}
	if facts.Base != "" && !facts.Unborn {
		// One process: comparing with a branch never fetched fails.
		if facts.AheadOfBase, facts.BehindBase, err = ws.Git.CompareWith(ctx, publishRemote+"/"+facts.Base); err != nil {
			facts.Base = "" // never fetched: no comparison possible
		}
	}

	if ws.Operations != nil {
		if ops, err := ws.Operations.Recent(ctx, operationSearchDepth); err == nil {
			if op, ok := oplog.LastUndoable(ops); ok {
				view.Undoable = &OperationView{ID: op.ID, Kind: string(op.Kind), Title: op.Title}
			}
		}
	}

	next := situation.Next(facts)
	view.Branch, view.Detached, view.Uncommitted = facts.Branch, facts.Detached, facts.Uncommitted
	view.Upstream, view.Ahead, view.Behind = facts.Upstream, facts.Ahead, facts.Behind
	view.Base, view.AheadOfBase, view.BehindBase = facts.Base, facts.AheadOfBase, facts.BehindBase
	view.GitHub = gitHubKnowledgeNames[facts.GitHub]
	view.Next = ActionView{Key: next.Key, Params: next.Params}
	return view, nil
}

// pullRequestState adds the checks of the pull request's last commit and its
// reviewers' decision. Either may be missing (a token without the checks
// permission, a timeout): status then simply does not know.
func pullRequestState(ctx context.Context, ws Workspace, repo forge.Repository, pr ports.PullRequest, head string,
	facts *situation.PullRequest, view *StatusView) {
	if pr.HeadSHA != "" {
		head = pr.HeadSHA
	}
	if head != "" {
		runs, runsErr := ws.Forge.CheckRuns(ctx, repo, head)
		statuses, statusErr := ws.Forge.CommitStatuses(ctx, repo, head)
		if runsErr == nil && statusErr == nil {
			ci := review.Summarize(runs, statuses)
			facts.CI, view.CI = ci, &ci
		}
	}
	if reviews, err := ws.Forge.Reviews(ctx, repo, pr.Number); err == nil {
		verdict := review.Decide(reviews)
		facts.Review, view.Review = verdict, &verdict
	}
}

func problemOf(err error) *ProblemView {
	err = Known(err)
	var f *Fault
	if errors.As(err, &f) {
		return &ProblemView{Code: string(f.Code), Key: f.Key, Params: f.Params}
	}
	return &ProblemView{Code: "SHIP-E0001", Key: "app.unexpected_error", Params: map[string]string{"error": err.Error()}}
}
