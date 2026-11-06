// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package convention

import (
	"reflect"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
)

func ptr(s string) *string { return &s }

func TestZeroRulesAreTheBuiltInOnes(t *testing.T) {
	var r Rules
	if r.SubjectMax() != 50 || r.HeaderMax() != 72 || r.Width() != 72 ||
		r.branchMaxWords() != 4 || r.branchMaxLength() != 32 {
		t.Fatalf("limits = %d %d %d %d %d", r.SubjectMax(), r.HeaderMax(), r.Width(), r.branchMaxWords(), r.branchMaxLength())
	}
	if !reflect.DeepEqual(r.CommitTypes(), CommitTypes()) {
		t.Fatal("the zero rules must use the built-in table")
	}
}

func TestWithTypes(t *testing.T) {
	table := WithTypes([]TypeChange{
		{Type: "feat", Gitmoji: ptr("🚀"), Label: ptr("enhancement")},
		{Type: "wip", Remove: true},
		{Type: "deps", Gitmoji: ptr("⬆️"), BranchPrefix: ptr("chore")},
		{Type: "nothing", Remove: true}, // removing an unknown type changes nothing
	})
	r := Rules{Types: table}
	if len(table) != len(CommitTypes()) {
		t.Fatalf("%d types", len(table))
	}
	feat, _ := r.LookupCommitType("feat")
	if feat.Gitmoji != "🚀" || feat.Label != "enhancement" || feat.BranchPrefix != BranchFeature || !feat.AllowsBreakingChange {
		t.Errorf("feat = %+v", feat)
	}
	if _, ok := r.LookupCommitType("wip"); ok {
		t.Error("wip is removed")
	}
	deps := table[len(table)-1]
	if deps.Type != "deps" || deps.ReleaseImpact != NoRelease || deps.AllowsBreakingChange || deps.Label != "" {
		t.Errorf("deps = %+v", deps)
	}
	if deps.DescriptionKey() != "convention.type.custom.description" || feat.DescriptionKey() != "convention.type.feat.description" {
		t.Error("an added type has the generic description")
	}
	if _, ok := LookupCommitType("deps"); ok {
		t.Error("the built-in table must not change")
	}
}

func TestRulesValidate(t *testing.T) {
	types := WithTypes([]TypeChange{{Type: "deps", Gitmoji: ptr("⬆️"), BranchPrefix: ptr("chore")}, {Type: "style", Remove: true}})
	tests := []struct {
		name  string
		rules Rules
		title string
		want  []fault.Code
	}{
		{"built-in", Rules{}, "✨ feat(api): add login", nil},
		{"longer subject allowed", Rules{SubjectMaxLength: 60, HeaderMaxLength: 90},
			"✨ feat: " + strings.Repeat("a", 60), nil},
		{"shorter subject", Rules{SubjectMaxLength: 20}, "✨ feat: " + strings.Repeat("a", 21), []fault.Code{fault.SubjectTooLong}},
		{"shorter header", Rules{HeaderMaxLength: 30}, "✨ feat(api): " + strings.Repeat("a", 20), []fault.Code{fault.HeaderTooLong}},
		{"added type", Rules{Types: types}, "⬆️ deps: bump yaml", nil},
		{"scope listed", Rules{Scopes: []string{"api", "ui"}}, "✨ feat(ui): add menu", nil},
		{"scope not listed", Rules{Scopes: []string{"api", "ui"}}, "✨ feat(db): add index", []fault.Code{fault.ScopeNotAllowed}},
		{"no scope, any allowed", Rules{Scopes: []string{"api"}}, "✨ feat: add index", nil},
		{"scope required", Rules{ScopeRequired: true}, "✨ feat: add index", []fault.Code{fault.ScopeRequired}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := tt.rules.ParseHeader(tt.title)
			if err != nil {
				t.Fatal(err)
			}
			var got []fault.Code
			for _, f := range tt.rules.Validate(h) {
				got = append(got, f.Code)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Validate = %v, want %v", got, tt.want)
			}
		})
	}
	if _, err := (Rules{Types: types}).ParseHeader("💄 style: tidy"); err == nil {
		t.Error("a removed type must not parse")
	}
	if _, err := ParseHeader("⬆️ deps: bump yaml"); err == nil {
		t.Error("an added type is unknown to the built-in rules")
	}
}

