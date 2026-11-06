// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/core/rules"
	"github.com/GregoireF/ship-it/internal/core/settings"
	"github.com/GregoireF/ship-it/internal/ports"
)

type staticToken string

func (s staticToken) Token(context.Context, string) (ports.Token, error) {
	if s == "" {
		return ports.Token{}, ports.ErrNotAuthenticated
	}
	return ports.NewToken(string(s), "test"), nil
}

var repo = forge.Repository{Host: "github.com", Owner: "acme", Name: "shop"}

func newClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	c := New(staticToken("t0ken"), "ship-it/test")
	c.BaseURL = server.URL
	return c
}

func TestRequestsCarryTheExpectedHeaders(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		for header, want := range map[string]string{
			"Authorization":        "Bearer t0ken",
			"X-Github-Api-Version": APIVersion,
			"Accept":               "application/vnd.github+json",
			"User-Agent":           "ship-it/test",
		} {
			if got := r.Header.Get(header); got != want {
				t.Errorf("%s = %q, want %q", header, got, want)
			}
		}
		if r.URL.Path != "/repos/acme/shop" {
			t.Errorf("path = %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"default_branch":"main","html_url":"https://github.com/acme/shop"}`)
	})
	info, err := c.Repository(context.Background(), repo)
	if err != nil || info.DefaultBranch != "main" {
		t.Fatalf("Repository = %+v, %v", info, err)
	}
}

// TestNoAnswerIsUnreachable: a transport failure (proxy, DNS, refused
// connection) names the host instead of leaving Go's error alone.
func TestNoAnswerIsUnreachable(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close() // nothing listens there any more
	c := New(staticToken("t0ken"), "ship-it/test")
	c.BaseURL = server.URL
	_, err := c.Repository(context.Background(), repo)
	var unreachable *ports.UnreachableError
	if !errors.As(err, &unreachable) || unreachable.Host != "github.com" {
		t.Fatalf("Repository() with no server = %v", err)
	}
	// A cancelled run is not GitHub's fault.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Repository(ctx, repo); errors.As(err, &unreachable) {
		t.Errorf("a cancelled request reads as unreachable: %v", err)
	}
}

func TestPullRequests(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if got := r.URL.Query().Get("head"); got != "acme:feat/vat" {
				t.Errorf("head filter = %q", got)
			}
			if r.URL.Query().Get("state") != "open" {
				t.Error("only open pull requests must be looked up")
			}
			fmt.Fprint(w, `[]`)
		case http.MethodPost:
			var in map[string]any
			data, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(data, &in); err != nil {
				t.Fatal(err)
			}
			if in["head"] != "feat/vat" || in["base"] != "main" || in["draft"] != true || in["title"] != "✨ feat: add vat" {
				t.Errorf("payload = %v", in)
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"number":7,"html_url":"https://github.com/acme/shop/pull/7","draft":true}`)
		}
	})
	ctx := context.Background()
	if _, found, err := c.FindOpenPullRequest(ctx, repo, "feat/vat"); found || err != nil {
		t.Fatalf("FindOpenPullRequest = %v, %v", found, err)
	}
	pr, err := c.CreatePullRequest(ctx, repo, ports.PullRequestSpec{Title: "✨ feat: add vat", Head: "feat/vat", Base: "main", Draft: true})
	if err != nil || pr.Number != 7 || !pr.Draft || !strings.HasSuffix(pr.URL, "/pull/7") {
		t.Fatalf("CreatePullRequest = %+v, %v", pr, err)
	}
}

func TestLabelsFollowPagination(t *testing.T) {
	var serverURL string
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Query().Get("page") == "":
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/acme/shop/labels?per_page=100&page=2>; rel="next", <%s/repos/acme/shop/labels?per_page=100&page=2>; rel="last"`, serverURL, serverURL))
			fmt.Fprint(w, `[{"name":"kind/bug"},{"name":"size/XS"}]`)
		case r.Method == http.MethodGet:
			fmt.Fprint(w, `[{"name":"kind/feature"}]`)
		case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/shop/issues/7/labels":
			data, _ := io.ReadAll(r.Body)
			if string(data) != `{"labels":["kind/bug","size/XS"]}` {
				t.Errorf("body = %s", data)
			}
			fmt.Fprint(w, `[]`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
		}
	})
	serverURL = c.BaseURL
	labels, err := c.Labels(context.Background(), repo)
	if err != nil || len(labels) != 3 || labels[0].Name != "kind/bug" || labels[2].Name != "kind/feature" {
		t.Fatalf("Labels = %v, %v", labels, err)
	}
	if err := c.AddLabels(context.Background(), repo, 7, []string{"kind/bug", "size/XS"}); err != nil {
		t.Fatal(err)
	}
}

