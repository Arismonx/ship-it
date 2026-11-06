// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package convention

import "testing"

// Precedence example from semver.org, rule 11.
func TestComparePrecedence(t *testing.T) {
	ordered := []string{
		"1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta",
		"1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-rc.1", "1.0.0",
	}
	for i := 0; i+1 < len(ordered); i++ {
		a, _ := ParseVersion(ordered[i])
		b, _ := ParseVersion(ordered[i+1])
		if Compare(a, b) != -1 || Compare(b, a) != 1 {
			t.Errorf("expected %s < %s", ordered[i], ordered[i+1])
		}
	}
	x, _ := ParseVersion("1.0.0+build.1")
	y, _ := ParseVersion("1.0.0+build.2")
	if Compare(x, y) != 0 {
		t.Error("build metadata must be ignored")
	}
}

func TestParseVersion(t *testing.T) {
	ok := map[string]Version{
		"v1.2.3":              {Major: 1, Minor: 2, Patch: 3},
		"0.1.0":               {Minor: 1},
		"1.0.0-rc.1+sha.5114": {Major: 1, Pre: "rc.1", Build: "sha.5114"},
	}
	for in, want := range ok {
		got, err := ParseVersion(in)
		if err != nil || got != want {
			t.Errorf("ParseVersion(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	for _, in := range []string{"1.2", "01.2.3", "1.2.3-01", "v", "1.2.3-", "latest"} {
		if _, err := ParseVersion(in); err == nil {
			t.Errorf("ParseVersion(%q) accepted an invalid version", in)
		}
	}
}

func TestNext(t *testing.T) {
	tests := []struct {
		from   string
		impact ReleaseImpact
		want   string
	}{
		{"1.2.3", PatchRelease, "1.2.4"},
		{"1.2.3", MinorRelease, "1.3.0"},
		{"1.2.3", MajorRelease, "2.0.0"},
		{"1.2.3-rc.1", PatchRelease, "1.2.4"},
		{"0.4.2", MajorRelease, "0.5.0"}, // 0.x: breaking bumps minor (ADR-0013)
		{"0.4.2", MinorRelease, "0.5.0"},
		{"1.2.3", NoRelease, "1.2.3"},
	}
	for _, tt := range tests {
		v, _ := ParseVersion(tt.from)
		if got := v.Next(tt.impact).String(); got != tt.want {
			t.Errorf("%s.Next(%d) = %s, want %s", tt.from, tt.impact, got, tt.want)
		}
	}
	v, _ := ParseVersion("1.4.0")
	if v.Tag() != "v1.4.0" {
		t.Errorf("Tag() = %s", v.Tag())
	}
}

func TestReleaseImpactOf(t *testing.T) {
	cases := map[Header]ReleaseImpact{
		{Type: Feature}:                     MinorRelease,
		{Type: BugFix}:                      PatchRelease,
		{Type: SecurityFix}:                 PatchRelease,
		{Type: Documentation}:               NoRelease,
		{Type: Refactoring, Breaking: true}: MajorRelease,
	}
	for h, want := range cases {
		if got := ReleaseImpactOf(h); got != want {
			t.Errorf("ReleaseImpactOf(%+v) = %d, want %d", h, got, want)
		}
	}
}
