// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package gitexec

import (
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/ports"
)

func nul(records ...string) []byte { return []byte(strings.Join(records, "\x00") + "\x00") }

const oid = "0123456789abcdef0123456789abcdef01234567"

func TestParseStatus(t *testing.T) {
	out := nul(
		"# branch.oid "+oid,
		"# branch.head feat/42-login",
		"# branch.upstream origin/feat/42-login",
		"# branch.ab +2 -1",
		"# branch.future-header ignored",
		"1 .M N... 100644 100644 100644 "+oid+" "+oid+" src/app.go",
		"1 A. N... 000000 100644 100644 "+oid+" "+oid+" docs/with space.md",
		"2 R. N... 100644 100644 100644 "+oid+" "+oid+" R100 new/name.go", "old/name.go",
		"u UU N... 100644 100644 100644 100644 "+oid+" "+oid+" "+oid+" conflict.go",
		"1 .M SC.. 160000 160000 160000 "+oid+" "+oid+" vendor/sub",
		"? untracked.txt",
		"! ignored.log",
	)
	s, err := ParseStatus(out)
	if err != nil {
		t.Fatal(err)
	}
	b := s.Branch
	if b.Name != "feat/42-login" || b.Commit != oid || b.Upstream != "origin/feat/42-login" || b.Ahead != 2 || b.Behind != 1 {
		t.Errorf("branch = %+v", b)
	}
	want := []ports.FileChange{
		{Path: "src/app.go", Staged: ports.Unchanged, Unstaged: ports.Modified, State: ports.Tracked},
		{Path: "docs/with space.md", Staged: ports.Added, Unstaged: ports.Unchanged, State: ports.Tracked},
		{Path: "new/name.go", OriginalPath: "old/name.go", Staged: ports.Renamed, Unstaged: ports.Unchanged, State: ports.Tracked},
		{Path: "conflict.go", Staged: ports.Unmerged, Unstaged: ports.Unmerged, State: ports.Conflicted},
		{Path: "vendor/sub", Staged: ports.Unchanged, Unstaged: ports.Modified, State: ports.Tracked, Submodule: true},
		{Path: "untracked.txt", Staged: ports.Unchanged, Unstaged: ports.Unchanged, State: ports.Untracked},
		{Path: "ignored.log", Staged: ports.Unchanged, Unstaged: ports.Unchanged, State: ports.Ignored},
	}
	if len(s.Files) != len(want) {
		t.Fatalf("got %d files, want %d: %+v", len(s.Files), len(want), s.Files)
	}
	for i := range want {
		if s.Files[i] != want[i] {
			t.Errorf("file %d = %+v, want %+v", i, s.Files[i], want[i])
		}
	}
}

func TestParseStatusInitialAndDetached(t *testing.T) {
	s, err := ParseStatus(nul("# branch.oid (initial)", "# branch.head (detached)"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Branch.Commit != "" || !s.Branch.Detached || s.Branch.Name != "" {
		t.Errorf("branch = %+v", s.Branch)
	}
	if !s.Clean() {
		t.Error("empty status must be clean")
	}
}

func TestParseStatusRejectsGarbage(t *testing.T) {
	for _, out := range [][]byte{nul("1 M"), nul("z what"), nul("2 R. N... 100644 100644 100644 " + oid + " " + oid + " R100 only")[:0:0]} {
		if len(out) == 0 {
			continue
		}
		if _, err := ParseStatus(out); err == nil {
			t.Errorf("ParseStatus(%q) accepted garbage", out)
		}
	}
	// A rename at the very end without its original path.
	if _, err := ParseStatus([]byte("2 R. N... 100644 100644 100644 " + oid + " " + oid + " R100 a")); err == nil {
		t.Error("rename without original path accepted")
	}
}

func FuzzParseStatus(f *testing.F) {
	f.Add(nul("# branch.head main", "1 .M N... 100644 100644 100644 "+oid+" "+oid+" a b"))
	f.Add(nul("? x", "! y"))
	f.Fuzz(func(t *testing.T, out []byte) {
		_, _ = ParseStatus(out) // must never panic
	})
}
