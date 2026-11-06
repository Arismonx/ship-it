// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package config

import (
	"errors"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
)

func layer(name string, kind LayerKind, data string) Layer {
	return Layer{Name: name, Path: name + ".json", Kind: kind, Data: []byte(data)}
}

func mustMerge(t *testing.T, target Target, layers ...Layer) Effective {
	t.Helper()
	eff, err := MergeFor(layers, target)
	if err != nil {
		t.Fatal(err)
	}
	return eff
}

func TestPrecedence(t *testing.T) {
	env, err := EnvironmentLayers([]string{"SHIP_PULLREQUEST_MAXLINES=500", "PATH=/bin"})
	if err != nil {
		t.Fatal(err)
	}
	flags, err := FlagLayers([]string{"ship.pullRequest.maxLines=600"})
	if err != nil {
		t.Fatal(err)
	}
	user := layer("user", KindUser, `{"ship": {"pullRequest": {"maxLines": 100}}}`)
	repo := layer("repository", KindRepository, `{"ship": {"pullRequest": {"maxLines": 200}}}`)

	for _, tt := range []struct {
		layers []Layer
		want   int
		source string
	}{
		{[]Layer{user}, 100, "user"},
		{[]Layer{user, repo}, 200, "repository"},
		{append([]Layer{user, repo}, env...), 500, "SHIP_PULLREQUEST_MAXLINES"},
		{append(append([]Layer{user, repo}, env...), flags...), 600, "--set ship.pullRequest.maxLines"},
	} {
		eff := mustMerge(t, Target{}, tt.layers...)
		if got := eff.Settings.Ship.PullRequest.MaxLines; got != tt.want || eff.Source("ship.pullRequest.maxLines") != tt.source {
			t.Errorf("maxLines = %d from %q, want %d from %q", got, eff.Source("ship.pullRequest.maxLines"), tt.want, tt.source)
		}
	}
}

func TestOverridesApplyWhenEveryFileMatches(t *testing.T) {
	repo := layer("repository", KindRepository, `{
	  "ship": {"gates": {"enabled": true}},
	  "overrides": [
	    {"paths": ["docs/**", "*.md"], "ship": {"gates": {"enabled": false}}},
	    {"paths": ["apps/web/**"], "ship": {"pullRequest": {"maxLines": 800}}}
	  ]
	}`)
	for _, tt := range []struct {
		paths []string
		gates bool
		lines int
	}{
		{nil, true, 400},
		{[]string{"docs/a.md", "README.md"}, false, 400},
		{[]string{"docs/a.md", "main.go"}, true, 400}, // not every file
		{[]string{"apps/web/src/x.ts"}, true, 800},
	} {
		eff := mustMerge(t, Target{Paths: tt.paths}, repo)
		if eff.Settings.Ship.Gates.Enabled != tt.gates || eff.Settings.Ship.PullRequest.MaxLines != tt.lines {
			t.Errorf("paths %v: gates %v, maxLines %d", tt.paths, eff.Settings.Ship.Gates.Enabled, eff.Settings.Ship.PullRequest.MaxLines)
		}
	}
	eff := mustMerge(t, Target{Paths: []string{"docs/a.md"}}, repo)
	if got := eff.Source("ship.gates.enabled"); got != "repository overrides[0]" {
		t.Errorf("source = %q", got)
	}
}

func TestALaterLayerBeatsTheOverridesOfAPreset(t *testing.T) {
	preset := layer("ship-it:test", KindPreset, `{"overrides": [{"paths": ["docs/**"], "ship": {"gates": {"enabled": false}}}]}`)
	repo := layer("repository", KindRepository, `{"ship": {"gates": {"enabled": true}}}`)
	eff := mustMerge(t, Target{Paths: []string{"docs/a.md"}}, preset, repo)
	if !eff.Settings.Ship.Gates.Enabled {
		t.Fatal("an explicit setting of the repository must beat a preset's override")
	}
}

