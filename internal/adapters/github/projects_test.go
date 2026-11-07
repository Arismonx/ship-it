// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package github

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/core/roadmap"
	"github.com/GregoireF/ship-it/internal/ports"
)

// projectServer answers like GitHub for one project of kind (orgs or users)
// owned by "acme", number 7; the other kind is unknown.
func projectServer(t *testing.T, kind string, calls *[]string) *Client {
	t.Helper()
	var c *Client
	c = newClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*calls = append(*calls, strings.TrimSpace(r.Method+" "+r.URL.RequestURI()+" "+string(body)))
		base := "/" + kind + "/acme/projectsV2/7"
		switch {
		case r.URL.Path == base && r.Method == http.MethodGet:
			fmt.Fprint(w, `{"id":4957,"number":7,"title":"Plan"}`)
		case r.URL.Path == base+"/fields":
			fmt.Fprint(w, `[{"id":1,"name":"Title","data_type":"title"},
				{"id":2,"name":"Status","data_type":"single_select","options":[{"id":"t","name":{"raw":"Todo","html":"Todo"}},{"id":"d","name":{"raw":"Done","html":"Done"}}]},
				{"id":3,"name":"Target date","data_type":"date"}]`)
		case r.URL.Path == base+"/items" && r.Method == http.MethodGet:
			// The next page lives under /organizations/{id}, as GitHub links it.
			w.Header().Set("Link", `<`+c.BaseURL+`/organizations/9919/projectsV2/7/items?per_page=100&after=X>; rel="next"`)
			fmt.Fprint(w, `[{"id":90,"content_type":"Issue","content":{"number":12,"repository_url":"https://api.github.com/repos/acme/shop"},
				"fields":[{"id":2,"data_type":"single_select","value":{"id":"t","name":{"raw":"Todo"}}},{"id":3,"data_type":"date","value":"2026-12-15T01:00:00+01:00"}]},
				{"id":91,"content_type":"Issue","content":{"number":12,"repository_url":"https://api.github.com/repos/acme/other"}},
				{"id":92,"content_type":"DraftIssue","content":{"number":0}}]`)
		case r.URL.Path == "/organizations/9919/projectsV2/7/items":
			fmt.Fprint(w, `[{"id":93,"content_type":"Issue","content":{"number":13,"repository_url":"https://api.github.com/repos/Acme/Shop"},
				"fields":[{"id":2,"data_type":"single_select","value":null}]}]`)
		case r.URL.Path == base+"/items" && r.Method == http.MethodPost:
			fmt.Fprint(w, `{"id":94,"content_type":"Issue"}`)
		case strings.HasPrefix(r.URL.Path, base+"/items/"):
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"message":"Not Found"}`)
		}
	})
	return c
}

func TestProjectRequests(t *testing.T) {
	for _, kind := range []string{forge.ProjectOfOrganization, forge.ProjectOfUser} {
		t.Run(kind, func(t *testing.T) {
			var calls []string
			c := projectServer(t, kind, &calls)
			ctx := context.Background()
			// The short form: the kind is found by asking.
			project, fields, err := c.ProjectFields(ctx, repo, forge.Project{Owner: "acme", Number: 7})
			if err != nil || project.Kind != kind || len(fields) != 3 {
				t.Fatalf("ProjectFields = %+v, %+v, %v", project, fields, err)
			}
			if f := fields[1]; f.Type != roadmap.SingleSelectField || !reflect.DeepEqual(f.Options, []roadmap.FieldOption{{ID: "t", Name: "Todo"}, {ID: "d", Name: "Done"}}) {
				t.Errorf("status field = %+v", f)
			}
			items, err := c.ProjectItems(ctx, repo, project, []int64{2, 3})
			want := []roadmap.ProjectItem{
				{ID: 90, Issue: 12, Values: map[int64]string{2: "t", 3: "2026-12-15"}},
				{ID: 93, Issue: 13, Values: map[int64]string{}},
			}
			if err != nil || !reflect.DeepEqual(items, want) {
				t.Fatalf("ProjectItems = %+v, %v (other repositories and drafts left out)", items, err)
			}
			if id, err := c.AddProjectItem(ctx, repo, project, 1012); err != nil || id != 94 {
				t.Fatalf("AddProjectItem = %d, %v", id, err)
			}
			if err := c.SetProjectField(ctx, repo, project, 94, 2, "d"); err != nil {
				t.Fatal(err)
			}
			if err := c.SetProjectField(ctx, repo, project, 94, 3, ""); err != nil {
				t.Fatal(err)
			}
			if err := c.RemoveProjectItem(ctx, repo, project, 94); err != nil {
				t.Fatal(err)
			}
			base := "/" + kind + "/acme/projectsV2/7"
			for _, want := range []string{
				"GET " + base + "/items?fields=2%2C3&per_page=100",
				"POST " + base + `/items {"id":1012,"type":"Issue"}`,
				"PATCH " + base + `/items/94 {"fields":[{"id":2,"value":"d"}]}`,
				"PATCH " + base + `/items/94 {"fields":[{"id":3,"value":null}]}`,
				"DELETE " + base + "/items/94",
			} {
				if !slices.Contains(calls, want) {
					t.Errorf("missing request %s in %q", want, calls)
				}
			}
		})
	}
}

func TestProjectScopeMissing(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-OAuth-Scopes", "repo, read:org")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"Not Found"}`)
	})
	_, _, err := c.ProjectFields(context.Background(), repo, forge.Project{Owner: "acme", Kind: forge.ProjectOfOrganization, Number: 7})
	if !errors.Is(err, ports.ErrProjectScope) {
		t.Fatalf("err = %v, want ErrProjectScope", err)
	}

	// read:project is enough to read, not to write.
	c = newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-OAuth-Scopes", "repo, read:project")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"Forbidden"}`)
	})
	project := forge.Project{Owner: "acme", Kind: forge.ProjectOfOrganization, Number: 7}
	if _, err := c.ProjectItems(context.Background(), repo, project, nil); errors.Is(err, ports.ErrProjectScope) || !errors.Is(err, ports.ErrForbidden) {
		t.Errorf("read with read:project: %v", err)
	}
	if err := c.SetProjectField(context.Background(), repo, project, 1, 2, "x"); !errors.Is(err, ports.ErrProjectScope) {
		t.Errorf("write with read:project: %v", err)
	}
}

