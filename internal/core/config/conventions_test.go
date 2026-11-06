// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package config

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/convention"
	"github.com/GregoireF/ship-it/internal/core/fault"
)

func TestConventionsDefaultsAreTheBuiltInOnes(t *testing.T) {
	got := Defaults().Conventions()
	want := convention.Rules{SubjectMaxLength: 50, HeaderMaxLength: 72, BodyWidth: 72, Scopes: []string{},
		BranchMaxWords: 4, BranchMaxLength: 32}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Conventions() = %+v, want %+v", got, want)
	}
	if !reflect.DeepEqual(got.CommitTypes(), convention.CommitTypes()) {
		t.Fatal("the default table must be the built-in one")
	}
}

func TestConventionSettings(t *testing.T) {
	repo := Layer{Name: "repository", Path: ".github/ship.json", Data: []byte(`{"ship": {
		"commits": {"subjectMaxLength": 60, "headerMaxLength": 90, "bodyWidth": 80,
			"scopes": ["api", "ui"], "scopeRequired": true, "scopeFrom": "none"},
		"branches": {"maxWords": 6, "maxLength": 40, "issue": "none"}}}`)}
	eff, err := Merge([]Layer{repo})
	if err != nil {
		t.Fatal(err)
	}
	got := eff.Settings.Conventions()
	want := convention.Rules{SubjectMaxLength: 60, HeaderMaxLength: 90, BodyWidth: 80, Scopes: []string{"api", "ui"},
		ScopeRequired: true, BranchMaxWords: 6, BranchMaxLength: 40, BranchWithoutIssue: true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Conventions() = %+v, want %+v", got, want)
	}
	if eff.Settings.Ship.Commits.ScopeFrom != ScopeFromNone || eff.Source("ship.branches.issue") != "repository" {
		t.Fatalf("settings = %+v", eff.Settings.Ship)
	}
}

func TestCommitTypesMergeAcrossLayers(t *testing.T) {
	user := Layer{Name: "user", Path: "user.json", Data: []byte(`{"ship": {"commits": {"types": {
		"feat": {"emoji": "🚀"},
		"wip": false,
		"deps": {"emoji": "⬆️"}}}}}`)}
	repo := Layer{Name: "repository", Path: ".github/ship.json", Data: []byte(`{"ship": {"commits": {"types": {
		"feat": {"label": "enhancement"},
		"deps": {"branchPrefix": "chore", "label": ""},
		"style": false}}}}`)}
	eff, err := Merge([]Layer{user, repo})
	if err != nil {
		t.Fatal(err)
	}
	if eff.Source("ship.commits.types") != "repository" {
		t.Errorf("source = %s", eff.Source("ship.commits.types"))
	}
	rules := eff.Settings.Conventions()
	feat, _ := rules.LookupCommitType("feat")
	if feat.Gitmoji != "🚀" || feat.Label != "enhancement" || feat.BranchPrefix != "feat" || feat.ReleaseImpact != convention.MinorRelease {
		t.Errorf("feat = %+v: the layers change different fields of one type", feat)
	}
	for _, removed := range []convention.CommitType{"wip", "style"} {
		if _, ok := rules.LookupCommitType(removed); ok {
			t.Errorf("%s is still in the table", removed)
		}
	}
	types := rules.CommitTypes()
	if last := types[len(types)-1]; last.Type != "deps" || last.Gitmoji != "⬆️" || last.BranchPrefix != "chore" || last.Label != "" {
		t.Errorf("added type = %+v, want it last", last)
	}
	if len(types) != len(convention.CommitTypes())-1 {
		t.Errorf("%d types", len(types))
	}
}

func TestCommitTypeRemovedThenRedefined(t *testing.T) {
	user := Layer{Name: "user", Path: "user.json", Data: []byte(`{"ship": {"commits": {"types": {"fix": false}}}}`)}
	repo := Layer{Name: "repository", Path: ".github/ship.json", Data: []byte(`{"ship": {"commits": {"types": {"fix": {"label": "defect"}}}}}`)}
	eff, err := Merge([]Layer{user, repo})
	if err != nil {
		t.Fatal(err)
	}
	fix, ok := eff.Settings.Conventions().LookupCommitType("fix")
	if !ok || fix.Gitmoji != "🐛" || fix.Label != "defect" {
		t.Fatalf("fix = %+v, %v: a stronger layer brings a removed type back", fix, ok)
	}
}

