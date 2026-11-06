// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package doclinks finds broken relative links in the repository's Markdown:
// a link to a file that does not exist, or to a heading that the file does
// not have. Links to the web are not followed.
package doclinks

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// skipped are the directories whose Markdown is not documentation: tools'
// state, dependencies, test fixtures.
var skipped = map[string]bool{".git": true, ".claude": true, "node_modules": true, "testdata": true, "dist": true, "out": true}

var (
	link    = regexp.MustCompile(`\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	heading = regexp.MustCompile(`^#{1,6}\s+(.*?)\s*#*\s*$`)
	anchor  = regexp.MustCompile(`<a\s+(?:id|name)="([^"]+)"`)
	code    = regexp.MustCompile("`[^`]*`")
)

// Check returns one line per broken link under root, as "file:line: link".
func Check(root string) ([]string, error) {
	var broken []string
	anchors := map[string]map[string]bool{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && skipped[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		lines, err := readLines(path)
		if err != nil {
			return err
		}
		for number, target := range links(lines) {
			for _, t := range target {
				if problem := checkLink(root, path, t, anchors); problem != "" {
					rel, _ := filepath.Rel(root, path)
					broken = append(broken, fmt.Sprintf("%s:%d: %s (%s)", filepath.ToSlash(rel), number, t, problem))
				}
			}
		}
		return nil
	})
	return broken, err
}

func readLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n"), nil
}

// links returns the link targets of each line (1-based), outside code
// blocks and inline code.
func links(lines []string) map[int][]string {
	found := map[int][]string{}
	fenced := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		for _, m := range link.FindAllStringSubmatch(code.ReplaceAllString(line, ""), -1) {
			found[i+1] = append(found[i+1], m[1])
		}
	}
	return found
}

// checkLink returns why target, found in file, is broken, or "".
func checkLink(root, file, target string, anchors map[string]map[string]bool) string {
	if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
		return ""
	}
	path, fragment, _ := strings.Cut(target, "#")
	resolved := file
	if path != "" {
		if strings.HasPrefix(path, "/") {
			resolved = filepath.Join(root, filepath.FromSlash(path))
		} else {
			resolved = filepath.Join(filepath.Dir(file), filepath.FromSlash(path))
		}
		if _, err := os.Stat(resolved); err != nil {
			return "no such file"
		}
	}
	if fragment == "" || filepath.Ext(resolved) != ".md" {
		return ""
	}
	if anchors[resolved] == nil {
		lines, err := readLines(resolved)
		if err != nil {
			return err.Error()
		}
		anchors[resolved] = headingAnchors(lines)
	}
	if !anchors[resolved][strings.ToLower(fragment)] {
		return "no such heading"
	}
	return ""
}

// headingAnchors returns the anchors GitHub gives the headings of a file,
// plus explicit <a id> and <a name> targets.
func headingAnchors(lines []string) map[string]bool {
	anchors := map[string]bool{}
	seen := map[string]int{}
	fenced := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		for _, m := range anchor.FindAllStringSubmatch(line, -1) {
			anchors[strings.ToLower(m[1])] = true
		}
		m := heading.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		slug := Slug(m[1])
		if n := seen[slug]; n > 0 {
			anchors[fmt.Sprintf("%s-%d", slug, n)] = true
		} else {
			anchors[slug] = true
		}
		seen[slug]++
	}
	return anchors
}

var markup = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)|<[^>]+>`)

// Slug is GitHub's anchor for a heading: lower case, links reduced to their
// text, punctuation and symbols dropped, spaces turned into hyphens.
func Slug(text string) string {
	text = markup.ReplaceAllStringFunc(text, func(s string) string {
		if strings.HasPrefix(s, "[") {
			return s[1:strings.Index(s, "]")]
		}
		return ""
	})
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return b.String()
}
