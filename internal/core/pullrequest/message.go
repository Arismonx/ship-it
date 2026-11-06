// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package pullrequest

import (
	"regexp"
	"strings"
)

// trailerLine matches a git trailer ("Signed-off-by: …", "Co-authored-by: …",
// "Refs: #3"): a token of letters, digits and dashes, a colon, a value.
var trailerLine = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*: \S`)

// issueLine matches a line that only links issues ("Closes #12", "Fixes
// acme/shop#3, #4"): the pull request links the issue on its own line.
var issueLine = regexp.MustCompile(`(?i)^(close[sd]?|fix(e[sd])?|resolve[sd]?|refs?)\s+([\w.-]+/[\w.-]+)?#\d+([,\s]+([\w.-]+/[\w.-]+)?#\d+)*\.?$`)

// ParseMessage splits a commit message into its subject (first line) and the
// prose of its body: the trailing block of trailers and the lines that only
// link issues are dropped, as the pull request states those itself.
func ParseMessage(message string) (subject, body string) {
	message = strings.ReplaceAll(message, "\r\n", "\n")
	subject, body, _ = strings.Cut(strings.TrimSpace(message), "\n")
	paragraphs := strings.Split(strings.TrimSpace(body), "\n\n")
	// git reads trailers only in the last paragraph.
	trailerOrIssue := func(line string) bool { return trailerLine.MatchString(line) || issueLine.MatchString(line) }
	if last := paragraphs[len(paragraphs)-1]; last != "" && allLines(last, trailerOrIssue) {
		paragraphs = paragraphs[:len(paragraphs)-1]
	}
	var kept []string
	for _, p := range paragraphs {
		var lines []string
		for _, line := range strings.Split(p, "\n") {
			if !issueLine.MatchString(strings.TrimSpace(line)) {
				lines = append(lines, line)
			}
		}
		if text := strings.TrimSpace(strings.Join(lines, "\n")); text != "" {
			kept = append(kept, text)
		}
	}
	return strings.TrimSpace(subject), strings.Join(kept, "\n\n")
}

func allLines(paragraph string, match func(string) bool) bool {
	for _, line := range strings.Split(paragraph, "\n") {
		if !match(strings.TrimSpace(line)) {
			return false
		}
	}
	return true
}

// TrailerValues returns the values of the trailer token (such as "Tested")
// in the trailer block of a message, without regard to the token's case.
func TrailerValues(message, token string) []string {
	message = strings.TrimSpace(strings.ReplaceAll(message, "\r\n", "\n"))
	paragraphs := strings.Split(message, "\n\n")
	if len(paragraphs) < 2 {
		return nil
	}
	var values []string
	for _, line := range strings.Split(paragraphs[len(paragraphs)-1], "\n") {
		name, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok && strings.EqualFold(name, token) && strings.TrimSpace(value) != "" {
			values = append(values, strings.TrimSpace(value))
		}
	}
	return values
}
