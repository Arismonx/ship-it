// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/adapters/footprint"
	"github.com/GregoireF/ship-it/internal/platform"
	"github.com/GregoireF/ship-it/internal/surface/cli"
)

// TestUninstall: #55. ship uninstall lists, asks a human, then removes
// .git/ship and ship-it's agent entries, keeping the other servers; --user
// removes the user's ship-it directories.
func TestUninstall(t *testing.T) {
	s := newScenario(t, nil)
	home := t.TempDir()
	env := platform.Environment{GOOS: "linux", Home: home, Getenv: func(string) string { return "" }, PolicyPath: filepath.Join(home, "policy.json")}
	s.deps.Footprint = footprint.New(s.git, env)
	s.write("app.go", "package app\n")
	s.sh("add", "app.go")
	s.sh("commit", "--quiet", "-m", "🎉 chore: init")
	if code, _, stderr := s.ship("checkpoint"); code != cli.ExitOK {
		t.Fatalf("checkpoint: %d %s", code, stderr)
	}
	s.write(".mcp.json", `{"mcpServers": {"github": {"type": "http", "url": "https://api.githubcopilot.com/mcp/"}}}`)
	for _, client := range []string{"claude", "cursor"} {
		if code, _, stderr := s.ship("mcp", "install", "--client", client, "--yes"); code != cli.ExitOK {
			t.Fatalf("mcp install %s: %d %s", client, code, stderr)
		}
	}
	shipDir := filepath.Join(s.dir, ".git", "ship")

	// Nobody to say yes: nothing goes.
	code, _, stderr := s.ship("uninstall")
	if code != cli.ExitNeedsHuman || !strings.Contains(stderr, ".git") || !strings.Contains(stderr, "1 checkpoint goes") {
		t.Fatalf("without --yes: %d\n%s", code, stderr)
	}
	if code, _, _ := s.ship("uninstall", "--dry-run"); code != cli.ExitOK || !exists(shipDir) {
		t.Fatal("--dry-run removed something")
	}

	if code, _, stderr = s.ship("uninstall", "--yes"); code != cli.ExitOK {
		t.Fatalf("uninstall: %d\n%s", code, stderr)
	}
	// The skills ship mcp install wrote go too, with their empty folders.
	if exists(filepath.Join(s.dir, ".claude")) || exists(filepath.Join(s.dir, ".agents")) {
		t.Fatal("the Agent Skill and its folders should be gone")
	}
	shared, err := os.ReadFile(filepath.Join(s.dir, ".mcp.json"))
	if exists(shipDir) || exists(filepath.Join(s.dir, ".cursor", "mcp.json")) || err != nil ||
		strings.Contains(string(shared), "ship-it") || !strings.Contains(string(shared), "githubcopilot") {
		t.Fatalf("after uninstall: .git/ship %v, .mcp.json %q (%v)", exists(shipDir), shared, err)
	}
	if code, _, stderr = s.ship("uninstall", "--yes"); code != cli.ExitOK || !strings.Contains(stderr, "Nothing") {
		t.Fatalf("again: %d\n%s", code, stderr)
	}

	config := filepath.Join(home, ".config", "ship-it")
	if err := os.MkdirAll(config, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "install-salt"), []byte("salt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr = s.ship("uninstall", "--user", "--yes"); code != cli.ExitOK || exists(config) {
		t.Fatalf("--user: %d\n%s", code, stderr)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