func TestContexts(t *testing.T) {
	user := layer("user", KindUser, `{
	  "ship": {"issues": {"keyword": "Closes"}},
	  "contexts": [
	    {"match": {"owner": "acme-corp"}, "ship": {"issues": {"keyword": "Fixes"}}},
	    {"match": {"path": "~/clients/gouv/**"}, "ship": {"publish": {"mode": "never"}}},
	    {"match": {"host": "github.example.gouv.fr", "owner": "dinum"}, "ship": {"pullRequest": {"maxLines": 200}}}
	  ]
	}`)
	for _, tt := range []struct {
		target  Target
		keyword string
		publish PublishMode
		lines   int
	}{
		{Target{}, "Closes", PublishAuto, 400},
		{Target{Owner: "ACME-corp", Host: "github.com"}, "Fixes", PublishAuto, 400},
		{Target{Dir: `C:\Users\ada\clients\gouv\site`, Home: `C:\Users\ada`, FoldCase: true}, "Closes", PublishNever, 400},
		{Target{Dir: "/home/ada/clients/gouv/site", Home: "/home/ada"}, "Closes", PublishNever, 400},
		{Target{Dir: "/home/ada/other/gouv/site", Home: "/home/ada"}, "Closes", PublishAuto, 400},
		{Target{Owner: "dinum", Host: "github.com"}, "Closes", PublishAuto, 400}, // both criteria must hold
		{Target{Owner: "dinum", Host: "github.example.gouv.fr"}, "Closes", PublishAuto, 200},
	} {
		eff := mustMerge(t, tt.target, user)
		s := eff.Settings.Ship
		if s.Issues.Keyword != tt.keyword || s.Publish.Mode != tt.publish || s.PullRequest.MaxLines != tt.lines {
			t.Errorf("%+v: keyword %s, publish %s, maxLines %d", tt.target, s.Issues.Keyword, s.Publish, s.PullRequest.MaxLines)
		}
	}
}

func TestBlocksAreValidatedEvenWhenTheyDoNotMatch(t *testing.T) {
	for name, tt := range map[string]struct {
		layer Layer
		code  fault.Code
		key   string
	}{
		"context on a repository": {layer("repository", KindRepository, `{"contexts": []}`), fault.ConfigUnknownKey, "contexts"},
		"override in a user file": {layer("user", KindUser, `{"overrides": []}`), fault.ConfigUnknownKey, "overrides"},
		"bad type in a context":   {layer("user", KindUser, `{"contexts": [{"match": {"owner": "x"}, "ship": {"publish": {"mode": 1}}}]}`), fault.ConfigWrongType, "ship.publish.mode"},
		"empty match":             {layer("user", KindUser, `{"contexts": [{"match": {}}]}`), fault.ConfigInvalidValue, "contexts[0].match"},
		"unknown criterion":       {layer("user", KindUser, `{"contexts": [{"match": {"org": "x"}}]}`), fault.ConfigUnknownKey, "contexts[0].match.org"},
		"no paths":                {layer("repository", KindRepository, `{"overrides": [{"ship": {}}]}`), fault.ConfigInvalidValue, "overrides[0].paths"},
		"bad glob":                {layer("repository", KindRepository, `{"overrides": [{"paths": ["["]}]}`), fault.ConfigInvalidValue, "overrides[0].paths"},
		"github in an override":   {layer("repository", KindRepository, `{"overrides": [{"paths": ["a"], "github": {}}]}`), fault.ConfigUnknownKey, "overrides[0].github"},
		"bad extends":             {layer("repository", KindRepository, `{"extends": ["../outside.json"]}`), fault.ConfigInvalidValue, "extends"},
	} {
		_, err := MergeFor([]Layer{tt.layer}, Target{})
		var f *fault.Fault
		if !errors.As(err, &f) || f.Code != tt.code || f.Params["key"] != tt.key {
			t.Errorf("%s: err = %v, want %s on %s", name, err, tt.code, tt.key)
		}
	}
}

