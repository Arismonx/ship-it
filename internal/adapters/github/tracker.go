// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/core/roadmap"
	"github.com/GregoireF/ship-it/internal/ports"
)

var _ ports.Tracker = (*Client)(nil)

// maxIssuePages bounds the scan of a repository's issues (100 per page).
const maxIssuePages = 30

type milestoneJSON struct {
	Number      int     `json:"number"`
	Title       string  `json:"title"`
	Description *string `json:"description"`
	State       string  `json:"state"`
	DueOn       *string `json:"due_on"`
	OpenIssues  int     `json:"open_issues"`
	Closed      int     `json:"closed_issues"`
}

func (m milestoneJSON) roadmap() roadmap.Milestone {
	out := roadmap.Milestone{Number: m.Number, Title: m.Title, Open: m.State == "open", OpenIssues: m.OpenIssues, ClosedIssues: m.Closed}
	if m.Description != nil {
		out.Description = *m.Description
	}
	if m.DueOn != nil && len(*m.DueOn) >= 10 {
		out.Due = (*m.DueOn)[:10]
	}
	return out
}

// Milestones implements ports.Tracker (GET /repos/{owner}/{repo}/milestones?state=all).
func (c *Client) Milestones(ctx context.Context, repo forge.Repository) ([]roadmap.Milestone, error) {
	var milestones []roadmap.Milestone
	next := repoPath(repo) + "/milestones?state=all&per_page=100"
	for page := 0; next != "" && page < maxIssuePages; page++ {
		var out []milestoneJSON
		header, err := c.doWithHeader(ctx, repo, http.MethodGet, next, nil, &out)
		if err != nil {
			return nil, err
		}
		for _, m := range out {
			milestones = append(milestones, m.roadmap())
		}
		next = nextPage(header.Get("Link"), c.baseURL(repo))
	}
	return milestones, nil
}

// milestoneBody writes the fields GitHub takes: due_on is an ISO 8601
// timestamp (YYYY-MM-DDTHH:MM:SSZ). null clears it on an update, but a
// creation refuses it ("422 For 'properties/due_on', nil is not a string"):
// there, no date means no due_on at all.
func milestoneBody(m roadmap.Milestone, update bool) map[string]any {
	body := map[string]any{"title": m.Title, "description": m.Description}
	if update {
		body["due_on"] = nil
	}
	if m.Due != "" {
		if day, err := time.Parse("2006-01-02", m.Due); err == nil {
			body["due_on"] = day.UTC().Format(time.RFC3339)
		}
	}
	return body
}

// CreateMilestone implements ports.Tracker.
func (c *Client) CreateMilestone(ctx context.Context, repo forge.Repository, m roadmap.Milestone) (int, error) {
	var out milestoneJSON
	err := c.do(ctx, repo, http.MethodPost, repoPath(repo)+"/milestones", milestoneBody(m, false), &out)
	return out.Number, err
}

// UpdateMilestone implements ports.Tracker.
func (c *Client) UpdateMilestone(ctx context.Context, repo forge.Repository, m roadmap.Milestone) error {
	return c.do(ctx, repo, http.MethodPatch, fmt.Sprintf("%s/milestones/%d", repoPath(repo), m.Number), milestoneBody(m, true), nil)
}

// DeleteMilestone implements ports.Tracker.
func (c *Client) DeleteMilestone(ctx context.Context, repo forge.Repository, number int) error {
	err := c.do(ctx, repo, http.MethodDelete, fmt.Sprintf("%s/milestones/%d", repoPath(repo), number), nil, nil)
	if errors.Is(err, ports.ErrNotFound) {
		return nil // already gone
	}
	return err
}

