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
//
// Pull requests write their entries in changelog.d/ (#148), never in
// CHANGELOG.md, so that they do not conflict:
//
//	go run ./tools/releasenotes -pending                      # what the next release says
//	go run ./tools/releasenotes -release v0.2.0 2026-12-01    # date it in CHANGELOG.md, remove the fragments
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
	args := os.Args[1:]
	if len(args) == 0 || (args[0] == "-release" && len(args) != 3) || (args[0] != "-release" && len(args) != 1) {
		fmt.Fprintln(os.Stderr, "usage: go run ./tools/releasenotes <version> | -pending | -release <version> <YYYY-MM-DD>")
		os.Exit(2)
	}
	content, err := os.ReadFile("CHANGELOG.md")
	if err == nil {
		err = run(args, string(content))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "releasenotes:", err)
		os.Exit(1)
	}
}

func run(args []string, changelog string) error {
	switch args[0] {
	case "-pending", "-release":
		fragments, files, err := readFragments(FragmentDir)
		if err != nil {
			return err
		}
		if args[0] == "-pending" {
			notes, err := pending(changelog, fragments)
			if err == nil {
				fmt.Println(notes)
			}
			return err
		}
		out, err := release(changelog, fragments, args[1], args[2], "")
		if err != nil {
			return err
		}
		if err := os.WriteFile("CHANGELOG.md", []byte(out), 0o644); err != nil {
			return err
		}
		for _, f := range files {
			if err := os.Remove(f); err != nil {
				return err
			}
		}
		fmt.Fprintf(os.Stderr, "CHANGELOG.md: [%s] dated %s from %d fragments; add its summary line.\n", strings.TrimPrefix(args[1], "v"), args[2], len(files))
		return nil
	}
	return write(os.Stdout, changelog, args[0])
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
