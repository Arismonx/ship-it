// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package convention

import "testing"

func repeat(s string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}

func TestDetectGitmojiPlacement(t *testing.T) {
	for _, tt := range []struct {
		name     string
		subjects []string
		want     GitmojiPlacement
		ok       bool
	}{
		{"young repository", repeat("feat: add x", 4), GitmojiFirst, false},
		{"no emoji", repeat("fix(api): handle nil", 6), GitmojiOmitted, true},
		{"emoji first", repeat("✨ feat: add x", 6), GitmojiFirst, true},
		{"emoji after the type", repeat("feat: ✨ add x", 6), GitmojiAfterType, true},
		{"non-conventional subjects ignored", append(repeat("Merge pull request #1 from x", 20), repeat("feat: add x", 5)...), GitmojiOmitted, true},
		{"majority", append(repeat("🐛 fix: a", 2), repeat("fix: b", 4)...), GitmojiOmitted, true},
		{"tie goes to first", append(repeat("🐛 fix: a", 3), repeat("fix: b", 3)...), GitmojiFirst, true},
	} {
		got, ok := DetectGitmojiPlacement(tt.subjects)
		if got != tt.want || ok != tt.ok {
			t.Errorf("%s: got %v, %v; want %v, %v", tt.name, got, ok, tt.want, tt.ok)
		}
	}
}

func TestParsePlacement(t *testing.T) {
	for in, want := range map[string]GitmojiPlacement{"first": GitmojiFirst, "after-type": GitmojiAfterType, "none": GitmojiOmitted} {
		if got, ok := ParsePlacement(in); !ok || got != want {
			t.Errorf("ParsePlacement(%q) = %v, %v", in, got, ok)
		}
	}
	if _, ok := ParsePlacement("auto"); ok {
		t.Error(`"auto" must ask for detection`)
	}
}