func TestCreateAndRenameLabels(t *testing.T) {
	var calls []string
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		calls = append(calls, r.Method+" "+r.URL.EscapedPath()+" "+string(data))
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{}`)
	})
	ctx := context.Background()
	label := settings.Label{Name: "kind/feature", Color: "1d76db", Description: "New feature or request"}
	if err := c.CreateLabel(ctx, repo, label); err != nil {
		t.Fatal(err)
	}
	if err := c.UpdateLabel(ctx, repo, "good first issue", label); err != nil {
		t.Fatal(err)
	}
	want := []string{
		`POST /repos/acme/shop/labels {"color":"1d76db","description":"New feature or request","name":"kind/feature"}`,
		`PATCH /repos/acme/shop/labels/good%20first%20issue {"color":"1d76db","description":"New feature or request","new_name":"kind/feature"}`,
	}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls =\n%s", strings.Join(calls, "\n"))
	}
}

func TestMergedPullRequests(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case q.Get("state") == "all" && q.Get("head") == "acme:feat/vat":
			fmt.Fprint(w, `[{"number":7,"state":"closed","merged_at":"2026-10-22T10:00:00Z","head":{"ref":"feat/vat","sha":"c1"}}]`)
		case q.Get("state") == "closed" && q.Get("sort") == "updated":
			fmt.Fprint(w, `[{"number":7,"merged_at":"2026-10-22T10:00:00Z","head":{"ref":"feat/vat","sha":"c1"}},
				{"number":6,"merged_at":null,"head":{"ref":"fix/abandoned","sha":"c0"}}]`)
		default:
			t.Errorf("unexpected query %s", r.URL.RawQuery)
		}
	})
	pr, found, err := c.LatestPullRequest(context.Background(), repo, "feat/vat")
	if err != nil || !found || !pr.Merged || pr.HeadSHA != "c1" || pr.HeadBranch != "feat/vat" {
		t.Fatalf("LatestPullRequest = %+v, %v, %v", pr, found, err)
	}
	merged, err := c.MergedPullRequests(context.Background(), repo, 50)
	if err != nil || len(merged) != 1 || merged[0].Number != 7 {
		t.Fatalf("MergedPullRequests = %+v, %v", merged, err)
	}
}

func TestClosePullRequest(t *testing.T) {
	var calls []string
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		calls = append(calls, r.Method+" "+r.URL.Path+" "+string(data))
		fmt.Fprint(w, `{}`)
	})
	if err := c.ClosePullRequest(context.Background(), repo, 8, "Closed with ship undo."); err != nil {
		t.Fatal(err)
	}
	want := `POST /repos/acme/shop/issues/8/comments {"body":"Closed with ship undo."}` + "\n" +
		`PATCH /repos/acme/shop/pulls/8 {"state":"closed"}`
	if strings.Join(calls, "\n") != want {
		t.Fatalf("calls:\n%s", strings.Join(calls, "\n"))
	}
}

func TestErrorMapping(t *testing.T) {
	tests := []struct {
		status  int
		headers map[string]string
		body    string
		want    error
	}{
		{401, nil, `{"message":"Bad credentials"}`, ports.ErrTokenRejected},
		{403, map[string]string{"X-RateLimit-Remaining": "0"}, `{"message":"API rate limit exceeded"}`, ports.ErrRateLimited},
		{429, nil, `{}`, ports.ErrRateLimited},
		{404, nil, `{"message":"Not Found"}`, ports.ErrNotFound},
		{403, map[string]string{"X-RateLimit-Remaining": "4999"}, `{"message":"Resource not accessible by integration"}`, ports.ErrForbidden},
	}
	for _, tt := range tests {
		c := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
			for k, v := range tt.headers {
				w.Header().Set(k, v)
			}
			w.WriteHeader(tt.status)
			fmt.Fprint(w, tt.body)
		})
		_, err := c.Repository(context.Background(), repo)
		if !errors.Is(err, tt.want) {
			t.Errorf("status %d: error = %v, want %v", tt.status, err, tt.want)
		}
	}
}

// TestBranchRules reads the rules of a branch with a slash in its name,
// across two pages, with the parameters of the pattern rules.
func TestBranchRules(t *testing.T) {
	var serverURL string
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/shop/rules/branches/feat/12-login" || r.URL.Query().Get("per_page") != "100" {
			t.Errorf("unexpected %s", r.URL)
		}
		if r.URL.Query().Get("page") == "" {
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/acme/shop/rules/branches/feat/12-login?per_page=100&page=2>; rel="next"`, serverURL))
			fmt.Fprint(w, `[{"type":"branch_name_pattern","ruleset_source_type":"Repository","ruleset_source":"acme/shop","ruleset_id":42,
				"parameters":{"name":"Jira key","negate":false,"operator":"regex","pattern":"^[a-z]+/[A-Z]+-[0-9]+"}},
				{"type":"required_signatures","ruleset_id":42}]`)
			return
		}
		fmt.Fprint(w, `[{"type":"commit_author_email_pattern","ruleset_id":7,"parameters":{"operator":"ends_with","pattern":"@acme.example","negate":true}},
			{"type":"pull_request","ruleset_id":7,"parameters":{"required_approving_review_count":1}}]`)
	})
	serverURL = c.BaseURL
	got, err := c.BranchRules(context.Background(), repo, "feat/12-login")
	if err != nil || len(got) != 4 {
		t.Fatalf("BranchRules = %+v, %v", got, err)
	}
	name := got[0]
	if name.Type != rules.BranchNamePattern || name.RulesetID != 42 || name.Pattern == nil ||
		*name.Pattern != (rules.Pattern{Name: "Jira key", Operator: rules.Regex, Pattern: "^[a-z]+/[A-Z]+-[0-9]+"}) {
		t.Fatalf("branch rule = %+v %+v", name, name.Pattern)
	}
	if got[1].Type != rules.RequiredSignatures || got[1].Pattern != nil {
		t.Fatalf("signatures = %+v", got[1])
	}
	if p := got[2].Pattern; p == nil || !p.Negate || p.Operator != rules.EndsWith {
		t.Fatalf("email rule = %+v", got[2])
	}
	if got[3].Type != "pull_request" || got[3].Pattern != nil {
		t.Fatalf("other rules are kept, without pattern: %+v", got[3])
	}
}

