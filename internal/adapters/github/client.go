// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/core/review"
	"github.com/GregoireF/ship-it/internal/core/rules"
	"github.com/GregoireF/ship-it/internal/core/settings"
	"github.com/GregoireF/ship-it/internal/ports"
)

// APIVersion is the REST API version ship-it is written against. Requests
// without the header get 2022-11-28, supported until 2028-03-10
// (docs.github.com, "API Versions", checked 2026-10-23).
const APIVersion = "2026-03-10"

// maxResponseSize bounds what is read from one response.
const maxResponseSize = 10 << 20

// Client implements ports.Forge over the REST API.
type Client struct {
	Credentials ports.Credentials
	// HTTP defaults to a client with a 30 s timeout.
	HTTP *http.Client
	// UserAgent is required by GitHub; defaults to "ship-it".
	UserAgent string
	// BaseURL overrides the API root derived from the host (tests).
	BaseURL string
	// Log receives one INFO record per request: method, path template,
	// status, duration and remaining rate limit. Never the token, the body
	// or the repository name. nil logs nothing.
	Log *slog.Logger
}

var _ ports.Forge = (*Client)(nil)

// New returns a Client authenticating with credentials.
func New(credentials ports.Credentials, userAgent string) *Client {
	return &Client{Credentials: credentials, UserAgent: userAgent, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// APIError is an error response GitHub explained.
type APIError struct {
	Status           int
	Message          string
	Details          []string
	DocumentationURL string
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("github: %d %s", e.Status, e.Message)
	if len(e.Details) > 0 {
		msg += " (" + strings.Join(e.Details, "; ") + ")"
	}
	return msg
}

// Repository implements ports.Forge.
func (c *Client) Repository(ctx context.Context, repo forge.Repository) (ports.RepositoryInfo, error) {
	var out struct {
		DefaultBranch string `json:"default_branch"`
		HTMLURL       string `json:"html_url"`
		Fork          bool   `json:"fork"`
		Parent        *struct {
			Name  string `json:"name"`
			Owner struct {
				Login string `json:"login"`
			} `json:"owner"`
			DefaultBranch string `json:"default_branch"`
			CloneURL      string `json:"clone_url"`
			SSHURL        string `json:"ssh_url"`
		} `json:"parent"`
	}
	err := c.do(ctx, repo, http.MethodGet, repoPath(repo), nil, &out)
	info := ports.RepositoryInfo{DefaultBranch: out.DefaultBranch, HTMLURL: out.HTMLURL}
	if p := out.Parent; out.Fork && p != nil && p.Owner.Login != "" && p.Name != "" {
		info.Parent = &ports.ParentRepository{
			Repository:    forge.Repository{Host: repo.Host, Owner: p.Owner.Login, Name: p.Name},
			DefaultBranch: p.DefaultBranch, CloneURL: p.CloneURL, SSHURL: p.SSHURL,
		}
	}
	return info, err
}

type pullRequestJSON struct {
	Number   int     `json:"number"`
	HTMLURL  string  `json:"html_url"`
	Draft    bool    `json:"draft"`
	State    string  `json:"state"`
	MergedAt *string `json:"merged_at"`
	Head     struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
	User *struct {
		Login string `json:"login"`
	} `json:"user"`
}

func (p pullRequestJSON) port() ports.PullRequest {
	pr := ports.PullRequest{
		Number: p.Number, URL: p.HTMLURL, Draft: p.Draft, State: p.State,
		Merged: p.MergedAt != nil, HeadBranch: p.Head.Ref, HeadSHA: p.Head.SHA,
	}
	if p.User != nil {
		pr.Author = p.User.Login
	}
	for _, l := range p.Labels {
		pr.Labels = append(pr.Labels, l.Name)
	}
	return pr
}

// FindOpenPullRequest implements ports.Forge.
func (c *Client) FindOpenPullRequest(ctx context.Context, repo forge.Repository, branch string) (ports.PullRequest, bool, error) {
	query := url.Values{"state": {"open"}, "head": {headOf(repo, branch)}, "per_page": {"1"}}
	var out []pullRequestJSON
	if err := c.do(ctx, repo, http.MethodGet, repoPath(repo)+"/pulls?"+query.Encode(), nil, &out); err != nil || len(out) == 0 {
		return ports.PullRequest{}, false, err
	}
	return out[0].port(), true, nil
}

// LatestPullRequest implements ports.Forge. The list is sorted by creation,
// newest first (the API default).
func (c *Client) LatestPullRequest(ctx context.Context, repo forge.Repository, branch string) (ports.PullRequest, bool, error) {
	query := url.Values{"state": {"all"}, "head": {headOf(repo, branch)}, "per_page": {"1"}}
	var out []pullRequestJSON
	if err := c.do(ctx, repo, http.MethodGet, repoPath(repo)+"/pulls?"+query.Encode(), nil, &out); err != nil || len(out) == 0 {
		return ports.PullRequest{}, false, err
	}
	return out[0].port(), true, nil
}

// headOf is the head filter of a branch: "<owner>:<branch>", the owner of
// repo unless branch names its own (a fork's branch). Branch names never
// hold ":" (git check-ref-format).
func headOf(repo forge.Repository, branch string) string {
	if strings.Contains(branch, ":") {
		return branch
	}
	return repo.Owner + ":" + branch
}

// MergedPullRequests implements ports.Forge: closed pull requests, most
// recently updated first, keeping the merged ones.
func (c *Client) MergedPullRequests(ctx context.Context, repo forge.Repository, limit int) ([]ports.PullRequest, error) {
	query := url.Values{"state": {"closed"}, "sort": {"updated"}, "direction": {"desc"}, "per_page": {strconv.Itoa(min(limit, 100))}}
	var out []pullRequestJSON
	if err := c.do(ctx, repo, http.MethodGet, repoPath(repo)+"/pulls?"+query.Encode(), nil, &out); err != nil {
		return nil, err
	}
	var merged []ports.PullRequest
	for _, p := range out {
		if p.MergedAt != nil {
			merged = append(merged, p.port())
		}
	}
	return merged, nil
}

// OpenPullRequestPatches implements ports.Forge: GET /pulls?state=open&base=…
// (newest first), then the files of each with their patch. GitHub leaves the
// patch out of a binary or a very large file; the caller treats such a file
// as changed as a whole.
func (c *Client) OpenPullRequestPatches(ctx context.Context, repo forge.Repository, base string, limit int) ([]ports.PullRequestPatches, error) {
	query := url.Values{"state": {"open"}, "sort": {"created"}, "direction": {"desc"}, "per_page": {strconv.Itoa(min(max(limit, 1), 100))}}
	if base != "" {
		query.Set("base", base)
	}
	var out []struct {
		pullRequestJSON
		Title string `json:"title"`
	}
	if err := c.do(ctx, repo, http.MethodGet, repoPath(repo)+"/pulls?"+query.Encode(), nil, &out); err != nil {
		return nil, err
	}
	if len(out) > limit {
		out = out[:limit]
	}
	open := make([]ports.PullRequestPatches, 0, len(out))
	for _, p := range out {
		pr := ports.PullRequestPatches{PullRequest: p.port(), Head: p.Head.Ref, Title: p.Title}
		next := fmt.Sprintf("%s/pulls/%d/files?per_page=100", repoPath(repo), p.Number)
		for page := 0; next != "" && page < maxFilePages; page++ {
			var files []struct {
				Filename         string `json:"filename"`
				PreviousFilename string `json:"previous_filename"`
				Status           string `json:"status"`
				Patch            string `json:"patch"`
			}
			header, err := c.doWithHeader(ctx, repo, http.MethodGet, next, nil, &files)
			if err != nil {
				return nil, err
			}
			for _, f := range files {
				pr.Files = append(pr.Files, ports.FilePatch{Path: f.Filename, PreviousPath: f.PreviousFilename, Status: f.Status, Patch: f.Patch})
			}
			next = nextPage(header.Get("Link"), c.baseURL(repo))
		}
		open = append(open, pr)
	}
	return open, nil
}

// CurrentUser implements ports.Forge (GET /user).
func (c *Client) CurrentUser(ctx context.Context, repo forge.Repository) (ports.User, error) {
	var out struct {
		Login string  `json:"login"`
		ID    int64   `json:"id"`
		Name  *string `json:"name"`
	}
	if err := c.do(ctx, repo, http.MethodGet, "/user", nil, &out); err != nil {
		return ports.User{}, err
	}
	user := ports.User{Login: out.Login, ID: out.ID}
	if out.Name != nil {
		user.Name = *out.Name
	}
	return user, nil
}

// ClosePullRequest implements ports.Forge: the comment first, so that the
// reason is on the pull request before anyone is notified of the closing.
func (c *Client) ClosePullRequest(ctx context.Context, repo forge.Repository, number int, comment string) error {
	if comment != "" {
		path := fmt.Sprintf("%s/issues/%d/comments", repoPath(repo), number)
		if err := c.do(ctx, repo, http.MethodPost, path, map[string]string{"body": comment}, nil); err != nil {
			return err
		}
	}
	path := fmt.Sprintf("%s/pulls/%d", repoPath(repo), number)
	return c.do(ctx, repo, http.MethodPatch, path, map[string]string{"state": "closed"}, nil)
}

// CreatePullRequest implements ports.Forge.
func (c *Client) CreatePullRequest(ctx context.Context, repo forge.Repository, spec ports.PullRequestSpec) (ports.PullRequest, error) {
	in := map[string]any{"title": spec.Title, "body": spec.Body, "head": spec.Head, "base": spec.Base, "draft": spec.Draft}
	var out pullRequestJSON
	err := c.do(ctx, repo, http.MethodPost, repoPath(repo)+"/pulls", in, &out)
	return out.port(), err
}

// maxLabelPages bounds the label listing (100 per page).
const maxLabelPages = 20

// Labels implements ports.Forge, following the Link header from page to page.
func (c *Client) Labels(ctx context.Context, repo forge.Repository) ([]settings.Label, error) {
	var labels []settings.Label
	next := repoPath(repo) + "/labels?per_page=100"
	for page := 0; next != "" && page < maxLabelPages; page++ {
		var out []struct {
			Name        string `json:"name"`
			Color       string `json:"color"`
			Description string `json:"description"`
		}
		header, err := c.doWithHeader(ctx, repo, http.MethodGet, next, nil, &out)
		if err != nil {
			return nil, err
		}
		for _, l := range out {
			labels = append(labels, settings.Label{Name: l.Name, Color: l.Color, Description: l.Description})
		}
		next = nextPage(header.Get("Link"), c.baseURL(repo))
	}
	return labels, nil
}

// CreateLabel implements ports.Forge.
func (c *Client) CreateLabel(ctx context.Context, repo forge.Repository, label settings.Label) error {
	in := map[string]string{"name": label.Name, "color": label.Color, "description": label.Description}
	return c.do(ctx, repo, http.MethodPost, repoPath(repo)+"/labels", in, nil)
}

// UpdateLabel implements ports.Forge.
func (c *Client) UpdateLabel(ctx context.Context, repo forge.Repository, name string, label settings.Label) error {
	in := map[string]string{"new_name": label.Name, "color": label.Color, "description": label.Description}
	return c.do(ctx, repo, http.MethodPatch, repoPath(repo)+"/labels/"+url.PathEscape(name), in, nil)
}

// AddLabels implements ports.Forge. Pull requests are issues for labels.
func (c *Client) AddLabels(ctx context.Context, repo forge.Repository, number int, labels []string) error {
	if len(labels) == 0 {
		return nil
	}
	path := fmt.Sprintf("%s/issues/%d/labels", repoPath(repo), number)
	return c.do(ctx, repo, http.MethodPost, path, map[string][]string{"labels": labels}, nil)
}

// RequestReviewers implements ports.Forge with POST
// /repos/{owner}/{repo}/pulls/{n}/requested_reviewers.
func (c *Client) RequestReviewers(ctx context.Context, repo forge.Repository, number int, users, teams []string) error {
	if len(users) == 0 && len(teams) == 0 {
		return nil
	}
	in := map[string][]string{"reviewers": nonNil(users), "team_reviewers": nonNil(teams)}
	path := fmt.Sprintf("%s/pulls/%d/requested_reviewers", repoPath(repo), number)
	return c.do(ctx, repo, http.MethodPost, path, in, nil)
}

// nonNil sends [] rather than null for an empty list.
func nonNil(list []string) []string {
	if list == nil {
		return []string{}
	}
	return list
}

// RemoveLabel implements ports.Forge. A 404 means the label is not there any
// more, which is the wanted outcome.
func (c *Client) RemoveLabel(ctx context.Context, repo forge.Repository, number int, label string) error {
	path := fmt.Sprintf("%s/issues/%d/labels/%s", repoPath(repo), number, url.PathEscape(label))
	if err := c.do(ctx, repo, http.MethodDelete, path, nil, nil); err != nil && !errors.Is(err, ports.ErrNotFound) {
		return err
	}
	return nil
}

// IssueTitle implements ports.Forge with GET /repos/{owner}/{repo}/issues/{n},
// which also answers for pull requests.
func (c *Client) IssueTitle(ctx context.Context, repo forge.Repository, number int) (string, error) {
	var out struct {
		Title string `json:"title"`
	}
	err := c.do(ctx, repo, http.MethodGet, fmt.Sprintf("%s/issues/%d", repoPath(repo), number), nil, &out)
	return out.Title, err
}

// maxCheckPages bounds the listings of checks and reviews (100 per page).
const maxCheckPages = 10

// CheckRuns implements ports.Forge with GET
// /repos/{owner}/{repo}/commits/{ref}/check-runs (filter=latest: the latest
// run of each check).
func (c *Client) CheckRuns(ctx context.Context, repo forge.Repository, commit string) ([]review.CheckRun, error) {
	var runs []review.CheckRun
	next := fmt.Sprintf("%s/commits/%s/check-runs?filter=latest&per_page=100", repoPath(repo), url.PathEscape(commit))
	for page := 0; next != "" && page < maxCheckPages; page++ {
		var out struct {
			CheckRuns []struct {
				Name       string `json:"name"`
				Status     string `json:"status"`
				Conclusion string `json:"conclusion"`
				HTMLURL    string `json:"html_url"`
			} `json:"check_runs"`
		}
		header, err := c.doWithHeader(ctx, repo, http.MethodGet, next, nil, &out)
		if err != nil {
			return nil, err
		}
		for _, r := range out.CheckRuns {
			runs = append(runs, review.CheckRun{Name: r.Name, Status: r.Status, Conclusion: r.Conclusion, URL: r.HTMLURL})
		}
		next = nextPage(header.Get("Link"), c.baseURL(repo))
	}
	return runs, nil
}

// CommitStatuses implements ports.Forge with the combined status, GET
// /repos/{owner}/{repo}/commits/{ref}/status, which keeps the latest status
// of each context.
func (c *Client) CommitStatuses(ctx context.Context, repo forge.Repository, commit string) ([]review.CommitStatus, error) {
	var out struct {
		Statuses []struct {
			Context   string `json:"context"`
			State     string `json:"state"`
			TargetURL string `json:"target_url"`
		} `json:"statuses"`
	}
	route := fmt.Sprintf("%s/commits/%s/status?per_page=100", repoPath(repo), url.PathEscape(commit))
	if err := c.do(ctx, repo, http.MethodGet, route, nil, &out); err != nil {
		return nil, err
	}
	statuses := make([]review.CommitStatus, 0, len(out.Statuses))
	for _, s := range out.Statuses {
		statuses = append(statuses, review.CommitStatus{Context: s.Context, State: s.State, URL: s.TargetURL})
	}
	return statuses, nil
}

// Reviews implements ports.Forge with GET
// /repos/{owner}/{repo}/pulls/{number}/reviews, in chronological order.
func (c *Client) Reviews(ctx context.Context, repo forge.Repository, number int) ([]review.Review, error) {
	var reviews []review.Review
	next := fmt.Sprintf("%s/pulls/%d/reviews?per_page=100", repoPath(repo), number)
	for page := 0; next != "" && page < maxCheckPages; page++ {
		var out []struct {
			State string `json:"state"`
			User  *struct {
				Login string `json:"login"`
			} `json:"user"`
		}
		header, err := c.doWithHeader(ctx, repo, http.MethodGet, next, nil, &out)
		if err != nil {
			return nil, err
		}
		for _, r := range out {
			if r.User != nil { // a deleted account reviews as nobody
				reviews = append(reviews, review.Review{Author: r.User.Login, State: r.State})
			}
		}
		next = nextPage(header.Get("Link"), c.baseURL(repo))
	}
	return reviews, nil
}

// maxFileContent bounds FileContent: the contents API returns files up to
// 1 MB in its JSON form, far above any configuration file.
const maxFileContent = 1 << 20

// FileContent implements ports.Forge with GET /repos/{owner}/{repo}/contents/{path}?ref=,
// whose default JSON form carries the file in base64.
func (c *Client) FileContent(ctx context.Context, repo forge.Repository, path, commit string) ([]byte, error) {
	var out struct {
		Type     string `json:"type"`
		Encoding string `json:"encoding"`
		Content  string `json:"content"`
	}
	escaped := make([]string, 0, strings.Count(path, "/")+1)
	for _, segment := range strings.Split(path, "/") {
		escaped = append(escaped, url.PathEscape(segment))
	}
	route := fmt.Sprintf("%s/contents/%s?ref=%s", repoPath(repo), strings.Join(escaped, "/"), url.QueryEscape(commit))
	if err := c.do(ctx, repo, http.MethodGet, route, nil, &out); err != nil {
		return nil, err
	}
	if out.Type != "file" || out.Encoding != "base64" {
		return nil, fmt.Errorf("github: %s is not a file (%s, %s)", path, out.Type, out.Encoding)
	}
	data, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(out.Content, "\n", ""))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFileContent {
		return nil, fmt.Errorf("github: %s is larger than 1 MB", path)
	}
	return data, nil
}

// Bounds of the pull request listings, 100 per page: GitHub lists at most 250
// commits and 3,000 files of a pull request.
const (
	maxCommitPages = 3
	maxFilePages   = 30
)

// PullRequestContent implements ports.Forge with GET pulls/{n}, then its
// commits and files, page after page.
func (c *Client) PullRequestContent(ctx context.Context, repo forge.Repository, number int) (ports.PullRequestContent, error) {
	base := fmt.Sprintf("%s/pulls/%d", repoPath(repo), number)
	var pr pullRequestJSON
	if err := c.do(ctx, repo, http.MethodGet, base, nil, &pr); err != nil {
		return ports.PullRequestContent{}, err
	}
	content := ports.PullRequestContent{PullRequest: pr.port()}
	next := base + "/commits?per_page=100"
	for page := 0; next != "" && page < maxCommitPages; page++ {
		var out []struct {
			Commit struct {
				Message string `json:"message"`
			} `json:"commit"`
		}
		header, err := c.doWithHeader(ctx, repo, http.MethodGet, next, nil, &out)
		if err != nil {
			return ports.PullRequestContent{}, err
		}
		for _, commit := range out {
			content.CommitMessages = append(content.CommitMessages, commit.Commit.Message)
		}
		next = nextPage(header.Get("Link"), c.baseURL(repo))
	}
	next = base + "/files?per_page=100"
	for page := 0; next != "" && page < maxFilePages; page++ {
		var out []struct {
			Filename  string `json:"filename"`
			Additions int    `json:"additions"`
			Deletions int    `json:"deletions"`
		}
		header, err := c.doWithHeader(ctx, repo, http.MethodGet, next, nil, &out)
		if err != nil {
			return ports.PullRequestContent{}, err
		}
		for _, f := range out {
			content.Files = append(content.Files, ports.FileStat{Path: f.Filename, Added: f.Additions, Deleted: f.Deletions})
		}
		next = nextPage(header.Get("Link"), c.baseURL(repo))
	}
	return content, nil
}

// maxRulePages bounds the rules listing (100 per page, the maximum).
const maxRulePages = 10

// BranchRules implements ports.Forge with GET /repos/{owner}/{repo}/rules/branches/{branch}:
// the active rules of every ruleset (repository, organization, enterprise)
// that targets a branch of that name; rulesets in "evaluate" or "disabled"
// enforcement are not returned (docs.github.com, "REST API endpoints for
// rules", checked 2026-10-24). The list is paginated like the others.
func (c *Client) BranchRules(ctx context.Context, repo forge.Repository, branch string) ([]rules.Rule, error) {
	segments := strings.Split(branch, "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	var found []rules.Rule
	next := repoPath(repo) + "/rules/branches/" + strings.Join(segments, "/") + "?per_page=100"
	for page := 0; next != "" && page < maxRulePages; page++ {
		var out []struct {
			Type       string          `json:"type"`
			Parameters json.RawMessage `json:"parameters"`
			RulesetID  int             `json:"ruleset_id"`
		}
		header, err := c.doWithHeader(ctx, repo, http.MethodGet, next, nil, &out)
		if err != nil {
			return nil, err
		}
		for _, r := range out {
			rule := rules.Rule{Type: rules.Type(r.Type), RulesetID: r.RulesetID}
			switch rule.Type {
			case rules.BranchNamePattern, rules.CommitMessagePattern, rules.CommitAuthorEmailPattern, rules.CommitterEmailPattern:
				var p rules.Pattern
				if err := json.Unmarshal(r.Parameters, &p); err != nil {
					return nil, fmt.Errorf("github: rule %s: %w", r.Type, err)
				}
				rule.Pattern = &p
			}
			found = append(found, rule)
		}
		next = nextPage(header.Get("Link"), c.baseURL(repo))
	}
	return found, nil
}

// nextPage returns the path of the rel="next" link, relative to base, or "".
func nextPage(link, base string) string {
	for _, part := range strings.Split(link, ",") {
		target, rel, ok := strings.Cut(part, ";")
		if !ok || !strings.Contains(rel, `rel="next"`) {
			continue
		}
		target = strings.Trim(strings.TrimSpace(target), "<>")
		if path, ok := strings.CutPrefix(target, base); ok {
			return path
		}
	}
	return ""
}

func repoPath(repo forge.Repository) string {
	return "/repos/" + url.PathEscape(repo.Owner) + "/" + url.PathEscape(repo.Name)
}

func (c *Client) do(ctx context.Context, repo forge.Repository, method, path string, in, out any) error {
	_, err := c.doWithHeader(ctx, repo, method, path, in, out)
	return err
}

func (c *Client) baseURL(repo forge.Repository) string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return repo.APIBaseURL()
}

// doWithHeader sends one request and decodes the JSON response into out. It
// returns the response headers (pagination, rate limit).
func (c *Client) doWithHeader(ctx context.Context, repo forge.Repository, method, path string, in, out any) (http.Header, error) {
	token, err := c.Credentials.Token(ctx, repo.Host)
	if err != nil {
		return nil, err
	}
	var payload []byte
	if in != nil {
		if payload, err = json.Marshal(in); err != nil {
			return nil, err
		}
	}
	header, err := c.send(ctx, repo, method, path, payload, in != nil, out, token)
	// A token cached for the life of a long process (ship mcp) may have
	// been refreshed since (gh auth refresh): forget it and try once more
	// with the current one. A 401 means GitHub did nothing, so even a POST
	// is safe to send again.
	if forgetter, ok := c.Credentials.(ports.TokenForgetter); ok && errors.Is(err, ports.ErrTokenRejected) {
		forgetter.Forget(repo.Host)
		if fresh, ferr := c.Credentials.Token(ctx, repo.Host); ferr == nil && fresh.Reveal() != token.Reveal() {
			return c.send(ctx, repo, method, path, payload, in != nil, out, fresh)
		}
	}
	return header, err
}

// send performs one request with token.
func (c *Client) send(ctx context.Context, repo forge.Repository, method, path string, payload []byte, hasBody bool, out any, token ports.Token) (http.Header, error) {
	var body io.Reader
	if hasBody {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL(repo)+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", APIVersion)
	req.Header.Set("Authorization", "Bearer "+token.Reveal())
	req.Header.Set("User-Agent", c.userAgent())
	if hasBody {
		req.Header.Set("Content-Type", "application/json")
	}

	start := time.Now()
	resp, err := c.httpClient().Do(req)
	if err != nil {
		c.log(ctx, method, path, 0, time.Since(start), nil, err)
		if ctx.Err() == nil {
			// The transport failed (proxy, DNS, refused connection): the
			// surfaces say GitHub does not answer instead of Go's words.
			err = &ports.UnreachableError{Host: repo.Host, Err: err}
		}
		return nil, err
	}
	defer resp.Body.Close()
	c.log(ctx, method, path, resp.StatusCode, time.Since(start), resp.Header, nil)
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return resp.Header, responseError(resp, data)
	}
	if out == nil || len(data) == 0 { // 204 No Content
		return resp.Header, nil
	}
	return resp.Header, json.Unmarshal(data, out)
}

// responseError maps an error response to the ports errors when one fits,
// keeping GitHub's explanation.
func responseError(resp *http.Response, data []byte) error {
	var payload struct {
		Message          string `json:"message"`
		DocumentationURL string `json:"documentation_url"`
		Errors           []struct {
			Message  string `json:"message"`
			Code     string `json:"code"`
			Field    string `json:"field"`
			Resource string `json:"resource"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(data, &payload)
	apiErr := &APIError{Status: resp.StatusCode, Message: payload.Message, DocumentationURL: payload.DocumentationURL}
	if apiErr.Message == "" {
		apiErr.Message = http.StatusText(resp.StatusCode)
	}
	for _, e := range payload.Errors {
		detail := e.Message
		if detail == "" {
			detail = strings.TrimSpace(e.Resource + " " + e.Field + " " + e.Code)
		}
		apiErr.Details = append(apiErr.Details, detail)
	}

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return errors.Join(ports.ErrTokenRejected, apiErr)
	case resp.StatusCode == http.StatusTooManyRequests,
		resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0":
		return errors.Join(ports.ErrRateLimited, apiErr)
	case resp.StatusCode == http.StatusNotFound:
		return errors.Join(ports.ErrNotFound, apiErr)
	case resp.StatusCode == http.StatusForbidden:
		return errors.Join(ports.ErrForbidden, apiErr)
	}
	return apiErr
}

// log records one request. Transport errors are unwrapped: *url.Error
// repeats the full URL, which names the repository.
func (c *Client) log(ctx context.Context, method, path string, status int, elapsed time.Duration, header http.Header, err error) {
	if c.Log == nil {
		return
	}
	attrs := []any{"step", "github", "method", method, "path", PathTemplate(path), "status", status, "duration_ms", elapsed.Milliseconds()}
	if remaining := header.Get("X-RateLimit-Remaining"); remaining != "" {
		if n, convErr := strconv.Atoi(remaining); convErr == nil {
			attrs = append(attrs, "rate_remaining", n)
		}
	}
	level := slog.LevelInfo
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		attrs = append(attrs, "error", err.Error())
		level = slog.LevelWarn
	} else if status >= 400 {
		level = slog.LevelWarn
	}
	c.Log.Log(ctx, level, "github.request", attrs...)
}

var (
	repoPrefix = regexp.MustCompile(`^/repos/[^/]+/[^/]+`)
	// orgPrefix and userPrefix hide the owner of a project.
	orgPrefix  = regexp.MustCompile(`^/orgs/[^/]+`)
	userPrefix = regexp.MustCompile(`^/users/[^/]+`)
	number     = regexp.MustCompile(`/[0-9]+(/|$)`)
	labelName  = regexp.MustCompile(`/labels/[^/]+$`)
	branchName = regexp.MustCompile(`/rules/branches/.+$`)
	// commitRef and contentPath hide what identifies the user's work.
	commitRef   = regexp.MustCompile(`/commits/[^/]+`)
	contentPath = regexp.MustCompile(`/contents/.+$`)
	// mergeRequest hides the id of an asynchronous merge request.
	mergeRequest = regexp.MustCompile(`/merge-async/[^/]+$`)
)

// PathTemplate turns a request path into its route, as GitHub documents it:
// "/repos/acme/shop/pulls/42?state=open" becomes "/repos/{owner}/{repo}/pulls/{number}".
// Names and numbers identify the user's work: only the route is logged.
func PathTemplate(path string) string {
	path, _, _ = strings.Cut(path, "?")
	path = repoPrefix.ReplaceAllString(path, "/repos/{owner}/{repo}")
	path = orgPrefix.ReplaceAllString(path, "/orgs/{org}")
	path = userPrefix.ReplaceAllString(path, "/users/{username}")
	path = labelName.ReplaceAllString(path, "/labels/{name}")
	path = branchName.ReplaceAllString(path, "/rules/branches/{branch}")
	path = commitRef.ReplaceAllString(path, "/commits/{ref}")
	path = contentPath.ReplaceAllString(path, "/contents/{path}")
	path = mergeRequest.ReplaceAllString(path, "/merge-async/{uuid}")
	return number.ReplaceAllString(path, "/{number}$1")
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func (c *Client) userAgent() string {
	if c.UserAgent != "" {
		return c.UserAgent
	}
	return "ship-it"
}
