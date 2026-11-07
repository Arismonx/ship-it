// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/adapters/fsconfig"
	"github.com/GregoireF/ship-it/internal/platform"
	"github.com/GregoireF/ship-it/internal/surface/cli"
)

// TestConfigSetUnsetThenUndo edits the repository and user files through the
// CLI, then undoes every edit: the files come back byte for byte. The user
// configuration directory and git's global file are temporary.
func TestConfigSetUnsetThenUndo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	s := newScenario(t, nil)
	configHome := filepath.Join(home, "config")
	env := platform.Environment{
		GOOS: runtime.GOOS, Home: home, PolicyPath: filepath.Join(home, "no-policy.json"),
		Getenv: func(name string) string {
			if name == "AppData" || name == "XDG_CONFIG_HOME" {
				return configHome
			}
			return ""
		},
	}
	s.deps.Config = &fsconfig.Files{Git: s.git, Env: env}
	s.write("README.md", "# test\n")
	s.sh("add", "--all")
	s.sh("commit", "--quiet", "-m", "🎉 chore: init")

	repoFile := filepath.Join(s.dir, ".github", "ship.json")
	original := "{\r\n\t// Reviewed by the team.\r\n\t\"version\": 1,\r\n\t\"ship\": {\r\n\t\t\"publish\": {\"mode\": \"never\"} /* local only */\r\n\t}\r\n}\r\n"
	if err := os.MkdirAll(filepath.Dir(repoFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(repoFile, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	read := func(path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	if code, _, stderr := s.ship("config", "set", "ship.pullRequest.maxLines", "600"); code != cli.ExitOK {
		t.Fatalf("config set: exit %d\n%s", code, stderr)
	}
	want := "{\r\n\t// Reviewed by the team.\r\n\t\"version\": 1,\r\n\t\"ship\": {\r\n\t\t\"publish\": {\"mode\": \"never\"}, /* local only */\r\n\t\t\"pullRequest\": {\r\n\t\t\t\"maxLines\": 600\r\n\t\t}\r\n\t}\r\n}\r\n"
	if got := read(repoFile); got != want {
		t.Fatalf("repository file =\n%q\nwant\n%q", got, want)
	}
	if code, stdout, stderr := s.ship("config", "get", "ship.pullRequest.maxLines"); code != cli.ExitOK || stdout != "600\n" || !strings.Contains(stderr, filepath.Join(".github", "ship.json")) {
		t.Fatalf("config get: exit %d\n%s%s", code, stdout, stderr)
	}
	if code, _, stderr := s.ship("config", "unset", "ship.publish.mode"); code != cli.ExitOK {
		t.Fatalf("config unset: exit %d\n%s", code, stderr)
	}
	if got := read(repoFile); strings.Contains(got, "publish") || !strings.Contains(got, "// Reviewed by the team.") {
		t.Fatalf("after unset:\n%s", got)
	}
	// Where the user file lives depends on the OS (Library/Application Support
	// on macOS): ask the platform package, as ship does.
	userDir, err := env.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	userFile := filepath.Join(userDir, "config.json")
	if code, _, stderr := s.ship("config", "set", "ship.issues.keyword", "Refs", "--user"); code != cli.ExitOK {
		t.Fatalf("config set --user: exit %d\n%s", code, stderr)
	}
	if got := read(userFile); !strings.Contains(got, `"keyword": "Refs"`) || !strings.Contains(got, `"$schema"`) {
		t.Fatalf("user file:\n%s", got)
	}
	code, stdout, _ := s.ship("config", "explain", "ship.issues.keyword")
	if code != cli.ExitOK || !strings.Contains(stdout, "SHIP_ISSUES_KEYWORD") || !strings.Contains(stdout, "config.json") || !strings.Contains(stdout, "Closes, Fixes, Resolves, Refs") {
		t.Fatalf("config explain: exit %d\n%s", code, stdout)
	}
	// The ids of the built-in secret rules, for ship.secrets.disabledRules.
	code, stdout, _ = s.ship("config", "explain", "ship.secrets.disabledRules")
	if code != cli.ExitOK || !strings.Contains(stdout, "private_key, github_token, aws_key") || !strings.Contains(stdout, "sensitive_file") {
		t.Fatalf("config explain ship.secrets.disabledRules: exit %d\n%s", code, stdout)
	}
	if code, _, stderr := s.ship("config", "get", "ship.isues.keyword"); code != cli.ExitError || !strings.Contains(stderr, "SHIP-E0702") || !strings.Contains(stderr, "ship.issues.keyword") {
		t.Fatalf("unknown key: exit %d\n%s", code, stderr)
	}

	// Undo, newest first: the user file goes, then the repository file comes
	// back as it was, byte for byte.
	for i := range 3 {
		if code, _, stderr := s.ship("undo"); code != cli.ExitOK {
			t.Fatalf("undo %d: exit %d\n%s", i, code, stderr)
		}
	}
	if _, err := os.Stat(userFile); !os.IsNotExist(err) {
		t.Errorf("the user file created by set is still there: %v", err)
	}
	if got := read(repoFile); got != original {
		t.Errorf("repository file after undo =\n%q\nwant\n%q", got, original)
	}
	if code, stdout, _ := s.ship("log"); code != cli.ExitOK || strings.Count(stdout, "changed the setting") != 3 {
		t.Errorf("ship log: exit %d\n%s", code, stdout)
	}
}
