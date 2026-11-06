// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package doclinks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRepositoryLinksResolve fails on any broken relative link of the
// repository's Markdown, so that moving or renaming a page updates its links.
func TestRepositoryLinksResolve(t *testing.T) {
	broken, err := Check(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range broken {
		t.Error(b)
	}
}

func TestSlug(t *testing.T) {
	for heading, want := range map[string]string{
		"Organisation policy":                  "organisation-policy",
		"6. Planning: ROADMAP.md → GitHub":     "6-planning-roadmapmd--github",
		"`ship undo` and [the log](log.md)":    "ship-undo-and-the-log",
		"Pull requests that read like a human": "pull-requests-that-read-like-a-human",
		"⛵ ship-it":                            "-ship-it",
	} {
		if got := Slug(heading); got != want {
			t.Errorf("Slug(%q) = %q, want %q", heading, got, want)
		}
	}
}

func TestCheckFindsBrokenLinks(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.md", "# Title\n\n## Same\n\n## Same\n\n[ok](b.md#part) [ok](#same-1) [web](https://example.com/x)\n"+
		"[gone](missing.md) [bad](b.md#nothing)\n\n```\n[in code](nowhere.md)\n```\n`[inline](nowhere.md)`\n")
	write("b.md", "## Part\n")
	broken, err := Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(broken, "\n")
	if len(broken) != 2 || !strings.Contains(got, "a.md:8: missing.md (no such file)") || !strings.Contains(got, "b.md#nothing (no such heading)") {
		t.Errorf("broken links:\n%s", got)
	}
}