type issueJSON struct {
	ID        int64   `json:"id"`
	Number    int     `json:"number"`
	Title     string  `json:"title"`
	Body      *string `json:"body"`
	State     string  `json:"state"`
	Milestone *struct {
		Number int `json:"number"`
	} `json:"milestone"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
	PullRequest *struct{} `json:"pull_request"`
	Assignees   []struct {
		Login string `json:"login"`
	} `json:"assignees"`
}

func (i issueJSON) roadmap() roadmap.Issue {
	out := roadmap.Issue{Number: i.Number, ID: i.ID, Title: i.Title, Open: i.State == "open"}
	if i.Body != nil {
		out.Body = *i.Body
	}
	if i.Milestone != nil {
		out.Milestone = i.Milestone.Number
	}
	for _, l := range i.Labels {
		out.Labels = append(out.Labels, l.Name)
	}
	for _, a := range i.Assignees {
		out.Assignees = append(out.Assignees, a.Login)
	}
	return out
}

// Issue implements ports.Tracker (GET /issues/{n}).
func (c *Client) Issue(ctx context.Context, repo forge.Repository, number int) (roadmap.Issue, error) {
	var out issueJSON
	err := c.do(ctx, repo, http.MethodGet, fmt.Sprintf("%s/issues/%d", repoPath(repo), number), nil, &out)
	return out.roadmap(), err
}

// AddAssignees implements ports.Tracker (POST /issues/{n}/assignees, which
// keeps the current assignees; GitHub silently ignores a login without push
// access).
func (c *Client) AddAssignees(ctx context.Context, repo forge.Repository, number int, logins []string) error {
	if len(logins) == 0 {
		return nil
	}
	return c.do(ctx, repo, http.MethodPost, fmt.Sprintf("%s/issues/%d/assignees", repoPath(repo), number),
		map[string][]string{"assignees": logins}, nil)
}

// RemoveAssignees implements ports.Tracker (DELETE /issues/{n}/assignees,
// which keeps the other assignees).
func (c *Client) RemoveAssignees(ctx context.Context, repo forge.Repository, number int, logins []string) error {
	if len(logins) == 0 {
		return nil
	}
	return c.do(ctx, repo, http.MethodDelete, fmt.Sprintf("%s/issues/%d/assignees", repoPath(repo), number),
		map[string][]string{"assignees": logins}, nil)
}

// TrackedIssues implements ports.Tracker: the repository's issues are
// listed (GET /issues?state=all, pull requests filtered out, as the API
// lists them together) and kept when their body holds marker; the numbered
// ones not found that way are fetched one by one.
func (c *Client) TrackedIssues(ctx context.Context, repo forge.Repository, marker string, numbers []int) ([]roadmap.Issue, error) {
	var issues []roadmap.Issue
	seen := map[int]bool{}
	next := repoPath(repo) + "/issues?state=all&per_page=100"
	for page := 0; next != "" && page < maxIssuePages; page++ {
		var out []issueJSON
		header, err := c.doWithHeader(ctx, repo, http.MethodGet, next, nil, &out)
		if err != nil {
			return nil, err
		}
		for _, i := range out {
			if i.PullRequest != nil {
				continue
			}
			is := i.roadmap()
			if strings.Contains(is.Body, marker) || slices.Contains(numbers, is.Number) {
				issues = append(issues, is)
				seen[is.Number] = true
			}
		}
		next = nextPage(header.Get("Link"), c.baseURL(repo))
	}
	for _, n := range numbers {
		if seen[n] {
			continue
		}
		var out issueJSON
		err := c.do(ctx, repo, http.MethodGet, fmt.Sprintf("%s/issues/%d", repoPath(repo), n), nil, &out)
		if errors.Is(err, ports.ErrNotFound) {
			continue // the roadmap names an issue that does not exist: Plan creates it
		}
		if err != nil {
			return nil, err
		}
		if out.PullRequest == nil {
			issues = append(issues, out.roadmap())
		}
	}
	return issues, nil
}

// SubIssues implements ports.Tracker (GET /issues/{n}/sub_issues).
func (c *Client) SubIssues(ctx context.Context, repo forge.Repository, parent int) ([]int, error) {
	var numbers []int
	next := fmt.Sprintf("%s/issues/%d/sub_issues?per_page=100", repoPath(repo), parent)
	for page := 0; next != "" && page < maxIssuePages; page++ {
		var out []issueJSON
		header, err := c.doWithHeader(ctx, repo, http.MethodGet, next, nil, &out)
		if err != nil {
			return nil, err
		}
		for _, i := range out {
			numbers = append(numbers, i.Number)
		}
		next = nextPage(header.Get("Link"), c.baseURL(repo))
	}
	return numbers, nil
}

// CreateIssue implements ports.Tracker (POST /issues).
func (c *Client) CreateIssue(ctx context.Context, repo forge.Repository, title, body string, milestone int, labels []string) (roadmap.Issue, error) {
	in := map[string]any{"title": title, "body": body}
	if milestone > 0 {
		in["milestone"] = milestone
	}
	if len(labels) > 0 {
		in["labels"] = labels
	}
	var out issueJSON
	err := c.do(ctx, repo, http.MethodPost, repoPath(repo)+"/issues", in, &out)
	return out.roadmap(), err
}

// UpdateIssue implements ports.Tracker (PATCH /issues/{n}).
func (c *Client) UpdateIssue(ctx context.Context, repo forge.Repository, number int, u ports.IssueUpdate) error {
	in := map[string]any{}
	if u.Title != nil {
		in["title"] = *u.Title
	}
	if u.Body != nil {
		in["body"] = *u.Body
	}
	if u.Milestone != nil {
		if *u.Milestone > 0 {
			in["milestone"] = *u.Milestone
		} else {
			in["milestone"] = nil
		}
	}
	if u.State != "" {
		in["state"] = u.State
		if u.StateReason != "" {
			in["state_reason"] = u.StateReason
		}
	}
	return c.do(ctx, repo, http.MethodPatch, fmt.Sprintf("%s/issues/%d", repoPath(repo), number), in, nil)
}

// CommentIssue implements ports.Tracker.
func (c *Client) CommentIssue(ctx context.Context, repo forge.Repository, number int, comment string) error {
	return c.do(ctx, repo, http.MethodPost, fmt.Sprintf("%s/issues/%d/comments", repoPath(repo), number), map[string]string{"body": comment}, nil)
}

// SetParent implements ports.Tracker (POST /issues/{parent}/sub_issues with
// the child's global id, replacing its current parent).
func (c *Client) SetParent(ctx context.Context, repo forge.Repository, parent int, child int64) error {
	return c.do(ctx, repo, http.MethodPost, fmt.Sprintf("%s/issues/%d/sub_issues", repoPath(repo), parent),
		map[string]any{"sub_issue_id": child, "replace_parent": true}, nil)
}

// RemoveParent implements ports.Tracker (DELETE /issues/{parent}/sub_issue).
func (c *Client) RemoveParent(ctx context.Context, repo forge.Repository, parent int, child int64) error {
	err := c.do(ctx, repo, http.MethodDelete, fmt.Sprintf("%s/issues/%d/sub_issue", repoPath(repo), parent),
		map[string]any{"sub_issue_id": child}, nil)
	if errors.Is(err, ports.ErrNotFound) {
		return nil
	}
	return err
}
