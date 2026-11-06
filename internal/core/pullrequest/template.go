// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package pullrequest

import (
	"regexp"
	"strings"
)

// A repository's pull request template is its convention: ship-it fills the
// sections it has facts for and leaves everything else as the maintainers
// wrote it — guidance comments, checklists, release-note blocks (decision of
// 2026-10-23: what a project imposes wins over ship-it's format).

// slot is a kind of section ship-it can fill.
type slot int

const (
	noSlot slot = iota
	issueSlot
	summarySlot
	testingSlot
	notesSlot
	discloseSlot
)

// headingLine matches an ATX heading ("## What this PR does").
var headingLine = regexp.MustCompile(`^ {0,3}#{1,6}\s+(.*?)\s*#*\s*$`)

// slotWords classify a heading by the words it contains, most specific
// first: "Which issue(s) this PR fixes" is about the issue, not a fix.
var slotWords = []struct {
	slot  slot
	words []string
}{
	// Sections only the author can answer.
	{noSlot, []string{"type of", "kind of", "checklist", "screenshot", "user-facing", "release note"}},
	{discloseSlot, []string{"ai usage", "ai-assisted", "ai assisted", "use of ai", "ai disclosure", "generative ai", "llm"}},
	{issueSlot, []string{"issue", "ticket", "related", "closes", "fixes", "resolves", "linked"}},
	{testingSlot, []string{"test", "verif", "validat", "qa"}},
	{notesSlot, []string{"breaking", "migration", "upgrade"}},
	{summarySlot, []string{"summary", "description", "what", "why", "motivation", "context", "changes", "overview", "purpose", "about"}},
}

func classify(heading string) slot {
	h := strings.ToLower(heading)
	for _, sw := range slotWords {
		for _, w := range sw.words {
			if strings.Contains(h, w) {
				return sw.slot
			}
		}
	}
	return noSlot
}

// FillTemplate writes the sections into a repository's template. A section
// is filled only when its body holds nothing but guidance comments and blank
// lines; what ship-it has no section for in the template is placed first
// (issue, summary) or dropped (testing, notes), never invented.
func FillTemplate(template string, s Sections) string {
	template = strings.ReplaceAll(template, "\r\n", "\n")
	lines := strings.Split(template, "\n")
	content := map[slot]string{issueSlot: s.Issue, summarySlot: s.Summary, testingSlot: s.Testing, notesSlot: s.Notes, discloseSlot: s.Disclose}
	used := map[slot]bool{}

	var out []string
	inComment := false
	for i := 0; i < len(lines); i++ {
		out = append(out, lines[i])
		m := headingLine.FindStringSubmatch(lines[i])
		wasInComment := inComment
		inComment = commentStateAfter(lines[i], inComment)
		if m == nil || wasInComment {
			continue // a commented-out heading is guidance too
		}
		kind := classify(m[1])
		text := content[kind]
		if kind == noSlot || used[kind] || text == "" {
			continue
		}
		end := sectionEnd(lines, i+1)
		if !onlyGuidance(lines[i+1 : end]) {
			continue // the template already says something here
		}
		// Keep the guidance comments (invisible), then the facts.
		body := strings.TrimRight(strings.Join(lines[i+1:end], "\n"), "\n ")
		if strings.TrimSpace(body) != "" {
			out = append(out, body)
		}
		out = append(out, "", text, "")
		used[kind] = true
		i, inComment = end-1, false // sectionEnd stops outside comments
	}

	var lead []string
	for _, k := range []slot{issueSlot, summarySlot} {
		if !used[k] && content[k] != "" {
			lead = append(lead, content[k])
		}
	}
	body := strings.TrimSpace(strings.Join(out, "\n"))
	if len(lead) > 0 {
		body = strings.Join(lead, "\n\n") + "\n\n" + body
	}
	if !used[discloseSlot] && s.Disclose != "" && !s.DisclosedByLabel {
		body += "\n\n" + s.Disclose
	}
	return body + "\n\n" + Marker + "\n"
}

// sectionEnd returns the index of the next heading at or after from.
func sectionEnd(lines []string, from int) int {
	inComment := false
	for i := from; i < len(lines); i++ {
		if !inComment && headingLine.MatchString(lines[i]) {
			return i
		}
		inComment = commentStateAfter(lines[i], inComment)
	}
	return len(lines)
}

// onlyGuidance reports whether lines hold only HTML comments and blank lines.
func onlyGuidance(lines []string) bool {
	text := strings.Join(lines, "\n")
	for {
		start := strings.Index(text, "<!--")
		if start < 0 {
			break
		}
		end := strings.Index(text[start:], "-->")
		if end < 0 {
			text = text[:start]
			break
		}
		text = text[:start] + text[start+end+3:]
	}
	return strings.TrimSpace(text) == ""
}

// commentStateAfter tracks whether an HTML comment is still open after line.
func commentStateAfter(line string, open bool) bool {
	for {
		if open {
			end := strings.Index(line, "-->")
			if end < 0 {
				return true
			}
			line, open = line[end+3:], false
			continue
		}
		start := strings.Index(line, "<!--")
		if start < 0 {
			return false
		}
		line, open = line[start+4:], true
	}
}
