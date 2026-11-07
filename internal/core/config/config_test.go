// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package config

import (
	"errors"
	"reflect"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/forge"
)

func TestMergeLayers(t *testing.T) {
	user := Layer{Name: "user", Path: "~/.config/ship-it/config.json",
		Data: []byte(`{"ship": {"publish": {"mode": "never"}, "issues": {"keyword": "Fixes"}}}`)}
	repo := Layer{Name: "repository", Path: ".github/ship.json",
		Data: []byte(`{"$schema": "x", "version": 1, "ship": {"publish": {"mode": "auto"}, "pullRequest": {"labels": false}},
		 "github": {"labels": {"areas": ["api", "ui"]}}}`)}

	eff, err := Merge([]Layer{user, repo})
	if err != nil {
		t.Fatal(err)
	}
	s := eff.Settings
	if s.Ship.Publish.Mode != PublishAuto || s.Ship.Issues.Keyword != "Fixes" || s.Ship.PullRequest.Labels ||
		s.Ship.PullRequest.Draft != DraftUnattended || !reflect.DeepEqual(s.GitHub.Labels.Areas, []string{"api", "ui"}) {
		t.Fatalf("settings = %+v", s)
	}
	for key, want := range map[string]string{
		"ship.publish.mode":       "repository",
		"ship.issues.keyword":     "user",
		"ship.pullRequest.labels": "repository",
		"ship.pullRequest.draft":  "default",
	} {
		if got := eff.Source(key); got != want {
			t.Errorf("Source(%s) = %s, want %s", key, got, want)
		}
	}
}

func TestMergeWithoutFilesGivesDefaults(t *testing.T) {
	eff, err := Merge([]Layer{{Name: "repository", Data: []byte("  \n")}})
	if err != nil || !reflect.DeepEqual(eff.Settings, Defaults()) || len(eff.Sources) != 0 {
		t.Fatalf("Merge = %+v, %v", eff, err)
	}
}

func TestMergeErrors(t *testing.T) {
	tests := []struct {
		name   string
		data   string
		code   fault.Code
		params map[string]string
	}{
		{"syntax", "{\n  \"ship\": {\n    \"publish\" \"auto\"\n  }\n}", fault.ConfigInvalidJSON, map[string]string{"line": "3"}},
		{"unknown key", `{"ship": {"pullRequest": {"approvers": "auto"}}}`, fault.ConfigUnknownKey,
			map[string]string{"key": "ship.pullRequest.approvers", "known": "ship.pullRequest.draft, ship.pullRequest.base, ship.pullRequest.labels, ship.pullRequest.labelPrefixes, ship.pullRequest.sizes, ship.pullRequest.maxLines, ship.pullRequest.template, ship.pullRequest.sections, ship.pullRequest.reviewers, ship.pullRequest.teamReviewers, ship.pullRequest.assignSelf, ship.pullRequest.milestone"}},
		{"wrong type", `{"ship": {"pullRequest": {"labels": "yes"}}}`, fault.ConfigWrongType, map[string]string{"expected": "true or false"}},
		{"enum", `{"ship": {"issues": {"keyword": "closes"}}}`, fault.ConfigInvalidValue, map[string]string{"allowed": "Closes, Fixes, Resolves, Refs"}},
		{"array item", `{"github": {"labels": {"areas": ["api", 3]}}}`, fault.ConfigWrongType, map[string]string{"key": "github.labels.areas"}},
		{"too new", `{"version": 2}`, fault.ConfigTooNew, map[string]string{"supported": "1"}},
		{"integer type", `{"ship": {"pullRequest": {"maxLines": "400"}}}`, fault.ConfigWrongType, map[string]string{"expected": "integer"}},
		{"integer minimum", `{"ship": {"pullRequest": {"maxLines": 0}}}`, fault.ConfigInvalidValue, map[string]string{"allowed": "≥ 1"}},
		{"log size minimum", `{"ship": {"logs": {"maxBytes": 1000}}}`, fault.ConfigInvalidValue, map[string]string{"key": "ship.logs.maxBytes", "allowed": "≥ 1048576"}},
		{"black box minimum", `{"ship": {"logs": {"blackBoxBytes": 65535}}}`, fault.ConfigInvalidValue, map[string]string{"key": "ship.logs.blackBoxBytes", "allowed": "≥ 65536"}},
		{"keep runs minimum", `{"ship": {"logs": {"keepRuns": 0}}}`, fault.ConfigInvalidValue, map[string]string{"key": "ship.logs.keepRuns", "allowed": "≥ 1"}},
		{"unknown log key", `{"ship": {"logs": {"level": "debug"}}}`, fault.ConfigUnknownKey,
			map[string]string{"key": "ship.logs.level", "known": "ship.logs.keepRuns, ship.logs.maxBytes, ship.logs.blackBoxBytes"}},
		{"bad version", `{"version": "one"}`, fault.ConfigInvalidValue, nil},
		{"two values", `{} {}`, fault.ConfigInvalidJSON, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Merge([]Layer{{Name: "repository", Path: ".github/ship.json", Data: []byte(tt.data)}})
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
				t.Error("the file must be named in the error")
			}
		})
	}
}

func TestRenderRoundTrip(t *testing.T) {
	if got := string(Render(Defaults())); got != "{\n  \"$schema\": \""+SchemaURL+"\",\n  \"version\": 1\n}\n" {
		t.Fatalf("defaults render:\n%s", got)
	}
	s := Defaults()
	s.Ship.Publish.Mode = PublishNever
	s.GitHub.Labels.Areas = []string{"api"}
	rendered := Render(s)
	want := "{\n  \"$schema\": \"" + SchemaURL + "\",\n  \"version\": 1,\n" +
		"  \"ship\": {\n    \"publish\": {\n      \"mode\": \"never\"\n    }\n  },\n" +
		"  \"github\": {\n    \"labels\": {\n      \"areas\": [\"api\"]\n    }\n  }\n}\n"
	if string(rendered) != want {
		t.Fatalf("Render =\n%s\nwant\n%s", rendered, want)
	}
	eff, err := Merge([]Layer{{Name: "repository", Data: rendered}})
	if err != nil || !reflect.DeepEqual(eff.Settings, s) {
		t.Fatalf("round trip = %+v, %v", eff.Settings, err)
	}
}