func TestValidationErrorKeepsGitHubDetails(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprint(w, `{"message":"Validation Failed","errors":[{"resource":"PullRequest","code":"custom","message":"No commits between main and feat/x"}]}`)
	})
	_, err := c.CreatePullRequest(context.Background(), repo, ports.PullRequestSpec{})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 422 || !strings.Contains(err.Error(), "No commits between main and feat/x") {
		t.Fatalf("error = %v", err)
	}
}

func TestMissingTokenStopsBeforeAnyRequest(t *testing.T) {
	c := newClient(t, func(http.ResponseWriter, *http.Request) { t.Error("no request may be sent without a token") })
	c.Credentials = staticToken("")
	if _, err := c.Repository(context.Background(), repo); !errors.Is(err, ports.ErrNotAuthenticated) {
		t.Fatalf("error = %v", err)
	}
}

func TestIssueTitle(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/repos/acme/shop/issues/42" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		fmt.Fprint(w, `{"number":42,"title":"Cart crashes on an empty basket"}`)
	})
	title, err := c.IssueTitle(context.Background(), repo, 42)
	if err != nil || title != "Cart crashes on an empty basket" {
		t.Fatalf("IssueTitle = %q, %v", title, err)
	}
}

func TestFileContent(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/ship-config/contents/presets/base.json" || r.URL.Query().Get("ref") != "3f2a" {
			t.Errorf("request = %s", r.URL)
		}
		fmt.Fprint(w, `{"type":"file","encoding":"base64","content":"eyJzaGlwIjp7\nfX0="}`)
	})
	data, err := c.FileContent(context.Background(), forge.Repository{Host: "github.com", Owner: "acme", Name: "ship-config"}, "presets/base.json", "3f2a")
	if err != nil || string(data) != `{"ship":{}}` {
		t.Fatalf("FileContent = %q, %v", data, err)
	}
}

