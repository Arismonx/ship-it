// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package settings

import (
	"regexp"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/config"
)

// gitHubDefaults are the labels of a new repository (docs/design/git-github-standards.md §5).
var gitHubDefaults = []Label{
	{Name: "bug", Color: "d73a4a", Description: "Something isn't working"},
	{Name: "documentation", Color: "0075ca"},
	{Name: "duplicate", Color: "cfd3d7"},
	{Name: "enhancement", Color: "a2eeef"},
	{Name: "good first issue", Color: "7057ff"},
	{Name: "help wanted", Color: "008672"},
	{Name: "invalid", Color: "e4e669"},
	{Name: "question", Color: "d876e3"},
	{Name: "wontfix", Color: "ffffff"},
	{Name: "accessibility", Color: "000000"},
}

func TestPlanLabelsOnANewRepository(t *testing.T) {
	changes := PlanLabels(ShipItTaxonomy([]string{"api"}), gitHubDefaults)
	renames := map[string]string{}
	created := 0
	for _, c := range changes {
		switch c.Kind {
		case RenameLabel:
			renames[c.From] = c.Label.Name
		case CreateLabel:
			created++
		case UpdateLabel:
			t.Errorf("unexpected update %+v", c)
		}
	}
	want := map[string]string{
		"bug": "kind/bug", "enhancement": "kind/feature", "documentation": "kind/docs", "question": "kind/support",
		"accessibility": "kind/accessibility", "duplicate": "triage/duplicate", "invalid": "triage/invalid", "wontfix": "triage/wontfix",
	}
	if len(renames) != len(want) {
		t.Fatalf("renames = %v", renames)
	}
	for from, to := range want {
		if renames[from] != to {
			t.Errorf("rename %s → %s, want %s", from, renames[from], to)
		}
	}
	if created != len(ShipItTaxonomy([]string{"api"}))-len(want) {
		t.Errorf("created %d", created)
	}
	if changes[0].Kind != RenameLabel {
		t.Error("renames must come first")
	}
}

func TestPlanLabelsIsIdempotentAndNeverDeletes(t *testing.T) {
	wanted := ShipItTaxonomy(nil)
	existing := append([]Label{{Name: "customer/acme", Color: "000000"}}, wanted...)
	if changes := PlanLabels(wanted, existing); len(changes) != 0 {
		t.Fatalf("a synced repository needs no change: %+v", changes)
	}
	drifted := append([]Label(nil), wanted...)
	drifted[0].Color = "FF0000"
	drifted[1].Name = strings.ToUpper(drifted[1].Name) // case differs only
	changes := PlanLabels(wanted, drifted)
	if len(changes) != 1 || changes[0].Kind != UpdateLabel || changes[0].From != wanted[0].Name {
		t.Fatalf("changes = %+v", changes)
	}
}

func TestRenameNeverMergesIntoAnExistingLabel(t *testing.T) {
	// Both "bug" and "kind/bug" exist: renaming would fail on GitHub.
	existing := []Label{{Name: "bug"}, {Name: "kind/bug", Color: alarmColor, Description: "Something does not work as intended"}}
	for _, c := range PlanLabels(ShipItTaxonomy(nil)[:1], existing) {
		t.Errorf("unexpected change %+v", c)
	}
}

func TestTaxonomyIsWellFormed(t *testing.T) {
	color := regexp.MustCompile(`^[0-9a-f]{6}$`)
	seen := map[string]bool{}
	for _, l := range ShipItTaxonomy([]string{"api", " ", "ui"}) {
		if !color.MatchString(l.Color) || l.Description == "" || seen[l.Name] || len(l.Description) > 100 {
			t.Errorf("malformed or duplicate label %+v", l)
		}
		seen[l.Name] = true
	}
	if !seen["area/api"] || !seen["area/ui"] || seen["area/"] {
		t.Error("areas not turned into labels")
	}
	if WantedLabels(config.Settings{GitHub: config.GitHubSettings{Labels: config.LabelsSettings{Taxonomy: config.TaxonomyNone}}}) != nil {
		t.Error("taxonomy none wants no label")
	}
}

func TestSizeColorsRampFromGreenToRed(t *testing.T) {
	sizes := map[string]string{}
	for _, l := range ShipItTaxonomy(nil) {
		if strings.HasPrefix(l.Name, "size/") {
			sizes[l.Name] = l.Color
		}
	}
	if sizes["size/XS"] != "009900" || sizes["size/XXL"] != "ee0000" || len(sizes) != 6 {
		t.Fatalf("size colors = %v (Kubernetes ramp expected)", sizes)
	}
}

func TestWantedLabelsFollowThePrefixesAndSizes(t *testing.T) {
	s := config.Defaults()
	if got := WantedLabels(s); !hasLabel(got, "size/XXL", "1000 or more changed lines, generated files excluded") ||
		!hasLabel(got, "size/XS", "0-9 changed lines, generated files excluded") || !hasLabel(got, "kind/bug", "") {
		t.Fatalf("default labels = %v", got)
	}
	s.Ship.PullRequest.LabelPrefixes = config.LabelPrefixes{Kind: "type: ", Size: "lines/"}
	s.Ship.PullRequest.Sizes = config.Sizes{"small": 50, "large": 0}
	got := WantedLabels(s)
	if !hasLabel(got, "type: bug", "") || !hasLabel(got, "lines/small", "0-49 changed lines, generated files excluded") ||
		!hasLabel(got, "lines/large", "50 or more changed lines, generated files excluded") {
		t.Fatalf("labels = %v", got)
	}
	for _, l := range got {
		if strings.HasPrefix(l.Name, "kind/") || strings.HasPrefix(l.Name, "size/") {
			t.Errorf("label %s ignores the prefixes", l.Name)
		}
	}
}

func hasLabel(labels []Label, name, description string) bool {
	for _, l := range labels {
		if l.Name == name && (description == "" || l.Description == description) {
			return true
		}
	}
	return false
}
