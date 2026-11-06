// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package glob matches repository paths against the patterns users write in
// ship-it's configuration: `*`, `?` and `[...]` within a path segment (the
// syntax of path.Match), `**` for any number of segments, and — as in
// .gitignore — a pattern without a slash matches a file name at any depth
// ("*.md" matches "README.md" and "docs/guide.md").
//
// Paths and patterns use forward slashes and are relative to the repository
// root; a leading "./" or "/" in a pattern anchors it to the root.
//
// Allowed imports: standard library without I/O.
package glob

import (
	"path"
	"strings"
)

// Valid reports whether pattern is well formed (balanced brackets, no
// trailing backslash), so that configuration errors surface when the file is
// read, not when a path happens to be tested.
func Valid(pattern string) bool {
	for _, segment := range strings.Split(normalize(pattern), "/") {
		if segment == "**" {
			continue
		}
		if _, err := path.Match(segment, ""); err != nil {
			return false
		}
	}
	return true
}

// Match reports whether p matches pattern. A malformed pattern matches
// nothing (Valid rejects it first).
func Match(pattern, p string) bool {
	return matchPath(pattern, segmentsOf(p))
}

// Any reports whether p matches one of patterns. The path is split once.
func Any(patterns []string, p string) bool {
	segments := segmentsOf(p)
	for _, pattern := range patterns {
		if matchPath(pattern, segments) {
			return true
		}
	}
	return false
}

// segmentsOf splits a cleaned, slash-separated path.
func segmentsOf(p string) []string {
	return strings.Split(strings.TrimPrefix(path.Clean(strings.ReplaceAll(p, `\`, "/")), "./"), "/")
}

func matchPath(pattern string, segments []string) bool {
	pattern = normalize(pattern)
	if pattern == "" {
		return false
	}
	if !strings.Contains(pattern, "/") {
		if pattern != "**" && pattern != "." { // the segments matchSegments treats apart
			// A name at any depth ("**/name"): the last segment, as one
			// segment can only match the last one.
			ok, err := path.Match(pattern, segments[len(segments)-1])
			return err == nil && ok
		}
		pattern = "**/" + pattern
	}
	return matchSegments(strings.Split(pattern, "/"), segments)
}

// normalize uses forward slashes, removes the anchoring "./" or "/" (every
// pattern with a slash is anchored), and a trailing "/" ("docs/" is "docs/**").
func normalize(pattern string) string {
	pattern = strings.TrimSpace(strings.ReplaceAll(pattern, `\`, "/"))
	anchored := strings.HasPrefix(pattern, "/") || strings.HasPrefix(pattern, "./")
	pattern = strings.TrimPrefix(strings.TrimPrefix(pattern, "./"), "/")
	if strings.HasSuffix(pattern, "/") {
		pattern += "**"
	}
	if anchored && !strings.Contains(pattern, "/") {
		pattern = "./" + pattern // keep it anchored: a root file only
	}
	return pattern
}

func matchSegments(pattern, segments []string) bool {
	for len(pattern) > 0 {
		head := pattern[0]
		if head == "**" {
			// Collapse "**/**", then try every split point.
			for len(pattern) > 1 && pattern[1] == "**" {
				pattern = pattern[1:]
			}
			if len(pattern) == 1 {
				return true
			}
			for i := 0; i <= len(segments); i++ {
				if matchSegments(pattern[1:], segments[i:]) {
					return true
				}
			}
			return false
		}
		if head == "." { // anchored root marker from normalize
			pattern = pattern[1:]
			continue
		}
		if len(segments) == 0 {
			return false
		}
		if ok, err := path.Match(head, segments[0]); err != nil || !ok {
			return false
		}
		pattern, segments = pattern[1:], segments[1:]
	}
	return len(segments) == 0
}
