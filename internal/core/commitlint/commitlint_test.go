// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package commitlint

import (
	"reflect"
	"slices"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/convention"
	"github.com/GregoireF/ship-it/internal/core/fault"
)

// printed is the shape of `commitlint --print-config json` (19.8.1), trimmed:
// the headerPattern RegExp prints as {}.
const printed = `{"extends":["@commitlint/config-conventional"],"parserPreset":{"parserOpts":{"headerPattern":{},"headerCorrespondence":["type","scope","subject"]}},
"rules":{"body-leading-blank":[1,"always"],"header-max-length":[2,"always",100],"subject-case":[2,"never",["sentence-case","start-case","pascal-case","upper-case"]],
"subject-empty":[2,"never"],"subject-full-stop":[2,"never","."],"type-enum":[2,"always",["build","chore","ci","docs","feat","fix","perf","refactor","revert","style","test"]],
"scope-enum":[1,"always",["api"]],"disabled":[0]}}`

func TestParseConfig(t *testing.T) {
	cfg, err := ParseConfig([]byte("npm warn something\n" + printed))
	if err != nil {
		t.Fatal(err)
	}
	if r := cfg.Rules["type-enum"]; r.Level != 2 || r.Applicable != "always" || len(r.strings()) != 11 {
		t.Fatalf("type-enum = %+v", r)
	}
	if r := cfg.Rules["header-max-length"]; r.Level != 2 {
		t.Fatalf("header-max-length = %+v", r)
	} else if n, ok := r.number(); !ok || n != 100 {
		t.Fatalf("limit = %d", n)
	}
	if r := cfg.Rules["disabled"]; r.Level != 0 || r.Applicable != "always" {
		t.Fatalf("a bare level = %+v", r)
	}
	if _, err := ParseConfig([]byte("not json")); err == nil {
		t.Fatal("garbage must fail")
	}
}

func TestDetection(t *testing.T) {
	if !DeclaredInPackageJSON([]byte(`{"name":"x","commitlint":{"extends":["@commitlint/config-conventional"]}}`)) {
		t.Fatal("package.json field")
	}
	if DeclaredInPackageJSON([]byte(`{"name":"x"}`)) || DeclaredInPackageJSON([]byte(`nope`)) {
		t.Fatal("no field")
	}
	if len(ConfigFiles()) != 16 || len(Binaries()) != 2 {
		t.Fatal("documented file names")
	}
	if got := PrintConfigCommand(); len(got) != 6 || got[1] != "--no" || got[2] != "--" {
		t.Fatalf("never downloads: %q", got)
	}
}

func rules(t *testing.T, json string) Config {
	t.Helper()
	cfg, err := ParseConfig([]byte(`{"rules":` + json + `}`))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestAdapt(t *testing.T) {
	security := convention.Header{Type: convention.SecurityFix, Scope: "auth", Subject: "rotate keys"}
	tests := []struct {
		name    string
		header  convention.Header
		config  string
		want    string
		reason  string
		faultKy string
	}{
		{"nothing to change", convention.Header{Type: convention.Feature, Subject: "add vat"}, `{"type-enum":[2,"always",["feat"]]}`, "✨ feat: add vat", "", ""},
		{"warnings are not followed", convention.Header{Type: convention.SecurityFix, Subject: "x"}, `{"type-enum":[1,"always",["feat"]]}`, "🔒️ security: x", "", ""},
		{"security becomes fix, keeping its gitmoji", security, `{"type-enum":[2,"always",["feat","fix"]]}`, "🔒️ fix(auth): rotate keys", "plan.reason.commitlint.type", ""},
		{"type-enum never", convention.Header{Type: convention.WorkInProgress, Subject: "x"}, `{"type-enum":[2,"never",["wip"]]}`, "🚧 chore: x", "plan.reason.commitlint.type", ""},
		{"ambiguous type is refused", convention.Header{Type: convention.Performance, Subject: "x"}, `{"type-enum":[2,"always",["feat","fix","refactor"]]}`, "", "", "deliver.commitlint.type_not_allowed"},
		{"the only allowed type is taken", convention.Header{Type: convention.Feature, Subject: "x"}, `{"type-enum":[2,"always",["fix"]]}`, "✨ fix: x", "plan.reason.commitlint.type", ""},
		{"no known type allowed", convention.Header{Type: convention.Feature, Subject: "x"}, `{"type-enum":[2,"always",["story"]]}`, "", "", "deliver.commitlint.type_not_allowed"},
		{"scope not allowed is dropped", security, `{"scope-enum":[2,"always",["api"]]}`, "🔒️ security: rotate keys", "plan.reason.commitlint.scope", ""},
		{"scope required", convention.Header{Type: convention.Feature, Subject: "x"}, `{"scope-empty":[2,"never"]}`, "", "", "deliver.commitlint.scope_required"},
		{"subject full stop always", convention.Header{Type: convention.Feature, Subject: "add vat"}, `{"subject-full-stop":[2,"always","."]}`, "✨ feat: add vat.", "plan.reason.commitlint.subject", ""},
		{"subject full stop never", convention.Header{Type: convention.Feature, Subject: "add vat."}, `{"subject-full-stop":[2,"never","."]}`, "✨ feat: add vat", "plan.reason.commitlint.subject", ""},
		{"sentence case required", convention.Header{Type: convention.Feature, Subject: "add vat"}, `{"subject-case":[2,"always","sentence-case"]}`, "✨ feat: Add vat", "plan.reason.commitlint.subject", ""},
		{"sentence case refused", convention.Header{Type: convention.Feature, Subject: "Add vat"}, `{"subject-case":[2,"never",["sentence-case","upper-case"]]}`, "✨ feat: add vat", "plan.reason.commitlint.subject", ""},
		{"an acronym is not sentence case", convention.Header{Type: convention.Feature, Subject: "API keys"}, `{"subject-case":[2,"never",["sentence-case","start-case"]]}`, "✨ feat: API keys", "", ""},
		{"lower case required", convention.Header{Type: convention.Feature, Subject: "add API keys"}, `{"subject-case":[2,"always",["lower-case"]]}`, "✨ feat: add api keys", "plan.reason.commitlint.subject", ""},
		{"long header drops the scope", convention.Header{Type: convention.Feature, Scope: "billing", Subject: "add vat"}, `{"header-max-length":[2,"always",17]}`, "✨ feat: add vat", "plan.reason.commitlint.scope", ""},
		{"too long even so", convention.Header{Type: convention.Feature, Subject: "add the vat rates"}, `{"header-max-length":[2,"always",10]}`, "", "", "deliver.commitlint.too_long"},
		{"subject too long", convention.Header{Type: convention.Feature, Subject: "add vat"}, `{"subject-max-length":[2,"always",5]}`, "", "", "deliver.commitlint.too_long"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reasons, f := Adapt(tt.header, convention.GitmojiFirst, rules(t, tt.config), convention.Rules{})
			if tt.faultKy != "" {
				if f == nil || f.Key != tt.faultKy || f.Code != fault.CommitlintUnsatisfiable {
					t.Fatalf("fault = %v", f)
				}
				return
			}
			if f != nil {
				t.Fatal(f)
			}
			if title := convention.Format(got, convention.GitmojiFirst); title != tt.want {
				t.Fatalf("title = %q, want %q", title, tt.want)
			}
			if (tt.reason == "") != (len(reasons) == 0) || (tt.reason != "" && reasons[0].Key != tt.reason) {
				t.Fatalf("reasons = %+v", reasons)
			}
		})
	}
}

