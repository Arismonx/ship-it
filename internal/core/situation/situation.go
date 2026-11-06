// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package situation reads where the user stands and suggests the one next
// action that moves the work forward (`ship status`).
//
// Allowed imports: standard library without I/O, other core packages.
package situation

import (
	"strconv"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/review"
)

// GitHubKnowledge says how much ship status knows about GitHub.
type GitHubKnowledge int

// What the GitHub lookup gave.
const (
	// GitHubNotUsed: no GitHub remote, a purely local repository.
	GitHubNotUsed GitHubKnowledge = iota
	// GitHubUnknown: offline, no token, or the call failed.
	GitHubUnknown
	// GitHubKnown: PullRequest is authoritative (nil means no open PR).
	GitHubKnown
)

// PullRequest is the open pull request of the branch.
type PullRequest struct {
	Number int
	URL    string
	Draft  bool
	// CI and Review are what GitHub says of its last commit and its
	// reviewers; zero values when unknown.
	CI     review.CI
	Review review.Verdict
}

// Facts describe the repository, gathered by app.
type Facts struct {
	Branch   string
	Detached bool
	// OnTrunk: the branch is main, master, develop or trunk.
	OnTrunk bool
	// Unborn: no commit yet.
	Unborn              bool
	OperationInProgress string
	Uncommitted         int

	// Upstream tracking, as `git status` reports it.
	Upstream string
	Ahead    int
	Behind   int

	// Base is the default branch the work goes to; empty when unknown.
	Base        string
	AheadOfBase int
	BehindBase  int

	GitHub      GitHubKnowledge
	PullRequest *PullRequest

	// Remote is the remote ship publishes to; NoRemote says it does not
	// exist and ForeignRemote that it is not on GitHub: publishing cannot
	// work, only local commits can.
	Remote        string
	NoRemote      bool
	ForeignRemote bool
}

// Action is the suggested next step, as an i18n key and its parameters.
type Action struct {
	Key    string
	Params map[string]string
}

func action(key string, kv ...string) Action {
	a := Action{Key: key}
	if len(kv) > 0 {
		a.Params = map[string]string{}
		for i := 0; i+1 < len(kv); i += 2 {
			a.Params[kv[i]] = kv[i+1]
		}
	}
	return a
}

// Next returns the single most useful next action. The order matters: an
// unfinished git operation blocks everything, uncommitted work comes before
// publishing, and publishing before waiting for review.
func Next(f Facts) Action {
	switch {
	case f.OperationInProgress != "":
		return action("status.next.finish_operation", "operation", f.OperationInProgress)
	case f.Uncommitted > 0:
		return action("status.next.ship", "count", strconv.Itoa(f.Uncommitted))
	case f.Unborn || f.OnTrunk || f.Detached:
		return action("status.next.start")
	case f.Behind > 0:
		// Someone else pushed to this branch: integrate before anything else.
		return action("status.next.pull", "count", strconv.Itoa(f.Behind), "upstream", f.Upstream)
	case f.Base != "" && f.AheadOfBase == 0:
		return action("status.next.nothing_ahead", "base", f.Base)
	case f.Upstream == "" || f.Ahead > 0, f.GitHub == GitHubKnown && f.PullRequest == nil:
		switch {
		case f.ForeignRemote:
			return action("status.next.local_only", "remote", f.Remote)
		case f.NoRemote:
			return action("status.next.local_only_no_remote", "remote", f.Remote)
		}
		return action("status.next.publish")
	case f.PullRequest != nil && f.PullRequest.CI.State == review.CIFailing:
		// A red check comes first: no reviewer should spend time on it.
		return action("status.next.fix_ci", "checks", strings.Join(f.PullRequest.CI.Failing, ", "), "url", f.PullRequest.URL)
	case f.PullRequest != nil && f.PullRequest.Review.Decision == review.ChangesRequested:
		return action("status.next.address_review", "reviewers", strings.Join(f.PullRequest.Review.Requesters, ", "), "url", f.PullRequest.URL)
	case f.PullRequest != nil && f.PullRequest.Draft:
		return action("status.next.mark_ready", "url", f.PullRequest.URL)
	case f.PullRequest != nil && f.PullRequest.CI.State == review.CIPending:
		return action("status.next.await_ci", "count", strconv.Itoa(f.PullRequest.CI.Pending), "url", f.PullRequest.URL)
	case f.PullRequest != nil && f.PullRequest.Review.Decision == review.Approved:
		return action("status.next.merge", "reviewers", strings.Join(f.PullRequest.Review.Approvers, ", "), "url", f.PullRequest.URL)
	case f.PullRequest != nil:
		return action("status.next.await_review", "url", f.PullRequest.URL)
	default:
		return action("status.next.up_to_date")
	}
}
