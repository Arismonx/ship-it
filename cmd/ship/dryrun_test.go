// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"encoding/json"
	"io/fs"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/adapters/ghauth"
	"github.com/GregoireF/ship-it/internal/adapters/github"
	"github.com/GregoireF/ship-it/internal/surface/cli"
)

// state is everything a dry run must leave as it was: HEAD, refs, the
// working tree, stashes, the remote, and ship-it's own checkpoints and log.
func (s *scenario) state(bare string) string {
	s.t.Helper()
	var b strings.Builder
	for _, args := range [][]string{
		{"rev-parse", "HEAD"}, {"branch", "--show-current"}, {"status", "--porcelain", "--untracked-files=all"},
		{"for-each-ref"}, {"stash", "list"}, {"--git-dir", bare, "for-each-ref"},
	} {
		b.WriteString(s.sh(args...) + "\n")
	}
	_ = filepath.WalkDir(filepath.Join(s.dir, ".git", "ship"), func(path string, d fs.DirEntry, err error) error {
		if err == nil {
			info, _ := d.Info()
			b.WriteString(path)
			if info != nil && !d.IsDir() {
				b.WriteString(" " + info.ModTime().String())
			}
			b.WriteString("\n")
		}
		return nil
	})
	return b.String()
}

// TestDryRunChangesNothing runs `ship --dry-run`, then `ship undo --dry-run`
// after a real delivery, on a real repository with a remote: every check
// runs, the plan is printed, and nothing at all changes.
func TestDryRunChangesNothing(t *testing.T) {
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

	before := s.state(bare)
	code, stdout, stderr := s.ship("--dry-run")
	if code != cli.ExitOK {
		t.Fatalf("ship --dry-run: exit %d\n%s", code, stderr)
	}
	for _, want := range []string{"Dry run: nothing was changed", "take a checkpoint", "create the branch feat/", "commit every change as \"✨ feat",
		"push feat/", "open a pull request \"✨ feat", "into main", "add the labels kind/feature, size/XS to the new pull request"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the plan lacks %q:\n%s", want, stdout)
		}
	}
	if after := s.state(bare); after != before {
		t.Fatalf("ship --dry-run changed the repository:\nbefore\n%s\nafter\n%s", before, after)
	}
	if len(api.created) != 0 || len(api.labeled) != 0 {
		t.Fatalf("ship --dry-run wrote to GitHub: %v %v", api.created, api.labeled)
	}

	code, stdout, stderr = s.ship("--dry-run", "--json")
	var shipped struct {
		Delivery struct {
			DryRun  bool   `json:"dry_run"`
			Commit  string `json:"commit"`
			Branch  string `json:"branch"`
			Planned []struct {
				Key string `json:"key"`
			} `json:"planned"`
		} `json:"delivery"`
		Publication struct {
			Planned []struct {
				Key    string `json:"key"`
				Remote bool   `json:"remote"`
			} `json:"planned"`
		} `json:"publication"`
	}
	if err := json.Unmarshal([]byte(stdout), &shipped); code != cli.ExitOK || err != nil {
		t.Fatalf("ship --dry-run --json: exit %d, %v\n%s\n%s", code, err, stdout, stderr)
	}
	if !shipped.Delivery.DryRun || shipped.Delivery.Commit != "" || !strings.HasPrefix(shipped.Delivery.Branch, "feat/") ||
		len(shipped.Delivery.Planned) == 0 || len(shipped.Publication.Planned) == 0 {
		t.Errorf("JSON = %+v", shipped)
	}
	if after := s.state(bare); after != before {
		t.Fatal("ship --dry-run --json changed the repository")
	}

	// After a real delivery, undo --dry-run says what it would revert.
	if code, _, stderr := s.ship("--yes", "--local"); code != cli.ExitOK {
		t.Fatalf("ship --yes --local: exit %d\n%s", code, stderr)
	}
	before = s.state(bare)
	code, stdout, stderr = s.ship("undo", "--dry-run")
	if code != cli.ExitOK || !strings.Contains(stdout, "put HEAD back on main") || !strings.Contains(stdout, "delete the local branch feat/") {
		t.Fatalf("ship undo --dry-run: exit %d\n%s\n%s", code, stdout, stderr)
	}
	if after := s.state(bare); after != before {
		t.Fatalf("ship undo --dry-run changed the repository:\nbefore\n%s\nafter\n%s", before, after)
	}
	if code, _, stderr := s.ship("undo"); code != cli.ExitOK {
		t.Fatalf("the real undo after a dry run: exit %d\n%s", code, stderr)
	}
}
