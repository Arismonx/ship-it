// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package github

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestPullRequestContent(t *testing.T) {
	var serverURL string
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path + "?" + r.URL.RawQuery {
		case "/repos/acme/shop/pulls/7?":
			fmt.Fprint(w, `{"number":7,"html_url":"https://github.com/acme/shop/pull/7","labels":[{"name":"size/S"}]}`)
		case "/repos/acme/shop/pulls/7/commits?per_page=100":
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/acme/shop/pulls/7/commits?per_page=100&page=2>; rel="next"`, serverURL))
			fmt.Fprint(w, `[{"commit":{"message":"✨ feat: a\n\nbody"}}]`)
		case "/repos/acme/shop/pulls/7/commits?per_page=100&page=2":
			fmt.Fprint(w, `[{"commit":{"message":"🐛 fix: b"}}]`)
		case "/repos/acme/shop/pulls/7/files?per_page=100":
			fmt.Fprint(w, `[{"filename":"a.go","additions":3,"deletions":1,"status":"modified"},{"filename":"go.sum","additions":40,"deletions":0}]`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	serverURL = c.BaseURL
	content, err := c.PullRequestContent(context.Background(), repo, 7)
	if err != nil {
		t.Fatal(err)
	}
	pr := content.PullRequest
	if pr.Number != 7 || len(pr.Labels) != 1 || pr.Labels[0] != "size/S" {
		t.Errorf("pull request = %+v", pr)
	}
	if len(content.CommitMessages) != 2 || content.CommitMessages[1] != "🐛 fix: b" {
		t.Errorf("messages = %q", content.CommitMessages)
	}
	if len(content.Files) != 2 || content.Files[0].Path != "a.go" || content.Files[0].Added != 3 || content.Files[0].Deleted != 1 {
		t.Errorf("files = %+v", content.Files)
	}
}
