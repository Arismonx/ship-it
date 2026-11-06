// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package convention

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/fault"
)

// Generated branch names are short by design. Conventional Branch 1.1.0 only
// asks for "short and descriptive" names; the bounds (DefaultBranchMaxWords,
// DefaultBranchMaxLength) are ship-it choices (docs/design/git-github-standards.md,
// NB5): a branch such as "feat/123-" plus the description stays around 40
// characters, readable in terminals and PR lists.

// fallbackBranchDescription is used when a subject has no usable word.
const fallbackBranchDescription = "work"

// AgentPrefix is the Conventional Branch 1.1.0 prefix that marks work shipped
// by an AI agent; it replaces the purpose prefix.
type AgentPrefix string

// Agent prefixes defined by Conventional Branch 1.1.0.
const (
	NoAgent      AgentPrefix = ""
	AnyAgent     AgentPrefix = "ai"
	ClaudeAgent  AgentPrefix = "claude"
	CodexAgent   AgentPrefix = "codex"
	CopilotAgent AgentPrefix = "copilot"
	CursorAgent  AgentPrefix = "cursor"
)

// BranchOptions refine how BranchName builds a name.
type BranchOptions struct {
	// Issue is the linked issue number, written first; zero means none.
	Issue int
	// Agent, when set, replaces the purpose prefix.
	Agent AgentPrefix
	// Hotfix marks an urgent production fix; it applies to bug and security
	// fixes only and selects the "hotfix/" prefix.
	Hotfix bool
	// Ticket is an external tracker's ticket ("ACME-123"), written first in
	// upper case — trackers only recognise it so — instead of Issue.
	Ticket string
}

// acceptedPrefixes lists every prefix ValidateBranch accepts: the purpose
// prefixes, their long aliases from the specification and the agent prefixes.
var acceptedPrefixes = map[string]bool{
	string(BranchFeature): true, "feature": true,
	string(BranchBugFix): true, "bugfix": true,
	string(BranchHotfix): true, string(BranchRelease): true, string(BranchMaintenance): true,
	string(AnyAgent): true, string(ClaudeAgent): true, string(CodexAgent): true,
	string(CopilotAgent): true, string(CursorAgent): true,
}

// trunkBranches carry no prefix (Conventional Branch 1.1.0).
var trunkBranches = map[string]bool{"main": true, "master": true, "develop": true}