func TestCommitTypesErrors(t *testing.T) {
	tests := []struct {
		name   string
		types  string
		code   fault.Code
		params map[string]string
	}{
		{"not an object", `["feat"]`, fault.ConfigWrongType, map[string]string{"key": "ship.commits.types"}},
		{"true", `{"feat": true}`, fault.ConfigWrongType, map[string]string{"key": "ship.commits.types.feat"}},
		{"type name", `{"Feat2": {"emoji": "✨"}}`, fault.ConfigInvalidValue,
			map[string]string{"key": "ship.commits.types.Feat2", "allowed": typeNameAllowed}},
		{"unknown field", `{"feat": {"gitmoji": "✨"}}`, fault.ConfigUnknownKey,
			map[string]string{"key": "ship.commits.types.feat.gitmoji", "known": "emoji, branchPrefix, label, release"}},
		{"field type", `{"feat": {"emoji": 1}}`, fault.ConfigWrongType, map[string]string{"expected": "string"}},
		{"empty emoji", `{"feat": {"emoji": " "}}`, fault.ConfigInvalidValue, map[string]string{"key": "ship.commits.types.feat.emoji"}},
		{"branch prefix", `{"feat": {"branchPrefix": "Feature/"}}`, fault.ConfigInvalidValue,
			map[string]string{"key": "ship.commits.types.feat.branchPrefix", "value": "Feature/", "allowed": branchPrefixAllowed}},
		{"branch prefix separator", `{"feat": {"branchPrefix": "feat--x"}}`, fault.ConfigInvalidValue, nil},
		{"label", `{"feat": {"label": "kind/feature"}}`, fault.ConfigInvalidValue,
			map[string]string{"key": "ship.commits.types.feat.label", "allowed": labelAllowed}},
		{"new type without prefix", `{"deps": {"emoji": "⬆️"}}`, fault.ConfigInvalidValue,
			map[string]string{"key": "ship.commits.types.deps", "allowed": "a new type needs an emoji and a branchPrefix"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := `{"ship": {"commits": {"types": ` + tt.types + `}}}`
			_, err := Merge([]Layer{{Name: "repository", Path: ".github/ship.json", Data: []byte(data)}})
			var f *fault.Fault
			if !errors.As(err, &f) || f.Code != tt.code {
				t.Fatalf("error = %v, want %s", err, tt.code)
			}
			for k, want := range tt.params {
				if f.Params[k] != want {
					t.Errorf("%s = %q, want %q", k, f.Params[k], want)
				}
			}
			if f.Params["file"] != ".github/ship.json" {
				t.Errorf("file = %q: the file must be named", f.Params["file"])
			}
		})
	}
}

func TestEveryTypeRemovedIsRefused(t *testing.T) {
	var all []string
	for _, spec := range convention.CommitTypes() {
		all = append(all, `"`+string(spec.Type)+`": false`)
	}
	data := `{"ship": {"commits": {"types": {` + strings.Join(all, ", ") + `}}}}`
	_, err := Merge([]Layer{{Name: "repository", Path: ".github/ship.json", Data: []byte(data)}})
	var f *fault.Fault
	if !errors.As(err, &f) || f.Code != fault.ConfigInvalidValue || f.Params["allowed"] != "at least one commit type left" {
		t.Fatalf("error = %v", err)
	}
}

func TestConventionMinimums(t *testing.T) {
	for key, value := range map[string]string{
		"ship.commits.subjectMaxLength": "19", "ship.commits.headerMaxLength": "29", "ship.commits.bodyWidth": "39",
		"ship.branches.maxWords": "0", "ship.branches.maxLength": "7",
	} {
		if _, err := FlagLayers([]string{key + "=" + value}); err != nil {
			t.Fatal(err)
		}
		layers, _ := FlagLayers([]string{key + "=" + value})
		_, err := Merge(layers)
		var f *fault.Fault
		if !errors.As(err, &f) || f.Code != fault.ConfigInvalidValue || f.Params["key"] != key {
			t.Errorf("%s=%s: error = %v", key, value, err)
		}
	}
	for _, bad := range []string{"ship.commits.scopeFrom=paths", "ship.branches.issue=last", "ship.commits.scopes=API"} {
		layers, err := FlagLayers([]string{bad})
		if err == nil {
			_, err = Merge(layers)
		}
		if !errors.Is(err, &fault.Fault{Code: fault.ConfigInvalidValue}) {
			t.Errorf("%s: error = %v", bad, err)
		}
	}
}

func TestCommitTypesFromEnvironment(t *testing.T) {
	layers, err := EnvironmentLayers([]string{`SHIP_COMMITS_TYPES={"hotfix": {"emoji": "🚑️", "branchPrefix": "hotfix", "label": "bug"}}`})
	if err != nil {
		t.Fatal(err)
	}
	eff, err := Merge(layers)
	if err != nil {
		t.Fatal(err)
	}
	if spec, ok := eff.Settings.Conventions().LookupCommitType("hotfix"); !ok || spec.BranchPrefix != "hotfix" {
		t.Fatalf("hotfix = %+v, %v", spec, ok)
	}
	if eff.Source("ship.commits.types") != "SHIP_COMMITS_TYPES" {
		t.Errorf("source = %s", eff.Source("ship.commits.types"))
	}
	if _, err := EnvironmentLayers([]string{"SHIP_COMMITS_TYPES=feat"}); !errors.Is(err, &fault.Fault{Code: fault.ConfigWrongType}) {
		t.Errorf("not JSON: error = %v", err)
	}
}

