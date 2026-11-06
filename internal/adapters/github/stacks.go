// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/ports"
)

// The stacks API, REST version 2026-03-10 (GitHub Docs, "REST API endpoints
// for stacks", https://docs.github.com/en/rest/pulls/stacks, read
// 2026-11-05):
//
//	GET  /repos/{owner}/{repo}/stacks?pull_request=N   list (200, 404, 422)
//	POST /repos/{owner}/{repo}/stacks                  create, {"pull_requests": [bottom … top]} (201, 404, 422)
//	GET  /repos/{owner}/{repo}/stacks/{stack_number}   get (200, 404)
//	POST …/stacks/{stack_number}/add                   append on top (200, 404, 409, 422)
//	POST …/stacks/{stack_number}/unstack               200 with what is left, 204 once dissolved (404, 409, 422)
//
// The asynchronous merge API (GitHub Docs, "Merge a pull request
// asynchronously", https://docs.github.com/en/rest/pulls/pulls#merge-a-pull-request-asynchronously,
// generally available since 2026-10-01 per
// https://github.blog/changelog/2026-10-01-github-async-merge-api-generally-available):
//
//	PUT /repos/{owner}/{repo}/pulls/{pull_number}/merge-async          {"sha", "merge_action", "bypass_rules"} (200 done or queued, 202 accepted, 400, 403, 404, 409, 422)
//	GET /repos/{owner}/{repo}/pulls/{pull_number}/merge-async/{uuid}   poll (200, 403, 404 after 24 h)
//
// Both answer {"status": "pending"|"merged"|"enqueued"|"failed", "details": {…}},
// the pending details carrying the uuid. The page gives the schema, not a
// literal example: the field names below follow the schema.

var _ ports.Stacks = (*Client)(nil)

type stackJSON struct {
	Number int  `json:"number"`
	Open   bool `json:"open"`
	Base   struct {
		Ref string `json:"ref"`
	} `json:"base"`
	PullRequests []stackPullRequestJSON `json:"pull_requests"`
}

type stackPullRequestJSON struct {
	Number   int     `json:"number"`
	HTMLURL  string  `json:"html_url"`
	State    string  `json:"state"`
	Draft    bool    `json:"draft"`
	MergedAt *string `json:"merged_at"`
	Body     *string `json:"body"`
	Head     struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
	// Stack is the pull request resource's "stack" object (id, number,
	// size, position, base), absent outside a stack.
	Stack *struct {
		Number int `json:"number"`
	} `json:"stack"`
}

func (p stackPullRequestJSON) port() ports.StackPullRequest {
	pr := ports.StackPullRequest{
		Number: p.Number, URL: p.HTMLURL, State: p.State, Draft: p.Draft, Merged: p.MergedAt != nil,
		HeadBranch: p.Head.Ref, HeadSHA: p.Head.SHA, BaseBranch: p.Base.Ref,
	}
	if p.Stack != nil {
		pr.StackNumber = p.Stack.Number
	}
	if p.Body != nil {
		pr.Body = *p.Body
	}
	return pr
}

func (s stackJSON) port() ports.Stack {
	st := ports.Stack{Number: s.Number, Base: s.Base.Ref, Open: s.Open}
	for _, p := range s.PullRequests {
		pr := p.port()
		pr.StackNumber = s.Number
		st.PullRequests = append(st.PullRequests, pr)
	}
	return st
}

// stackError adds the ports meaning of the statuses the stacks API documents.
func stackError(err error) error {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return err
	}
	switch apiErr.Status {
	case http.StatusConflict:
		return errors.Join(ports.ErrConflict, err)
	case http.StatusUnprocessableEntity, http.StatusBadRequest:
		return errors.Join(ports.ErrInvalid, err)
	}
	return err
}

// StacksOf implements ports.Stacks. The repository was read before (the
// token sees it), so a 404 here means the server has no stacks API.
func (c *Client) StacksOf(ctx context.Context, repo forge.Repository, number int) ([]ports.Stack, error) {
	query := url.Values{"per_page": {"100"}}
	if number > 0 {
		query.Set("pull_request", strconv.Itoa(number))
	}
	var out []stackJSON
	if err := c.do(ctx, repo, http.MethodGet, repoPath(repo)+"/stacks?"+query.Encode(), nil, &out); err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return nil, errors.Join(ports.ErrStacksUnavailable, err)
		}
		return nil, stackError(err)
	}
	stacks := make([]ports.Stack, 0, len(out))
	for _, s := range out {
		stacks = append(stacks, s.port())
	}
	return stacks, nil
}

