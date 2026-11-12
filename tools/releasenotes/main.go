// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Command releasenotes prints the section of CHANGELOG.md for one version,
// without its heading: the notes of the GitHub release, which GoReleaser
// would otherwise fill with a list of commits (release.yml passes them with
// --release-notes). Run it from the module root:
//
//	go run ./tools/releasenotes v0.1.2 > notes.md
//
// It fails when the version has no dated section, so that a release never
// goes out without its notes.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./tools/releasenotes <version>")
		os.Exit(2)
	}
	changelog, err := os.ReadFile("CHANGELOG.md")
	if err == nil {
		err = write(os.Stdout, string(changelog), os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "releasenotes:", err)
		os.Exit(1)
	}
}

// heading is a version heading of Keep a Changelog: "## [0.1.2] - 2026-11-12".
var heading = regexp.MustCompile(`(?m)^## \[([^\]]+)\](.*)$`)

// write prints the section of version ("v0.1.2" or "0.1.2"), its body only,
// with the link definitions at the end of the file left out.
func write(w io.Writer, changelog, version string) error {
	version = strings.TrimPrefix(version, "v")
	matches := heading.FindAllStringSubmatchIndex(changelog, -1)
	for i, m := range matches {
		if changelog[m[2]:m[3]] != version {
			continue
		}
		if !regexp.MustCompile(`\d{4}-\d{2}-\d{2}`).MatchString(changelog[m[4]:m[5]]) {
			return fmt.Errorf("the section of %s has no release date", version)
		}
		end := len(changelog)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		var body []string
		for _, line := range strings.Split(changelog[m[1]:end], "\n") {
			if regexp.MustCompile(`^\[[^\]]+\]: `).MatchString(line) {
				continue // a link definition of the file's footer
			}
			body = append(body, line)
		}
		notes := strings.TrimSpace(strings.Join(body, "\n"))
		if notes == "" {
			return fmt.Errorf("the section of %s is empty", version)
		}
		_, err := fmt.Fprintln(w, notes)
		return err
	}
	return errors.New("CHANGELOG.md has no section for " + version)
}
