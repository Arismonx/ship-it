// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package glob

import (
	"strings"
	"testing"
)

func TestMatch(t *testing.T) {
	for _, tt := range []struct {
		pattern, path string
		want          bool
	}{
		{"*.md", "README.md", true},
		{"*.md", "docs/guide/intro.md", true},
		{"*.md", "docs/guide.mdx", false},
		{"/README.md", "README.md", true},
		{"/README.md", "docs/README.md", false},
		{"./*.md", "docs/a.md", false},
		{"docs/**", "docs/a.md", true},
		{"docs/**", "docs/a/b/c.md", true},
		{"docs/**", "docsx/a.md", false},
		{"docs/", "docs/a/b.md", true},
		{"apps/*/src/**", "apps/web/src/x.ts", true},
		{"apps/*/src/**", "apps/web/lib/x.ts", false},
		{"**/testdata/**", "internal/core/testdata/a.txt", true},
		{"**/*.test.ts", "src/a.test.ts", true},
		{"**/*.test.ts", "a.test.ts", true},
		{"src/**/*.go", "src/a.go", true},
		{"src/**/*.go", "src/x/y/a.go", true},
		{`docs\**`, `docs\a.md`, true},
		{"[ab].txt", "b.txt", true},
		{"", "a", false},
		{"[", "[", false},
	} {
		if got := Match(tt.pattern, tt.path); got != tt.want {
			t.Errorf("Match(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}

func TestValid(t *testing.T) {
	for pattern, want := range map[string]bool{"docs/**": true, "*.md": true, "[": false, "a/[b": false, `a\`: true} {
		if got := Valid(pattern); got != want {
			t.Errorf("Valid(%q) = %v, want %v", pattern, got, want)
		}
	}
}

// FuzzMatch: no pattern panics or loops, "**" matches every path, and a
// name pattern matches as "**/<name>" does (matchPath's shortcut).
func FuzzMatch(f *testing.F) {
	f.Add("a/**/b/**/c", "a/x/b/y/c")
	f.Add("[", "x")
	f.Add("*.md", "docs/guide.md")
	f.Add("x?", "a/b/xy")
	f.Fuzz(func(t *testing.T, pattern, p string) {
		Match(pattern, p)
		if p != "" && !Match("**", p) {
			t.Fatalf(`"**" must match %q`, p)
		}
		if name := normalize(pattern); name != "" && !strings.Contains(name, "/") {
			if got, want := Match(pattern, p), matchSegments(strings.Split("**/"+name, "/"), segmentsOf(p)); got != want {
				t.Fatalf("Match(%q, %q) = %v, \"**/\" form says %v", pattern, p, got, want)
			}
		}
	})
}