func TestTypeChoices(t *testing.T) {
	tests := []struct {
		name   string
		t      convention.CommitType
		config string
		want   []convention.CommitType
	}{
		{"allowed", convention.Performance, `{"type-enum":[2,"always",["perf"]]}`, nil},
		{"no type-enum", convention.Performance, `{}`, nil},
		{"a warning only", convention.Performance, `{"type-enum":[1,"always",["fix"]]}`, nil},
		{"one close type: Adapt takes it", convention.SecurityFix, `{"type-enum":[2,"always",["feat","fix"]]}`, nil},
		{"several close types", convention.Performance, `{"type-enum":[2,"always",["feat","fix","refactor"]]}`,
			[]convention.CommitType{convention.Refactoring, convention.BugFix}},
		{"no close type: every allowed one ship-it knows", convention.Feature, `{"type-enum":[2,"always",["fix","feature","chore"]]}`,
			[]convention.CommitType{convention.BugFix, convention.Maintenance}},
		{"none ship-it knows", convention.Feature, `{"type-enum":[2,"always",["feature"]]}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TypeChoices(tt.t, rules(t, tt.config), convention.Rules{})
			if !slices.Equal(got, tt.want) {
				t.Fatalf("TypeChoices = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSubjectLengthSeesTheGitmojiAfterTheType(t *testing.T) {
	h := convention.Header{Type: convention.Feature, Subject: "add"}
	cfg := rules(t, `{"subject-max-length":[2,"always",4]}`)
	if _, _, f := Adapt(h, convention.GitmojiFirst, cfg, convention.Rules{}); f != nil {
		t.Fatal(f)
	}
	if _, _, f := Adapt(h, convention.GitmojiAfterType, cfg, convention.Rules{}); f == nil {
		t.Fatal("\"✨ add\" is 5 code units for commitlint's parser")
	}
}

func TestLengthCountsUTF16(t *testing.T) {
	if Length("✨ a") != 3 || Length("🐛 a") != 4 || Length("é") != 1 {
		t.Fatal(Length("✨ a"), Length("🐛 a"))
	}
}

func TestAdaptFollowsTheTypeTable(t *testing.T) {
	emoji, prefix := "⬆️", "chore"
	conventions := convention.Rules{Types: convention.WithTypes([]convention.TypeChange{
		{Type: "deps", Gitmoji: &emoji, BranchPrefix: &prefix}, {Type: "chore", Remove: true}})}
	cfg := rules(t, `{"type-enum":[2,"always",["feat","fix","deps","chore"]]}`)
	// build's close types are chore (removed from the table) and ci (not
	// allowed): the choice is among the allowed types of the table.
	got := TypeChoices(convention.BuildSystem, cfg, conventions)
	if !reflect.DeepEqual(got, []convention.CommitType{"feat", "fix", "deps"}) {
		t.Errorf("TypeChoices = %v", got)
	}
	one := rules(t, `{"type-enum":[2,"always",["deps"]]}`)
	h, _, f := Adapt(convention.Header{Type: convention.BuildSystem, Subject: "bump yaml"}, convention.GitmojiFirst, one, conventions)
	if f != nil || h.Type != "deps" {
		t.Errorf("Adapt = %+v, %v", h, f)
	}
}
