// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/adapters/ghauth"
	"github.com/GregoireF/ship-it/internal/adapters/github"
	"github.com/GregoireF/ship-it/internal/surface/cli"
)

// TestPublishRebasesOnTheMovedBase: feat/vat was pushed with its pull
// request; main moved on; ship publish replays feat/vat on main and pushes it
// with a lease.
func TestPublishRebasesOnTheMovedBase(t *testing.T) {
	api := &fakeGitHub{}
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
	s.sh("config", "url."+filepath.ToSlash(bare)+".insteadOf", "https://github.com/acme/shop.git")
	s.write("README.md", "# shop\n")
	s.sh("add", "--all")
	s.sh("commit", "--quiet", "-m", "🎉 chore: init")
	s.sh("push", "--quiet", "origin", "main")

	s.write("vat.go", "package shop\n\nconst VAT = 20\n")
	if code, _, stderr := s.ship("--yes"); code != cli.ExitOK {
		t.Fatalf("first ship: exit %d\n%s", code, stderr)
	}
	branch := s.sh("branch", "--show-current")
	pushed := s.sh("rev-parse", "HEAD")

	// Meanwhile, main moves on GitHub (another pull request merged).
	other := filepath.Join(t.TempDir(), "other")
	s.sh("clone", "--quiet", bare, other)
	s.sh("-C", other, "config", "user.name", "teammate")
	s.sh("-C", other, "config", "user.email", "mate@example.invalid")
	s.sh("-C", other, "commit", "--quiet", "--allow-empty", "-m", "📝 docs: other work")
	s.sh("-C", other, "push", "--quiet", "origin", "main")

	code, _, stderr := s.ship("publish")
	if code != cli.ExitOK || !strings.Contains(stderr, "Updated on main (rebase)") {
		t.Fatalf("ship publish: exit %d\n%s", code, stderr)
	}
	remoteMain := s.sh("--git-dir", bare, "rev-parse", "main")
	remoteBranch := s.sh("--git-dir", bare, "rev-parse", branch)
	if remoteBranch == pushed || s.sh("--git-dir", bare, "rev-parse", branch+"~1") != remoteMain {
		t.Fatal("the remote branch must be the rebased one, on top of the new main")
	}
	if code, stdout, _ := s.ship("log"); code != cli.ExitOK || !strings.Contains(stdout, "rebased") {
		t.Fatalf("ship log:\n%s", stdout)
	}
}