func TestRulesFormatTakesTheGitmojiOfTheTable(t *testing.T) {
	r := Rules{Types: WithTypes([]TypeChange{{Type: "feat", Gitmoji: ptr("🚀")}})}
	if got := r.Format(Header{Type: Feature, Subject: "add login"}, GitmojiFirst); got != "🚀 feat: add login" {
		t.Errorf("Format = %q", got)
	}
	if got := r.Format(Header{Gitmoji: "✨", Type: Feature, Subject: "add login"}, GitmojiFirst); got != "✨ feat: add login" {
		t.Errorf("a written gitmoji stays: %q", got)
	}
}

func TestRulesFitKeepsARequiredScope(t *testing.T) {
	h := Header{Type: Feature, Scope: "api", Subject: strings.Repeat("a", 50)}
	if got := (Rules{HeaderMaxLength: 30}).FitToMaxLength(h); got.Scope != "" {
		t.Error("the scope goes when the header is too long")
	}
	if got := (Rules{HeaderMaxLength: 30, ScopeRequired: true}).FitToMaxLength(h); got.Scope != "api" {
		t.Error("a required scope stays")
	}
}

func TestRulesBranchName(t *testing.T) {
	types := WithTypes([]TypeChange{{Type: "docs", BranchPrefix: ptr("docs")}, {Type: "deps", Gitmoji: ptr("⬆️"), BranchPrefix: ptr("deps")}})
	tests := []struct {
		name  string
		rules Rules
		t     CommitType
		opts  BranchOptions
		want  string
	}{
		{"built-in", Rules{}, Feature, BranchOptions{Issue: 42}, "feat/42-add-login-form-validation"},
		{"more words", Rules{BranchMaxWords: 6, BranchMaxLength: 60}, Feature, BranchOptions{Issue: 42}, "feat/42-add-login-form-validation-mobile-screens"},
		{"fewer words", Rules{BranchMaxWords: 2}, Feature, BranchOptions{}, "feat/add-login"},
		{"shorter", Rules{BranchMaxLength: 10}, Feature, BranchOptions{}, "feat/add-login"},
		{"without issue", Rules{BranchWithoutIssue: true}, Feature, BranchOptions{Issue: 42, Ticket: "ACME-1"}, "feat/add-login-form-validation"},
		{"prefix of the table", Rules{Types: types}, Documentation, BranchOptions{}, "docs/add-login-form-validation"},
		{"added type", Rules{Types: types}, "deps", BranchOptions{}, "deps/add-login-form-validation"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.rules.BranchName(tt.t, "add login form validation for mobile screens", tt.opts)
			if got != tt.want {
				t.Fatalf("BranchName = %q, want %q", got, tt.want)
			}
			if err := tt.rules.ValidateBranch(got, nil); err != nil {
				t.Fatalf("%s is refused: %v", got, err)
			}
		})
	}
	if ValidateBranch("deps/bump-yaml") == nil {
		t.Error("deps/ is no built-in prefix")
	}
}

func TestRulesCommitMessageWrapsAtTheBodyWidth(t *testing.T) {
	why := strings.Repeat("word ", 20)
	for _, width := range []int{40, 72, 100} {
		msg := (Rules{BodyWidth: width}).CommitMessage("✨ feat: x", why, "")
		for _, line := range strings.Split(msg, "\n")[2:] {
			if len(line) > width {
				t.Errorf("width %d: line %q", width, line)
			}
		}
	}
	if (Rules{BodyWidth: 40}).CommitMessage("✨ feat: x", why, "") == CommitMessage("✨ feat: x", why, "") {
		t.Error("the width must change the wrapping")
	}
}

