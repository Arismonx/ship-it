// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/adapters/ghauth"
	"github.com/GregoireF/ship-it/internal/adapters/github"
	"github.com/GregoireF/ship-it/internal/surface/cli"
)

// TestFixAndUndo repairs a repository for real (identity, a tracked .env
// file, a mirror remote), then ship undo puts every byte back.
func TestFixAndUndo(t *testing.T) {
	// Never touch the developer's own ~/.gitconfig.
	globalConfig := filepath.Join(t.TempDir(), "gitconfig")
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
	// This test needs a repository without identity: drop the one TestMain
	// gives through the environment.
	t.Setenv("GIT_CONFIG_COUNT", "0")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user":
			fmt.Fprint(w, `{"login":"ada","id":583231,"name":"Ada Lovelace"}`)
		case "/repos/acme/shop/pulls":
			fmt.Fprint(w, `[]`)
		default:
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	credentials := ghauth.New(nil)
	credentials.Getenv = func(name string) string { return map[string]string{"SHIP_TOKEN": "t0ken"}[name] }
	forge := github.New(credentials, "ship-it/test")
	forge.BaseURL = server.URL

	s := newScenario(t, forge)
	s.sh("config", "--unset", "user.name")
	s.sh("config", "--unset", "user.email")
	s.sh("remote", "add", "origin", "https://github.com/acme/shop.git")
	s.sh("remote", "add", "backup", "https://example.invalid/backup.git")
	s.sh("config", "remote.backup.mirror", "true")
	s.write(".gitignore", "dist/\n")
	s.write(".env.local", "API=dev\n")
	s.write("app.ts", "export {}\n")
	s.sh("add", "--all")
	s.sh("-c", "user.name=setup", "-c", "user.email=setup@example.invalid", "commit", "--quiet", "-m", "🎉 chore: init")
	gitignoreBefore := s.sh("show", "HEAD:.gitignore")

	code, stdout, _ := s.ship("fix")
	if code != cli.ExitNeedsHuman || !strings.Contains(stdout, "Ada Lovelace <583231+ada@users.noreply.github.com>") ||
		!strings.Contains(stdout, "Stop tracking 1 file ") || !strings.Contains(stdout, "Stop mirroring remote backup") {
		t.Fatalf("ship fix (list): exit %d\n%s", code, stdout)
	}
	if code, _, stderr := s.ship("fix", "--yes"); code != cli.ExitOK {
		t.Fatalf("ship fix --yes: exit %d\n%s", code, stderr)
	}
	global, _ := os.ReadFile(globalConfig)
	if !strings.Contains(string(global), "583231+ada@users.noreply.github.com") {
		t.Fatalf("global identity not written:\n%s", global)
	}
	mirror := func() string {
		value, _, _ := s.git.ConfigValue(t.Context(), "remote.backup.mirror")
		return value
	}
	if strings.Contains(s.sh("ls-files"), ".env.local") || mirror() != "" {
		t.Fatal(".env.local must be untracked and the mirror removed")
	}
	if _, err := os.Stat(filepath.Join(s.dir, ".env.local")); err != nil {
		t.Fatal(".env.local must stay on disk")
	}

	if code, _, stderr := s.ship("undo"); code != cli.ExitOK {
		t.Fatalf("ship undo: exit %d\n%s", code, stderr)
	}
	global, _ = os.ReadFile(globalConfig)
	if strings.Contains(string(global), "noreply") || !strings.Contains(s.sh("ls-files"), ".env.local") || mirror() != "true" {
		t.Fatalf("undo incomplete; global config:\n%s", global)
	}
	if got, _ := os.ReadFile(filepath.Join(s.dir, ".gitignore")); string(got) != gitignoreBefore+"\n" {
		t.Fatalf(".gitignore = %q, want %q", got, gitignoreBefore+"\n")
	}
}
