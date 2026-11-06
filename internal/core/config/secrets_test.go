// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package config

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/secrets"
)

func TestDisabledRulesAreTheBuiltinRules(t *testing.T) {
	var ids []string
	for _, id := range secrets.BuiltinRules() {
		ids = append(ids, string(id))
	}
	if got := strings.Split(builtinSecretRules, ","); !slices.Equal(got, ids) {
		t.Fatalf("itemEnum of ship.secrets.disabledRules = %v, want secrets.BuiltinRules() = %v", got, ids)
	}
	info, _ := Describe("ship.secrets.disabledRules")
	if !slices.Equal(info.Allowed, ids) || info.Type != "array" {
		t.Fatalf("Describe = %+v", info)
	}
}

func TestSecretsSettings(t *testing.T) {
	fingerprint := strings.Repeat("0a", 32)
	eff, err := Merge([]Layer{{Name: "repository", Path: ".github/ship.json", Data: []byte(`{"ship": {"secrets": {
		"rules": [{"id": "acme_token", "pattern": "(?i)acme_[0-9a-f]{24}", "keywords": ["acme_"]}],
		"disabledRules": ["secret_assignment"],
		"allow": ["` + fingerprint + `"]}}}`)}})
	if err != nil {
		t.Fatal(err)
	}
	s := eff.Settings.Ship.Secrets
	want := []SecretRule{{ID: "acme_token", Pattern: "(?i)acme_[0-9a-f]{24}", Keywords: []string{"acme_"}}}
	if !reflect.DeepEqual(s.Rules, want) || !slices.Equal(s.DisabledRules, []string{"secret_assignment"}) ||
		!slices.Equal(s.Allow, []string{fingerprint}) || eff.Source("ship.secrets.rules") != "repository" {
		t.Fatalf("secrets = %+v", s)
	}
	// What the configuration accepts, the scan compiles.
	for _, r := range s.Rules {
		if _, err := secrets.NewRule(r.ID, r.Pattern, r.Keywords); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSecretsSettingsErrors(t *testing.T) {
	rule := func(body string) string { return `{"ship": {"secrets": {"rules": [` + body + `]}}}` }
	tests := []struct {
		name, data string
		code       fault.Code
		key        string
		allowed    string
	}{
		{"unknown rule id", `{"ship": {"secrets": {"disabledRules": ["aws"]}}}`, fault.ConfigInvalidValue,
			"ship.secrets.disabledRules", strings.ReplaceAll(builtinSecretRules, ",", ", ")},
		{"fingerprint", `{"ship": {"secrets": {"allow": ["AKIA1234"]}}}`, fault.ConfigInvalidValue, "ship.secrets.allow", "SHA-256 fingerprints"},
		{"rules not an array", `{"ship": {"secrets": {"rules": {}}}}`, fault.ConfigWrongType, "ship.secrets.rules", ""},
		{"rule not an object", rule(`"x"`), fault.ConfigWrongType, "ship.secrets.rules[0]", ""},
		{"unknown rule key", rule(`{"id": "a", "regex": "x"}`), fault.ConfigUnknownKey, "ship.secrets.rules[0].regex", ""},
		{"built-in id", rule(`{"id": "aws_key", "pattern": "x", "keywords": ["x"]}`), fault.ConfigInvalidValue, "ship.secrets.rules[0].id", "built-in"},
		{"id format", rule(`{"id": "Acme Token", "pattern": "x", "keywords": ["x"]}`), fault.ConfigInvalidValue, "ship.secrets.rules[0].id", "lower-case"},
		{"no id", rule(`{"pattern": "x", "keywords": ["x"]}`), fault.ConfigInvalidValue, "ship.secrets.rules[0].id", ""},
		{"bad pattern", rule(`{"id": "a", "pattern": "(", "keywords": ["x"]}`), fault.ConfigInvalidValue, "ship.secrets.rules[0].pattern", "RE2"},
		{"empty match", rule(`{"id": "a", "pattern": "x*", "keywords": ["x"]}`), fault.ConfigInvalidValue, "ship.secrets.rules[0].pattern", "empty"},
		{"no keyword", rule(`{"id": "a", "pattern": "x"}`), fault.ConfigInvalidValue, "ship.secrets.rules[0].keywords", "keyword"},
		{"empty keyword", rule(`{"id": "a", "pattern": "x", "keywords": [""]}`), fault.ConfigInvalidValue, "ship.secrets.rules[0].keywords", "keyword"},
		{"duplicate id", rule(`{"id": "a", "pattern": "x", "keywords": ["x"]}, {"id": "a", "pattern": "y", "keywords": ["y"]}`),
			fault.ConfigInvalidValue, "ship.secrets.rules[1].id", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Merge([]Layer{{Name: "repository", Path: ".github/ship.json", Data: []byte(tt.data)}})
			var f *fault.Fault
			if !errors.As(err, &f) || f.Code != tt.code || f.Params["key"] != tt.key || !strings.Contains(f.Params["allowed"], tt.allowed) {
				t.Fatalf("error = %v (%v)", err, f)
			}
		})
	}
}

func TestSecretRulesFromTheEnvironment(t *testing.T) {
	layers, err := EnvironmentLayers([]string{`SHIP_SECRETS_RULES=[{"id": "a", "pattern": "x+", "keywords": ["x"]}]`, "SHIP_SECRETS_DISABLEDRULES=aws_key,npm_token"})
	if err != nil {
		t.Fatal(err)
	}
	eff, err := Merge(layers)
	if err != nil || len(eff.Settings.Ship.Secrets.Rules) != 1 || !slices.Equal(eff.Settings.Ship.Secrets.DisabledRules, []string{"aws_key", "npm_token"}) {
		t.Fatalf("settings = %+v, %v", eff.Settings.Ship.Secrets, err)
	}
	if _, err := FlagLayers([]string{"ship.secrets.rules=a,b"}); !errors.Is(err, &fault.Fault{Code: fault.ConfigWrongType}) {
		t.Fatalf("a list of objects reads as JSON only: %v", err)
	}
}

func TestListsOfObjectsInTheSchemaAndReference(t *testing.T) {
	schema := string(Schema())
	for _, want := range []string{`"required": [`, `"keywords"`, `"enum": [`, `"sensitive_file"`, `"^[0-9a-f]{64}$"`} {
		if !strings.Contains(schema, want) {
			t.Errorf("schema lacks %s", want)
		}
	}
	reference := string(Reference())
	for _, want := range []string{"| `ship.secrets.rules` | list of `{id, pattern, keywords}` |", "`SHIP_SECRETS_DISABLEDRULES`"} {
		if !strings.Contains(reference, want) {
			t.Errorf("reference lacks %s", want)
		}
	}
	if info, _ := Describe("ship.secrets.rules"); info.Type != "objects" {
		t.Fatalf("Describe = %+v", info)
	}
}
