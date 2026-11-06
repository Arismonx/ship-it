// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package pullrequest writes the body of a pull request and decides whether
// it opens as a draft (docs/reference/conventions.md, ADR-0010).
//
// Allowed imports: standard library without I/O, other core packages.
package pullrequest

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/GregoireF/ship-it/internal/core/convention"
)

// Marker ends every body ship-it writes: invisible on GitHub, it lets ship-it
// recognise its pull requests without signing them (decision of 2026-10-23:
// a pull request reads as written by its author, not by a tool).
const Marker = "<!-- ship-it -->"

// Commit is one commit of the pull request: its parsed header and the prose
// of its body (see ParseMessage).
type Commit struct {
	Header convention.Header
	Body   string
	// Tested holds the author's "Tested:" trailers: how they checked it.
	Tested []string
}

// BodyInput is what the body is written from. Only facts are used: what the
// developer wrote (commit bodies), what ship-it ran (gates) and links.
type BodyInput struct {
	// Issue is linked with Keyword when non-zero; IssueTitle, when known,
	// gives context to readers of the notification e-mail.
	Issue      int
	IssueTitle string
	Keyword    convention.IssueKeyword
	// Commits are the commits of the branch, oldest first.
	Commits []Commit
	// Checks are the commands that passed locally before the push.
	Checks []string
	// ChecksSkipped: the local checks were skipped (--skip-gates); reviewers
	// must know that only the CI vouches for this work.
	ChecksSkipped bool
	// AIAssistant names the model that wrote the plan, if any (ADR-0004):
	// disclosed, as projects such as Kubernetes ask in their templates.
	AIAssistant string
	// AILabel is the repository's own label for AI-assisted work, when it
	// has one: it then discloses the assistance instead of a sentence.
	AILabel string
	// Omit leaves sections out of ship-it's own body
	// (ship.pullRequest.sections); a repository template places them itself.
	Omit Omit
}

// Omit names the sections Body leaves out.
type Omit struct {
	// Why: the commit bodies; several commits are still listed.
	Why     bool
	Testing bool
	// Issue: the "Closes #12" line.
	Issue bool
}

// Sections are the parts of a body, rendered as Markdown without headings,
// so that a repository's own template can place them (FillTemplate).
type Sections struct {
	Issue    string // "Closes #12", or ""
	Summary  string // why, then what changed
	Testing  string // what ran before the push
	Notes    string // breaking changes
	Disclose string // AI assistance, for a template section about it
	// DisclosedByLabel: a repository label says it; no sentence is added
	// outside a template section.
	DisclosedByLabel bool
}

// Compose turns the input into sections; an empty section has nothing true
// to say and is left out of the body (no placeholder, no empty heading).
func Compose(in BodyInput) Sections {
	var s Sections
	if in.Issue > 0 {
		keyword := in.Keyword
		if keyword == "" {
			keyword = convention.DefaultIssueKeyword
		}
		s.Issue = convention.IssueReference(keyword, "", in.Issue)
		if in.IssueTitle != "" {
			s.Issue += " (" + strings.TrimSpace(in.IssueTitle) + ")"
		}
	}

	var why []string
	for _, c := range in.Commits {
		if c.Body != "" && !slices.Contains(why, c.Body) {
			why = append(why, c.Body)
		}
	}
	summary := strings.Join(why, "\n\n")
	// One commit: the title says what changed. Several: list them, in the
	// developer's words, without the commit-type syntax.
	if len(in.Commits) > 1 {
		var list strings.Builder
		for _, c := range in.Commits {
			fmt.Fprintf(&list, "- %s\n", Sentence(c.Header.Subject))
		}
		summary = strings.TrimSpace(summary + "\n\n" + list.String())
	}
	s.Summary = strings.TrimSpace(summary)

	var testing []string
	switch {
	case in.ChecksSkipped:
		testing = append(testing, "Local checks were skipped (`--skip-gates`): only CI vouches for this change.")
	case len(in.Checks) > 0:
		spans := make([]string, len(in.Checks))
		for i, c := range in.Checks {
			spans[i] = codeSpan(c)
		}
		testing = append(testing, enumerate(spans)+" pass locally.")
	}
	// What the author says they tested, in their words (never invented).
	var notes []string
	for _, c := range in.Commits {
		for _, t := range c.Tested {
			if !slices.Contains(notes, t) {
				notes = append(notes, t)
			}
		}
	}
	if len(notes) > 0 {
		testing = append(testing, "Tested by the author: "+strings.TrimSuffix(strings.Join(notes, "; "), ".")+".")
	}
	s.Testing = strings.Join(testing, "\n\n")

	var breaking []string
	for _, c := range in.Commits {
		if c.Header.Breaking {
			breaking = append(breaking, Sentence(c.Header.Subject))
		}
	}
	if len(breaking) > 0 {
		s.Notes = "**Breaking change:** " + strings.Join(breaking, "; ") + "."
	}
	if in.AIAssistant != "" {
		// A plain statement of fact, without a tool's signature.
		s.Disclose = "The commit message was drafted with AI assistance (" + in.AIAssistant + ")."
		s.DisclosedByLabel = in.AILabel != ""
	}
	return s
}

