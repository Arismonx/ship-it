// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package config

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/pullrequest"
)

func TestGitAndPullRequestSettings(t *testing.T) {
	eff, err := Merge([]Layer{{Name: "repository", Path: ".github/ship.json", Data: []byte(`{"ship": {
		"publish": {"update": "merge"},
		"git": {"remote": "fork"},
		"commits": {"signoff": "always", "sign": "never"},
		"tidy": {"auto": false, "deleteMerged": false},
		"pullRequest": {"draft": "never", "base": "develop",
			"labelPrefixes": {"kind": "type: ", "size": ""},
			"sizes": {"small": 50, "large": 0},
			"sections": {"why": false},
			"reviewers": ["ada", "grace-h"], "teamReviewers": ["backend"], "assignSelf": true, "milestone": "none"}}}`)}})
	if err != nil {
		t.Fatal(err)
	}
	s := eff.Settings.Ship
	if s.Publish.Update != UpdateMerge || s.Publish.Mode != PublishAuto || s.Git.Remote != "fork" ||
		s.Commits.Signoff != ModeAlways || s.Commits.Sign != ModeNever || s.Tidy.Auto || s.Tidy.DeleteMerged {
		t.Fatalf("settings = %+v", s)
	}
	pr := s.PullRequest
	if pr.Draft != DraftNever || pr.Base != "develop" || pr.LabelPrefixes.Kind != "type: " || pr.LabelPrefixes.Size != "" ||
		!reflect.DeepEqual(pr.Sizes, Sizes{"small": 50, "large": 0}) || pr.Sections.Why || !pr.Sections.Testing ||
		!reflect.DeepEqual(pr.Reviewers, []string{"ada", "grace-h"}) || !pr.AssignSelf || pr.Milestone != MilestoneNone {
		t.Fatalf("pull request settings = %+v", pr)
	}
	tax := eff.Settings.PullRequestLabels()
	if got := tax.SizeLabel(49); got != "small" {
		t.Errorf("SizeLabel(49) = %q", got)
	}
	if got := tax.SizeLabel(5000); got != "large" {
		t.Errorf("SizeLabel(5000) = %q", got)
	}
	if got := Defaults().PullRequestLabels().SizeLabel(999); got != "size/XL" {
		t.Errorf("default SizeLabel(999) = %q", got)
	}
}

func TestSizesReplaceTheWholeTable(t *testing.T) {
	user := Layer{Name: "user", Path: "user.json", Data: []byte(`{"ship": {"pullRequest": {"sizes": {"S": 10, "M": 0}}}}`)}
	repo := Layer{Name: "repository", Path: "ship.json", Data: []byte(`{"ship": {"pullRequest": {"sizes": {"tiny": 5, "huge": 0}}}}`)}
	eff, err := Merge([]Layer{user, repo})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(eff.Settings.Ship.PullRequest.Sizes, Sizes{"tiny": 5, "huge": 0}) {
		t.Errorf("sizes = %v", eff.Settings.Ship.PullRequest.Sizes)
	}
	if got := eff.Settings.Ship.PullRequest.Sizes.List(); !reflect.DeepEqual(got, []pullrequest.Size{{Name: "tiny", Below: 5}, {Name: "huge", Below: 0}}) {
		t.Errorf("List = %v", got)
	}
}

func TestGitAndPullRequestErrors(t *testing.T) {
	tests := []struct {
		name, data string
		code       fault.Code
		key        string
	}{
		{"update", `{"ship": {"publish": {"update": "squash"}}}`, fault.ConfigInvalidValue, "ship.publish.update"},
		{"remote", `{"ship": {"git": {"remote": "-x"}}}`, fault.ConfigInvalidValue, "ship.git.remote"},
		{"signoff", `{"ship": {"commits": {"signoff": "yes"}}}`, fault.ConfigInvalidValue, "ship.commits.signoff"},
		{"sign", `{"ship": {"commits": {"sign": true}}}`, fault.ConfigWrongType, "ship.commits.sign"},
		{"draft", `{"ship": {"pullRequest": {"draft": "sometimes"}}}`, fault.ConfigInvalidValue, "ship.pullRequest.draft"},
		{"old draft key", `{"ship": {"pullRequest": {"draftWhenUnattended": true}}}`, fault.ConfigUnknownKey, "ship.pullRequest.draftWhenUnattended"},
		{"base", `{"ship": {"pullRequest": {"base": "-main"}}}`, fault.ConfigInvalidValue, "ship.pullRequest.base"},
		{"same threshold", `{"ship": {"pullRequest": {"sizes": {"S": 10, "M": 10}}}}`, fault.ConfigInvalidValue, "ship.pullRequest.sizes.S"},
		{"two unbounded", `{"ship": {"pullRequest": {"sizes": {"L": 0, "XL": 0}}}}`, fault.ConfigInvalidValue, "ship.pullRequest.sizes.XL"},
		{"negative", `{"ship": {"pullRequest": {"sizes": {"S": -1}}}}`, fault.ConfigInvalidValue, "ship.pullRequest.sizes.S"},
		{"empty table", `{"ship": {"pullRequest": {"sizes": {}}}}`, fault.ConfigInvalidValue, "ship.pullRequest.sizes"},
		{"bad name", `{"ship": {"pullRequest": {"sizes": {"x l": 3}}}}`, fault.ConfigInvalidValue, "ship.pullRequest.sizes.x l"},
		{"not a number", `{"ship": {"pullRequest": {"sizes": {"S": "10"}}}}`, fault.ConfigWrongType, "ship.pullRequest.sizes.S"},
		{"section", `{"ship": {"pullRequest": {"sections": {"notes": true}}}}`, fault.ConfigUnknownKey, "ship.pullRequest.sections.notes"},
		{"reviewer", `{"ship": {"pullRequest": {"reviewers": ["@ada"]}}}`, fault.ConfigInvalidValue, "ship.pullRequest.reviewers"},
		{"milestone", `{"ship": {"pullRequest": {"milestone": "always"}}}`, fault.ConfigInvalidValue, "ship.pullRequest.milestone"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Merge([]Layer{{Name: "repository", Path: ".github/ship.json", Data: []byte(tt.data)}})
			var f *fault.Fault
			if !errors.As(err, &f) || f.Code != tt.code || f.Params["key"] != tt.key {
				t.Fatalf("error = %v, want %s on %s", err, tt.code, tt.key)
			}
		})
	}
}

func TestSizesFromFlagsAndSchema(t *testing.T) {
	layers, err := FlagLayers([]string{`ship.pullRequest.sizes={"S": 20, "L": 0}`, "ship.git.remote=fork"})
	if err != nil {
		t.Fatal(err)
	}
	eff, err := Merge(layers)
	if err != nil || eff.Settings.Ship.PullRequest.Sizes["S"] != 20 || eff.Settings.Ship.Git.Remote != "fork" {
		t.Fatalf("Merge = %+v, %v", eff.Settings.Ship, err)
	}
	if _, err := FlagLayers([]string{"ship.pullRequest.sizes=S"}); err == nil || !strings.Contains(err.Error(), "size name") {
		t.Errorf("a size table that is no JSON must say what is expected: %v", err)
	}
	if schema := string(Schema()); !strings.Contains(schema, `"sizes"`) || !strings.Contains(schema, `"minProperties": 1`) {
		t.Error("the schema must describe ship.pullRequest.sizes")
	}
	if !strings.Contains(string(Reference()), "size name → changed lines") {
		t.Error("the reference must describe ship.pullRequest.sizes")
	}
}