// branchDescriptionPattern implements the Conventional Branch character rules:
// lowercase letters and digits, separated by single hyphens or dots, never
// leading, trailing or adjacent to each other.
var branchDescriptionPattern = regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)*$`)

// BranchName builds a branch name with the built-in conventions
// (Rules.BranchName).
func BranchName(t CommitType, subject string, opts BranchOptions) string {
	return Rules{}.BranchName(t, subject, opts)
}

// BranchName builds "<prefix>/<issue>-<description>" for a change described
// by its commit type and subject; the issue part is left out when the
// repository asks so. The result always satisfies ValidateBranch.
func (r Rules) BranchName(t CommitType, subject string, opts BranchOptions) string {
	issuePart := ""
	switch {
	case r.BranchWithoutIssue:
	case opts.Ticket != "":
		issuePart = strings.ToUpper(opts.Ticket) + "-"
	case opts.Issue > 0:
		issuePart = strconv.Itoa(opts.Issue) + "-"
	}
	prefix := r.branchPrefixFor(t, opts)
	description := r.BranchDescription(withoutLeadingWord(subject, string(prefix)))
	if description == "" {
		description = fallbackBranchDescription
	}
	return string(prefix) + "/" + issuePart + description
}

// withoutLeadingWord drops a first word that repeats the branch prefix:
// "fix crash on startup" on a "fix/" branch becomes "crash on startup".
func withoutLeadingWord(subject, word string) string {
	words := branchWords(subject)
	if len(words) > 1 && words[0] == word && hasContentWord(words[1:]) {
		trimmed := strings.TrimSpace(subject)
		if i := strings.IndexFunc(trimmed, func(r rune) bool { return r == ' ' || r == '\t' }); i > 0 {
			return trimmed[i+1:]
		}
	}
	return subject
}

// branchPrefixFor picks the agent prefix, then hotfix, then the type's prefix.
func (r Rules) branchPrefixFor(t CommitType, opts BranchOptions) BranchPrefix {
	if opts.Agent != NoAgent {
		return BranchPrefix(opts.Agent)
	}
	spec, ok := r.LookupCommitType(t)
	if !ok {
		return BranchMaintenance
	}
	if opts.Hotfix && spec.BranchPrefix == BranchBugFix {
		return BranchHotfix
	}
	return spec.BranchPrefix
}

// BranchDescription shortens a subject with the built-in bounds
// (Rules.BranchDescription).
func BranchDescription(subject string) string { return Rules{}.BranchDescription(subject) }

// BranchDescription turns a subject into a short Conventional Branch
// description: ASCII-folded, lowercase, without stop words, within the
// repository's bounds in words and bytes.
func (r Rules) BranchDescription(subject string) string {
	maxWords, maxLength := r.branchMaxWords(), r.branchMaxLength()
	var kept []string
	length := 0
	for _, word := range branchWords(subject) {
		if stopWords[word] {
			continue
		}
		extra := len(word)
		if len(kept) > 0 {
			extra++ // separator
		}
		if len(kept) == maxWords || length+extra > maxLength {
			break
		}
		kept = append(kept, word)
		length += extra
	}
	if len(kept) == 0 {
		// A subject made only of stop words ("fix it") keeps its first word.
		if words := branchWords(subject); len(words) > 0 && len(words[0]) <= maxLength {
			return words[0]
		}
	}
	return strings.Join(kept, "-")
}

// UniqueBranchName returns name, or name suffixed with "-2", "-3"… when taken
// reports it already exists locally or on the remote.
func UniqueBranchName(name string, taken func(string) bool) string {
	candidate := name
	for n := 2; taken(candidate); n++ {
		candidate = name + "-" + strconv.Itoa(n)
	}
	return candidate
}

// IssueFromBranch returns the issue number a branch name starts with, as
// BranchName writes it ("feat/123-add-login" → 123), or 0.
func IssueFromBranch(name string) int {
	_, description, ok := strings.Cut(name, "/")
	if !ok {
		return 0
	}
	digits, _, _ := strings.Cut(description, "-")
	issue, err := strconv.Atoi(digits)
	if err != nil || issue <= 0 || digits[0] == '0' {
		return 0
	}
	return issue
}

// ValidateBranch checks a branch name against Conventional Branch 1.1.0 and
// the git-check-ref-format rules the specification does not already imply.
func ValidateBranch(name string) error {
	return ValidateBranchWith(name, nil)
}

// ValidateBranchWith is ValidateBranch for a repository whose tracker keys
// are known: a description may then start with an upper-case ticket of one of
// them ("feat/ACME-123-add-login"), the one exception to the lower-case rule
// (decision of 2026-10-24: Jira only recognises upper-case keys).
func ValidateBranchWith(name string, keys []string) error {
	return Rules{}.ValidateBranch(name, keys)
}

// ValidateBranch is ValidateBranchWith for the repository's conventions: the
// branch prefixes of its commit types are accepted too.
func (r Rules) ValidateBranch(name string, keys []string) error {
	if prefix, description, ok := strings.Cut(name, "/"); ok {
		if ticket := FindTicket(description, keys); ticket != "" && strings.HasPrefix(description, ticket+"-") {
			name = prefix + "/" + strings.ToLower(description[:len(ticket)]) + description[len(ticket):]
		}
	}
	if trunkBranches[name] {
		return nil
	}
	prefix, description, ok := strings.Cut(name, "/")
	if !ok || description == "" {
		return fault.New(fault.InvalidBranch, "convention.branch.missing_prefix", "branch", name)
	}
	if !acceptedPrefixes[prefix] && !slices.ContainsFunc(r.table(), func(s CommitTypeSpec) bool { return string(s.BranchPrefix) == prefix }) {
		return fault.New(fault.UnknownBranchPrefix, "convention.branch.unknown_prefix", "prefix", prefix)
	}
	if !branchDescriptionPattern.MatchString(description) {
		return fault.New(fault.InvalidBranch, "convention.branch.invalid_description", "branch", name)
	}
	if strings.HasSuffix(description, ".lock") {
		// git-check-ref-format: no component may end with ".lock".
		return fault.New(fault.InvalidBranch, "convention.branch.lock_suffix", "branch", name)
	}
	return nil
}

// hasContentWord reports whether words contains anything but stop words.
func hasContentWord(words []string) bool {
	for _, w := range words {
		if !stopWords[w] {
			return true
		}
	}
	return false
}

// branchWords splits text into lowercase ASCII words of letters and digits.
func branchWords(text string) []string {
	var words []string
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			words = append(words, current.String())
			current.Reset()
		}
	}
	for _, r := range strings.ToLower(text) {
		r = foldToASCII(r)
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			current.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return words
}

// stopWords are dropped from branch descriptions to keep them short. The list
// covers English and French function words only; verbs and nouns are kept.
var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "of": true, "for": true, "to": true, "in": true,
	"on": true, "at": true, "by": true, "with": true, "and": true, "or": true, "from": true,
	"into": true, "is": true, "it": true, "its": true, "this": true, "that": true, "be": true,
	"le": true, "la": true, "les": true, "l": true, "un": true, "une": true, "des": true,
	"de": true, "du": true, "d": true, "et": true, "ou": true, "pour": true, "dans": true,
	"sur": true, "avec": true, "au": true, "aux": true, "en": true, "par": true,
}

// foldToASCII maps common Latin letters with diacritics to ASCII, so that a
// French subject such as "Corriger l'été" yields "corriger-ete" without
// pulling golang.org/x/text into core.
func foldToASCII(r rune) rune {
	if folded, ok := asciiFolding[r]; ok {
		return folded
	}
	return r
}

var asciiFolding = map[rune]rune{
	'à': 'a', 'á': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a', 'å': 'a', 'æ': 'a',
	'ç': 'c', 'è': 'e', 'é': 'e', 'ê': 'e', 'ë': 'e',
	'ì': 'i', 'í': 'i', 'î': 'i', 'ï': 'i', 'ñ': 'n',
	'ò': 'o', 'ó': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o', 'ø': 'o', 'œ': 'o',
	'ù': 'u', 'ú': 'u', 'û': 'u', 'ü': 'u', 'ý': 'y', 'ÿ': 'y', 'ß': 's',
}
