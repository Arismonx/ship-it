// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package convention

import (
	"errors"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
)

func TestParseHeader(t *testing.T) {
	tests := []struct {
		in   string
		want Header
	}{
		{"✨ feat(ui): add dark mode", Header{Gitmoji: "✨", Type: Feature, Scope: "ui", Subject: "add dark mode"}},
		{"feat(ui): ✨ add dark mode", Header{Gitmoji: "✨", Type: Feature, Scope: "ui", Subject: "add dark mode"}},
		{"feat: add dark mode", Header{Type: Feature, Subject: "add dark mode"}},
		{"💥 feat(api)!: drop v1 routes", Header{Gitmoji: "💥", Type: Feature, Scope: "api", Breaking: true, Subject: "drop v1 routes"}},
		{"🔒 security: rotate keys", Header{Gitmoji: "🔒", Type: SecurityFix, Subject: "rotate keys"}},
		{"FIX: handle nil config", Header{Type: BugFix, Subject: "handle nil config"}},
		{"fix: first line\n\nbody is ignored", Header{Type: BugFix, Subject: "first line"}},
		{"wip: draft", Header{Type: WorkInProgress, Subject: "draft"}},
	}
	for _, tt := range tests {
		got, err := ParseHeader(tt.in)
		if err != nil {
			t.Errorf("ParseHeader(%q) error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseHeader(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestParseHeaderErrors(t *testing.T) {
	tests := []struct {
		in   string
		code fault.Code
	}{
		{"", fault.EmptyHeader},
		{"   ", fault.EmptyHeader},
		{"add dark mode", fault.MalformedHeader},
		{"feat add dark mode", fault.MalformedHeader},
		{"feat:missing space", fault.MalformedHeader},
		{"feature: add dark mode", fault.UnknownCommitType},
		{"✨ yolo(ui): add", fault.UnknownCommitType},
	}
	for _, tt := range tests {
		_, err := ParseHeader(tt.in)
		if !errors.Is(err, &fault.Fault{Code: tt.code}) {
			t.Errorf("ParseHeader(%q) error = %v, want code %s", tt.in, err, tt.code)
		}
	}
}

func TestFormat(t *testing.T) {
	h := Header{Type: Feature, Scope: "ui", Subject: "add dark mode"}
	cases := map[GitmojiPlacement]string{
		GitmojiFirst:     "✨ feat(ui): add dark mode",
		GitmojiAfterType: "feat(ui): ✨ add dark mode",
		GitmojiOmitted:   "feat(ui): add dark mode",
	}
	for style, want := range cases {
		if got := Format(h, style); got != want {
			t.Errorf("Format(style %d) = %q, want %q", style, got, want)
		}
	}
	breaking := Header{Type: Feature, Breaking: true, Subject: "drop v1 routes"}
	if got, want := Format(breaking, GitmojiFirst), "💥 feat!: drop v1 routes"; got != want {
		t.Errorf("Format(breaking) = %q, want %q", got, want)
	}
}

func TestValidate(t *testing.T) {
	long := strings.Repeat("a", DefaultSubjectMaxLength+1)
	tests := []struct {
		name string
		h    Header
		want []fault.Code
	}{
		{"valid", Header{Type: Feature, Scope: "ui", Subject: "add dark mode"}, nil},
		{"acronym kept", Header{Type: BugFix, Subject: "HEAD detection on Windows"}, nil},
		{"empty subject", Header{Type: Feature, Subject: " "}, []fault.Code{fault.EmptySubject}},
		{"capitalised", Header{Type: Feature, Subject: "Add dark mode"}, []fault.Code{fault.SubjectNotLowercase}},
		{"period", Header{Type: Feature, Subject: "add dark mode."}, []fault.Code{fault.SubjectEndsWithPeriod}},
		{"bad scope", Header{Type: Feature, Scope: "UI Kit", Subject: "add"}, []fault.Code{fault.InvalidScope}},
		{"subject too long", Header{Type: Feature, Subject: long}, []fault.Code{fault.SubjectTooLong}},
		{"header too long", Header{Type: Refactoring, Scope: strings.Repeat("s", 30), Subject: strings.Repeat("b", 45)},
			[]fault.Code{fault.HeaderTooLong}},
		{"breaking allowed on feat", Header{Type: Feature, Breaking: true, Subject: "drop v1 routes"}, nil},
		{"breaking not allowed on docs", Header{Type: Documentation, Breaking: true, Subject: "rewrite guide"},
			[]fault.Code{fault.BreakingNotAllowed}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Validate(tt.h)
			if len(got) != len(tt.want) {
				t.Fatalf("Validate() = %v, want codes %v", got, tt.want)
			}
			for i := range got {
				if got[i].Code != tt.want[i] {
					t.Errorf("fault %d = %s, want %s", i, got[i].Code, tt.want[i])
				}
			}
		})
	}
}

func TestFitToMaxLengthDropsScope(t *testing.T) {
	h := Header{Type: Refactoring, Scope: strings.Repeat("s", 30), Subject: strings.Repeat("b", 40)}
	got := FitToMaxLength(h)
	if got.Scope != "" {
		t.Fatalf("FitToMaxLength() kept scope %q", got.Scope)
	}
	if v := Validate(got); len(v) != 0 {
		t.Fatalf("FitToMaxLength() result still invalid: %v", v)
	}
}

func TestEveryTypeHasEmojiAndBranch(t *testing.T) {
	for _, info := range CommitTypes() {
		if info.Gitmoji == "" {
			t.Errorf("type %s has no gitmoji", info.Type)
		}
		if err := ValidateBranch(string(info.BranchPrefix) + "/x"); err != nil {
			t.Errorf("type %s maps to invalid branch prefix %q: %v", info.Type, info.BranchPrefix, err)
		}
	}
}

func TestNormalizeGitmoji(t *testing.T) {
	if NormalizeGitmoji("🔒️") != NormalizeGitmoji("🔒") {
		t.Fatal("variation selector not ignored")
	}
}

// FuzzHeaderRoundTrip checks that any header ParseHeader accepts survives a
// Format/ParseHeader round trip unchanged.
func FuzzHeaderRoundTrip(f *testing.F) {
	for _, s := range []string{
		"✨ feat(ui): add dark mode", "feat: x", "💥 feat(api)!: drop", "fix(a/b): ✨ y",
		"chore", ": :", "🔧 chore(): z",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		h, err := ParseHeader(s)
		if err != nil {
			return
		}
		h.Subject = strings.TrimSpace(h.Subject)
		if h.Subject == "" || strings.ContainsAny(h.Scope, "\r\n") {
			return
		}
		again, err := ParseHeader(Format(h, GitmojiFirst))
		if err != nil {
			t.Fatalf("re-parse of %q failed: %v", Format(h, GitmojiFirst), err)
		}
		if again.Type != h.Type || again.Scope != h.Scope || again.Breaking != h.Breaking {
			t.Fatalf("round trip changed %+v into %+v", h, again)
		}
	})
}

func TestWithType(t *testing.T) {
	for in, want := range map[string]string{
		"⚡️ perf(api)!: speed up perf counters": "⚡️ refactor(api)!: speed up perf counters",
		"perf: x":   "refactor: x",
		"İ perf: x": "İ refactor: x",
	} {
		if got, ok := WithType(in, Refactoring); !ok || got != want {
			t.Errorf("WithType(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	if _, ok := WithType("not a header", Refactoring); ok {
		t.Error("a line that is not a header cannot be retyped")
	}
	if _, ok := WithType("perf: x", "story"); ok {
		t.Error("an unknown type is refused")
	}
}
