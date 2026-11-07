// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/GregoireF/ship-it/internal/adapters/ghauth"
	"github.com/GregoireF/ship-it/internal/adapters/github"
	"github.com/GregoireF/ship-it/internal/surface/cli"
)

// fakeGitHub answers the three REST calls a publication makes.
type fakeGitHub struct {
	mu      sync.Mutex
	created []map[string]any
	labeled map[string][]string // by path
	// rules is the JSON GET rules/branches/{branch} returns for any branch;
	// asked records the branches asked for.
	rules string
	asked []string
}

func (f *fakeGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/shop":
		fmt.Fprint(w, `{"default_branch":"main","html_url":"https://github.com/acme/shop"}`)
	case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/shop/pulls":
		for i, pr := range f.created {
			if "acme:"+pr["head"].(string) == r.URL.Query().Get("head") {
				fmt.Fprintf(w, `[{"number":%d,"html_url":"https://github.com/acme/shop/pull/%d","draft":%v}]`, i+1, i+1, pr["draft"])
				return
			}
		}
		fmt.Fprint(w, `[]`)
	case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/shop/pulls":
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.created = append(f.created, in)
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"number":%d,"html_url":"https://github.com/acme/shop/pull/%d","draft":%v}`, len(f.created), len(f.created), in["draft"])
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/repos/acme/shop/rules/branches/"):
		f.asked = append(f.asked, strings.TrimPrefix(r.URL.Path, "/repos/acme/shop/rules/branches/"))
		if f.rules == "" {
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
			return
		}
		fmt.Fprint(w, f.rules)
	case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/shop/labels":
		fmt.Fprint(w, `[{"name":"kind/feature"},{"name":"kind/bug"},{"name":"size/XS"},{"name":"wontfix"}]`)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/labels"):
		var in struct{ Labels []string }
		_ = json.NewDecoder(r.Body).Decode(&in)
		if f.labeled == nil {
			f.labeled = map[string][]string{}
		}
		f.labeled[r.URL.Path] = in.Labels
		fmt.Fprint(w, `[]`)
	default:
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	}
}

// TestShipPublishes runs `ship --yes` on a repository whose origin is a
// github.com URL: fetches and pushes really go to a local bare repository (insteadOf)
// and the API is a local server.
func TestShipPublishes(t *testing.T) {
	api := &fakeGitHub{}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	credentials := ghauth.New(nil)
	credentials.Getenv = func(name string) string { return map[string]string{"SHIP_TOKEN": "t0ken"}[name] }
	forge := github.New(credentials, "ship-it/test")
	forge.BaseURL = server.URL

	s := newScenario(t, forge)
	bare := filepath.Join(t.TempDir(), "shop.git")
	s.sh("init", "--bare", "--quiet", bare)
	s.sh("remote", "add", "origin", "https://github.com/acme/shop.git")
	s.sh("config", "url."+filepath.ToSlash(bare)+".insteadOf", "https://github.com/acme/shop.git")
	s.write("README.md", "# shop\n")
	s.sh("add", "--all")
	s.sh("commit", "--quiet", "-m", "🎉 chore: init")
	s.sh("push", "--quiet", "origin", "main")
	s.sh("fetch", "--quiet", bare, "main:refs/remotes/origin/main")

	s.write("vat.go", "package shop\n\nconst VAT = 20\n")
	code, stdout, stderr := s.ship("--yes")
	if code != cli.ExitOK {
		t.Fatalf("ship --yes: exit %d\n%s", code, stderr)
	}
	branch := s.sh("branch", "--show-current")
	if pushed := s.sh("--git-dir", bare, "rev-parse", "refs/heads/"+branch); pushed != s.sh("rev-parse", "HEAD") {
		t.Fatalf("branch %s not pushed: %s", branch, pushed)
	}
	if upstream := s.sh("rev-parse", "--abbrev-ref", "@{upstream}"); upstream != "origin/"+branch {
		t.Errorf("upstream = %s", upstream)
	}
	if len(api.created) != 1 {
		t.Fatalf("pull requests created: %d", len(api.created))
	}
	pr := api.created[0]
	if pr["head"] != branch || pr["base"] != "main" || pr["draft"] != true || !strings.HasPrefix(pr["title"].(string), "✨ feat") {
		t.Errorf("pull request = %v", pr)
	}
	// One commit without a body: the title says why; the body only sums up
	// the diff, read by git on the pushed branch.
	if pr["body"] != "**Changes:** 1 file: 1 added (`vat.go`).\n\n<!-- ship-it -->\n" {
		t.Errorf("body:\n%s", pr["body"])
	}
	if !strings.Contains(stderr, "Draft pull request #1") || strings.TrimSpace(stdout[strings.LastIndex(strings.TrimSpace(stdout), "\n")+1:]) != "https://github.com/acme/shop/pull/1" {
		t.Errorf("output:\n%s\n%s", stdout, stderr)
	}

	if got := strings.Join(api.labeled["/repos/acme/shop/issues/1/labels"], ","); got != "kind/feature,size/XS" {
		t.Errorf("labels = %q, want the repository's own kind/feature and size/XS", got)
	}

	code, stdout, _ = s.ship("status")
	if code != cli.ExitOK || !strings.Contains(stdout, "#1 (draft)") || !strings.Contains(stdout, "Mark the draft ready for review") ||
		!strings.Contains(stdout, "1 ahead, 0 behind main") || strings.Contains(stdout, "cli.status.") {
		t.Fatalf("ship status: exit %d\n%s", code, stdout)
	}

	// A new commit published again updates the open pull request.
	s.write("vat.go", "package shop\n\nconst VAT = 21\n")
	s.sh("commit", "--quiet", "-am", "🐛 fix: update vat rate")
	if code, _, stderr := s.ship("publish"); code != cli.ExitOK || !strings.Contains(stderr, "pull request #1 is updated") || len(api.created) != 1 {
		t.Fatalf("ship publish: exit %d, created %d\n%s", code, len(api.created), stderr)
	}
	if pushed := s.sh("--git-dir", bare, "rev-parse", "refs/heads/"+branch); pushed != s.sh("rev-parse", "HEAD") {
		t.Fatal("the new commit was not pushed")
	}

	// Published work cannot be undone locally yet: undo refuses, changing nothing.
	if code, _, stderr := s.ship("undo"); code != cli.ExitError || !strings.Contains(stderr, "SHIP-E0406") {
		t.Fatalf("undo after publishing must refuse: exit %d\n%s", code, stderr)
	}
}