// Body returns the Markdown body of a pull request in ship-it's own format,
// for repositories without a template: prose first, then short sections.
func Body(in BodyInput) string {
	if in.Omit.Why {
		commits := make([]Commit, len(in.Commits))
		for i, c := range in.Commits {
			commits[i] = Commit{Header: c.Header, Tested: c.Tested}
		}
		in.Commits = commits
	}
	s := Compose(in)
	if in.Omit.Issue {
		s.Issue = ""
	}
	if in.Omit.Testing {
		s.Testing = ""
	}
	var parts []string
	for _, p := range []string{s.Issue, s.Summary} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if s.Testing != "" {
		parts = append(parts, "### Testing\n\n"+s.Testing)
	}
	if s.Notes != "" {
		parts = append(parts, "### Notes\n\n"+s.Notes)
	}
	if s.Disclose != "" && !s.DisclosedByLabel {
		parts = append(parts, s.Disclose)
	}
	parts = append(parts, Marker)
	return strings.Join(parts, "\n\n") + "\n"
}

// Sentence writes a commit subject as a sentence start: first letter in
// upper case ("handle an empty cart" → "Handle an empty cart"), acronyms and
// identifiers kept as they are.
func Sentence(subject string) string {
	subject = strings.TrimSpace(subject)
	r, size := utf8.DecodeRuneInString(subject)
	if size == 0 || !unicode.IsLower(r) {
		return subject
	}
	// "iOS", "npm", "gRPC": a lower-case word with upper case inside, or a
	// code identifier, is a name.
	word, _, _ := strings.Cut(subject, " ")
	if strings.ContainsAny(word, "_./`()") || strings.ToLower(word) != word || lowerCaseNames[word] {
		return subject
	}
	return string(unicode.ToUpper(r)) + subject[size:]
}

// lowerCaseNames are tools always written in lower case, even at the start of
// a sentence.
var lowerCaseNames = map[string]bool{
	"npm": true, "pnpm": true, "npx": true, "yarn": true, "bun": true, "deno": true,
	"git": true, "gh": true, "eslint": true, "vitest": true, "jq": true, "curl": true,
}

// enumerate joins items as prose: "a", "a and b", "a, b and c".
func enumerate(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

// codeSpan wraps s in backticks, using a longer fence when s contains some.
func codeSpan(s string) string {
	fence := "`"
	for strings.Contains(s, fence) {
		fence += "`"
	}
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") {
		return fence + " " + s + " " + fence
	}
	return fence + s + fence
}

// Draft reports whether the pull request must open as a draft: when nobody
// reviewed the plan in a terminal (--yes, MCP, daemon: the review moves to
// the pull request), or when a commit is work in progress.
func Draft(unattended bool, commits []convention.Header) bool {
	if unattended {
		return true
	}
	for _, c := range commits {
		if c.Type == convention.WorkInProgress {
			return true
		}
	}
	return false
}

// Title returns the pull request title: the single commit's header, or for
// several commits the header of the most significant one (the first breaking
// change, else the first feature, else the oldest commit).
func Title(commits []convention.Header) string {
	return TitleAs(commits, convention.GitmojiFirst)
}

// TitleAs is Title with the repository's gitmoji placement.
func TitleAs(commits []convention.Header, placement convention.GitmojiPlacement) string {
	return TitleWith(convention.Rules{}, commits, placement)
}

// TitleWith is TitleAs with the repository's commit types: a commit written
// without a gitmoji gets the one of its type in that table.
func TitleWith(rules convention.Rules, commits []convention.Header, placement convention.GitmojiPlacement) string {
	if len(commits) == 0 {
		return ""
	}
	best := commits[0]
	for _, c := range commits {
		if c.Breaking {
			return rules.Format(c, placement)
		}
		if c.Type == convention.Feature && best.Type != convention.Feature {
			best = c
		}
	}
	return rules.Format(best, placement)
}
