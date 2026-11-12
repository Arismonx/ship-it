// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// FragmentDir holds one changelog fragment per pull request (#148), so that
// pull requests never conflict on CHANGELOG.md: a fragment is a Markdown
// file of "### <section>" headings and "- " entries, assembled at release.
const FragmentDir = "changelog.d"

// Sections are Keep a Changelog's, in its order.
var Sections = []string{"Added", "Changed", "Deprecated", "Removed", "Fixed", "Security"}

// entries maps a section to its entries, each one "- " line and its
// continuation lines.
type entries map[string][]string

// parseEntries reads "### Section" blocks. It fails on an unknown section,
// or on text outside one.
func parseEntries(text, name string) (entries, error) {
	e := entries{}
	section := ""
	for i, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "" || strings.HasPrefix(trimmed, "<!--"):
		case strings.HasPrefix(trimmed, "### "):
			section = strings.TrimSpace(strings.TrimPrefix(trimmed, "### "))
			if !contains(Sections, section) {
				return nil, fmt.Errorf("%s:%d: unknown section %q (one of %s)", name, i+1, section, strings.Join(Sections, ", "))
			}
		case section == "":
			return nil, fmt.Errorf("%s:%d: text before any \"### <section>\" heading", name, i+1)
		case strings.HasPrefix(trimmed, "- "):
			e[section] = append(e[section], trimmed)
		default:
			items := e[section]
			if len(items) == 0 {
				return nil, fmt.Errorf("%s:%d: text that is not a \"- \" entry", name, i+1)
			}
			items[len(items)-1] += " " + trimmed
		}
	}
	return e, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// readFragments reads every fragment of dir, by file name.
func readFragments(dir string) (entries, []string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(files)
	all := entries{}
	var read []string
	for _, f := range files {
		if filepath.Base(f) == "README.md" {
			continue
		}
		content, err := os.ReadFile(f)
		if err != nil {
			return nil, nil, err
		}
		e, err := parseEntries(string(content), filepath.ToSlash(f))
		if err != nil {
			return nil, nil, err
		}
		for _, s := range Sections {
			all[s] = append(all[s], e[s]...)
		}
		read = append(read, f)
	}
	return all, read, nil
}

// render writes entries by section, in Keep a Changelog's order.
func render(e entries) string {
	var b strings.Builder
	for _, s := range Sections {
		if len(e[s]) == 0 {
			continue
		}
		fmt.Fprintf(&b, "### %s\n\n%s\n\n", s, strings.Join(e[s], "\n"))
	}
	return strings.TrimSpace(b.String())
}

// unreleasedBounds returns where the body of [Unreleased] starts and ends.
func unreleasedBounds(changelog string) (int, int, error) {
	matches := heading.FindAllStringSubmatchIndex(changelog, -1)
	for i, m := range matches {
		if changelog[m[2]:m[3]] == "Unreleased" {
			end := len(changelog)
			if i+1 < len(matches) {
				end = matches[i+1][0]
			}
			return m[1], end, nil
		}
	}
	return 0, 0, fmt.Errorf("CHANGELOG.md has no [Unreleased] section")
}

// pending is what the next release says: the entries still written under
// [Unreleased], then the fragments.
func pending(changelog string, fragments entries) (string, error) {
	start, end, err := unreleasedBounds(changelog)
	if err != nil {
		return "", err
	}
	body := changelog[start:end]
	// Prose under [Unreleased] (a pointer to changelog.d) is not an entry.
	var kept []string
	inSection := false
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "### ") {
			inSection = true
		}
		if inSection {
			kept = append(kept, line)
		}
	}
	written, err := parseEntries(strings.Join(kept, "\n"), "CHANGELOG.md [Unreleased]")
	if err != nil {
		return "", err
	}
	for _, s := range Sections {
		written[s] = append(written[s], fragments[s]...)
	}
	return render(written), nil
}

// unreleasedNote stays under [Unreleased] once its entries are fragments.
const unreleasedNote = "Entries wait in [changelog.d](changelog.d/) until the release: `go run ./tools/releasenotes -pending` shows them."

var linkLine = regexp.MustCompile(`(?m)^\[Unreleased\]: (\S+)/compare/(v[^.]+\.[^.]+\.[^.]+)\.\.\.HEAD$`)

// release turns [Unreleased] and the fragments into the dated section of
// version, and points the compare links at it.
func release(changelog string, fragments entries, version, date, intro string) (string, error) {
	version = strings.TrimPrefix(version, "v")
	if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`).MatchString(date) {
		return "", fmt.Errorf("the date %q is not YYYY-MM-DD", date)
	}
	notes, err := pending(changelog, fragments)
	if err != nil {
		return "", err
	}
	if notes == "" {
		return "", fmt.Errorf("nothing to release: [Unreleased] and %s are empty", FragmentDir)
	}
	if intro != "" {
		notes = strings.TrimSpace(intro) + "\n\n" + notes
	}
	start, end, _ := unreleasedBounds(changelog)
	out := changelog[:start] + "\n\n" + unreleasedNote + "\n\n## [" + version + "] - " + date + "\n\n" + notes + "\n\n" + changelog[end:]
	if m := linkLine.FindStringSubmatch(out); m != nil {
		out = linkLine.ReplaceAllString(out, "[Unreleased]: "+m[1]+"/compare/v"+version+"...HEAD\n["+version+"]: "+m[1]+"/compare/"+m[2]+"...v"+version)
	}
	return out, nil
}
