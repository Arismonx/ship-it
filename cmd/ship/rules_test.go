// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/adapters/ghauth"
	"github.com/GregoireF/ship-it/internal/adapters/github"
	"github.com/GregoireF/ship-it/internal/adapters/projectexec"
	"github.com/GregoireF/ship-it/internal/surface/cli"
)

// publishingScenario is a repository whose origin is github.com/acme/shop,
// pushed for real to a local bare repository, with the API on api.
func publishingScenario(t *testing.T, api *fakeGitHub) (*scenario, string) {
	t.Helper()
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
	return s, bare
}

// TestRulesetRenamesThenPushes: a ruleset requires "feature/"; ship-it's own
// branch is renamed before the push, the pull request comes from the new
// name, and the old name never reaches GitHub.
func TestRulesetRenamesThenPushes(t *testing.T) {
	api := &fakeGitHub{rules: `[{"type":"branch_name_pattern","ruleset_id":1,
		"parameters":{"name":"Team prefix","negate":false,"operator":"starts_with","pattern":"feature/"}}]`}
	s, bare := publishingScenario(t, api)
	s.write("README.md", "# shop\n")
	s.sh("add", "--all")
	s.sh("commit", "--quiet", "-m", "🎉 chore: init")
	s.sh("push", "--quiet", "origin", "main")
	s.sh("fetch", "--quiet", bare, "main:refs/remotes/origin/main")

	s.write("vat.go", "package shop\n\nconst VAT = 20\n")
	code, _, stderr := s.ship("--yes")
	if code != cli.ExitOK || !strings.Contains(stderr, "SHIP-W0616") || !strings.Contains(stderr, "Team prefix") {
		t.Fatalf("ship --yes: exit %d\n%s", code, stderr)
	}
	branch := s.sh("branch", "--show-current")
	if !strings.HasPrefix(branch, "feature/") {
		t.Fatalf("branch = %s", branch)
	}
	if pushed := s.sh("--git-dir", bare, "rev-parse", "refs/heads/"+branch); pushed != s.sh("rev-parse", "HEAD") {
		t.Fatalf("%s not pushed", branch)
	}
	if refs := s.sh("--git-dir", bare, "for-each-ref", "--format=%(refname)", "refs/heads/feat/"); refs != "" {
		t.Fatalf("the old name was pushed: %s", refs)
	}
	if len(api.created) != 1 || api.created[0]["head"] != branch {
		t.Fatalf("pull requests = %v", api.created)
	}
	if code, stdout, _ := s.ship("log"); code != cli.ExitOK || !strings.Contains(stdout, "renamed feat/") {
		t.Fatalf("ship log:\n%s", stdout)
	}
}

// TestProjectCommitlintDecidesTheTitle installs @commitlint/cli for real in
// a temporary project, offline only (npm's cache): its conventional parser
// refuses a leading gitmoji, so ship-it commits without one, and says why.
func TestProjectCommitlintDecidesTheTitle(t *testing.T) {
	npm, err := exec.LookPath("npm")
	if err != nil {
		t.Skip("npm is not installed")
	}
	s := newScenario(t, nil)
	s.deps.Runner = projectexec.New(s.git)
	s.write("package.json", `{"name": "shop", "private": true}`)
	s.write("commitlint.config.cjs", "module.exports = { extends: ['@commitlint/config-conventional'] };\n")
	s.write(".gitignore", "node_modules\n")
	install := exec.CommandContext(context.Background(), npm, "install", "--offline", "--no-audit", "--no-fund",
		"@commitlint/cli", "@commitlint/config-conventional")
	install.Dir = s.dir
	if out, err := install.CombinedOutput(); err != nil {
		t.Skipf("@commitlint/cli is not in npm's cache and this test never downloads: %v\n%s", err, lastLines(out))
	}
	s.sh("add", "--all")
	s.sh("commit", "--quiet", "-m", "chore: init")

	s.write("vat.go", "package shop\n\nconst VAT = 20\n")
	code, stdout, stderr := s.ship("plan")
	if code != cli.ExitOK || !strings.Contains(stdout+stderr, "commitlint") {
		t.Fatalf("ship plan: exit %d\n%s\n%s", code, stdout, stderr)
	}
	if code, _, stderr := s.ship("--yes", "--local"); code != cli.ExitOK {
		t.Fatalf("ship --yes --local: exit %d\n%s", code, stderr)
	}
	if subject := s.sh("log", "-1", "--format=%s"); strings.HasPrefix(subject, "✨") || !strings.HasPrefix(subject, "feat") {
		t.Fatalf("subject = %q", subject)
	}
}

func lastLines(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return strings.Join(lines[max(0, len(lines)-3):], "\n")
}

var _ = os.Getenv
