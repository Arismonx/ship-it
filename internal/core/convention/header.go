// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package convention

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/GregoireF/ship-it/internal/core/fault"
)

// GitmojiPlacement controls where the gitmoji is written (ADR-0007).
type GitmojiPlacement int

// Supported gitmoji placements.
const (
	GitmojiFirst     GitmojiPlacement = iota // "✨ feat(ui): subject" (default)
	GitmojiAfterType                         // "feat(ui): ✨ subject" (strict Conventional Commits)
	GitmojiOmitted                           // "feat(ui): subject"
)

// Header is the first line of a commit message, also used as pull request title.
type Header struct {
	Gitmoji  string // as written; empty when absent
	Type     CommitType
	Scope    string
	Breaking bool
	Subject  string
}

// headerPattern mirrors @gregoiref/commitlint-config: an optional leading
// token (the gitmoji) followed by a space, then type(scope)!: subject.
var headerPattern = regexp.MustCompile(`^(?:(\S+) )?([A-Za-z]+)(?:\(([^()\r\n]*)\))?(!)?: (.*)$`)

// scopePattern restricts scopes to what renders cleanly in titles and branches.
var scopePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]*$`)

// ParseHeader parses a header with the built-in conventions (Rules.ParseHeader).
func ParseHeader(s string) (Header, error) { return Rules{}.ParseHeader(s) }

// ParseHeader parses a header written with any gitmoji placement. It checks
// the grammar and that the type is in the repository's table only; use
// Validate for the style rules.
func (r Rules) ParseHeader(s string) (Header, error) {
	line := strings.TrimRight(firstLine(s), " \t\r")
	if strings.TrimSpace(line) == "" {
		return Header{}, fault.New(fault.EmptyHeader, "convention.header.empty")
	}
	m := headerPattern.FindStringSubmatch(line)
	if m == nil {
		return Header{}, fault.New(fault.MalformedHeader, "convention.header.malformed")
	}
	h := Header{
		Gitmoji:  m[1],
		Type:     CommitType(strings.ToLower(m[2])),
		Scope:    m[3],
		Breaking: m[4] == "!",
		Subject:  m[5],
	}
	if h.Gitmoji == "" {
		if gitmoji, rest, ok := splitLeadingGitmoji(h.Subject); ok {
			h.Gitmoji, h.Subject = gitmoji, rest
		}
	}
	if _, ok := r.LookupCommitType(h.Type); !ok {
		return Header{}, fault.New(fault.UnknownCommitType, "convention.header.unknown_type", "type", string(h.Type))
	}
	return h, nil
}

// Validate checks h against the built-in conventions (Rules.Validate).
func Validate(h Header) []*fault.Fault { return Rules{}.Validate(h) }

// Validate returns every rule h breaks, in a stable order; nil means valid.
func (r Rules) Validate(h Header) []*fault.Fault {
	var faults []*fault.Fault
	spec, known := r.LookupCommitType(h.Type)
	if !known {
		faults = append(faults, fault.New(fault.UnknownCommitType, "convention.header.unknown_type", "type", string(h.Type)))
	}
	if known && h.Breaking && !spec.AllowsBreakingChange {
		faults = append(faults, fault.New(fault.BreakingNotAllowed, "convention.header.breaking_not_allowed", "type", string(h.Type)))
	}
	if h.Scope != "" && !scopePattern.MatchString(h.Scope) {
		faults = append(faults, fault.New(fault.InvalidScope, "convention.header.invalid_scope", "scope", h.Scope))
	}
	switch {
	case h.Scope == "" && r.ScopeRequired:
		faults = append(faults, fault.New(fault.ScopeRequired, "convention.header.scope_required"))
	case h.Scope != "" && !r.ScopeAllowed(h.Scope):
		faults = append(faults, fault.New(fault.ScopeNotAllowed, "convention.header.scope_not_allowed",
			"scope", h.Scope, "allowed", strings.Join(r.Scopes, ", ")))
	}
	subject := strings.TrimSpace(h.Subject)
	if subject == "" {
		faults = append(faults, fault.New(fault.EmptySubject, "convention.header.empty_subject"))
	} else {
		if n := utf8.RuneCountInString(subjectWithoutTicket(subject)); n > r.SubjectMax() {
			faults = append(faults, fault.New(fault.SubjectTooLong, "convention.header.subject_too_long",
				"length", strconv.Itoa(n), "max", strconv.Itoa(r.SubjectMax())))
		}
		if first, _ := utf8.DecodeRuneInString(subject); unicode.IsUpper(first) && !startsWithAcronym(subject) {
			faults = append(faults, fault.New(fault.SubjectNotLowercase, "convention.header.subject_case"))
		}
		if strings.HasSuffix(subject, ".") {
			faults = append(faults, fault.New(fault.SubjectEndsWithPeriod, "convention.header.subject_period"))
		}
	}
	if n := utf8.RuneCountInString(r.Format(h, GitmojiFirst)); n > r.HeaderMax() {
		faults = append(faults, fault.New(fault.HeaderTooLong, "convention.header.header_too_long",
			"length", strconv.Itoa(n), "max", strconv.Itoa(r.HeaderMax())))
	}
	return faults
}

// Format renders h with the given gitmoji placement. A header without a
// gitmoji gets the canonical one of its type in the built-in table (💥 for
// breaking changes); Rules.Format takes it from a repository's table.
func Format(h Header, placement GitmojiPlacement) string {
	gitmoji := h.Gitmoji
	if gitmoji == "" {
		gitmoji = CanonicalGitmoji(h.Type, h.Breaking)
	}
	var b strings.Builder
	if placement == GitmojiFirst && gitmoji != "" {
		b.WriteString(gitmoji)
		b.WriteByte(' ')
	}
	b.WriteString(string(h.Type))
	if h.Scope != "" {
		b.WriteByte('(')
		b.WriteString(h.Scope)
		b.WriteByte(')')
	}
	if h.Breaking {
		b.WriteByte('!')
	}
	b.WriteString(": ")
	if placement == GitmojiAfterType && gitmoji != "" {
		b.WriteString(gitmoji)
		b.WriteByte(' ')
	}
	b.WriteString(strings.TrimSpace(h.Subject))
	return b.String()
}

// Format renders h as Format does, a missing gitmoji taken from the
// repository's table.
func (r Rules) Format(h Header, placement GitmojiPlacement) string {
	if h.Gitmoji == "" {
		h.Gitmoji = r.CanonicalGitmoji(h.Type, h.Breaking)
	}
	return Format(h, placement)
}

// FitToMaxLength fits h to the built-in conventions (Rules.FitToMaxLength).
func FitToMaxLength(h Header) Header { return Rules{}.FitToMaxLength(h) }

// FitToMaxLength drops the scope when the header exceeds the longest header
// (decision NC1), unless the repository requires one. It never truncates the
// subject: an overlong subject is reported by Validate so that a human or the
// planner rewrites it.
func (r Rules) FitToMaxLength(h Header) Header {
	if !r.ScopeRequired && utf8.RuneCountInString(r.Format(h, GitmojiFirst)) > r.HeaderMax() {
		h.Scope = ""
	}
	return h
}

// FrequentScopes returns the scopes used by at least minUses of the commit
// subjects, most used first (ties by name). It feeds `ship init`'s area/
// labels. Subjects that are not conventional headers are ignored. It reads
// types with the built-in table (Rules.FrequentScopes).
func FrequentScopes(subjects []string, minUses int) []string {
	return Rules{}.FrequentScopes(subjects, minUses)
}

// FrequentScopes is FrequentScopes with the repository's commit types.
func (r Rules) FrequentScopes(subjects []string, minUses int) []string {
	uses := map[string]int{}
	for _, s := range subjects {
		if h, err := r.ParseHeader(s); err == nil && h.Scope != "" {
			uses[h.Scope]++
		}
	}
	var scopes []string
	for scope, n := range uses {
		if n >= minUses {
			scopes = append(scopes, scope)
		}
	}
	slices.SortFunc(scopes, func(a, b string) int {
		if uses[a] != uses[b] {
			return uses[b] - uses[a]
		}
		return strings.Compare(a, b)
	})
	return scopes
}

// CanonicalGitmoji returns the gitmoji of t in the built-in table, or 💥 for
// a breaking change.
func CanonicalGitmoji(t CommitType, breaking bool) string {
	return Rules{}.CanonicalGitmoji(t, breaking)
}

// CanonicalGitmoji returns the gitmoji of t in the repository's table, or 💥
// for a breaking change.
func (r Rules) CanonicalGitmoji(t CommitType, breaking bool) string {
	if breaking {
		return GitmojiBreakingChange
	}
	if spec, ok := r.LookupCommitType(t); ok {
		return spec.Gitmoji
	}
	return ""
}

// splitLeadingGitmoji detects a leading non-ASCII, non-letter token such as
// "✨ " at the start of a subject ("feat(ui): ✨ subject").
func splitLeadingGitmoji(s string) (gitmoji, rest string, ok bool) {
	i := strings.IndexByte(s, ' ')
	if i <= 0 {
		return "", s, false
	}
	token := s[:i]
	for _, r := range token {
		if r < utf8.RuneSelf || unicode.IsLetter(r) || unicode.IsDigit(r) {
			return "", s, false
		}
	}
	return token, s[i+1:], true
}

func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}

// startsWithAcronym accepts subjects starting with an all-caps word such as
// "HEAD" or "API", which git's SubmittingPatches allows to stay capitalised.
func startsWithAcronym(s string) bool {
	word := s
	if i := strings.IndexAny(s, " -_/"); i > 0 {
		word = s[:i]
	}
	if utf8.RuneCountInString(word) < 2 {
		return false
	}
	for _, r := range word {
		if unicode.IsLetter(r) && !unicode.IsUpper(r) {
			return false
		}
	}
	return true
}

// WrittenType returns the type a header line is written with, in lower case,
// whatever the table: "" when the line is not a header.
func WrittenType(line string) CommitType {
	m := headerPattern.FindStringSubmatch(firstLine(line))
	if m == nil {
		return ""
	}
	return CommitType(strings.ToLower(m[2]))
}

// WithType rewrites the type of a header line with the built-in table
// (Rules.WithType).
func WithType(line string, t CommitType) (string, bool) { return Rules{}.WithType(line, t) }

// WithType rewrites the type of a header line, keeping every other byte as
// written (gitmoji, scope, breaking mark, subject). The type's position comes
// from the header grammar itself, so no other occurrence of the word, nor a
// case change of the line, can move it. ok is false when line is not a
// header or t is not in the repository's table.
func (r Rules) WithType(line string, t CommitType) (string, bool) {
	if _, known := r.LookupCommitType(t); !known {
		return line, false
	}
	line = firstLine(line)
	m := headerPattern.FindStringSubmatchIndex(line)
	if m == nil {
		return line, false
	}
	return line[:m[4]] + string(t) + line[m[5]:], true // group 2: the type
}
