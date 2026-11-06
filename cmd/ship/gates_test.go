// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/adapters/ghauth"
	"github.com/GregoireF/ship-it/internal/adapters/github"
	"github.com/GregoireF/ship-it/internal/adapters/projectexec"
	"github.com/GregoireF/ship-it/internal/surface/cli"
)

// TestRedGateBlocksThePush runs real npm scripts: a red lint stops the push
// with exit code 3 and keeps the commit local; once fixed, ship publish goes.
func TestRedGateBlocksThePush(t *testing.T) {
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm is not installed")
	}
	api := &fakeGitHub{}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	credentials := ghauth.New(nil)
	credentials.Getenv = func(name string) string { return map[string]string{"SHIP_TOKEN": "t0ken"}[name] }
	forge := github.New(credentials, "ship-it/test")
	forge.BaseURL = server.URL

	s := newScenario(t, forge)
	s.deps.Runner = projectexec.New(s.git)
	bare := filepath.Join(t.TempDir(), "shop.git")
	s.sh("init", "--bare", "--quiet", bare)
	s.sh("remote", "add", "origin", "https://github.com/acme/shop.git")
	s.sh("config", "url."+filepath.ToSlash(bare)+".insteadOf", "https://github.com/acme/shop.git")
	s.write("package.json", `{"name": "shop", "private": true, "scripts": {"lint": "node lint.js"}}`)
	s.write("lint.js", "console.error('src/vat.js:1 no-var'); process.exit(1)\n")
	s.write(".gitignore", "node_modules\n")
	s.sh("add", "--all")
	s.sh("commit", "--quiet", "-m", "🎉 chore: init")
	s.sh("push", "--quiet", "origin", "main")
	s.sh("fetch", "--quiet", bare, "main:refs/remotes/origin/main")

	s.write("vat.js", "var VAT = 20\n")
	code, _, stderr := s.ship("--yes")
	if code != cli.ExitGate || !strings.Contains(stderr, "✗ lint") || !strings.Contains(stderr, "│ src/vat.js:1 no-var") ||
		!strings.Contains(stderr, "SHIP-E0902") || !strings.Contains(stderr, "Committed locally") {
		t.Fatalf("red lint: exit %d\n%s", code, stderr)
	}
	branch := s.sh("branch", "--show-current")
	if out, _ := s.git.Run(t.Context(), "--git-dir", bare, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch); len(out) != 0 {
		t.Fatal("nothing may be pushed after a red gate")
	}
	if len(api.created) != 0 {
		t.Fatal("no pull request after a red gate")
	}

	s.write("lint.js", "process.exit(0)\n")
	s.sh("commit", "--quiet", "-am", "🐛 fix(lint): accept var")
	if code, _, stderr := s.ship("publish"); code != cli.ExitOK || !strings.Contains(stderr, "✓ lint") || len(api.created) != 1 {
		t.Fatalf("green lint: exit %d\n%s", code, stderr)
	}
}

// TestGatesSeeNewFiles: ship check gates and ship.gates.beforeCommit check
// the files the commit would take, untracked ones included, and say that
// nothing was committed.
func TestGatesSeeNewFiles(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not installed")
	}
	s := newScenario(t, nil)
	s.deps.Runner = projectexec.New(s.git)
	s.write("go.mod", "module example.com/shop\n\ngo 1.21\n")
	s.write("vat.go", "package shop\n\n// VAT is the rate.\nconst VAT = 20\n")
	s.write(".gitignore", "ignored.go\n")
	s.sh("add", "--all")
	s.sh("commit", "--quiet", "-m", "🎉 chore: init")

	s.write("rate.go", "package shop\n\n// Rate is a rate.\nconst Rate    =   1\n")
	s.write("tax.go", "package shop\n\n// Tax is a tax.\nconst Tax    =   2\n")
	code, _, stderr := s.ship("check", "gates")
	if code != cli.ExitGate || !strings.Contains(stderr, "rate.go") || !strings.Contains(stderr, "tax.go") ||
		!strings.Contains(stderr, "nothing was committed") || !strings.Contains(stderr, "run ship check gates again") ||
		strings.Contains(stderr, "nothing was pushed") {
		t.Fatalf("check gates: exit %d\n%s", code, stderr)
	}

	code, _, stderr = s.ship("--set", "ship.gates.beforeCommit=true", "--yes", "--local")
	if code != cli.ExitGate || !strings.Contains(stderr, "rate.go") || !strings.Contains(stderr, "run ship again") ||
		strings.Contains(stderr, "nothing was pushed") {
		t.Fatalf("before commit: exit %d\n%s", code, stderr)
	}
	if count := s.sh("rev-list", "--count", "--all"); count != "1" {
		t.Fatalf("a red gate before the commit must not commit: %s commits", count)
	}

	// An ignored file is never committed: the checks leave it alone.
	s.write("rate.go", "package shop\n\n// Rate is a rate.\nconst Rate = 1\n")
	s.write("tax.go", "package shop\n\n// Tax is a tax.\nconst Tax = 2\n")
	s.write("ignored.go", "package shop\n\nconst ignored    =   3\n")
	if code, _, stderr := s.ship("check", "gates"); code != cli.ExitOK {
		t.Fatalf("formatted: exit %d\n%s", code, stderr)
	}
}

func TestGoGatesRunForReal(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not installed")
	}
	api := &fakeGitHub{}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	credentials := ghauth.New(nil)
	credentials.Getenv = func(name string) string { return map[string]string{"SHIP_TOKEN": "t0ken"}[name] }
	forge := github.New(credentials, "ship-it/test")
	forge.BaseURL = server.URL

	s := newScenario(t, forge)
	s.deps.Runner = projectexec.New(s.git)
	bare := filepath.Join(t.TempDir(), "shop.git")
	s.sh("init", "--bare", "--quiet", bare)
	s.sh("remote", "add", "origin", "https://github.com/acme/shop.git")
	s.sh("config", "url."+filepath.ToSlash(bare)+".insteadOf", "https://github.com/acme/shop.git")
	s.write("go.mod", "module example.com/shop\n\ngo 1.21\n")
	s.write("vat.go", "package shop\n\n// VAT is the rate.\nconst VAT = 20\n")
	s.sh("add", "--all")
	s.sh("commit", "--quiet", "-m", "🎉 chore: init")
	s.sh("push", "--quiet", "origin", "main")
	s.sh("fetch", "--quiet", bare, "main:refs/remotes/origin/main")

	s.write("vat.go", "package shop\n\n// VAT is the rate.\nconst VAT    =   21\n")
	code, _, stderr := s.ship("--yes")
	if code != cli.ExitGate || !strings.Contains(stderr, "gofmt") || !strings.Contains(stderr, "vat.go") || len(api.created) != 0 {
		t.Fatalf("unformatted Go: exit %d\n%s", code, stderr)
	}

	s.write("vat.go", "package shop\n\n// VAT is the rate.\nconst VAT = 21\n")
	s.sh("commit", "--quiet", "-am", "🎨 style: gofmt")
	if code, _, stderr := s.ship("publish"); code != cli.ExitOK || !strings.Contains(stderr, "go test") || len(api.created) != 1 {
		t.Fatalf("formatted Go: exit %d\n%s", code, stderr)
	}
}