func TestChecksAndReviews(t *testing.T) {
	var serverURL string
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/acme/shop/commits/abc/check-runs" && r.URL.Query().Get("page") == "":
			if r.URL.Query().Get("filter") != "latest" {
				t.Errorf("filter = %q", r.URL.Query().Get("filter"))
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/acme/shop/commits/abc/check-runs?filter=latest&per_page=100&page=2>; rel="next"`, serverURL))
			fmt.Fprint(w, `{"total_count":2,"check_runs":[{"name":"test","status":"completed","conclusion":"failure","html_url":"https://x/1"}]}`)
		case r.URL.Path == "/repos/acme/shop/commits/abc/check-runs":
			fmt.Fprint(w, `{"total_count":2,"check_runs":[{"name":"lint","status":"in_progress","conclusion":null}]}`)
		case r.URL.Path == "/repos/acme/shop/commits/abc/status":
			fmt.Fprint(w, `{"state":"pending","statuses":[{"context":"ci/legacy","state":"success","target_url":null}]}`)
		case r.URL.Path == "/repos/acme/shop/pulls/7/reviews":
			fmt.Fprint(w, `[{"state":"CHANGES_REQUESTED","user":{"login":"bob"}},{"state":"APPROVED","user":null}]`)
		default:
			t.Errorf("unexpected %s", r.URL)
		}
	})
	serverURL = c.BaseURL
	ctx := context.Background()
	runs, err := c.CheckRuns(ctx, repo, "abc")
	if err != nil || len(runs) != 2 || runs[0].Conclusion != "failure" || runs[1].Status != "in_progress" {
		t.Fatalf("CheckRuns = %+v, %v", runs, err)
	}
	statuses, err := c.CommitStatuses(ctx, repo, "abc")
	if err != nil || len(statuses) != 1 || statuses[0].Context != "ci/legacy" {
		t.Fatalf("CommitStatuses = %+v, %v", statuses, err)
	}
	reviews, err := c.Reviews(ctx, repo, 7)
	if err != nil || len(reviews) != 1 || reviews[0].Author != "bob" {
		t.Fatalf("Reviews = %+v, %v (a deleted account is skipped)", reviews, err)
	}
}

// rotatingCredentials hands out "old" until Forget, then "new".
type rotatingCredentials struct{ forgotten bool }

func (r *rotatingCredentials) Token(context.Context, string) (ports.Token, error) {
	if r.forgotten {
		return ports.NewToken("new", "test"), nil
	}
	return ports.NewToken("old", "test"), nil
}

func (r *rotatingCredentials) Forget(string) { r.forgotten = true }

func TestARejectedTokenIsLookedUpAgain(t *testing.T) {
	var auth []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = append(auth, r.Header.Get("Authorization"))
		if r.Header.Get("Authorization") != "Bearer new" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"message":"Bad credentials"}`)
			return
		}
		fmt.Fprint(w, `{"default_branch":"main"}`)
	}))
	t.Cleanup(server.Close)
	c := New(&rotatingCredentials{}, "ship-it/test")
	c.BaseURL = server.URL
	info, err := c.Repository(context.Background(), repo)
	if err != nil || info.DefaultBranch != "main" || len(auth) != 2 {
		t.Fatalf("Repository = %+v, %v after %q", info, err, auth)
	}
}
