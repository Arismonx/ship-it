// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package github

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/GregoireF/ship-it/internal/ports"
)

func TestOpenPullRequestPatches(t *testing.T) {
	var c *Client
	var listed string
	c = newClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/acme/shop/pulls":
			listed = r.URL.RawQuery
			fmt.Fprint(w, `[{"number":7,"title":"Add VAT","html_url":"u7","head":{"ref":"feat/vat"}},
				{"number":5,"title":"Old","html_url":"u5","head":{"ref":"fix/old"}}]`)
		case r.URL.Path == "/repos/acme/shop/pulls/7/files" && r.URL.Query().Get("page") == "":
			w.Header().Set("Link", `<`+c.BaseURL+`/repos/acme/shop/pulls/7/files?per_page=100&page=2>; rel="next"`)
			fmt.Fprint(w, `[{"filename":"shop.go","status":"modified","patch":"@@ -12,2 +12,3 @@\n-a\n+b"}]`)
		case r.URL.Path == "/repos/acme/shop/pulls/7/files":
			fmt.Fprint(w, `[{"filename":"logo.png","status":"modified"},
				{"filename":"helpers.go","previous_filename":"util.go","status":"renamed"}]`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	got, err := c.OpenPullRequestPatches(context.Background(), repo, "main", 1)
	if err != nil {
		t.Fatal(err)
	}
	if listed != "base=main&direction=desc&per_page=1&sort=created&state=open" {
		t.Errorf("listed with %q", listed)
	}
	want := []ports.FilePatch{
		{Path: "shop.go", Status: "modified", Patch: "@@ -12,2 +12,3 @@\n-a\n+b"},
		{Path: "logo.png", Status: "modified"},
		{Path: "helpers.go", PreviousPath: "util.go", Status: "renamed"},
	}
	if len(got) != 1 || got[0].PullRequest.Number != 7 || got[0].Head != "feat/vat" || got[0].Title != "Add VAT" || !reflect.DeepEqual(got[0].Files, want) {
		t.Fatalf("OpenPullRequestPatches = %+v (limit 1: the newest only, every page of files)", got)
	}
}
