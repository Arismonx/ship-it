// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package convention

import "strings"

// CommitType is a Conventional Commits type. Its value is the token written
// in commit headers ("feat", "fix"…); constants are named after their meaning.
type CommitType string

// Commit types supported by ship-it, aligned with @gregoiref/commitlint-config.
const (
	Feature               CommitType = "feat"
	BugFix                CommitType = "fix"
	Documentation         CommitType = "docs"
	Styling               CommitType = "style"
	Refactoring           CommitType = "refactor"
	Performance           CommitType = "perf"
	Testing               CommitType = "test"
	BuildSystem           CommitType = "build"
	ContinuousIntegration CommitType = "ci"
	Maintenance           CommitType = "chore"
	SecurityFix           CommitType = "security"
	Reversion             CommitType = "revert"
	WorkInProgress        CommitType = "wip"
)

// ReleaseImpact is the semantic-versioning increment a change triggers.
type ReleaseImpact int

// Release impacts in increasing order.
const (
	NoRelease ReleaseImpact = iota
	PatchRelease
	MinorRelease
	MajorRelease
)

// BranchPrefix is a Conventional Branch 1.1.0 prefix, written before the
// first "/" of a branch name.
type BranchPrefix string

// Purpose prefixes defined by Conventional Branch 1.1.0. The specification
// only knows these five; every other commit type is maintenance from a
// branching point of view (decision NB1, docs/design/git-github-standards.md).
const (
	BranchFeature     BranchPrefix = "feat"
	BranchBugFix      BranchPrefix = "fix"
	BranchHotfix      BranchPrefix = "hotfix"
	BranchRelease     BranchPrefix = "release"
	BranchMaintenance BranchPrefix = "chore"
)

// CommitTypeSpec describes how a commit type is rendered, branched and released.
type CommitTypeSpec struct {
	Type CommitType
	// Gitmoji is the official gitmoji (gitmoji.dev), written with its
	// variation selector U+FE0F when gitmoji uses one.
	Gitmoji string
	// BranchPrefix is the Conventional Branch prefix for work of this type.
	BranchPrefix BranchPrefix
	// ReleaseImpact is the increment the type triggers; a breaking change
	// always triggers MajorRelease, whatever its type.
	ReleaseImpact ReleaseImpact
	// AllowsBreakingChange is true for the types that may carry "!" or a
	// BREAKING CHANGE footer (feat, fix, refactor, as in @gregoiref/cz-config).
	AllowsBreakingChange bool
	// Label is the kind of the pull requests of this type, written after
	// "kind/" (docs/design/git-github-standards.md §5 bis); empty for none.
	Label string
}

// DescriptionKey is the i18n key of the human description of the type; a
// type a repository added has a generic one.
func (s CommitTypeSpec) DescriptionKey() string {
	if !IsBuiltInType(s.Type) {
		return "convention.type.custom.description"
	}
	return "convention.type." + string(s.Type) + ".description"
}

// Gitmojis used for breaking changes and release commits.
const (
	GitmojiBreakingChange = "💥"
	GitmojiRelease        = "🔖"
)

// commitTypeSpecs is in display order (menus, documentation). Labels are the
// kinds of docs/design/git-github-standards.md §5 bis; revert and wip have none.
var commitTypeSpecs = []CommitTypeSpec{
	{Type: Feature, Gitmoji: "✨", BranchPrefix: BranchFeature, ReleaseImpact: MinorRelease, AllowsBreakingChange: true, Label: "feature"},
	{Type: BugFix, Gitmoji: "🐛", BranchPrefix: BranchBugFix, ReleaseImpact: PatchRelease, AllowsBreakingChange: true, Label: "bug"},
	{Type: Documentation, Gitmoji: "📝", BranchPrefix: BranchMaintenance, ReleaseImpact: NoRelease, Label: "docs"},
	{Type: Styling, Gitmoji: "💄", BranchPrefix: BranchMaintenance, ReleaseImpact: NoRelease, Label: "chore"},
	{Type: Refactoring, Gitmoji: "♻️", BranchPrefix: BranchMaintenance, ReleaseImpact: NoRelease, AllowsBreakingChange: true, Label: "refactor"},
	{Type: Performance, Gitmoji: "⚡️", BranchPrefix: BranchMaintenance, ReleaseImpact: PatchRelease, Label: "perf"},
	{Type: Testing, Gitmoji: "✅", BranchPrefix: BranchMaintenance, ReleaseImpact: NoRelease, Label: "test"},
	{Type: BuildSystem, Gitmoji: "📦️", BranchPrefix: BranchMaintenance, ReleaseImpact: NoRelease, Label: "build"},
	{Type: ContinuousIntegration, Gitmoji: "👷", BranchPrefix: BranchMaintenance, ReleaseImpact: NoRelease, Label: "ci"},
	{Type: Maintenance, Gitmoji: "🔧", BranchPrefix: BranchMaintenance, ReleaseImpact: NoRelease, Label: "chore"},
	// A security fix is a bug fix from a branching point of view.
	{Type: SecurityFix, Gitmoji: "🔒️", BranchPrefix: BranchBugFix, ReleaseImpact: PatchRelease, Label: "security"},
	{Type: Reversion, Gitmoji: "⏪️", BranchPrefix: BranchMaintenance, ReleaseImpact: PatchRelease},
	{Type: WorkInProgress, Gitmoji: "🚧", BranchPrefix: BranchMaintenance, ReleaseImpact: NoRelease},
}

// CommitTypes returns ship-it's built-in commit types in display order; a
// repository's own table is Rules.CommitTypes.
func CommitTypes() []CommitTypeSpec { return Rules{}.CommitTypes() }

// LookupCommitType returns the specification of t in the built-in table.
func LookupCommitType(t CommitType) (CommitTypeSpec, bool) { return Rules{}.LookupCommitType(t) }

// variationSelector16 requests emoji presentation. gitmoji writes it on some
// emojis but many tools drop it, so comparisons ignore it.
const variationSelector16 = "️"

// NormalizeGitmoji strips variation selectors so that "🔒" and "🔒️" compare equal.
func NormalizeGitmoji(e string) string {
	return strings.ReplaceAll(e, variationSelector16, "")
}
