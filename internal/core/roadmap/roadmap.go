// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package roadmap reads ROADMAP.md, the source of truth of a project's plan
// (ADR-0016): every `## ` heading that holds a checklist is a milestone, its
// top-level items are issues, indented items are sub-issues. It never talks
// to GitHub: app compares the parsed roadmap with the repository (Plan) and
// writes the issue numbers back (Annotate). Decisions of 2026-11-01:
//
//	## Phase 1 — Foundations
//	**Due:** 2026-12-15                ← milestone due date (ISO 8601)
//	- [ ] feat: CLI skeleton #12       ← issue; "feat:" gives kind/feature
//	  Text indented under an item      ← the issue's description
//	  - [ ] git exec layer #13         ← sub-issue
//	- [x] ADRs #14                     ← a closed issue
//	**Exit:** a junior ships a PR.     ← milestone description
//
// Allowed imports: standard library without I/O, other core packages.
package roadmap

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/GregoireF/ship-it/internal/core/convention"
)

// MaxTitleLength is GitHub's limit for an issue title, in characters.
const MaxTitleLength = 256

// Roadmap is a parsed ROADMAP.md.
type Roadmap struct {
	Phases []Phase
}

// Phase is a milestone.
type Phase struct {
	Title string
	// Line is the 1-based line of the heading.
	Line int
	// Due is the "**Due:**" date (YYYY-MM-DD), "" when absent.
	Due string
	// Exit is the "**Exit:**" text: the milestone's description.
	Exit  string
	Items []Item
}

// Item is an issue, and its children are sub-issues.
type Item struct {
	// Text is the item as written, without its checkbox nor its "#n".
	Text string
	// Kind is the commit type of a "feat: …" prefix, "" without one; the
	// prefix is not part of Title.
	Kind convention.CommitType
	Done bool
	// Number is the linked issue ("#12"), 0 before the first sync.
	Number int
	// Line is the 1-based line of the item.
	Line int
	// Description holds the text lines indented under the item.
	Description []string
	Children    []Item
}

// Title is the issue's title: the item's text without its type prefix,
// shortened at a word boundary when GitHub's limit would cut it (the full
// text then opens the description, see Body).
func (it Item) Title() string {
	text := it.textWithoutKind()
	if utf8.RuneCountInString(text) <= MaxTitleLength {
		return text
	}
	runes := []rune(text)[:MaxTitleLength-1]
	cut := string(runes)
	if i := strings.LastIndex(cut, " "); i > MaxTitleLength/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;:") + "…"
}

func (it Item) textWithoutKind() string {
	if it.Kind == "" {
		return it.Text
	}
	_, rest, _ := strings.Cut(it.Text, ":")
	return strings.TrimSpace(rest)
}

var (
	itemLine   = regexp.MustCompile(`^(\s*)[-*] \[( |x|X)\] (.*)$`)
	numberTail = regexp.MustCompile(`\s+#([1-9][0-9]*)\s*$`)
	kindPrefix = regexp.MustCompile(`^([a-z]+): \S`)
	dueLine    = regexp.MustCompile(`^\*\*Due:\*\*\s*(\d{4}-\d{2}-\d{2})\s*$`)
	exitLine   = regexp.MustCompile(`^\*\*Exit:\*\*\s*(.+)$`)
)

// Parse reads a roadmap with the built-in commit types (ParseWith).
func Parse(markdown []byte) Roadmap { return ParseWith(markdown, convention.Rules{}) }

// ParseWith reads a roadmap. Headings without a checklist (an introduction, a
// legend) are not milestones. Indentation is measured in spaces (a tab
// counts as four); any deeper item is a child of the item above. A "kind:"
// prefix is one of the repository's commit types (rules, ship.commits.types).
func ParseWith(markdown []byte, rules convention.Rules) Roadmap {
	var r Roadmap
	var phase *Phase
	inFence := false
	lines := strings.Split(strings.ReplaceAll(string(markdown), "\r\n", "\n"), "\n")
	for i, raw := range lines {
		lineNo := i + 1
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if title, ok := strings.CutPrefix(raw, "## "); ok {
			r.Phases = append(r.Phases, Phase{Title: strings.TrimSpace(title), Line: lineNo})
			phase = &r.Phases[len(r.Phases)-1]
			continue
		}
		if strings.HasPrefix(raw, "# ") || phase == nil {
			continue
		}
		if m := dueLine.FindStringSubmatch(trimmed); m != nil {
			phase.Due = m[1]
			continue
		}
		if m := exitLine.FindStringSubmatch(trimmed); m != nil {
			phase.Exit = strings.TrimSpace(m[1])
			continue
		}
		if m := itemLine.FindStringSubmatch(raw); m != nil {
			item := parseItem(m[3], m[2] != " ", lineNo, rules)
			if indent(m[1]) == 0 || len(phase.Items) == 0 {
				phase.Items = append(phase.Items, item)
			} else {
				parent := &phase.Items[len(phase.Items)-1]
				parent.Children = append(parent.Children, item)
			}
			continue
		}
		// Indented prose under an item is its description (of the deepest
		// item above it).
		if trimmed != "" && indent(leading(raw)) > 0 && len(phase.Items) > 0 {
			parent := &phase.Items[len(phase.Items)-1]
			if n := len(parent.Children); n > 0 && indent(leading(raw)) > 2 {
				parent.Children[n-1].Description = append(parent.Children[n-1].Description, trimmed)
			} else {
				parent.Description = append(parent.Description, trimmed)
			}
		}
	}
	// A heading without items is not a milestone.
	kept := r.Phases[:0]
	for _, p := range r.Phases {
		if len(p.Items) > 0 {
			kept = append(kept, p)
		}
	}
	r.Phases = kept
	return r
}

func parseItem(text string, done bool, line int, rules convention.Rules) Item {
	it := Item{Done: done, Line: line}
	if m := numberTail.FindStringSubmatchIndex(text); m != nil {
		it.Number, _ = strconv.Atoi(text[m[2]:m[3]])
		text = text[:m[0]]
	}
	it.Text = strings.TrimSpace(text)
	if m := kindPrefix.FindStringSubmatch(it.Text); m != nil {
		if _, known := rules.LookupCommitType(convention.CommitType(m[1])); known {
			it.Kind = convention.CommitType(m[1])
		}
	}
	return it
}

func leading(s string) string { return s[:len(s)-len(strings.TrimLeft(s, " \t"))] }

func indent(ws string) int {
	return strings.Count(ws, " ") + 4*strings.Count(ws, "\t")
}
