// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/ports"
)

func TestPathTemplate(t *testing.T) {
	for in, want := range map[string]string{
		"/repos/acme/shop":                                            "/repos/{owner}/{repo}",
		"/repos/acme/shop/pulls?state=open&head=x":                    "/repos/{owner}/{repo}/pulls",
		"/repos/acme/shop/pulls/42":                                   "/repos/{owner}/{repo}/pulls/{number}",
		"/repos/acme/shop/issues/42/comments":                         "/repos/{owner}/{repo}/issues/{number}/comments",
		"/repos/acme/shop/issues/42/labels/size%2FS":                  "/repos/{owner}/{repo}/issues/{number}/labels/{name}",
		"/repos/acme/shop/labels/bug":                                 "/repos/{owner}/{repo}/labels/{name}",
		"/repos/acme/shop/rules/branches/feat/12-x?per_page=100":      "/repos/{owner}/{repo}/rules/branches/{branch}",
		"/repos/acme/shop/commits/3f2a9c1e/check-runs?filter=latest":  "/repos/{owner}/{repo}/commits/{ref}/check-runs",
		"/repos/acme/shop/commits/3f2a9c1e/status":                    "/repos/{owner}/{repo}/commits/{ref}/status",
		"/repos/acme/shop/contents/presets/acme-internal.json?ref=3f": "/repos/{owner}/{repo}/contents/{path}",
		"/orgs/acme/projectsV2/7/items/91?fields=1,2":                 "/orgs/{org}/projectsV2/{number}/items/{number}",
		"/users/ada/projectsV2/3/fields?per_page=100":                 "/users/{username}/projectsV2/{number}/fields",
		"/organizations/9919/projectsV2/7/items?after=Y3Vy":           "/organizations/{number}/projectsV2/{number}/items",
		"/repos/acme/shop/issues/42/assignees":                        "/repos/{owner}/{repo}/issues/{number}/assignees",
		"/user":                                                       "/user",
	} {
		if got := PathTemplate(in); got != want {
			t.Errorf("PathTemplate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRequestsAreLoggedWithoutSecrets(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "4987")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusUnprocessableEntity)
			fmt.Fprint(w, `{"message":"Validation Failed"}`)
			return
		}
		fmt.Fprint(w, `[{"number":42,"html_url":"https://github.com/acme/shop/pull/42"}]`)
	})
	var buf bytes.Buffer
	c.Log = slog.New(slog.NewJSONHandler(&buf, nil))
	ctx := context.Background()
	if _, _, err := c.FindOpenPullRequest(ctx, repo, "feat/secret-project"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreatePullRequest(ctx, repo, ports.PullRequestSpec{Title: "confidential title", Head: "x", Base: "main"}); err == nil {
		t.Fatal("422 should fail")
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 records:\n%s", buf.String())
	}
	var get, post map[string]any
	json.Unmarshal([]byte(lines[0]), &get)
	json.Unmarshal([]byte(lines[1]), &post)
	if get["msg"] != "github.request" || get["method"] != "GET" || get["path"] != "/repos/{owner}/{repo}/pulls" ||
		get["status"] != float64(200) || get["rate_remaining"] != float64(4987) || get["step"] != "github" || get["level"] != "INFO" {
		t.Errorf("GET record = %v", get)
	}
	if _, ok := get["duration_ms"]; !ok {
		t.Error("duration_ms missing")
	}
	if post["status"] != float64(422) || post["level"] != "WARN" {
		t.Errorf("POST record = %v", post)
	}
	for _, leak := range []string{"t0ken", "acme", "shop", "secret-project", "confidential"} {
		if strings.Contains(buf.String(), leak) {
			t.Errorf("%q leaked into the log:\n%s", leak, buf.String())
		}
	}
}