func TestCommitTypesRenderRoundTrip(t *testing.T) {
	emoji, prefix := "⬆️", "chore"
	s := Defaults()
	s.Ship.Commits.Types = CommitTypes{"deps": {Emoji: &emoji, BranchPrefix: &prefix}, "wip": {Removed: true}}
	rendered := Render(s)
	if !strings.Contains(string(rendered), `"types": {"deps":{"emoji":"⬆️","branchPrefix":"chore"},"wip":false}`) {
		t.Fatalf("Render =\n%s", rendered)
	}
	eff, err := Merge([]Layer{{Name: "repository", Data: rendered}})
	if err != nil || !reflect.DeepEqual(eff.Settings, s) {
		t.Fatalf("round trip = %+v, %v", eff.Settings.Ship.Commits, err)
	}
}

func TestCommitTypesSchemaAndReference(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal(Schema(), &schema); err != nil {
		t.Fatal(err)
	}
	commits := schema["properties"].(map[string]any)["ship"].(map[string]any)["properties"].(map[string]any)["commits"].(map[string]any)
	types := commits["properties"].(map[string]any)["types"].(map[string]any)
	if types["type"] != "object" || types["additionalProperties"] == nil {
		t.Fatalf("types schema = %v", types)
	}
	info, ok := Describe("ship.commits.types")
	if !ok || info.Type != "object" || info.Default != "{}" || info.Env != "SHIP_COMMITS_TYPES" {
		t.Fatalf("Describe = %+v", info)
	}
	if !strings.Contains(string(Reference()), "| `ship.commits.types` | object: type → `{emoji, branchPrefix, label, release}` or `false` | `{}` |") {
		t.Error("the reference must describe ship.commits.types")
	}
	if !slices.Contains(Keys(), "ship.branches.issue") {
		t.Error("ship.branches.issue must be a key")
	}
}

func TestExplainCommitTypesOfOneLayer(t *testing.T) {
	// The repository only labels a type the user file adds: alone, the
	// repository layer is incomplete, which Explain must accept.
	user := Layer{Name: "user", Path: "user.json", Data: []byte(`{"ship": {"commits": {"types": {"deps": {"emoji": "⬆️", "branchPrefix": "chore"}}}}}`)}
	repo := Layer{Name: "repository", Path: ".github/ship.json", Data: []byte(`{"ship": {"commits": {"types": {"deps": {"label": "dependencies"}}}}}`)}
	x, err := Explain([]Layer{user, repo}, Target{}, "ship.commits.types")
	if err != nil {
		t.Fatal(err)
	}
	if len(x.Layers) != 2 || x.Source != "repository" || !strings.Contains(x.Value, `"label":"dependencies"`) {
		t.Fatalf("Explain = %+v", x)
	}
}

func TestPolicyLocksCommitTypes(t *testing.T) {
	policy := Layer{Name: PolicyLayerName, Path: "/etc/ship-it/policy.json",
		Data: []byte(`{"organization": "Acme", "enforce": {"ship": {"commits": {"types": {"wip": false}}}}}`)}
	eff, err := Merge([]Layer{policy})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := eff.Settings.Conventions().LookupCommitType("wip"); ok || eff.Source("ship.commits.types") != PolicyLayerName {
		t.Fatalf("policy = %+v", eff.Policy)
	}
}

func TestCommitTypeRelease(t *testing.T) {
	repo := Layer{Name: "repository", Path: ".github/ship.json", Data: []byte(`{"ship": {"commits": {"types": {
		"deps": {"emoji": "⬆️", "branchPrefix": "chore", "release": "patch"},
		"docs": {"release": "minor"}}}}}`)}
	eff, err := Merge([]Layer{repo})
	if err != nil {
		t.Fatal(err)
	}
	rules := eff.Settings.Conventions()
	for name, want := range map[convention.CommitType]convention.ReleaseImpact{
		"deps": convention.PatchRelease, "docs": convention.MinorRelease, "feat": convention.MinorRelease, "test": convention.NoRelease,
	} {
		if spec, _ := rules.LookupCommitType(name); spec.ReleaseImpact != want {
			t.Errorf("%s: %s, want %s", name, spec.ReleaseImpact, want)
		}
	}
	bad := Layer{Name: "repository", Path: "f", Data: []byte(`{"ship": {"commits": {"types": {"feat": {"release": "huge"}}}}}`)}
	_, err = Merge([]Layer{bad})
	var f *fault.Fault
	if !errors.As(err, &f) || f.Code != fault.ConfigInvalidValue || f.Params["key"] != "ship.commits.types.feat.release" ||
		f.Params["allowed"] != "none, patch, minor, major" {
		t.Fatalf("err = %v", err)
	}
}
