// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Command authorship checks that a pull request says how it was written
// (#147): exactly one box of the template's "Authorship" section ticked. The
// body is read from the BODY variable, never from a command line, since it
// is untrusted input:
//
//	BODY="$(gh pr view 42 --json body --jq .body)" go run ./tools/authorship
//
// Exit codes: 0 one box ticked, 1 none or several.
package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// Boxes are the template's choices, as they start in the body.
var Boxes = []string{
	"A human wrote it.",
	"An agent wrote it under close human direction.",
	"An agent wrote it on its own",
}

// ticked matches a checked task list item: "- [x] …" or "* [X] …".
var ticked = regexp.MustCompile(`(?m)^\s*[-*]\s+\[[xX]\]\s+(.+)$`)

// Ticked returns the template's boxes ticked in body, in the template's
// order.
func Ticked(body string) []string {
	var found []string
	matches := ticked.FindAllStringSubmatch(body, -1)
	for _, box := range Boxes {
		for _, m := range matches {
			if strings.HasPrefix(strings.TrimSpace(m[1]), box) {
				found = append(found, box)
				break
			}
		}
	}
	return found
}

// check writes the verdict and returns the exit code.
func check(w io.Writer, body string) int {
	found := Ticked(body)
	if len(found) == 1 {
		fmt.Fprintf(w, "Authorship: %s\n", found[0])
		return 0
	}
	what := "No box is ticked"
	if len(found) > 1 {
		what = fmt.Sprintf("%d boxes are ticked", len(found))
	}
	fmt.Fprintf(w, "%s in the \"Authorship\" section of the pull request: tick exactly one, by editing the description.\n", what)
	fmt.Fprintln(w, "Agents are welcome; the person who opens the pull request reads and runs the change, and says so:")
	fmt.Fprintln(w, "https://github.com/GregoireF/ship-it/blob/main/CONTRIBUTING.md#pull-requests")
	return 1
}

func main() {
	os.Exit(check(os.Stdout, os.Getenv("BODY")))
}