func TestRulesClosest(t *testing.T) {
	r := Rules{Types: WithTypes([]TypeChange{{Type: "perf", Remove: true}, {Type: "refactor", Remove: true}, {Type: "chore", Remove: true}})}
	if got, ok := r.Closest(Performance); !ok || got != BugFix {
		t.Errorf("Closest(perf) = %s, %v", got, ok)
	}
	if got, ok := r.Closest(Maintenance); !ok || got != Feature {
		t.Errorf("Closest(chore) = %s, %v: the first type of the table", got, ok)
	}
	if got, ok := r.Closest(Feature); ok || got != Feature {
		t.Errorf("Closest(feat) = %s, %v", got, ok)
	}
}

func TestValidators(t *testing.T) {
	for _, tt := range []struct {
		check func(string) bool
		value string
		want  bool
	}{
		{ValidTypeName, "deps", true}, {ValidTypeName, "Deps", false}, {ValidTypeName, "dep-s", false}, {ValidTypeName, "", false},
		{ValidBranchPrefix, "feat", true}, {ValidBranchPrefix, "v1.x", true}, {ValidBranchPrefix, "feat/", false},
		{ValidBranchPrefix, "-feat", false}, {ValidBranchPrefix, "", false},
		{ValidLabel, "", true}, {ValidLabel, "feature", true}, {ValidLabel, "kind/feature", false},
	} {
		if got := tt.check(tt.value); got != tt.want {
			t.Errorf("%q: %v, want %v", tt.value, got, tt.want)
		}
	}
	if WrittenType("⬆️ Deps(x): y") != "deps" || WrittenType("nothing") != "" {
		t.Error("WrittenType reads the grammar only")
	}
}

func TestReleaseOfTheRepositoryTypes(t *testing.T) {
	major, none := MajorRelease, NoRelease
	r := Rules{Types: WithTypes([]TypeChange{
		{Type: "deps", Gitmoji: ptr("⬆️"), BranchPrefix: ptr("chore")},
		{Type: "hotfix", Gitmoji: ptr("🚑️"), BranchPrefix: ptr("fix"), Release: &major},
		{Type: "perf", Release: &none},
	})}
	for h, want := range map[Header]ReleaseImpact{
		{Type: "feat"}:                 MinorRelease,
		{Type: "perf"}:                 NoRelease,
		{Type: "deps"}:                 NoRelease,
		{Type: "hotfix"}:               MajorRelease,
		{Type: "deps", Breaking: true}: MajorRelease,
		{Type: "unknown"}:              NoRelease,
	} {
		if got := r.ReleaseImpactOf(h); got != want {
			t.Errorf("%s: %s, want %s", h.Type, got, want)
		}
	}
	if ReleaseImpactOf(Header{Type: "perf"}) != PatchRelease {
		t.Error("the built-in table must not change")
	}
	for _, name := range ReleaseNames() {
		if impact, ok := ParseRelease(name); !ok || impact.String() != name {
			t.Errorf("ParseRelease(%q) = %s, %v", name, impact, ok)
		}
	}
	if _, ok := ParseRelease("huge"); ok {
		t.Error("an unknown release is refused")
	}
}

func TestHabitsReadTheRepositoryTypes(t *testing.T) {
	r := Rules{Types: WithTypes([]TypeChange{{Type: "deps", Gitmoji: ptr("⬆️"), BranchPrefix: ptr("chore")}})}
	subjects := []string{"deps: ⬆️ bump a", "deps: ⬆️ bump b", "deps(api): ⬆️ bump c", "deps: ⬆️ bump d", "deps: ⬆️ bump e"}
	if p, ok := r.DetectGitmojiPlacement(subjects); !ok || p != GitmojiAfterType {
		t.Errorf("with deps: %v, %v", p, ok)
	}
	if _, ok := DetectGitmojiPlacement(subjects); ok {
		t.Error("the built-in table does not know deps")
	}
	if got := r.FrequentScopes(append(subjects, "deps(api): x"), 2); strings.Join(got, ",") != "api" {
		t.Errorf("scopes = %v", got)
	}
}
