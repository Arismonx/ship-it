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
	"testing"

	"github.com/GregoireF/ship-it/internal/adapters/ghauth"
	"github.com/GregoireF/ship-it/internal/adapters/github"
	"github.com/GregoireF/ship-it/internal/surface/cli"
)

// TestShipAfterAMerge: the pull request of feat/vat was squash-merged on
// GitHub; running ship with new work goes back to the up-to-date main with
// that work, removes feat/vat, and ships the work on a fresh branch. Then
// ship undo takes everything back, one operation at a time.
func TestShipAfterAMerge(t *testing.T) {
	var mergedHead string
	var closed []string
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case r.URL.Path == "/repos/acme/shop/issues/8/comments" && r.Method == http.MethodPost:
			closed = append(closed, "comment")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{}`)
		case r.URL.Path == "/repos/acme/shop/pulls/8" && r.Method == http.MethodPatch:
			closed = append(closed, "close")
			fmt.Fprint(w, `{}`)
		case r.URL.Path == "/repos/acme/shop/pulls" && q.Get("state") == "all" && q.Get("head") != "acme:feat/vat":
			fmt.Fprintf(w, `[{"number":8,"state":"open","head":{"ref":%q}}]`, strings.TrimPrefix(q.Get("head"), "acme:"))
		case r.URL.Path == "/repos/acme/shop" && r.Method == http.MethodGet:
			fmt.Fprint(w, `{"default_branch":"main"}`)
		case r.URL.Path == "/repos/acme/shop/pulls" && q.Get("state") == "all" && q.Get("head") == "acme:feat/vat":
			fmt.Fprintf(w, `[{"number":7,"state":"closed","merged_at":"2026-10-22T10:00:00Z","head":{"ref":"feat/vat","sha":%q}}]`, mergedHead)
		case r.URL.Path == "/repos/acme/shop/pulls" && q.Get("state") == "closed":
			fmt.Fprintf(w, `[{"number":7,"merged_at":"2026-10-22T10:00:00Z","head":{"ref":"feat/vat","sha":%q}}]`, mergedHead)
		case r.URL.Path == "/repos/acme/shop/pulls" && r.Method == http.MethodGet:
			fmt.Fprint(w, `[]`)
		case r.URL.Path == "/repos/acme/shop/pulls" && r.Method == http.MethodPost:
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"number":8,"html_url":"https://github.com/acme/shop/pull/8","head":{"ref":%q}}`, in["head"])
		default:
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		}
	})
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	credentials := ghauth.New(nil)
	credentials.Getenv = func(name string) string { return map[string]string{"SHIP_TOKEN": "t0ken"}[name] }
	forge := github.New(credentials, "ship-it/test")
	forge.BaseURL = server.URL

	s := newScenario(t, forge)
	bare := filepath.Join(t.TempDir(), "shop.git")
	s.sh("init", "--bare", "--quiet", "--initial-branch=main", bare)
	s.sh("remote", "add", "origin", "https://github.com/acme/shop.git")
	// Fetch and push really go to the local bare repository; the configured
	// URL still names acme/shop on GitHub.
	s.sh("config", "url."+filepath.ToSlash(bare)+".insteadOf", "https://github.com/acme/shop.git")
	s.write("README.md", "# shop\n")
	s.sh("add", "--all")
	s.sh("commit", "--quiet", "-m", "🎉 chore: init")
	s.sh("push", "--quiet", "origin", "main")

	// feat/vat was pushed, then squash-merged on GitHub as one new commit.
	s.sh("switch", "--quiet", "--create", "feat/vat")
	s.write("vat.go", "package shop\n\nconst VAT = 20\n")
	s.sh("add", "--all")
	s.sh("commit", "--quiet", "-m", "✨ feat: add vat")
	s.sh("push", "--quiet", "origin", "feat/vat")
	mergedHead = s.sh("rev-parse", "HEAD")
	tree := s.sh("rev-parse", "HEAD^{tree}")
	main := s.sh("--git-dir", bare, "rev-parse", "main")
	squash := s.sh("--git-dir", bare, "commit-tree", tree, "-p", main, "-m", "✨ feat: add vat (#7)")
	s.sh("--git-dir", bare, "update-ref", "refs/heads/main", squash)

	// New work started on the merged branch.
	s.write("README.md", "# shop\n\nVAT included.\n")
	code, _, stderr := s.ship("--yes", "--skip-gates")
	if code != cli.ExitOK {
		t.Fatalf("ship: exit %d\n%s", code, stderr)
	}
	for _, want := range []string{"Pull request #7 of feat/vat is merged: back on main", "Merged branches removed: feat/vat"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr misses %q:\n%s", want, stderr)
		}
	}
	branch := s.sh("branch", "--show-current")
	if branch == "feat/vat" || branch == "main" || s.sh("rev-parse", "HEAD~1") != squash {
		t.Fatalf("the new work must ship on a fresh branch from the merged main; on %s", branch)
	}
	if s.sh("branch", "--list", "feat/vat") != "" {
		t.Fatal("feat/vat should be removed")
	}

	// Undo, step by step, newest first.
	// 1. The publication changes GitHub: no terminal, no --yes, no change.
	if code, _, stderr := s.ship("undo"); code != cli.ExitNeedsHuman || !strings.Contains(stderr, "SHIP-E0407") || len(closed) != 0 {
		t.Fatalf("undo without consent: exit %d\n%s", code, stderr)
	}
	if code, _, stderr := s.ship("undo", "--yes"); code != cli.ExitOK || strings.Join(closed, ",") != "comment,close" {
		t.Fatalf("undo --yes: exit %d, calls %v\n%s", code, closed, stderr)
	}
	if out := s.sh("--git-dir", bare, "branch", "--list", branch); out != "" {
		t.Fatalf("remote branch %s should be deleted", branch)
	}
	// 2. The delivery: back on main, the work uncommitted again.
	if code, _, stderr := s.ship("undo"); code != cli.ExitOK || s.sh("branch", "--show-current") != "main" ||
		!strings.Contains(s.sh("status", "--porcelain"), "README.md") {
		t.Fatalf("undo the delivery: exit %d\n%s", code, stderr)
	}
	// 3. The tidy: feat/vat comes back at its merged commit.
	if code, _, stderr := s.ship("undo"); code != cli.ExitOK || s.sh("rev-parse", "feat/vat") != mergedHead {
		t.Fatalf("undo the tidy: exit %d\n%s", code, stderr)
	}
}
