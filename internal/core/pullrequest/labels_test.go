// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package pullrequest

import (
	"reflect"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/convention"
)

func TestSizeLabel(t *testing.T) {
	for lines, want := range map[int]string{0: "size/XS", 9: "size/XS", 10: "size/S", 29: "size/S", 30: "size/M",
		99: "size/M", 100: "size/L", 499: "size/L", 500: "size/XL", 999: "size/XL", 1000: "size/XXL"} {
		if got := SizeLabel(lines); got != want {
			t.Errorf("SizeLabel(%d) = %s, want %s", lines, got, want)
		}
	}
}

func TestRefreshLabels(t *testing.T) {
	tests := []struct {
		name                 string
		current, wanted      []string
		wantAdd, wantRemoved []string
	}{
		{"grew", []string{"kind/feature", "size/S", "priority/p1"}, []string{"kind/feature", "size/M"}, []string{"size/M"}, []string{"size/S"}},
		{"same size", []string{"kind/feature", "size/S"}, []string{"kind/feature", "kind/bug", "size/S"}, nil, nil},
		{"a reviewer removed a kind: not re-added", []string{"size/S"}, []string{"kind/bug", "size/M"}, []string{"size/M"}, []string{"size/S"}},
		{"no size on the pull request yet", []string{"kind/bug"}, []string{"kind/bug", "size/XS"}, []string{"size/XS"}, nil},
		{"no size label in the repository", []string{"size/S"}, []string{"kind/feature"}, nil, nil},
	}
	for _, tt := range tests {
		add, remove := RefreshLabels(tt.current, tt.wanted)
		if !reflect.DeepEqual(add, tt.wantAdd) || !reflect.DeepEqual(remove, tt.wantRemoved) {
			t.Errorf("%s: add %v remove %v, want %v %v", tt.name, add, remove, tt.wantAdd, tt.wantRemoved)
		}
	}
}

func TestLabels(t *testing.T) {
	commits := []convention.Header{header(t, "feat(api)!: drop v1"), header(t, "fix: nil"), header(t, "feat: add vat")}
	kubernetes := []string{"kind/feature", "kind/bug", "kind/breaking", "size/S", "size/M", "enhancement", "bug"}
	defaults := []string{"bug", "enhancement", "documentation", "wontfix"}
	tests := []struct {
		name     string
		existing []string
		want     []string
	}{
		{"ship-it taxonomy", kubernetes, []string{"kind/feature", "kind/breaking", "kind/bug", "size/M"}},
		{"GitHub defaults", defaults, []string{"enhancement", "bug"}},
		{"no labels at all", nil, nil},
	}
	for _, tt := range tests {
		if got := Labels(convention.Rules{}, commits, 42, tt.existing); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: Labels = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestLabelsFollowTheTypeTable(t *testing.T) {
	label, none := "enhancement", ""
	rules := convention.Rules{Types: convention.WithTypes([]convention.TypeChange{
		{Type: "feat", Label: &label}, {Type: "fix", Label: &none}})}
	existing := []string{"kind/enhancement", "kind/bug", "kind/feature"}
	commits := []convention.Header{{Type: convention.Feature}, {Type: convention.BugFix}}
	if got := Labels(rules, commits, 3, existing); !reflect.DeepEqual(got, []string{"kind/enhancement"}) {
		t.Errorf("Labels = %v", got)
	}
	if got := KindLabel(rules, convention.Feature, existing); got != "kind/enhancement" {
		t.Errorf("KindLabel(feat) = %q", got)
	}
	if got := KindLabel(rules, convention.BugFix, existing); got != "" {
		t.Errorf("KindLabel(fix) = %q: no label", got)
	}
	if got := KindLabel(convention.Rules{}, convention.Feature, []string{"enhancement"}); got != "enhancement" {
		t.Errorf("built-in KindLabel = %q", got)
	}
}
