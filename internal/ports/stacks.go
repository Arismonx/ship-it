// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package ports

import (
	"context"
	"errors"

	"github.com/GregoireF/ship-it/internal/core/forge"
)

// Stack is a GitHub stack of pull requests (REST API version 2026-03-10).
type Stack struct {
	Number int `json:"number"`
	// Base is the branch the bottom pull request targets.
	Base string `json:"base"`
	Open bool   `json:"open"`
	// PullRequests are listed from the bottom up.
	PullRequests []StackPullRequest `json:"pull_requests"`
}

// StackPullRequest is a pull request as the stacks API and the pull request
// resource describe it.
type StackPullRequest struct {
	Number     int    `json:"number"`
	URL        string `json:"url,omitempty"`
	State      string `json:"state,omitempty"`
	Merged     bool   `json:"merged,omitempty"`
	Draft      bool   `json:"draft,omitempty"`
	HeadBranch string `json:"head_branch,omitempty"`
	HeadSHA    string `json:"head_sha,omitempty"`
	BaseBranch string `json:"base_branch,omitempty"`
	// Body is the pull request's description (PullRequest only).
	Body string `json:"body,omitempty"`
	// StackNumber is the stack it belongs to (its "stack" object), 0 if none.
	StackNumber int `json:"stack_number,omitempty"`
}

// MergeStatus is the state of an asynchronous merge request: "pending",
// "merged", "enqueued" (in the merge queue, not merged yet) or "failed".
type MergeStatus struct {
	Status string `json:"status"`
	// UUID identifies the request, to poll it.
	UUID string `json:"uuid,omitempty"`
	// Message explains a failure; SHA is the merge commit once merged.
	Message string `json:"message,omitempty"`
	SHA     string `json:"sha,omitempty"`
}

// Stacks is the stacks API and the few pull request calls a stack needs.
// A classic chain of pull requests (GHES) uses the pull request calls only.
type Stacks interface {
	// StacksOf lists the stacks of the repository, the one holding pull
	// request number only when number > 0. ErrStacksUnavailable when the
	// server has no stacks API.
	StacksOf(ctx context.Context, repo forge.Repository, number int) ([]Stack, error)
	// CreateStack groups pull requests, bottom first, each based on the head
	// branch of the previous one.
	CreateStack(ctx context.Context, repo forge.Repository, numbers []int) (Stack, error)
	// AddToStack appends pull requests on top of stack number.
	AddToStack(ctx context.Context, repo forge.Repository, number int, numbers []int) (Stack, error)
	// Unstack removes the unmerged pull requests from stack number; dissolved
	// reports that none is left (GitHub then removes the stack).
	Unstack(ctx context.Context, repo forge.Repository, number int) (dissolved bool, err error)
	// PullRequest reads one pull request, base branch included.
	PullRequest(ctx context.Context, repo forge.Repository, number int) (StackPullRequest, error)
	// SetPullRequestBase changes the branch a pull request targets.
	SetPullRequestBase(ctx context.Context, repo forge.Repository, number int, base string) error
	// SetPullRequestBody replaces the description of a pull request: the
	// "Part 2 of 3" line of a classic chain.
	SetPullRequestBody(ctx context.Context, repo forge.Repository, number int, body string) error
	// MergeAsync asks GitHub to merge pull request number (and, in a stack,
	// every one below it) in the background, only if its head is still sha.
	// Repository rules always apply: ship-it never asks to bypass them.
	MergeAsync(ctx context.Context, repo forge.Repository, number int, sha string) (MergeStatus, error)
	// MergeResult polls a merge request.
	MergeResult(ctx context.Context, repo forge.Repository, number int, uuid string) (MergeStatus, error)
}

var (
	// ErrStacksUnavailable: the server has no stacks API (GitHub Enterprise
	// Server today); ship-it falls back to a classic chain.
	ErrStacksUnavailable = errors.New("ports: no stacks API on this GitHub server")
	// ErrConflict: GitHub reports a concurrent change (409).
	ErrConflict = errors.New("ports: GitHub reports a concurrent change")
	// ErrInvalid: GitHub refused the request as invalid (422).
	ErrInvalid = errors.New("ports: GitHub refused the request")
)
