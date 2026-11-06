// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package convention

import (
	"regexp"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/fault"
)

// Developer Certificate of Origin 1.1 (https://developercertificate.org/):
// a contributor certifies a commit by a "Signed-off-by: Name <email>" trailer,
// which `git commit -s` adds.

// Trailer is one "key: value" line of a commit message's trailer block.
type Trailer struct {
	Key   string
	Value string
}

// gitGeneratedPrefixes are the trailers git itself writes; one of them lets a
// block hold up to 75% other lines (git's trailer.c).
var gitGeneratedPrefixes = []string{"Signed-off-by: ", "(cherry picked from commit "}

// Trailers returns the trailers of message the way git reads them
// (git-interpret-trailers, trailer.c): the trailer block is the last
// paragraph, never the title, and either (i) is all trailers or (ii) holds a
// git-generated trailer and at least 25% trailer lines. Values may be folded
// over lines that start with whitespace. Only ":" separates a key from its
// value; a commit message has no "---" patch divider to look for.
func Trailers(message string) []Trailer {
	lines := strings.Split(strings.ReplaceAll(message, "\r\n", "\n"), "\n")
	start := trailerBlockStart(lines)
	if start < 0 {
		return nil
	}
	var trailers []Trailer
	for _, line := range lines[start:] {
		if isBlank(line) {
			continue
		}
		if startsWithSpace(line) {
			if n := len(trailers); n > 0 { // continuation: unfold
				trailers[n-1].Value = strings.TrimSpace(trailers[n-1].Value + " " + strings.TrimSpace(line))
			}
			continue
		}
		sep := findSeparator(line)
		if sep < 1 {
			continue // a non-trailer line of a 25% block
		}
		trailers = append(trailers, Trailer{
			Key:   strings.TrimRight(line[:sep], " \t"),
			Value: strings.TrimSpace(line[sep+1:]),
		})
	}
	return trailers
}

// trailerBlockStart ports find_trailer_block_start of git's trailer.c; it
// returns the index of the first line of the block, or -1.
func trailerBlockStart(lines []string) int {
	titleEnd := len(lines)
	for i, line := range lines {
		if isBlank(line) {
			titleEnd = i
			break
		}
	}
	onlySpaces, recognized := true, false
	trailerLines, nonTrailerLines, possibleContinuations := 0, 0, 0
	for i := len(lines) - 1; i >= titleEnd; i-- {
		line := lines[i]
		if isBlank(line) {
			if onlySpaces {
				continue
			}
			nonTrailerLines += possibleContinuations
			if recognized && trailerLines*3 >= nonTrailerLines {
				return i + 1
			}
			if trailerLines > 0 && nonTrailerLines == 0 {
				return i + 1
			}
			return -1
		}
		onlySpaces = false
		generated := false
		for _, prefix := range gitGeneratedPrefixes {
			if strings.HasPrefix(line, prefix) {
				generated = true
				break
			}
		}
		switch {
		case generated:
			trailerLines++
			possibleContinuations = 0
			recognized = true
		case findSeparator(line) >= 1 && !startsWithSpace(line):
			trailerLines++
			possibleContinuations = 0
		case startsWithSpace(line):
			possibleContinuations++
		default:
			nonTrailerLines += 1 + possibleContinuations
			possibleContinuations = 0
		}
	}
	return -1
}

// findSeparator ports find_separator of git's trailer.c with ":" as the only
// separator: the key is letters, digits and "-", optionally followed by
// spaces; "https://…" is not a trailer.
func findSeparator(line string) int {
	whitespace := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if c == ':' {
			if !whitespace && strings.HasPrefix(line[i:], "://") {
				return -1
			}
			return i
		}
		if !whitespace && (isASCIIAlnum(c) || c == '-') {
			continue
		}
		if i != 0 && (c == ' ' || c == '\t') {
			whitespace = true
			continue
		}
		break
	}
	return -1
}

func isASCIIAlnum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

