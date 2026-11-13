// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
)

// SkipLabel lets a pull request whose title says feat or fix go without a
// fragment, when users see nothing of it.
const SkipLabel = "changelog/skip"

// userVisible matches the types whose changes users see: a gitmoji, then
// the type, an optional scope, an optional "!", and ":".
var userVisible = regexp.MustCompile(`^(?:\S+\s+)?(feat|fix|perf|security)(?:\([^)]*\))?!?:`)

// checkFragment says whether a pull request has the changelog entry its
// title calls for (#164): a feat, fix, perf or security pull request adds a
// fragment to changelog.d/, unless it carries SkipLabel. It writes what to
// do and returns false when one is missing.
func checkFragment(w io.Writer, title string, labels, changed []string) bool {
	m := userVisible.FindStringSubmatch(strings.TrimSpace(title))
	if m == nil {
		fmt.Fprintln(w, "changelog: the title's type needs no entry.")
		return true
	}
	for _, l := range labels {
		if strings.TrimSpace(l) == SkipLabel {
			fmt.Fprintf(w, "changelog: %s, so no entry is needed.\n", SkipLabel)
			return true
		}
	}
	for _, f := range changed {
		if path.Dir(f) == FragmentDir && strings.HasSuffix(f, ".md") && path.Base(f) != "README.md" {
			fmt.Fprintf(w, "changelog: %s has the entry.\n", f)
			return true
		}
	}
	fmt.Fprintf(w, "A %s pull request changes what users see: add its changelog entry as a file in %s/, for example %s/<your-branch>.md:\n\n", m[1], FragmentDir, FragmentDir)
	fmt.Fprintln(w, "    ### Added   (or Changed, Fixed, Security…)")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "    - What changes for users, in one line (#<issue>).")
	fmt.Fprintln(w, "")
	fmt.Fprintf(w, "How: https://github.com/GregoireF/ship-it/blob/main/%s/README.md\n", FragmentDir)
	fmt.Fprintf(w, "Users see nothing of it (tests, CI, refactoring)? A maintainer adds the %s label.\n", SkipLabel)
	return false
}