// CreateStack implements ports.Stacks.
func (c *Client) CreateStack(ctx context.Context, repo forge.Repository, numbers []int) (ports.Stack, error) {
	var out stackJSON
	err := c.do(ctx, repo, http.MethodPost, repoPath(repo)+"/stacks", map[string][]int{"pull_requests": numbers}, &out)
	if errors.Is(err, ports.ErrNotFound) {
		return ports.Stack{}, errors.Join(ports.ErrStacksUnavailable, err)
	}
	return out.port(), stackError(err)
}

// AddToStack implements ports.Stacks.
func (c *Client) AddToStack(ctx context.Context, repo forge.Repository, number int, numbers []int) (ports.Stack, error) {
	var out stackJSON
	path := fmt.Sprintf("%s/stacks/%d/add", repoPath(repo), number)
	err := c.do(ctx, repo, http.MethodPost, path, map[string][]int{"pull_requests": numbers}, &out)
	return out.port(), stackError(err)
}

// Unstack implements ports.Stacks: a 204 has no body, so a stack without a
// number in the answer is a dissolved one.
func (c *Client) Unstack(ctx context.Context, repo forge.Repository, number int) (bool, error) {
	var out stackJSON
	path := fmt.Sprintf("%s/stacks/%d/unstack", repoPath(repo), number)
	if err := c.do(ctx, repo, http.MethodPost, path, nil, &out); err != nil {
		return false, stackError(err)
	}
	return out.Number == 0 || len(out.PullRequests) == 0, nil
}

// PullRequest implements ports.Stacks with GET pulls/{n}.
func (c *Client) PullRequest(ctx context.Context, repo forge.Repository, number int) (ports.StackPullRequest, error) {
	var out stackPullRequestJSON
	err := c.do(ctx, repo, http.MethodGet, fmt.Sprintf("%s/pulls/%d", repoPath(repo), number), nil, &out)
	return out.port(), err
}

// SetPullRequestBase implements ports.Stacks with PATCH pulls/{n}.
func (c *Client) SetPullRequestBase(ctx context.Context, repo forge.Repository, number int, base string) error {
	path := fmt.Sprintf("%s/pulls/%d", repoPath(repo), number)
	return stackError(c.do(ctx, repo, http.MethodPatch, path, map[string]string{"base": base}, nil))
}

// SetPullRequestBody implements ports.Stacks with PATCH pulls/{n}.
func (c *Client) SetPullRequestBody(ctx context.Context, repo forge.Repository, number int, body string) error {
	path := fmt.Sprintf("%s/pulls/%d", repoPath(repo), number)
	return stackError(c.do(ctx, repo, http.MethodPatch, path, map[string]string{"body": body}, nil))
}

type mergeStatusJSON struct {
	Status  string `json:"status"`
	Details struct {
		Message string `json:"message"`
		UUID    string `json:"uuid"`
		SHA     string `json:"sha"`
	} `json:"details"`
}

func (m mergeStatusJSON) port(uuid string) ports.MergeStatus {
	s := ports.MergeStatus{Status: m.Status, UUID: m.Details.UUID, Message: m.Details.Message, SHA: m.Details.SHA}
	if s.UUID == "" {
		s.UUID = uuid
	}
	return s
}

// MergeAsync implements ports.Stacks. bypass_rules is always false and
// merge_action "default": GitHub's rules and merge queue decide, never
// ship-it. The merge method is the repository's default (not sent).
func (c *Client) MergeAsync(ctx context.Context, repo forge.Repository, number int, sha string) (ports.MergeStatus, error) {
	in := map[string]any{"merge_action": "default", "bypass_rules": false}
	if sha != "" {
		in["sha"] = sha
	}
	var out mergeStatusJSON
	path := fmt.Sprintf("%s/pulls/%d/merge-async", repoPath(repo), number)
	if err := c.do(ctx, repo, http.MethodPut, path, in, &out); err != nil {
		return ports.MergeStatus{}, stackError(err)
	}
	if out.Status == "" {
		return ports.MergeStatus{}, fmt.Errorf("github: merge-async answered without a status")
	}
	return out.port(""), nil
}

// MergeResult implements ports.Stacks.
func (c *Client) MergeResult(ctx context.Context, repo forge.Repository, number int, uuid string) (ports.MergeStatus, error) {
	var out mergeStatusJSON
	path := fmt.Sprintf("%s/pulls/%d/merge-async/%s", repoPath(repo), number, url.PathEscape(uuid))
	if err := c.do(ctx, repo, http.MethodGet, path, nil, &out); err != nil {
		return ports.MergeStatus{}, stackError(err)
	}
	return out.port(uuid), nil
}