func isBlank(line string) bool { return strings.TrimSpace(line) == "" }

func startsWithSpace(line string) bool {
	return line != "" && strings.ContainsRune(" \t\n\r\v\f", rune(line[0]))
}

// Signoff is one "Signed-off-by: Name <email>" trailer.
type Signoff struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Signoffs returns the sign-offs of a commit message's trailer block. The
// key is compared without case, like git compares trailer keys.
func Signoffs(message string) []Signoff {
	var signoffs []Signoff
	for _, t := range Trailers(message) {
		if !strings.EqualFold(t.Key, "Signed-off-by") {
			continue
		}
		open := strings.LastIndex(t.Value, " <")
		if open < 0 || !strings.HasSuffix(t.Value, ">") {
			signoffs = append(signoffs, Signoff{Name: t.Value}) // no email: never matches
			continue
		}
		signoffs = append(signoffs, Signoff{
			Name:  strings.TrimSpace(t.Value[:open]),
			Email: t.Value[open+2 : len(t.Value)-1],
		})
	}
	return signoffs
}

// SignedCommit is what the sign-off check needs to know of a commit.
type SignedCommit struct {
	Merge          bool
	AuthorName     string
	AuthorEmail    string
	CommitterName  string
	CommitterEmail string
	Message        string
}

// IsBotEmail reports whether email is a GitHub App's bot address
// ("…[bot]@users.noreply.github.com"), such as Dependabot's.
func IsBotEmail(email string) bool {
	return strings.HasSuffix(strings.ToLower(email), "[bot]@users.noreply.github.com")
}

// CheckSignoff validates the DCO sign-off of one commit, with the rule of the
// DCO GitHub App (github.com/dcoapp/app): merges and bots are exempt; one
// sign-off must carry the name of the author or the committer and the email
// of the author or the committer, compared without case. Unlike the app,
// only the trailer block counts, as for git itself.
func CheckSignoff(c SignedCommit) *fault.Fault {
	if c.Merge || IsBotEmail(c.AuthorEmail) {
		return nil
	}
	signoffs := Signoffs(c.Message)
	if len(signoffs) == 0 {
		return fault.New(fault.SignoffMissing, "convention.signoff.missing",
			"name", c.AuthorName, "email", c.AuthorEmail)
	}
	names := []string{strings.ToLower(c.AuthorName), strings.ToLower(c.CommitterName)}
	emails := []string{strings.ToLower(c.AuthorEmail), strings.ToLower(c.CommitterEmail)}
	for _, s := range signoffs {
		if contains(names, strings.ToLower(s.Name)) && contains(emails, strings.ToLower(s.Email)) {
			return nil
		}
	}
	got := make([]string, 0, len(signoffs))
	for _, s := range signoffs {
		got = append(got, s.Name+" <"+s.Email+">")
	}
	return fault.New(fault.SignoffMismatch, "convention.signoff.mismatch",
		"name", c.AuthorName, "email", c.AuthorEmail, "found", strings.Join(got, ", "))
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// dcoMention finds the DCO in a contributing guide: the acronym as a word, or
// the trailer, in any case.
var dcoMention = regexp.MustCompile(`\bDCO\b|(?i)signed-off-by|developer certificate of origin`)

// WantsSignoff reports whether a repository asks for DCO sign-offs
// (ship.commits.signoff auto): most of its recent commits carry one (merges
// and bots included in the count, as they rarely sign off), one of its
// contributing guides mentions the DCO or Signed-off-by, or it configures the
// DCO app (dcoApp: .github/dco.yml exists).
func WantsSignoff(messages, guides []string, dcoApp bool) bool {
	if dcoApp {
		return true
	}
	for _, g := range guides {
		if dcoMention.MatchString(g) {
			return true
		}
	}
	signed := 0
	for _, m := range messages {
		if len(Signoffs(m)) > 0 {
			signed++
		}
	}
	return len(messages) > 0 && 2*signed > len(messages)
}
