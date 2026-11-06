// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package pullrequest

import (
	"slices"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/convention"
)

// Default label prefixes (docs/design/git-github-standards.md §5).
const (
	DefaultKindPrefix = "kind/"
	DefaultSizePrefix = "size/"
)

// Size is one size label: Name applies under Below changed lines; Below 0
// applies to everything larger than the other sizes.
type Size struct {
	Name  string
	Below int
}

// DefaultSizes are the thresholds of Kubernetes
// (docs/design/git-github-standards.md §5 bis), smallest first.
var DefaultSizes = []Size{{"XS", 10}, {"S", 30}, {"M", 100}, {"L", 500}, {"XL", 1000}, {"XXL", 0}}

// Taxonomy names the kind and size labels (ship.pullRequest.labelPrefixes,
// ship.pullRequest.sizes). Its zero value is the built-in one: kind/, size/
// and Kubernetes' sizes.
type Taxonomy struct {
	KindPrefix, SizePrefix string
	// Sizes are ordered by Below, the unbounded one (Below 0) last; nil is
	// DefaultSizes.
	Sizes []Size
	// set: the prefixes were given (an empty prefix is a valid choice).
	set bool
}

// NewTaxonomy returns the taxonomy of the given prefixes and sizes, the
// sizes sorted by threshold (the unbounded one last).
func NewTaxonomy(kindPrefix, sizePrefix string, sizes []Size) Taxonomy {
	sorted := slices.Clone(sizes)
	slices.SortStableFunc(sorted, func(a, b Size) int {
		switch {
		case a.Below == b.Below:
			return strings.Compare(a.Name, b.Name)
		case a.Below == 0:
			return 1
		case b.Below == 0:
			return -1
		}
		return a.Below - b.Below
	})
	return Taxonomy{KindPrefix: kindPrefix, SizePrefix: sizePrefix, Sizes: sorted, set: true}
}

func (t Taxonomy) kindPrefix() string {
	if !t.set {
		return DefaultKindPrefix
	}
	return t.KindPrefix
}

func (t Taxonomy) sizePrefix() string {
	if !t.set {
		return DefaultSizePrefix
	}
	return t.SizePrefix
}

func (t Taxonomy) sizes() []Size {
	if t.Sizes == nil {
		return DefaultSizes
	}
	return t.Sizes
}

// gitHubDefaults are the labels GitHub creates in every new repository that
// match a kind (docs/design/git-github-standards.md §5), used when the
// repository was never migrated to the kind taxonomy.
var gitHubDefaults = map[string]string{
	"feature": "enhancement",
	"bug":     "bug",
	"docs":    "documentation",
}

// SizeLabel returns the size label for a number of changed lines, with the
// thresholds of Kubernetes (docs/design/git-github-standards.md §5 bis).
func SizeLabel(changedLines int) string { return Taxonomy{}.SizeLabel(changedLines) }

// SizeLabel returns the size label for a number of changed lines, "" when
// no size applies (a table without an unbounded size).
func (t Taxonomy) SizeLabel(changedLines int) string {
	for _, s := range t.sizes() {
		if s.Below == 0 || changedLines < s.Below {
			return t.sizePrefix() + s.Name
		}
	}
	return ""
}

// isSizeLabel reports whether l is one of the taxonomy's size labels. Only
// the configured names count: with an empty prefix, other labels are safe.
func (t Taxonomy) isSizeLabel(l string) bool {
	for _, s := range t.sizes() {
		if l == t.sizePrefix()+s.Name {
			return true
		}
	}
	return false
}

// HasSizeLabel reports whether one of labels is a size label.
func (t Taxonomy) HasSizeLabel(labels []string) bool {
	return slices.ContainsFunc(labels, t.isSizeLabel)
}

// RefreshLabels is Taxonomy.RefreshLabels with the built-in taxonomy.
func RefreshLabels(current, wanted []string) (add, remove []string) {
	return Taxonomy{}.RefreshLabels(current, wanted)
}

// RefreshLabels updates the size of a pull request that received new
// commits: the wanted size label replaces any other size label, which only
// ship-it sets. Kind labels are set at creation only: re-adding them later
// would undo a reviewer who removed one on purpose. Other labels are never
// touched.
func (t Taxonomy) RefreshLabels(current, wanted []string) (add, remove []string) {
	i := slices.IndexFunc(wanted, t.isSizeLabel)
	if i < 0 || slices.Contains(current, wanted[i]) {
		return nil, nil
	}
	for _, c := range current {
		if t.isSizeLabel(c) {
			remove = append(remove, c)
		}
	}
	return []string{wanted[i]}, remove
}

// aiLabels are the names projects give to a label for AI-assisted work, as
// observed in open-source repositories; the first one the
// repository has wins.
var aiLabels = []string{"ai-assisted", "kind/ai-assisted", "ai-generated", "llm-generated", "claude-code-assisted"}

// AILabel returns the repository's label for AI-assisted work, matched
// without regard to case, or "" when it has none.
func AILabel(existing []string) string {
	for _, want := range aiLabels {
		for _, l := range existing {
			if strings.EqualFold(l, want) {
				return l
			}
		}
	}
	return ""
}

// Labels is Taxonomy.Labels with the built-in taxonomy.
func Labels(rules convention.Rules, commits []convention.Header, changedLines int, existing []string) []string {
	return Taxonomy{}.Labels(rules, commits, changedLines, existing)
}

// Labels returns the labels to put on a pull request, among the ones the
// repository already has: ship-it never creates labels (that is `ship init`'s
// job), so a repository without them simply gets none.
// Kinds come from the repository's commit types (rules, ship.commits.types).
// changedLines must exclude generated files.
func (t Taxonomy) Labels(rules convention.Rules, commits []convention.Header, changedLines int, existing []string) []string {
	has := map[string]bool{}
	for _, l := range existing {
		has[l] = true
	}
	var labels []string
	add := func(label string) {
		if label != "" && has[label] && !slices.Contains(labels, label) {
			labels = append(labels, label)
		}
	}
	addKind := func(kind string) {
		label := t.kindPrefix() + kind
		if !has[label] {
			label = gitHubDefaults[kind]
		}
		add(label)
	}
	for _, c := range commits {
		if spec, ok := rules.LookupCommitType(c.Type); ok && spec.Label != "" {
			addKind(spec.Label)
		}
		if c.Breaking {
			addKind("breaking")
		}
	}
	add(t.SizeLabel(changedLines))
	return labels
}

// KindLabel is Taxonomy.KindLabel with the built-in taxonomy.
func KindLabel(rules convention.Rules, ct convention.CommitType, existing []string) string {
	return Taxonomy{}.KindLabel(rules, ct, existing)
}

// KindLabel returns the repository's label for a commit type: the kind label
// when it exists, else GitHub's default label of the same meaning, "" when
// the repository has neither or the type has no kind.
func (t Taxonomy) KindLabel(rules convention.Rules, ct convention.CommitType, existing []string) string {
	spec, ok := rules.LookupCommitType(ct)
	if !ok || spec.Label == "" {
		return ""
	}
	for _, l := range []string{t.kindPrefix() + spec.Label, gitHubDefaults[spec.Label]} {
		if l != "" && slices.Contains(existing, l) {
			return l
		}
	}
	return ""
}
