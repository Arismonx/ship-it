// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package config

import (
	"errors"
	"reflect"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
)

func TestDescribe(t *testing.T) {
	info, ok := Describe("ship.pullRequest.maxLines")
	if !ok || info.Type != "integer" || info.Minimum == nil || *info.Minimum != 1 || info.Default != "400" ||
		info.Env != "SHIP_PULLREQUEST_MAXLINES" || info.Doc == "" {
		t.Errorf("Describe(maxLines) = %+v, %v", info, ok)
	}
	info, _ = Describe("ship.publish.mode")
	if info.Type != "string" || !reflect.DeepEqual(info.Allowed, []string{"auto", "never"}) || info.Default != `"auto"` {
		t.Errorf("Describe(publish) = %+v", info)
	}
	if info, _ := Describe("github.labels.areas"); info.Type != "array" || info.Default != "[]" {
		t.Errorf("Describe(areas) = %+v", info)
	}
	for _, key := range []string{"ship", "ship.pullRequest", "nope", "ship.publish.x", ""} {
		if _, ok := Describe(key); ok {
			t.Errorf("Describe(%q) accepted a non-leaf key", key)
		}
	}
}

func TestSuggest(t *testing.T) {
	for typed, want := range map[string]string{
		"ship.pulRequest.maxLines": "ship.pullRequest.maxLines",
		"SHIP.PUBLISH.MODE":        "ship.publish.mode",
		"maxlines":                 "ship.pullRequest.maxLines",
		"managedby":                "github.managedBy",
		"zzzzzzzzzzzzzz":           "",
	} {
		if got := Suggest(typed); got != want {
			t.Errorf("Suggest(%q) = %q, want %q", typed, got, want)
		}
	}
	var f *fault.Fault
	if err := error(UnknownSetting("ship.publish.mod")); !errors.As(err, &f) || f.Code != fault.ConfigUnknownKey || f.Params["suggestion"] != "ship.publish.mode" {
		t.Errorf("UnknownSetting = %v", err)
	}
	if f := UnknownSetting("zzzzzzzzzzzzzz"); f.Key != "config.unknown_setting_none" {
		t.Errorf("UnknownSetting without suggestion = %v", f)
	}
}

func TestAssignmentValue(t *testing.T) {
	for _, tt := range []struct{ key, text, want string }{
		{"ship.gates.enabled", "no", "false"},
		{"ship.pullRequest.maxLines", " 600 ", "600"},
		{"ship.publish.mode", "never", `"never"`},
		{"github.labels.areas", "cli, core", `["cli","core"]`},
		{"ship.secrets.ignorePaths", `["a/<b>/**"]`, `["a/<b>/**"]`},
	} {
		got, err := AssignmentValue(tt.key, tt.text)
		if err != nil || got != tt.want {
			t.Errorf("AssignmentValue(%s, %q) = %s, %v; want %s", tt.key, tt.text, got, err, tt.want)
		}
	}
	var f *fault.Fault
	if _, err := AssignmentValue("ship.pullRequest.maxLines", "many"); !errors.As(err, &f) || f.Code != fault.ConfigWrongType {
		t.Errorf("AssignmentValue(maxLines, many) = %v", err)
	}
}

func TestExplain(t *testing.T) {
	layers := []Layer{
		{Name: "user", Path: "/u/config.json", Data: []byte(`{"ship": {"pullRequest": {"maxLines": 300}}}`)},
		{Name: "repository", Path: "/r/.github/ship.json", Data: []byte(`{
			"ship": {"pullRequest": {"maxLines": 500}},
			"overrides": [{"paths": ["docs/**"], "ship": {"pullRequest": {"maxLines": 2000}}}]
		}`)},
	}
	x, err := Explain(layers, Target{Paths: []string{"docs/a.md"}}, "ship.pullRequest.maxLines")
	if err != nil {
		t.Fatal(err)
	}
	want := []LayerValue{
		{Source: "user", Path: "/u/config.json", Value: "300"},
		{Source: "repository overrides[0]", Path: "/r/.github/ship.json", Value: "2000"},
	}
	if !reflect.DeepEqual(x.Layers, want) || x.Value != "2000" || x.Source != "repository overrides[0]" || x.SourcePath != "/r/.github/ship.json" {
		t.Errorf("Explain = %+v", x)
	}

	policy := Layer{Name: PolicyLayerName, Path: "/etc/ship-it/policy.json",
		Data: []byte(`{"organization": "Acme", "enforce": {"ship": {"gates": {"enabled": true}}}}`)}
	x, err = Explain(append(layers, policy), Target{}, "ship.gates.enabled")
	if err != nil || x.Source != PolicyLayerName || len(x.Layers) != 1 || x.Policy == nil || x.Value != "true" {
		t.Errorf("Explain(policy) = %+v, %v", x, err)
	}
	x, err = Explain(nil, Target{}, "ship.publish.mode")
	if err != nil || x.Source != "default" || x.Value != `"auto"` || len(x.Layers) != 0 {
		t.Errorf("Explain(defaults) = %+v, %v", x, err)
	}
	if _, err := Explain(nil, Target{}, "ship.publsh"); !errors.Is(err, &fault.Fault{Code: fault.ConfigUnknownKey}) {
		t.Errorf("Explain(unknown) = %v", err)
	}
}
