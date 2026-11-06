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

func lines(n int, text string) string { return strings.Repeat(text+"\n", n) }

// TestLargeWorkShipsInParts: 450 changed lines over two topics; the plan
// warns and suggests ship --only; shipping one part leaves the other alone,
// and undoing it works although the other part is still uncommitted.
func TestLargeWorkShipsInParts(t *testing.T) {
	s := newScenario(t, nil)
	s.write("README.md", "# shop\n")
	s.sh("add", "--all")
	s.sh("commit", "--quiet", "-m", "🎉 chore: init")
	for _, dir := range []string{"api", "ui"} {
		if err := os.MkdirAll(filepath.Join(s.dir, "src", dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	s.write(filepath.Join("src", "api", "vat.go"), "package api\n"+lines(299, "// rule"))
	s.write(filepath.Join("src", "ui", "cart.ts"), lines(150, "// view"))

	code, stdout, _ := s.ship("plan")
	if code != cli.ExitOK || !strings.Contains(stdout, "450 changed lines (more than 400)") ||
		!strings.Contains(stdout, "ship --only src/api/") || !strings.Contains(stdout, "ship --only src/ui/") {
		t.Fatalf("ship plan:\n%s", stdout)
	}

	if code, _, stderr := s.ship("--yes", "--local", "--only", "src/api/"); code != cli.ExitOK {
		t.Fatalf("ship --only: exit %d\n%s", code, stderr)
	}
	committed := s.sh("show", "--name-only", "--format=", "HEAD")
	if committed != "src/api/vat.go" || !strings.Contains(s.sh("status", "--porcelain"), "src/ui/") {
		t.Fatalf("committed %q; the ui part must stay uncommitted", committed)
	}

	if code, _, stderr := s.ship("undo"); code != cli.ExitOK {
		t.Fatalf("undo a partial delivery with other work around: exit %d\n%s", code, stderr)
	}
	status := s.sh("status", "--porcelain", "--untracked-files=all")
	if s.sh("branch", "--show-current") != "main" || !strings.Contains(status, "src/api/vat.go") || !strings.Contains(status, "src/ui/cart.ts") {
		t.Fatalf("after undo:\n%s", status)
	}
}
