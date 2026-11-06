// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package convention

import (
	"regexp"
	"slices"
	"strings"
)

// Built-in limits, counted in Unicode code points (decision NC1,
// docs/design/git-github-standards.md) except the branch description, in bytes
// of ASCII (NB5). ship.commits.* and ship.branches.* change them per
// repository.
const (
	// DefaultSubjectMaxLength bounds the subject, after "type(scope): ".
	DefaultSubjectMaxLength = 50
	// DefaultHeaderMaxLength bounds the whole first line, gitmoji included.
	DefaultHeaderMaxLength = 72
	// DefaultBodyWidth is the width commit bodies are wrapped at, the
	// widespread convention ("Wrap the body at 72 characters", rule 6 of
	// Chris Beams' "How to Write a Git Commit Message"): git indents bodies in
	// `git log`, and 72 leaves room for it in an 80-column terminal.
	DefaultBodyWidth = 72
	// DefaultBranchMaxWords bounds the number of words a branch name keeps
	// from the subject.
	DefaultBranchMaxWords = 4
	// DefaultBranchMaxLength bounds the description part of a branch name.
	DefaultBranchMaxLength = 32
)

// Rules are a repository's commit and branch conventions, as the settings
// ship.commits.* and ship.branches.* give them. The zero value is ship-it's
// built-in convention, so that code without settings needs none: an unset
// limit or table means the built-in one.
type Rules struct {
	SubjectMaxLength int
	HeaderMaxLength  int
	BodyWidth        int
	// Types is the commit type table, in display order; nil is the built-in
	// table (see WithTypes).
	Types []CommitTypeSpec
	// Scopes are the scopes a title may use; empty allows any.
	Scopes []string
	// ScopeRequired refuses a title without a scope.
	ScopeRequired bool
	// BranchMaxWords and BranchMaxLength bound the description of the branch
	// names BranchName builds.
	BranchMaxWords  int
	BranchMaxLength int
	// BranchWithoutIssue leaves the issue number and the ticket out of the
	// branch names BranchName builds (ship.branches.issue "none").
	BranchWithoutIssue bool
}

// SubjectMax returns the longest subject.
func (r Rules) SubjectMax() int { return orDefault(r.SubjectMaxLength, DefaultSubjectMaxLength) }

// HeaderMax returns the longest header, gitmoji included.
func (r Rules) HeaderMax() int { return orDefault(r.HeaderMaxLength, DefaultHeaderMaxLength) }

// Width returns the column bodies are wrapped at.
func (r Rules) Width() int { return orDefault(r.BodyWidth, DefaultBodyWidth) }

func (r Rules) branchMaxWords() int  { return orDefault(r.BranchMaxWords, DefaultBranchMaxWords) }
func (r Rules) branchMaxLength() int { return orDefault(r.BranchMaxLength, DefaultBranchMaxLength) }

func orDefault(n, fallback int) int {
	if n <= 0 {
		return fallback
	}
	return n
}

// table is the effective commit type table, never copied: callers must not
// change it.
func (r Rules) table() []CommitTypeSpec {
	if r.Types == nil {
		return commitTypeSpecs
	}
	return r.Types
}

// CommitTypes returns the commit types of the repository, in display order.
func (r Rules) CommitTypes() []CommitTypeSpec { return slices.Clone(r.table()) }

// LookupCommitType returns the specification of t in the repository's table.
// Matching is case-insensitive, as Conventional Commits types are.
func (r Rules) LookupCommitType(t CommitType) (CommitTypeSpec, bool) {
	needle := CommitType(strings.ToLower(string(t)))
	for _, spec := range r.table() {
		if spec.Type == needle {
			return spec, true
		}
	}
	return CommitTypeSpec{}, false
}

// ScopeAllowed reports whether a non-empty scope is one of Scopes, when the
// repository lists them.
func (r Rules) ScopeAllowed(scope string) bool {
	return len(r.Scopes) == 0 || slices.Contains(r.Scopes, scope)
}

