// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package convention

import (
	"strings"
	"unicode/utf8"
)

// TestedTrailer is the git trailer that records how the author tested the
// change ("Tested: login on Firefox and Safari"); the pull request's Testing
// section quotes it.
const TestedTrailer = "Tested"

// CommitMessage writes a commit message with the built-in body width
// (Rules.CommitMessage).
func CommitMessage(header, why, tested string) string {
	return Rules{}.CommitMessage(header, why, tested)
}

// CommitMessage writes a commit message: the header, the why as the body
// (wrapped at the repository's body width, paragraphs kept), and how it was
// tested as a trailer. Empty parts are left out: a header alone is a valid
// message.
func (r Rules) CommitMessage(header, why, tested string) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(header))
	b.WriteString("\n")
	if why = strings.TrimSpace(strings.ReplaceAll(why, "\r\n", "\n")); why != "" {
		for _, paragraph := range strings.Split(why, "\n\n") {
			if p := strings.TrimSpace(paragraph); p != "" {
				b.WriteString("\n" + Wrap(p, r.Width()) + "\n")
			}
		}
	}
	if tested = strings.Join(strings.Fields(tested), " "); tested != "" {
		b.WriteString("\n" + TestedTrailer + ": " + tested + "\n")
	}
	return b.String()
}

// Wrap breaks text into lines of at most width characters at spaces; a word
// longer than width (a URL) stays whole on its line. Existing line breaks are
// treated as spaces.
func Wrap(text string, width int) string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(text) {
		switch {
		case line == "":
			line = word
		case utf8.RuneCountInString(line)+1+utf8.RuneCountInString(word) <= width:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
