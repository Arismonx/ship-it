// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/surface/cli"
)

// TestDeliverThenUndo runs `ship --yes --local` then `ship undo`, and checks
// the repository is back where the user left it.
func TestDeliverThenUndo(t *testing.T) {
	s := newScenario(t, nil)
	s.sh("remote", "add", "origin", "https://example.invalid/repo.git")
	s.write("keep.go", "package keep\n")
	s.write("gone.go", "package gone\n")
	s.sh("add", "--all")
	s.sh("commit", "--quiet", "-m", "🎉 chore: init")
	h0 := s.sh("rev-parse", "HEAD")

	// Work in progress: one edit, one new file, one deletion.
	s.write("keep.go", "package keep\n\nfunc Keep() {}\n")
	s.write("new.go", "package keep\n")
	if err := os.Remove(filepath.Join(s.dir, "gone.go")); err != nil {
		t.Fatal(err)
	}
	statusBefore := s.sh("status", "--porcelain")

	if code, _, stderr := s.ship("--yes", "--local"); code != cli.ExitOK {
		t.Fatalf("ship --yes --local: exit %d\n%s", code, stderr)
	}
	branch := s.sh("branch", "--show-current")
	if branch == "main" || s.sh("status", "--porcelain") != "" {
		t.Fatalf("delivery did not commit on a new branch (on %s)", branch)
	}

	if code, _, stderr := s.ship("undo"); code != cli.ExitOK {
		t.Fatalf("ship undo: exit %d\n%s", code, stderr)
	}
	if got := s.sh("branch", "--show-current"); got != "main" {
		t.Errorf("branch after undo = %s, want main", got)
	}
	if got := s.sh("rev-parse", "HEAD"); got != h0 {
		t.Errorf("HEAD after undo = %s, want %s", got, h0)
	}
	if got := s.sh("status", "--porcelain"); got != statusBefore {
		t.Errorf("status after undo =\n%s\nwant\n%s", got, statusBefore)
	}
	if s.sh("branch", "--list", branch) != "" {
		t.Errorf("branch %s was not removed", branch)
	}
	if _, err := os.Stat(filepath.Join(s.dir, "gone.go")); !os.IsNotExist(err) {
		t.Error("gone.go came back; a deleted file must stay deleted")
	}

	if code, _, stderr := s.ship("undo"); code != cli.ExitError || !strings.Contains(stderr, "SHIP-E0403") {
		t.Fatalf("second undo: exit %d\n%s", code, stderr)
	}

	// ship log tells the story, newest first.
	code, stdout, stderr := s.ship("log")
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if code != cli.ExitOK || len(lines) != 2 || !strings.Contains(lines[0], "undid") ||
		!strings.Contains(lines[1], "delivered ") || !strings.Contains(lines[1], "(undone)") {
		t.Fatalf("ship log: exit %d\n%s", code, stdout)
	}
	// Nothing is left to undo: no entry carries the mark, so no legend.
	if strings.Contains(stdout, "↶") || strings.Contains(stderr, "↶") {
		t.Errorf("ship log shows the undo mark or its legend with nothing to undo:\n%s%s", stdout, stderr)
	}
}

// TestLogLegendFollowsTheMark: the legend of ↶ comes with an entry that
// carries it.
func TestLogLegendFollowsTheMark(t *testing.T) {
	s := newScenario(t, nil)
	s.write("a.go", "package a\n")
	s.sh("add", "--all")
	s.sh("commit", "--quiet", "-m", "🎉 chore: init")
	s.write("a.go", "package a\n\nfunc A() {}\n")
	if code, _, stderr := s.ship("--yes", "--local"); code != cli.ExitOK {
		t.Fatalf("ship --yes --local: exit %d\n%s", code, stderr)
	}
	code, stdout, stderr := s.ship("log")
	if code != cli.ExitOK || !strings.HasPrefix(stdout, "↶ ") || !strings.Contains(stderr, "what ship undo reverts next") {
		t.Fatalf("ship log: exit %d\n%s%s", code, stdout, stderr)
	}
}