func TestIssueAssignees(t *testing.T) {
	var calls []string
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		calls = append(calls, r.Method+" "+r.URL.Path+" "+string(body))
		fmt.Fprint(w, `{"id":12,"number":42,"title":"VAT","state":"open","assignees":[{"login":"ada"}]}`)
	})
	ctx := context.Background()
	is, err := c.Issue(ctx, repo, 42)
	if err != nil || !reflect.DeepEqual(is.Assignees, []string{"ada"}) {
		t.Fatalf("Issue = %+v, %v", is, err)
	}
	if err := c.AddAssignees(ctx, repo, 42, []string{"grace"}); err != nil {
		t.Fatal(err)
	}
	if want := `POST /repos/acme/shop/issues/42/assignees {"assignees":["grace"]}`; calls[1] != want {
		t.Errorf("call = %s, want %s", calls[1], want)
	}
	if err := c.RemoveAssignees(ctx, repo, 42, []string{"grace"}); err != nil {
		t.Fatal(err)
	}
	if want := `DELETE /repos/acme/shop/issues/42/assignees {"assignees":["grace"]}`; calls[2] != want {
		t.Errorf("call = %s, want %s", calls[2], want)
	}
}

// TestAddProjectItemAlreadyThere: GitHub's "Auto-add sub-issues to project"
// workflow adds a sub-issue as soon as its parent is in the project, so ship's
// own POST gets 422 "Content already exists in this project" (seen on
// ship-it's roadmap). The item GitHub added is used instead, on any page, and
// even when it shows in the listing only a moment after the 422.
func TestAddProjectItemAlreadyThere(t *testing.T) {
	defer func(d time.Duration) { projectItemLookupDelay = d }(projectItemLookupDelay)
	projectItemLookupDelay = time.Millisecond
	base := "/users/ada/projectsV2/3"
	listings := 0
	var c *Client
	c = newClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == base+"/items" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusUnprocessableEntity)
			fmt.Fprint(w, `{"message":"Content already exists in this project"}`)
		case r.URL.Path == base+"/items" && r.URL.Query().Get("after") == "" && listings == 0:
			listings++ // GitHub's workflow has not finished yet
			fmt.Fprint(w, `[]`)
		case r.URL.Path == base+"/items" && r.URL.Query().Get("after") == "":
			w.Header().Set("Link", `<`+c.BaseURL+base+`/items?per_page=100&after=X>; rel="next"`)
			fmt.Fprint(w, `[{"id":70,"content_type":"Issue","content":{"id":1011,"number":11}}]`)
		case r.URL.Path == base+"/items":
			fmt.Fprint(w, `[{"id":71,"content_type":"Issue","content":{"id":1012,"number":12}}]`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	project := forge.Project{Kind: forge.ProjectOfUser, Owner: "ada", Number: 3}
	if id, err := c.AddProjectItem(context.Background(), repo, project, 1012); err != nil || id != 71 {
		t.Fatalf("AddProjectItem = %d, %v; want the item GitHub added, 71", id, err)
	}
	if _, err := c.AddProjectItem(context.Background(), repo, project, 9999); err == nil {
		t.Error("an issue the project lacks: the 422 must stay an error")
	}
}
