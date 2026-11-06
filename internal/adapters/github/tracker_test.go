// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package github

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/roadmap"
)

func TestTrackerRequests(t *testing.T) {
	var calls []string
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		calls = append(calls, r.Method+" "+r.URL.Path+" "+string(body))
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/shop/milestones":
			fmt.Fprint(w, `[{"number":1,"title":"Phase 1","description":null,"state":"open","due_on":"2026-12-15T08:00:00Z","open_issues":2,"closed_issues":1}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/shop/issues":
			fmt.Fprint(w, `[{"id":11,"number":1,"title":"CLI","body":"x <!-- ship-it:roadmap -->","state":"open","milestone":{"number":1}},
				{"id":12,"number":2,"title":"a pull request","body":"<!-- ship-it:roadmap -->","state":"open","pull_request":{}},
				{"id":13,"number":3,"title":"by hand","body":"none","state":"closed"}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/shop/issues/9":
			fmt.Fprint(w, `{"id":19,"number":9,"title":"named","body":null,"state":"open"}`)
		default:
			fmt.Fprint(w, `{"number":4,"id":14}`)
		}
	})
	ctx := context.Background()
	ms, err := c.Milestones(ctx, repo)
	if err != nil || len(ms) != 1 || ms[0].Due != "2026-12-15" || ms[0].OpenIssues != 2 || ms[0].ClosedIssues != 1 {
		t.Fatalf("Milestones = %+v, %v", ms, err)
	}
	issues, err := c.TrackedIssues(ctx, repo, roadmap.Marker, []int{9})
	if err != nil || len(issues) != 2 || issues[0].Number != 1 || issues[0].Milestone != 1 || issues[1].Number != 9 {
		t.Fatalf("TrackedIssues = %+v, %v (pull requests and unmanaged issues left out)", issues, err)
	}
	if _, err := c.CreateMilestone(ctx, repo, roadmap.Milestone{Title: "Phase 2", Due: "2027-01-31"}); err != nil {
		t.Fatal(err)
	}
	if err := c.SetParent(ctx, repo, 1, 14); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`POST /repos/acme/shop/milestones {"description":"","due_on":"2027-01-31T00:00:00Z","title":"Phase 2"}`,
		`POST /repos/acme/shop/issues/1/sub_issues {"replace_parent":true,"sub_issue_id":14}`,
	} {
		found := false
		for _, c := range calls {
			found = found || c == want
		}
		if !found {
			t.Errorf("missing request %s in %q", want, calls)
		}
	}
}