func TestFlatten(t *testing.T) {
	entries := Flatten(Defaults())
	want := []Entry{
		{"ship.publish.mode", `"auto"`},
		{"ship.publish.update", `"rebase"`},
		{"ship.git.remote", `"origin"`},
		{"ship.commits.gitmoji", `"auto"`},
		{"ship.commits.subjectMaxLength", "50"},
		{"ship.commits.headerMaxLength", "72"},
		{"ship.commits.bodyWidth", "72"},
		{"ship.commits.types", "{}"},
		{"ship.commits.scopes", "[]"},
		{"ship.commits.scopeRequired", "false"},
		{"ship.commits.scopeFrom", `"folders"`},
		{"ship.commits.signoff", `"auto"`},
		{"ship.commits.sign", `"auto"`},
		{"ship.branches.maxWords", "4"},
		{"ship.branches.maxLength", "32"},
		{"ship.branches.issue", `"first"`},
		{"ship.pullRequest.draft", `"unattended"`},
		{"ship.pullRequest.base", `""`},
		{"ship.pullRequest.labels", "true"},
		{"ship.pullRequest.labelPrefixes.kind", `"kind/"`},
		{"ship.pullRequest.labelPrefixes.size", `"size/"`},
		{"ship.pullRequest.sizes", `{"XS":10,"S":30,"M":100,"L":500,"XL":1000,"XXL":0}`},
		{"ship.pullRequest.maxLines", "400"},
		{"ship.pullRequest.template", `"auto"`},
		{"ship.pullRequest.sections.why", "true"},
		{"ship.pullRequest.sections.testing", "true"},
		{"ship.pullRequest.sections.issue", "true"},
		{"ship.pullRequest.sections.changes", "true"},
		{"ship.pullRequest.reviewers", "[]"},
		{"ship.pullRequest.teamReviewers", "[]"},
		{"ship.pullRequest.assignSelf", "false"},
		{"ship.pullRequest.milestone", `"issue"`},
		{"ship.issues.keyword", `"Closes"`},
		{"ship.issues.trackerKeys", "[]"},
		{"ship.secrets.ignorePaths", "[]"},
		{"ship.secrets.rules", "[]"},
		{"ship.secrets.disabledRules", "[]"},
		{"ship.secrets.allow", "[]"},
		{"ship.gates.enabled", "true"},
		{"ship.gates.allowSkip", "true"},
		{"ship.gates.beforeCommit", "false"},
		{"ship.tidy.auto", "true"},
		{"ship.tidy.deleteMerged", "true"},
		{"ship.logs.keepRuns", "20"},
		{"ship.logs.maxBytes", "10485760"},
		{"ship.logs.blackBoxBytes", "1048576"},
		{"ship.stack.rebase", `"github"`},
		{"ship.stack.maxLayers", "5"},
		{"github.managedBy", `""`},
		{"github.labels.taxonomy", `"ship-it"`},
		{"github.labels.areas", "[]"},
		{"github.project.url", `""`},
		{"github.project.statusField", `"Status"`},
		{"github.project.todo", `"Todo"`},
		{"github.project.inProgress", `"In progress"`},
		{"github.project.done", `"Done"`},
		{"github.project.dateField", `""`},
		{"github.project.phaseField", `""`},
		{"ui.language", `"auto"`},
		{"ui.reviewScreen", "true"},
		{"ui.accessible", "false"},
		{"ui.color", `"auto"`},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("Flatten = %v", entries)
	}
}

func TestProjectURLIsValidated(t *testing.T) {
	field, _ := structFieldOfKey("github.project.url")
	if field.Tag.Get("pattern") != forge.ProjectPattern {
		t.Fatal("the pattern tag of github.project.url must be forge.ProjectPattern")
	}
	for value, valid := range map[string]bool{
		"https://github.com/orgs/acme/projects/7": true,
		"ada/3": true,
		"":      true,
		"https://github.com/acme/shop/projects/1": false,
		"acme": false,
	} {
		_, err := Merge([]Layer{{Name: "repository", Path: "ship.json", Data: []byte(`{"github": {"project": {"url": "` + value + `"}}}`)}})
		if valid != (err == nil) {
			t.Errorf("%q: err = %v", value, err)
		}
		if !valid && !errors.Is(err, &fault.Fault{Code: fault.ConfigInvalidValue}) {
			t.Errorf("%q: want ConfigInvalidValue, got %v", value, err)
		}
	}
}

func FuzzMerge(f *testing.F) {
	f.Add([]byte(`{"ship": {"publish": {"mode": "never"}}}`))
	f.Add([]byte(`{"github": {"labels": {"areas": []}}}`))
	f.Add([]byte(`[]`))
	f.Fuzz(func(t *testing.T, data []byte) {
		eff, err := Merge([]Layer{{Name: "repository", Path: "f", Data: data}})
		if err != nil {
			var f *fault.Fault
			if !errors.As(err, &f) {
				t.Fatalf("error is not a fault: %v", err)
			}
			return
		}
		if eff.Settings.Ship.Publish.Mode != PublishAuto && eff.Settings.Ship.Publish.Mode != PublishNever {
			t.Fatalf("invalid publish mode accepted: %q", eff.Settings.Ship.Publish)
		}
	})
}
