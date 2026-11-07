// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package pullrequest

import (
	"fmt"
	"slices"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/plan"
	"github.com/GregoireF/ship-it/internal/core/snapshot"
)

// Changes are the facts of a pull request's diff for its summary (#32): the
// files it adds, changes, renames and removes, and the declarations it adds
// and removes. Only what the diff says; GitHub's "Files changed" tab has the
// rest.
type Changes struct {
	Added, Changed, Renamed, Removed []string
	// Declared and Undeclared are the names of the declarations the diff adds
	// and removes; a declaration rewritten in place (its signature changed)
	// is in neither.
	Declared, Undeclared []string
}

// ChangesOf reads the net diff of a branch (snapshot.ParseUnifiedDiff).
func ChangesOf(files []snapshot.FileDiff) Changes {
	var c Changes
	for _, f := range files {
		switch f.Status {
		case snapshot.Added:
			c.Added = append(c.Added, f.Path)
		case snapshot.Deleted:
			c.Removed = append(c.Removed, f.Path)
		case snapshot.Renamed:
			c.Renamed = append(c.Renamed, f.Path)
		default:
			c.Changed = append(c.Changed, f.Path)
		}
		if f.Binary {
			continue
		}
		added := map[string]bool{}
		for _, l := range f.Added {
			if name := plan.DeclaredName(l.Text); name != "" {
				added[name] = true
			}
		}
		removed := map[string]bool{}
		for _, l := range f.Removed {
			if name := plan.DeclaredName(l); name != "" {
				removed[name] = true
			}
		}
		for name := range added {
			if !removed[name] && !slices.Contains(c.Declared, name) {
				c.Declared = append(c.Declared, name)
			}
		}
		for name := range removed {
			if !added[name] && !slices.Contains(c.Undeclared, name) {
				c.Undeclared = append(c.Undeclared, name)
			}
		}
	}
	slices.Sort(c.Declared)
	slices.Sort(c.Undeclared)
	return c
}

// listedFiles and listedNames bound what the summary names: past them it
// counts, so a large pull request still gets one short line.
const (
	listedFiles = 3
	listedNames = 6
)

// Line writes the changes as one Markdown line, or "" when there are none:
//
//	**Changes:** 3 files: 1 added (`vat.go`), 2 changed. Added `WithVAT`, `VAT`; removed `OldRate`.
func (c Changes) Line() string {
	total := len(c.Added) + len(c.Changed) + len(c.Renamed) + len(c.Removed)
	if total == 0 {
		return ""
	}
	var kinds []string
	for _, k := range []struct {
		files []string
		verb  string
		named bool
	}{
		{c.Added, "added", true},
		{c.Changed, "changed", false},
		{c.Renamed, "renamed", true},
		{c.Removed, "removed", true},
	} {
		if len(k.files) == 0 {
			continue
		}
		part := fmt.Sprintf("%d %s", len(k.files), k.verb)
		if k.named && len(k.files) <= listedFiles {
			part += " (" + spans(k.files, listedFiles) + ")"
		}
		kinds = append(kinds, part)
	}
	files := "files"
	if total == 1 {
		files = "file"
	}
	line := fmt.Sprintf("**Changes:** %d %s: %s.", total, files, strings.Join(kinds, ", "))
	var declarations []string
	if len(c.Declared) > 0 {
		declarations = append(declarations, "Added "+spans(c.Declared, listedNames))
	}
	if len(c.Undeclared) > 0 {
		verb := "removed "
		if len(declarations) == 0 {
			verb = "Removed "
		}
		declarations = append(declarations, verb+spans(c.Undeclared, listedNames))
	}
	if len(declarations) > 0 {
		line += " " + strings.Join(declarations, "; ") + "."
	}
	return line
}

// spans writes names as code spans, the first max of them, then how many
// more there are.
func spans(names []string, max int) string {
	shown := names
	if len(names) > max {
		shown = names[:max]
	}
	out := make([]string, len(shown))
	for i, n := range shown {
		out[i] = codeSpan(n)
	}
	text := strings.Join(out, ", ")
	if more := len(names) - len(shown); more > 0 {
		text += fmt.Sprintf(" and %d more", more)
	}
	return text
}
