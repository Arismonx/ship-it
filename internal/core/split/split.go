// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package split

import (
	"path"
	"slices"
	"strings"
)

// File is a changed file and its changed lines, generated files excluded by
// the caller.
type File struct {
	Path  string
	Lines int
}

// Group is a part of the work that can ship on its own: `ship --only Path`.
type Group struct {
	// Path is the directory (with a trailing "/") or the file of the group.
	Path  string   `json:"path"`
	Lines int      `json:"lines"`
	Files []string `json:"files"`
}

// containers are directories that hold the code rather than name a topic:
// the group goes one level deeper (src/api/… → src/api/).
var containers = map[string]bool{
	"src": true, "lib": true, "app": true, "apps": true, "packages": true, "internal": true, "pkg": true,
	"cmd": true, "server": true, "client": true, "components": true, "pages": true, "modules": true,
}

// Suggest proposes how to split work larger than maxLines into parts that
// ship separately, biggest first: one group per topic directory. It returns
// nil when the work is small enough, or when it cannot be split (one topic).
func Suggest(files []File, maxLines int) []Group {
	total := 0
	for _, f := range files {
		total += f.Lines
	}
	if total <= maxLines {
		return nil
	}
	groups := Areas(files)
	if len(groups) < 2 {
		return nil
	}
	return groups
}

// Areas groups files by topic directory, biggest first, whatever their size:
// the parts of a split, and the layers of a stack.
func Areas(files []File) []Group {
	changed := make(map[string]bool, len(files))
	for _, f := range files {
		changed[f.Path] = true
	}
	byKey := map[string]*Group{}
	var keys []string
	for _, f := range files {
		// A test goes with the file it tests when that file is part of the
		// work: a layer of tests alone would not build (vat_test.go calls
		// what vat.go adds), and both are the same topic.
		p := f.Path
		if s := subject(p); s != "" && changed[s] {
			p = s
		}
		key := topic(p)
		g, ok := byKey[key]
		if !ok {
			g = &Group{Path: key}
			byKey[key] = g
			keys = append(keys, key)
		}
		g.Lines += f.Lines
		g.Files = append(g.Files, f.Path)
	}
	groups := make([]Group, 0, len(keys))
	for _, k := range keys {
		groups = append(groups, *byKey[k])
	}
	slices.SortStableFunc(groups, func(a, b Group) int {
		if a.Lines != b.Lines {
			return b.Lines - a.Lines
		}
		return strings.Compare(a.Path, b.Path)
	})
	return groups
}

// topic is the directory naming what a file is about: the first segment
// that is not a container, or the file itself at the root.
func topic(p string) string {
	segments := strings.Split(path.Clean(p), "/")
	for i, s := range segments[:len(segments)-1] {
		if !containers[s] {
			return strings.Join(segments[:i+1], "/") + "/"
		}
	}
	if len(segments) == 1 {
		return p // a root file is its own topic
	}
	return strings.Join(segments[:len(segments)-1], "/") + "/"
}

// testSuffixes are the endings of test files next to the file they test,
// with the ending of that file: Go, JavaScript and TypeScript, Python.
var testSuffixes = [][2]string{
	{"_test.go", ".go"},
	{".test.ts", ".ts"}, {".spec.ts", ".ts"}, {".test.tsx", ".tsx"}, {".spec.tsx", ".tsx"},
	{".test.js", ".js"}, {".spec.js", ".js"}, {".test.jsx", ".jsx"}, {".spec.jsx", ".jsx"},
	{".test.mjs", ".mjs"}, {".spec.mjs", ".mjs"},
	{"_test.py", ".py"},
}

// subject is the file a test file tests, in the same directory (vat_test.go →
// vat.go, test_vat.py → vat.py), or "" when p is not a test file.
func subject(p string) string {
	dir, base := path.Split(p)
	for _, s := range testSuffixes {
		if stem, ok := strings.CutSuffix(base, s[0]); ok && stem != "" {
			return dir + stem + s[1]
		}
	}
	if stem, ok := strings.CutPrefix(base, "test_"); ok && strings.HasSuffix(stem, ".py") && stem != ".py" {
		return dir + stem
	}
	return ""
}

// Matches reports whether p is selected by one of the --only selections: a
// file, or a directory and everything under it.
func Matches(p string, only []string) bool {
	for _, o := range only {
		o = strings.TrimSuffix(path.Clean(strings.ReplaceAll(o, `\`, "/")), "/")
		if o == "." || p == o || strings.HasPrefix(p, o+"/") {
			return true
		}
	}
	return false
}
