// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/GregoireF/ship-it/internal/ports"
)

// stackBody follows the stack object of GitHub's schema (GitHub Docs, REST
// API endpoints for stacks).
const stackBody = `{"id":1,"number":7,"open":true,"base":{"ref":"main"},"pull_requests":[
 {"number":41,"state":"open","draft":false,"merged_at":null,"html_url":"https://github.com/acme/shop/pull/41","head":{"ref":"feat/a","sha":"a1"},"base":{"ref":"main"}},
 {"number":42,"state":"open","draft":true,"merged_at":null,"html_url":"https://github.com/acme/shop/pull/42","head":{"ref":"feat/b","sha":"b1"},"base":{"ref":"feat/a"}}]}`

func TestStacksAPI(t *testing.T) {
	var bodies []string
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		data, _ := json.Marshal(in)
		bodies = append(bodies, r.Method+" "+r.URL.Path+" "+string(data))
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/shop/stacks":
			if r.URL.Query().Get("pull_request") != "41" {
				t.Errorf("query = %s", r.URL.RawQuery)
			}
			fmt.Fprint(w, "["+stackBody+"]")
		case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/shop/stacks":
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, stackBody)
		case r.URL.Path == "/repos/acme/shop/stacks/7/add":
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, `{"message":"Another request is modifying the stack"}`)
		case r.URL.Path == "/repos/acme/shop/stacks/7/unstack":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPatch && r.URL.Path == "/repos/acme/shop/pulls/42":
			fmt.Fprint(w, `{}`)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/shop/pulls/42":
			fmt.Fprint(w, `{"number":42,"state":"open","merged_at":null,"head":{"ref":"feat/b","sha":"b1"},"base":{"ref":"feat/a"},"body":"Part 2 of 2, after #41.",
				"stack":{"id":1,"number":7,"size":2,"position":2,"base":{"ref":"main","sha":"m1"}}}`)
		default:
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		}
	})
	ctx := context.Background()
	stacks, err := c.StacksOf(ctx, repo, 41)
	if err != nil || len(stacks) != 1 || stacks[0].Number != 7 || stacks[0].Base != "main" || len(stacks[0].PullRequests) != 2 ||
		stacks[0].PullRequests[1].HeadBranch != "feat/b" || stacks[0].PullRequests[1].BaseBranch != "feat/a" {
		t.Fatalf("StacksOf = %+v, %v", stacks, err)
	}
	created, err := c.CreateStack(ctx, repo, []int{41, 42})
	if err != nil || created.Number != 7 {
		t.Fatalf("CreateStack = %+v, %v", created, err)
	}
	if _, err := c.AddToStack(ctx, repo, 7, []int{43}); !errors.Is(err, ports.ErrConflict) {
		t.Errorf("AddToStack 409 = %v", err)
	}
	if dissolved, err := c.Unstack(ctx, repo, 7); err != nil || !dissolved {
		t.Errorf("Unstack 204 = %v, %v", dissolved, err)
	}
	if err := c.SetPullRequestBase(ctx, repo, 42, "main"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetPullRequestBody(ctx, repo, 42, "Part 1 of 1."); err != nil {
		t.Fatal(err)
	}
	pr, err := c.PullRequest(ctx, repo, 42)
	if err != nil || pr.StackNumber != 7 || pr.BaseBranch != "feat/a" || pr.Body != "Part 2 of 2, after #41." {
		t.Errorf("PullRequest = %+v, %v", pr, err)
	}
	want := []string{
		`GET /repos/acme/shop/stacks null`,
		`POST /repos/acme/shop/stacks {"pull_requests":[41,42]}`,
		`POST /repos/acme/shop/stacks/7/add {"pull_requests":[43]}`,
		`POST /repos/acme/shop/stacks/7/unstack null`,
		`PATCH /repos/acme/shop/pulls/42 {"base":"main"}`,
		`PATCH /repos/acme/shop/pulls/42 {"body":"Part 1 of 1."}`,
		`GET /repos/acme/shop/pulls/42 null`,
	}
	if !reflect.DeepEqual(bodies, want) {
		t.Errorf("requests:\n%q", bodies)
	}
}

func TestUnstackLeavesTheLockedOnes(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, stackBody) })
	if dissolved, err := c.Unstack(context.Background(), repo, 7); err != nil || dissolved {
		t.Errorf("Unstack 200 = %v, %v", dissolved, err)
	}
}

func TestNoStacksAPI(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	})
	if _, err := c.StacksOf(context.Background(), repo, 0); !errors.Is(err, ports.ErrStacksUnavailable) {
		t.Errorf("404 on /stacks = %v, want ErrStacksUnavailable", err)
	}
	if _, err := c.CreateStack(context.Background(), repo, []int{1, 2}); !errors.Is(err, ports.ErrStacksUnavailable) {
		t.Errorf("create = %v", err)
	}
}

func TestCreateStackRefused(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprint(w, `{"message":"Validation Failed","errors":[{"message":"base ref must match the previous head ref"}]}`)
	})
	_, err := c.CreateStack(context.Background(), repo, []int{1, 2})
	var apiErr *APIError
	if !errors.Is(err, ports.ErrInvalid) || !errors.As(err, &apiErr) || apiErr.Details[0] != "base ref must match the previous head ref" {
		t.Errorf("422 = %v", err)
	}
}

func TestMergeAsync(t *testing.T) {
	var sent map[string]any
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/repos/acme/shop/pulls/42/merge-async":
			_ = json.NewDecoder(r.Body).Decode(&sent)
			w.WriteHeader(http.StatusAccepted)
			fmt.Fprint(w, `{"status":"pending","details":{"message":"queued","uuid":"u-1","merge_method":"squash","merge_action":"default","expected_head_sha":"b1","bypass_rules":false}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/shop/pulls/42/merge-async/u-1":
			fmt.Fprint(w, `{"status":"merged","details":{"message":"Merged","sha":"m2"}}`)
		default:
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		}
	})
	ctx := context.Background()
	st, err := c.MergeAsync(ctx, repo, 42, "b1")
	if err != nil || st.Status != "pending" || st.UUID != "u-1" {
		t.Fatalf("MergeAsync = %+v, %v", st, err)
	}
	if sent["bypass_rules"] != false || sent["sha"] != "b1" || sent["merge_action"] != "default" {
		t.Errorf("request = %v: never bypass the rules", sent)
	}
	st, err = c.MergeResult(ctx, repo, 42, "u-1")
	if err != nil || st.Status != "merged" || st.SHA != "m2" || st.UUID != "u-1" {
		t.Errorf("MergeResult = %+v, %v", st, err)
	}
	if got := PathTemplate("/repos/acme/shop/pulls/42/merge-async/u-1"); got != "/repos/{owner}/{repo}/pulls/{number}/merge-async/{uuid}" {
		t.Errorf("template = %s", got)
	}
}