// Closest returns the type of the repository's table that replaces t when the
// table lacks it: the first of CloseTypes it has, else the first type of the
// table. ok is false when t is in the table (nothing to replace).
func (r Rules) Closest(t CommitType) (closest CommitType, ok bool) {
	if _, known := r.LookupCommitType(t); known {
		return t, false
	}
	for _, c := range CloseTypes(t) {
		if _, known := r.LookupCommitType(c); known {
			return c, true
		}
	}
	if len(r.table()) == 0 {
		return t, false // settings always leave a type: defensive only
	}
	return r.table()[0].Type, true
}

// closeTypes lists, for each built-in type, the types that mean the same when
// a project does not use it, most specific first.
var closeTypes = map[CommitType][]CommitType{
	SecurityFix:           {BugFix},
	WorkInProgress:        {Maintenance},
	Performance:           {Refactoring, BugFix},
	BuildSystem:           {Maintenance, ContinuousIntegration},
	ContinuousIntegration: {BuildSystem, Maintenance},
	Styling:               {Maintenance},
	Testing:               {Maintenance},
	Documentation:         {Maintenance},
	Refactoring:           {Maintenance},
}

// CloseTypes returns the built-in types that mean about the same as t, most
// specific first; nil when none does.
func CloseTypes(t CommitType) []CommitType { return slices.Clone(closeTypes[t]) }

// TypeChange changes one commit type of the built-in table (the setting
// ship.commits.types): Remove drops it; otherwise each non-nil field replaces
// the type's own, and a type the table lacks is added.
type TypeChange struct {
	Type    CommitType
	Remove  bool
	Gitmoji *string
	// BranchPrefix is the Conventional Branch prefix of its branches.
	BranchPrefix *string
	// Label is the kind of its pull requests (kind/<label>); "" gives none.
	Label *string
	// Release is the increment the type triggers; a breaking change is
	// always a major one.
	Release *ReleaseImpact
}

// WithTypes returns the built-in table with changes applied in order. An
// added type comes after the built-in ones; unless its Release says
// otherwise it changes no version (NoRelease), and it cannot be breaking, as
// most built-in types. The caller
// validates the changes (ValidTypeName, ValidBranchPrefix, ValidLabel, a
// gitmoji and a branch prefix for an added type, a type left).
func WithTypes(changes []TypeChange) []CommitTypeSpec {
	table := CommitTypes()
	for _, c := range changes {
		t := CommitType(strings.ToLower(string(c.Type)))
		i := slices.IndexFunc(table, func(s CommitTypeSpec) bool { return s.Type == t })
		if c.Remove {
			if i >= 0 {
				table = slices.Delete(table, i, i+1)
			}
			continue
		}
		if i < 0 {
			table = append(table, CommitTypeSpec{Type: t, ReleaseImpact: NoRelease})
			i = len(table) - 1
		}
		if c.Gitmoji != nil {
			table[i].Gitmoji = *c.Gitmoji
		}
		if c.BranchPrefix != nil {
			table[i].BranchPrefix = BranchPrefix(*c.BranchPrefix)
		}
		if c.Label != nil {
			table[i].Label = *c.Label
		}
		if c.Release != nil {
			table[i].ReleaseImpact = *c.Release
		}
	}
	return table
}

// typeNamePattern is a type the header grammar reads, as it is written.
var typeNamePattern = regexp.MustCompile(`^[a-z]+$`)

// ValidTypeName reports whether name can be a commit type: lowercase letters
// only, as the header grammar reads them.
func ValidTypeName(name string) bool { return typeNamePattern.MatchString(name) }

// ValidBranchPrefix reports whether prefix follows the Conventional Branch
// character rules: lowercase letters and digits, separated by single hyphens
// or dots.
func ValidBranchPrefix(prefix string) bool { return branchDescriptionPattern.MatchString(prefix) }

// ValidLabel reports whether label can follow "kind/": empty (no label) or
// the Conventional Branch characters, as the built-in kinds.
func ValidLabel(label string) bool { return label == "" || branchDescriptionPattern.MatchString(label) }

// IsBuiltInType reports whether t is a type of ship-it's own table.
func IsBuiltInType(t CommitType) bool {
	_, ok := Rules{}.LookupCommitType(t)
	return ok
}
