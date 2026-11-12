// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package codeowners reads a CODEOWNERS file the way GitHub does (#46),
// following https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners
// (read on 2026-11-12):
//
//   - the file is .github/CODEOWNERS, CODEOWNERS or docs/CODEOWNERS, the
//     first found in that order, on the base branch;
//   - "#" starts a comment, also after the owners;
//   - patterns follow .gitignore, except that "\#" escaping, "!" negation
//     and "[ ]" ranges do not work: such lines are skipped, as GitHub skips
//     a line it cannot read;
//   - "docs/*" owns the files of docs/ but not those of its subdirectories;
//   - the last matching pattern wins, and a pattern without owners leaves
//     its paths without owners.
//
// Allowed imports: standard library without I/O, other core packages.
package codeowners

import (
	"strings"

	"github.com/GregoireF/ship-it/internal/core/glob"
)

// Locations are where GitHub looks for the file, in its order.
var Locations = []string{".github/CODEOWNERS", "CODEOWNERS", "docs/CODEOWNERS"}

// MaxBytes is GitHub's limit: a larger file is not loaded at all.
const MaxBytes = 3 << 20

// Rule is one line: a pattern and its owners (@user, @org/team or an
// email), none for an unowned path.
type Rule struct {
	Pattern string
	Owners  []string
}

// File is a parsed CODEOWNERS file.
type File struct {
	Rules []Rule
}

// Parse reads a CODEOWNERS file. Lines it cannot read are skipped.
func Parse(content []byte) File {
	var f File
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		pattern := fields[0]
		if strings.HasPrefix(pattern, "\\#") || strings.HasPrefix(pattern, "!") || strings.ContainsAny(pattern, "[]") || !glob.Valid(pattern) {
			continue
		}
		rule := Rule{Pattern: pattern}
		for _, owner := range fields[1:] {
			if strings.HasPrefix(owner, "#") {
				break
			}
			rule.Owners = append(rule.Owners, owner)
		}
		f.Rules = append(f.Rules, rule)
	}
	return f
}

// Owners returns the owners of path: those of the last matching rule, none
// when no rule matches or the last one has no owners.
func (f File) Owners(path string) []string {
	if strings.Trim(path, "/") == "" {
		return nil
	}
	for i := len(f.Rules) - 1; i >= 0; i-- {
		if matches(f.Rules[i].Pattern, path) {
			return f.Rules[i].Owners
		}
	}
	return nil
}

// matches applies one pattern to a file path. As in .gitignore, a pattern
// that names a directory owns everything under it, so the path's parent
// directories are tried too; a trailing "/" matches directories only, and a
// last segment "*" matches files of that directory only.
func matches(pattern, path string) bool {
	path = strings.TrimPrefix(path, "/")
	directoryOnly := strings.HasSuffix(pattern, "/")
	pattern = strings.TrimSuffix(pattern, "/")
	if pattern == "" {
		return false
	}
	if !directoryOnly && glob.Match(pattern, path) {
		return true
	}
	if strings.HasSuffix(pattern, "/*") {
		return false
	}
	segments := strings.Split(path, "/")
	for i := len(segments) - 1; i >= 1; i-- {
		if glob.Match(pattern, strings.Join(segments[:i], "/")) {
			return true
		}
	}
	return false
}
