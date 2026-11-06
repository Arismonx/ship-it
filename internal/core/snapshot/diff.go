// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package snapshot

import (
	"strconv"
	"strings"
)

// Line is an added line and its number in the new version of the file.
type Line struct {
	Number int
	Text   string
}

// FileDiff is what a unified diff says about one file.
type FileDiff struct {
	Path         string
	OriginalPath string // set for renames
	Status       ChangeStatus
	Binary       bool
	Added        []Line
	Removed      []string
}

// ParseUnifiedDiff parses the output of `git diff` (unified format, any
// context size). Files appear in diff order.
func ParseUnifiedDiff(diff string) []FileDiff {
	var files []FileDiff
	var current *FileDiff
	newLine := 0
	inHunk := false
	for _, raw := range strings.Split(diff, "\n") {
		line := strings.TrimSuffix(raw, "\r")
		switch {
		case strings.HasPrefix(line, "diff --git "):
			files = append(files, FileDiff{Status: Modified})
			current = &files[len(files)-1]
			current.Path, current.OriginalPath = pathsFromDiffHeader(line)
			if current.Path == current.OriginalPath {
				current.OriginalPath = ""
			}
			inHunk = false
		case current == nil:
			continue
		case !inHunk && strings.HasPrefix(line, "new file mode"):
			current.Status = Added
		case !inHunk && strings.HasPrefix(line, "deleted file mode"):
			current.Status = Deleted
		case !inHunk && strings.HasPrefix(line, "rename from "):
			current.Status = Renamed
			current.OriginalPath = strings.TrimPrefix(line, "rename from ")
		case !inHunk && strings.HasPrefix(line, "rename to "):
			current.Path = strings.TrimPrefix(line, "rename to ")
		case !inHunk && strings.HasPrefix(line, "Binary files "):
			current.Binary = true
		case !inHunk && strings.HasPrefix(line, "+++ "):
			if p := strings.TrimPrefix(line, "+++ "); p != "/dev/null" {
				current.Path = strings.TrimPrefix(p, "b/")
			}
		case !inHunk && strings.HasPrefix(line, "--- "):
			if p := strings.TrimPrefix(line, "--- "); p != "/dev/null" && current.Status == Deleted {
				current.Path = strings.TrimPrefix(p, "a/")
			}
		case strings.HasPrefix(line, "@@ "):
			newLine = hunkNewStart(line)
			inHunk = true
		case inHunk && strings.HasPrefix(line, "+"):
			current.Added = append(current.Added, Line{Number: newLine, Text: line[1:]})
			newLine++
		case inHunk && strings.HasPrefix(line, "-"):
			current.Removed = append(current.Removed, line[1:])
		case inHunk && strings.HasPrefix(line, " "):
			newLine++
		}
	}
	return files
}

// pathsFromDiffHeader reads "diff --git a/<old> b/<new>". With
// core.quotePath=false and paths without " b/", the split is unambiguous;
// the ---/+++ and rename lines refine it afterwards.
func pathsFromDiffHeader(line string) (path, original string) {
	rest := strings.TrimPrefix(line, "diff --git ")
	if i := strings.Index(rest, " b/"); i >= 0 {
		return rest[i+3:], strings.TrimPrefix(rest[:i], "a/")
	}
	return rest, ""
}

// hunkNewStart reads the new-file start line of "@@ -a,b +c,d @@".
func hunkNewStart(header string) int {
	plus := strings.Index(header, " +")
	if plus < 0 {
		return 0
	}
	field := header[plus+2:]
	if end := strings.IndexAny(field, ", "); end >= 0 {
		field = field[:end]
	}
	n, _ := strconv.Atoi(field)
	return n
}
