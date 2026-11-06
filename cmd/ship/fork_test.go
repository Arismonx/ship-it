// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/GregoireF/ship-it/internal/adapters/ghauth"
	"github.com/GregoireF/ship-it/internal/adapters/github"
	"github.com/GregoireF/ship-it/internal/surface/cli"
)

// forkGitHub is GitHub for ada's fork of acme/shop: the fork has a parent,
// pull requests open on the parent.
type forkGitHub struct {
	mu      sync.Mutex
	created []map[string]any
}

func (f *forkGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/ada/shop":
		fmt.Fprint(w, `{"default_branch":"main","fork":true,"parent":{"name":"shop","owner":{"login":"acme"},
			"default_branch":"main","clone_url":"https://github.com/acme/shop.git","ssh_url":"git@github.com:acme/shop.git"}}`)
	case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/shop/pulls":
		fmt.Fprint(w, `[]`)
	case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/shop/pulls":
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.created = append(f.created, in)
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"number":%d,"html_url":"https://github.com/acme/shop/pull/%d","draft":%v}`, len(f.created), len(f.created), in["draft"])
	default:
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	}
}

// TestShipPublishesFromAFork clones ada's fork, lets acme/shop move on, and
// runs `ship --yes`: the branch is rebased on the parent's main (fetched by
// URL, no remote added), pushed to the fork, and its pull request opens on
// the parent with head "ada:<branch>". Both repositories are local bare
// repositories (insteadOf); the API is a local server.
func TestShipPublishesFromAFork(t *testing.T) {
	api := &forkGitHub{}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	credentials := ghauth.New(nil)
	credentials.Getenv = func(name string) string { return map[string]string{"SHIP_TOKEN": "t0ken"}[name] }
	forge := github.New(credentials, "ship-it/test")
	forge.BaseURL = server.URL

	s := newScenario(t, forge)
	root := t.TempDir()
	parent, fork := filepath.Join(root, "acme-shop.git"), filepath.Join(root, "ada-shop.git")
	s.sh("init", "--bare", "--quiet", parent)
	s.sh("init", "--bare", "--quiet", fork)
	s.sh("remote", "add", "origin", "https://github.com/ada/shop.git")
	s.sh("config", "url."+filepath.ToSlash(fork)+".insteadOf", "https://github.com/ada/shop.git")
	s.sh("config", "url."+filepath.ToSlash(parent)+".insteadOf", "https://github.com/acme/shop.git")
	s.write("README.md", "# shop\n")
	s.sh("add", "--all")
	s.sh("commit", "--quiet", "-m", "🎉 chore: init")
	s.sh("push", "--quiet", "origin", "main")
	s.sh("push", "--quiet", "https://github.com/acme/shop.git", "main")
	s.sh("fetch", "--quiet", "origin")

	// The parent moves on: a maintainer's commit the fork does not have.
	other := filepath.Join(root, "maintainer")
	s.sh("clone", "--quiet", "--branch", "main", parent, other)
	if err := os.WriteFile(filepath.Join(other, "LICENSE"), []byte("Apache-2.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.sh("-C", other, "-c", "user.name=Maintainer", "-c", "user.email=m@example.invalid", "add", "LICENSE")
	s.sh("-C", other, "-c", "user.name=Maintainer", "-c", "user.email=m@example.invalid", "commit", "--quiet", "-m", "📄 docs: add the licence")
	s.sh("-C", other, "push", "--quiet", "origin", "main")
	parentMain := s.sh("--git-dir", parent, "rev-parse", "refs/heads/main")

	s.write("vat.go", "package shop\n\nconst VAT = 20\n")
	code, _, stderr := s.ship("--yes")
	if code != cli.ExitOK {
		t.Fatalf("ship --yes: exit %d\n%s", code, stderr)
	}
	branch := s.sh("branch", "--show-current")
	head := s.sh("rev-parse", "HEAD")
	if pushed := s.sh("--git-dir", fork, "rev-parse", "refs/heads/"+branch); pushed != head {
		t.Fatalf("branch %s not pushed to the fork: %s", branch, pushed)
	}
	if out, _ := s.git.Run(t.Context(), "--git-dir", parent, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch); len(out) != 0 {
		t.Fatal("ship pushed to the parent")
	}
	if _, err := s.git.Run(t.Context(), "merge-base", "--is-ancestor", parentMain, "HEAD"); err != nil {
		t.Fatalf("the branch is not on the parent's main: %v", err)
	}
	if remotes := s.sh("remote"); remotes != "origin" {
		t.Errorf("remotes = %q: the parent is fetched without adding one", remotes)
	}
	if len(api.created) != 1 {
		t.Fatalf("pull requests created on the parent: %d", len(api.created))
	}
	if pr := api.created[0]; pr["head"] != "ada:"+branch || pr["base"] != "main" {
		t.Errorf("pull request = %v", pr)
	}
	if !strings.Contains(stderr, "acme/shop/pull/1") && !strings.Contains(stderr, "#1") {
		t.Errorf("output:\n%s", stderr)
	}
}
