// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/surface/cli"
)

// TestRemoteNotOnGitHub: with a remote that is no GitHub repository (a bare
// repository on disk), ship, ship status and ship publish never advise ship
// publish, which cannot work there: they say how to point it to GitHub.
func TestRemoteNotOnGitHub(t *testing.T) {
	s := newScenario(t, nil)
	bare := filepath.Join(t.TempDir(), "shop.git")
	s.sh("init", "--bare", "--quiet", bare)
	s.sh("remote", "add", "origin", bare)
	s.write("vat.txt", "20\n")
	s.sh("add", "--all")
	s.sh("commit", "--quiet", "-m", "🎉 chore: init")

	s.write("vat.txt", "21\n")
	code, _, stderr := s.ship("--yes")
	if code != cli.ExitOK || strings.Contains(stderr, "ship publish") || !strings.Contains(stderr, "git remote set-url origin <url>") {
		t.Errorf("ship --yes: exit %d\n%s", code, stderr)
	}
	code, stdout, _ := s.ship("status", "--offline")
	if code != cli.ExitOK || strings.Contains(stdout, "ship publish") || !strings.Contains(stdout, "git remote set-url origin <url>") {
		t.Errorf("status: exit %d\n%s", code, stdout)
	}
	code, _, stderr = s.ship("publish")
	if code == cli.ExitOK || !strings.Contains(stderr, "SHIP-E0602") || !strings.Contains(stderr, "git remote set-url origin <url>") {
		t.Errorf("publish: exit %d\n%s", code, stderr)
	}
}
