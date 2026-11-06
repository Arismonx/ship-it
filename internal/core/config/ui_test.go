// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package config

import (
	"errors"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
)

func TestUISettings(t *testing.T) {
	user := Layer{Name: "user", Kind: KindUser, Path: "config.json", Data: []byte(`{"ui": {"language": "fr", "accessible": true},
		"contexts": [{"match": {"owner": "acme"}, "ui": {"color": "never"}}]}`)}
	repo := Layer{Name: "repository", Path: ".github/ship.json", Data: []byte(`{"ship": {"publish": {"mode": "never"}}}`)}
	flags, err := FlagLayers([]string{"ui.reviewScreen=false"})
	if err != nil {
		t.Fatal(err)
	}
	eff, err := MergeFor(append([]Layer{user, repo}, flags...), Target{Owner: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	want := UISettings{Language: "fr", ReviewScreen: false, Accessible: true, Color: ColorNever}
	if eff.Settings.UI != want || eff.Source("ui.color") != "user contexts[0]" || eff.Source("ui.reviewScreen") != "--set ui.reviewScreen" {
		t.Fatalf("ui = %+v, sources %v", eff.Settings.UI, eff.Sources)
	}
	env, err := EnvironmentLayers([]string{"SHIP_UI_COLOR=always", "SHIP_UI_REVIEWSCREEN=1"})
	if err != nil {
		t.Fatal(err)
	}
	eff, err = MergeFor(append([]Layer{user, repo}, env...), Target{Owner: "acme"})
	if err != nil || eff.Settings.UI.Color != ColorAlways || !eff.Settings.UI.ReviewScreen || eff.Source("ui.color") != "SHIP_UI_COLOR" {
		t.Fatalf("ui = %+v, %v", eff.Settings.UI, err)
	}
	if Defaults().UI != (UISettings{Language: LanguageAuto, ReviewScreen: true, Color: ColorAuto}) {
		t.Fatalf("defaults = %+v", Defaults().UI)
	}
}

func TestUISettingsErrors(t *testing.T) {
	for _, data := range []string{`{"ui": {"language": "de"}}`, `{"ui": {"color": "yes"}}`} {
		_, err := Merge([]Layer{{Name: "user", Path: "f", Data: []byte(data)}})
		var f *fault.Fault
		if !errors.As(err, &f) || f.Code != fault.ConfigInvalidValue || f.Params["allowed"] == "" {
			t.Errorf("%s: %v", data, err)
		}
	}
	// Interface settings are personal: a file shared with a team (the
	// repository's, a preset, an override) refuses them and says where they go.
	for _, tt := range []struct {
		layer Layer
		key   string
	}{
		{Layer{Name: "repository", Kind: KindRepository, Path: "f", Data: []byte(`{"ui": {"color": "never"}}`)}, "ui"},
		{Layer{Name: "ship-it:x", Kind: KindPreset, Path: "f", Data: []byte(`{"ui": {"language": "fr"}}`)}, "ui"},
		{Layer{Name: "repository", Kind: KindRepository, Path: "f",
			Data: []byte(`{"overrides": [{"paths": ["docs/**"], "ui": {"color": "never"}}]}`)}, "overrides[0].ui"},
	} {
		_, err := Merge([]Layer{tt.layer})
		var f *fault.Fault
		if !errors.As(err, &f) || f.Code != fault.ConfigUnknownKey || f.Key != "config.personal_only" || f.Params["key"] != tt.key {
			t.Errorf("%s: %v", tt.layer.Data, err)
		}
	}
	if !IsPersonalKey("ui.color") || IsPersonalKey("ship.publish.mode") || IsPersonalKey("uix") {
		t.Fatal("IsPersonalKey")
	}
	if _, err := FlagLayers([]string{"ui.language=fr"}); err != nil {
		t.Fatalf("--set ui.language: %v", err)
	}
}

func TestUIInTheSchemasAndReference(t *testing.T) {
	if strings.Contains(string(Schema()), `"ui": {`) || !strings.Contains(string(UserSchema()), `"ui": {`) {
		t.Fatal("ui belongs to the user schema only")
	}
	reference := string(Reference())
	for _, want := range []string{"| `ui.color` | `auto` \\| `always` \\| `never` |", "`SHIP_UI_LANGUAGE`", "| `ui.reviewScreen` | boolean | `true` |"} {
		if !strings.Contains(reference, want) {
			t.Errorf("reference lacks %s", want)
		}
	}
}
