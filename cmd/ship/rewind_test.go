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

// TestRewindThenUndo: #39. ship rewind lists the checkpoints with what
// changed in each, writes the chosen one back (a deleted file returns, a file
// created since stays), and ship undo puts every file back as it was.
func TestRewindThenUndo(t *testing.T) {
	s := newScenario(t, nil)
	read := func(name string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(s.dir, name))
		if err != nil {
			return "<missing>"
		}
		return string(data)
	}
	s.write("a.txt", "one\n")
	s.write("c.txt", "kept in the first checkpoint\n")
	s.sh("add", "--all")
	s.sh("commit", "--quiet", "-m", "🎉 chore: init")
	if code, _, stderr := s.ship("checkpoint", "--label", "first"); code != cli.ExitOK {
		t.Fatalf("checkpoint: %d %s", code, stderr)
	}
	s.write("a.txt", "two\n")
	if code, _, stderr := s.ship("checkpoint", "--label", "second"); code != cli.ExitOK {
		t.Fatalf("checkpoint: %d %s", code, stderr)
	}
	// Then the work goes wrong: a.txt rewritten, c.txt deleted, b.txt new.
	s.write("a.txt", "three, broken\n")
	if err := os.Remove(filepath.Join(s.dir, "c.txt")); err != nil {
		t.Fatal(err)
	}
	s.write("b.txt", "new\n")

	// Without a terminal, the list and a refusal to choose for the human.
	code, stdout, _ := s.ship("rewind")
	if code != cli.ExitNeedsHuman || !strings.Contains(stdout, "1. ") || !strings.Contains(stdout, "second") ||
		!strings.Contains(stdout, "a.txt") || !strings.Contains(stdout, "2. ") {
		t.Fatalf("list: %d\n%s", code, stdout)
	}
	if code, _, _ := s.ship("rewind", "2", "--dry-run"); code != cli.ExitOK || read("a.txt") != "three, broken\n" {
		t.Fatal("--dry-run wrote the files")
	}

	code, _, stderr := s.ship("rewind", "2", "--yes")
	if code != cli.ExitOK || read("a.txt") != "one\n" || read("c.txt") != "kept in the first checkpoint\n" || read("b.txt") != "new\n" ||
		!strings.Contains(stderr, "b.txt") {
		t.Fatalf("rewind: %d\n%s\na=%q c=%q b=%q", code, stderr, read("a.txt"), read("c.txt"), read("b.txt"))
	}

	if code, _, stderr = s.ship("undo", "--yes"); code != cli.ExitOK {
		t.Fatalf("undo: %d\n%s", code, stderr)
	}
	if read("a.txt") != "three, broken\n" || read("c.txt") != "<missing>" || read("b.txt") != "new\n" {
		t.Fatalf("after undo: a=%q c=%q b=%q", read("a.txt"), read("c.txt"), read("b.txt"))
	}
}
