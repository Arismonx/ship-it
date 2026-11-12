// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package overlap finds where two changes to the same base touch the same
// lines, the likely conflicts between a branch and the open pull requests
// (#40, #41). It reads the hunk headers of unified diffs only: the lines each
// change takes from the base file, in the base's numbering, so two diffs made
// on the same base compare directly.
//
// Allowed imports: standard library without I/O, other core packages.
package overlap

import (
	"slices"
	"strconv"
	"strings"
)

// Range is a run of lines of the base file, both ends included. A pure
// insertion after line n is the empty run between n and n+1, kept as
// {n, n}: it touches what changes line n or n+1.
type Range struct {
	Start, End int
}

// File is what one change does to one base file: the ranges it changes, or
// Whole when it deletes, renames or rewrites the file as binary, which
// conflicts with any change to it.
type File struct {
	Path   string
	Ranges []Range
	Whole  bool
}

// ParsePatch reads the hunk headers of one file's patch (the "patch" of
// GitHub's pull request files, or the hunks of a git diff) and returns the
// base ranges it changes. Lines that are not hunk headers are skipped.
func ParsePatch(patch string) []Range {
	var ranges []Range
	for _, line := range strings.Split(patch, "\n") {
		if r, ok := hunkRange(line); ok {
			ranges = append(ranges, r)
		}
	}
	return ranges
}

// hunkRange reads "@@ -start,count +… @@": count lines from start in the
// base; a missing count is 1, a count of 0 an insertion after start.
func hunkRange(line string) (Range, bool) {
	rest, ok := strings.CutPrefix(line, "@@ -")
	if !ok {
		return Range{}, false
	}
	old, _, ok := strings.Cut(rest, " ")
	if !ok {
		return Range{}, false
	}
	startText, countText, hasCount := strings.Cut(old, ",")
	start, err := strconv.Atoi(startText)
	if err != nil || start < 0 {
		return Range{}, false
	}
	count := 1
	if hasCount {
		if count, err = strconv.Atoi(countText); err != nil || count < 0 {
			return Range{}, false
		}
	}
	if count == 0 {
		return Range{Start: start, End: start}, true
	}
	return Range{Start: start, End: start + count - 1}, true
}

// ParseDiff reads a whole unified diff (git diff base...HEAD, any context
// size) into one File per base path.
func ParseDiff(diff string) []File {
	var files []File
	var current *File
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			files = append(files, File{Path: basePath(line)})
			current = &files[len(files)-1]
		case current == nil:
		case strings.HasPrefix(line, "deleted file mode"), strings.HasPrefix(line, "rename from "),
			strings.HasPrefix(line, "Binary files "), strings.HasPrefix(line, "GIT binary patch"):
			current.Whole = true
		case strings.HasPrefix(line, "new file mode"):
			// Not in the base: nothing of it can touch another change's lines.
			current.Path = ""
		default:
			if r, ok := hunkRange(line); ok {
				current.Ranges = append(current.Ranges, r)
			}
		}
	}
	return slices.DeleteFunc(files, func(f File) bool { return f.Path == "" })
}

// basePath is the base side of "diff --git a/<path> b/<path>".
func basePath(header string) string {
	rest := strings.TrimPrefix(header, "diff --git ")
	if i := strings.Index(rest, " b/"); i >= 0 {
		rest = rest[:i]
	}
	return strings.TrimPrefix(rest, "a/")
}

// Overlaps returns the base paths that both changes touch on the same or
// adjacent lines, sorted. Adjacent lines count, as git cannot merge two
// changes with no unchanged line between them.
func Overlaps(mine, theirs []File) []string {
	byPath := make(map[string]File, len(theirs))
	for _, f := range theirs {
		byPath[f.Path] = f
	}
	var paths []string
	for _, m := range mine {
		t, ok := byPath[m.Path]
		if !ok {
			continue
		}
		if m.Whole || t.Whole || touches(m.Ranges, t.Ranges) {
			paths = append(paths, m.Path)
		}
	}
	slices.Sort(paths)
	return slices.Compact(paths)
}

func touches(a, b []Range) bool {
	for _, x := range a {
		for _, y := range b {
			if x.Start <= y.End+1 && y.Start <= x.End+1 {
				return true
			}
		}
	}
	return false
}
