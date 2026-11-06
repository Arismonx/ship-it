// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package review summarises what GitHub says about a pull request: the state
// of its checks (check runs and commit statuses) and the reviewers' decision,
// for `ship status` and its next action. Facts: GitHub's REST documentation (check runs and
// commit statuses REST endpoints, GraphQL PullRequestReviewState).
//
// Allowed imports: standard library without I/O, other core packages.
package review

import (
	"slices"
	"strings"
)

// CheckRun is one GitHub check run: Status queued, in_progress, completed,
// waiting, requested or pending; Conclusion set once completed.
type CheckRun struct {
	Name, Status, Conclusion, URL string
}

// CommitStatus is one commit status (the older API some CI still uses):
// State error, failure, pending or success.
type CommitStatus struct {
	Context, State, URL string
}

// CIState is the overall state of the checks.
type CIState string

// Overall states, from worst to best.
const (
	CIFailing CIState = "failing"
	CIPending CIState = "pending"
	CIPassing CIState = "passing"
	// CINone: the commit has no check at all.
	CINone CIState = "none"
)

// CI is the summary of the checks of a commit.
type CI struct {
	State CIState `json:"state"`
	// Failing names the checks that failed, sorted.
	Failing []string `json:"failing,omitempty"`
	Pending int      `json:"pending,omitempty"`
	Total   int      `json:"total"`
}

// failedConclusions block a merge or ask for action. "cancelled" counts: a
// cancelled required check does not let the pull request through.
var failedConclusions = []string{"failure", "timed_out", "cancelled", "action_required"}

// Summarize combines check runs and commit statuses: failing when one failed,
// pending when one is not finished, passing when all passed (success,
// neutral or skipped), none without any.
func Summarize(runs []CheckRun, statuses []CommitStatus) CI {
	ci := CI{Total: len(runs) + len(statuses)}
	for _, r := range runs {
		switch {
		case r.Status != "completed":
			ci.Pending++
		case slices.Contains(failedConclusions, r.Conclusion):
			ci.Failing = append(ci.Failing, r.Name)
		}
	}
	for _, s := range statuses {
		switch s.State {
		case "pending":
			ci.Pending++
		case "error", "failure":
			ci.Failing = append(ci.Failing, s.Context)
		}
	}
	slices.Sort(ci.Failing)
	ci.Failing = slices.Compact(ci.Failing)
	switch {
	case ci.Total == 0:
		ci.State = CINone
	case len(ci.Failing) > 0:
		ci.State = CIFailing
	case ci.Pending > 0:
		ci.State = CIPending
	default:
		ci.State = CIPassing
	}
	return ci
}

// Review is one submitted review: State APPROVED, CHANGES_REQUESTED,
// COMMENTED, DISMISSED or PENDING.
type Review struct {
	Author, State string
}

// Decision is what the reviewers decided.
type Decision string

// Decisions.
const (
	ChangesRequested Decision = "changes_requested"
	Approved         Decision = "approved"
	NoDecision       Decision = "none"
)

// Verdict is the reviewers' decision and who made it.
type Verdict struct {
	Decision Decision `json:"decision"`
	// Approvers and Requesters are logins, sorted.
	Approvers  []string `json:"approvers,omitempty"`
	Requesters []string `json:"requesters,omitempty"`
}

// Decide reads reviews in chronological order (as GitHub lists them): the
// last approval or change request of each reviewer stands, a dismissal
// clears it, comments and pending reviews change nothing. One standing
// change request is enough to block.
func Decide(reviews []Review) Verdict {
	standing := map[string]string{}
	for _, r := range reviews {
		author := strings.ToLower(r.Author)
		switch r.State {
		case "APPROVED", "CHANGES_REQUESTED":
			standing[author] = r.State
		case "DISMISSED":
			delete(standing, author)
		}
	}
	var v Verdict
	for author, state := range standing {
		if state == "APPROVED" {
			v.Approvers = append(v.Approvers, author)
		} else {
			v.Requesters = append(v.Requesters, author)
		}
	}
	slices.Sort(v.Approvers)
	slices.Sort(v.Requesters)
	switch {
	case len(v.Requesters) > 0:
		v.Decision = ChangesRequested
	case len(v.Approvers) > 0:
		v.Decision = Approved
	default:
		v.Decision = NoDecision
	}
	return v
}