func TestEnvironmentAndFlags(t *testing.T) {
	layers, err := EnvironmentLayers([]string{
		"SHIP_GATES_ENABLED=no",
		"SHIP_GITHUB_LABELS_AREAS=api, ui",
		"SHIP_SECRETS_IGNOREPATHS=[\"testdata/**\"]",
		"SHIP_TOKEN=secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	eff := mustMerge(t, Target{}, layers...)
	if eff.Settings.Ship.Gates.Enabled || strings.Join(eff.Settings.GitHub.Labels.Areas, ",") != "api,ui" ||
		strings.Join(eff.Settings.Ship.Secrets.IgnorePaths, ",") != "testdata/**" {
		t.Fatalf("settings = %+v", eff.Settings)
	}
	if _, err := EnvironmentLayers([]string{"SHIP_PULLREQUEST_MAXLINES=lots"}); err == nil || !strings.Contains(err.Error(), "SHIP_PULLREQUEST_MAXLINES") {
		t.Errorf("a bad value must name its variable: %v", err)
	}
	if _, err := FlagLayers([]string{"ship.pullRequest.approvers=auto"}); err == nil {
		t.Error("an unknown key in --set must fail")
	}
	if _, err := FlagLayers([]string{"maxLines"}); err == nil {
		t.Error("--set without = must fail")
	}
	// Values are validated as in files: the enum and the minimum hold.
	flags, _ := FlagLayers([]string{"ship.publish.mode=sometimes"})
	if _, err := Merge(flags); err == nil {
		t.Error("an invalid enum value must fail")
	}
	if got := UnknownEnvironment([]string{"SHIP_TOKEN=x", "SHIP_PULLREQUEST_MAXLINE=3", "SHIP_GATES_ENABLED=1"}); strings.Join(got, ",") != "SHIP_PULLREQUEST_MAXLINE" {
		t.Errorf("UnknownEnvironment = %v", got)
	}
}

func TestEnvNamesAreUnique(t *testing.T) {
	seen := map[string]string{}
	for _, key := range Keys() {
		name := EnvName(key)
		if other, dup := seen[name]; dup {
			t.Errorf("%s and %s share %s", key, other, name)
		}
		if strings.Contains(strings.Join(notSettings, " "), name) {
			t.Errorf("%s collides with a reserved variable", name)
		}
		seen[name] = key
	}
}

func TestParseExtends(t *testing.T) {
	sha := strings.Repeat("ab12", 10)
	for in, want := range map[string]Extends{
		"ship-it:recommended": {Kind: BuiltinPreset, Name: "recommended"},
		"ship/base.json":      {Kind: FilePreset, Path: "ship/base.json"},
		"./base.jsonc":        {Kind: FilePreset, Path: "base.jsonc"},
		"github:acme/ship-config/base.json@" + sha: {Kind: GitHubPreset, Owner: "acme", Repository: "ship-config", Path: "base.json", Commit: sha},
	} {
		got, err := ParseExtends(in)
		if err != nil || got != want {
			t.Errorf("ParseExtends(%q) = %+v, %v", in, got, err)
		}
	}
	for _, bad := range []string{
		"ship-it:nope", "../base.json", "/etc/base.json", `C:\base.json`, "base.yaml",
		"github:acme/ship-config/base.json@main", "github:acme/ship-config/base.json@abc123",
		"github:acme/ship-config/../x.json@" + sha,
	} {
		if _, err := ParseExtends(bad); err == nil {
			t.Errorf("ParseExtends(%q) must fail", bad)
		}
	}
}

func TestBuiltinPresets(t *testing.T) {
	for name, check := range map[string]func(Settings) bool{
		"recommended": func(s Settings) bool { return s.Ship.PullRequest.MaxLines == 400 },
		"strict":      func(s Settings) bool { return s.Ship.PullRequest.MaxLines == 250 && !s.Ship.Gates.AllowSkip },
		"agency": func(s Settings) bool {
			return s.Ship.Publish.Mode == PublishNever && s.Ship.Issues.Keyword == "Refs" && s.GitHub.Labels.Taxonomy == TaxonomyNone
		},
	} {
		preset, ok := BuiltinPresetLayer(name)
		if !ok {
			t.Fatalf("no preset %s", name)
		}
		eff, err := Merge([]Layer{preset})
		if err != nil || !check(eff.Settings) {
			t.Errorf("ship-it:%s = %+v, %v", name, eff.Settings, err)
		}
		if _, err := ParseExtends("ship-it:" + name); err != nil {
			t.Errorf("ship-it:%s is not accepted in extends: %v", name, err)
		}
	}
}

func TestTrackerKeysAreValidated(t *testing.T) {
	if _, err := Merge([]Layer{layer("repository", KindRepository, `{"ship": {"issues": {"trackerKeys": ["ACME", "OPS_2"]}}}`)}); err != nil {
		t.Fatal(err)
	}
	_, err := Merge([]Layer{layer("repository", KindRepository, `{"ship": {"issues": {"trackerKeys": ["acme"]}}}`)})
	var f *fault.Fault
	if !errors.As(err, &f) || f.Code != fault.ConfigInvalidValue || f.Params["value"] != "acme" {
		t.Fatalf("err = %v", err)
	}
}
