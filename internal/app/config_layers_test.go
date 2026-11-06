// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/config"
	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/ports"
	"github.com/GregoireF/ship-it/internal/testkit"
)

var presetSHA = strings.Repeat("0123456789", 4)

func TestPresetsAreExpandedBeforeTheirFile(t *testing.T) {
	files := &testkit.FakeConfigFiles{
		Repository: `{"extends": ["ship-it:recommended", "ship/base.json", "github:agency/ship-config/base.json@` + presetSHA + `"],
		              "ship": {"pullRequest": {"maxLines": 300}}}`,
		Presets: map[string]string{"ship/base.json": `{"ship": {"issues": {"keyword": "Fixes"}, "pullRequest": {"maxLines": 100}}}`},
	}
	forge := &testkit.FakeForge{Contents: map[string]string{
		"agency/ship-config/base.json@" + presetSHA: `{"extends": ["shared.json"], "ship": {"publish": {"mode": "never"}}}`,
		// A file preset inside a GitHub preset is a file of that repository, at the same commit.
		"agency/ship-config/shared.json@" + presetSHA: `{"ship": {"gates": {"enabled": false}}}`,
	}}
	ws := Workspace{Git: testkit.HealthyRepository("main"), Config: files, Forge: forge}
	eff, err := LoadSettings(context.Background(), ws)
	if err != nil {
		t.Fatal(err)
	}
	s := eff.Settings.Ship
	if s.Issues.Keyword != "Fixes" || s.Publish.Mode != "never" || s.Gates.Enabled || s.PullRequest.MaxLines != 300 {
		t.Fatalf("settings = %+v", s)
	}
	if got := eff.Source("ship.issues.keyword"); got != "ship/base.json" {
		t.Errorf("source = %q", got)
	}
	// Pinned to a commit, the GitHub presets are cached: no second request.
	calls := forge.ContentCalls
	if _, err := LoadSettings(context.Background(), ws); err != nil || forge.ContentCalls != calls {
		t.Fatalf("cached presets fetched again: %d → %d calls, %v", calls, forge.ContentCalls, err)
	}
}

func TestPresetErrors(t *testing.T) {
	for name, tt := range map[string]struct {
		files *testkit.FakeConfigFiles
		code  fault.Code
	}{
		"missing file": {&testkit.FakeConfigFiles{Repository: `{"extends": ["nope.json"]}`}, fault.ConfigPresetUnavailable},
		"loop": {&testkit.FakeConfigFiles{Repository: `{"extends": ["a.json"]}`,
			Presets: map[string]string{"a.json": `{"extends": ["b.json"]}`, "b.json": `{"extends": ["a.json"]}`}}, fault.ConfigPresetCycle},
		"github offline": {&testkit.FakeConfigFiles{Repository: `{"extends": ["github:agency/c/x.json@` + presetSHA + `"]}`}, fault.ConfigPresetUnavailable},
	} {
		ws := Workspace{Git: testkit.HealthyRepository("main"), Config: tt.files, Forge: &testkit.FakeForge{Err: ports.ErrRateLimited}}
		_, err := LoadSettings(context.Background(), ws)
		if !errors.Is(err, &fault.Fault{Code: tt.code}) {
			t.Errorf("%s: err = %v, want %s", name, err, tt.code)
		}
	}
}

func TestEnvironmentFlagsAndContexts(t *testing.T) {
	files := &testkit.FakeConfigFiles{
		User:       `{"contexts": [{"match": {"owner": "acme"}, "ship": {"issues": {"keyword": "Resolves"}}}]}`,
		Repository: `{"ship": {"pullRequest": {"maxLines": 300}}}`,
	}
	ws := Workspace{Git: testkit.HealthyRepository("main"), Config: files, // origin is acme/shop
		Environ: []string{"SHIP_PULLREQUEST_MAXLINES=500"}, SettingFlags: []string{"ship.gates.enabled=false"}}
	eff, err := LoadSettings(context.Background(), ws)
	if err != nil {
		t.Fatal(err)
	}
	s := eff.Settings.Ship
	if s.Issues.Keyword != "Resolves" || s.PullRequest.MaxLines != 500 || s.Gates.Enabled {
		t.Fatalf("settings = %+v (sources %v)", s, eff.Sources)
	}
}

func TestOverridesFollowTheDeliveredFiles(t *testing.T) {
	gateCalls := func(changed, config string) []string {
		git := publishableGit(nil)
		git.DiffStats = []ports.FileStat{{Path: changed, Added: 3}}
		git.WorkingFiles = map[string]string{"package.json": `{"scripts": {"test": "vitest"}}`}
		runner := &testkit.FakeRunner{}
		ws, _ := publishWorkspace(git, &testkit.FakeForge{})
		ws.Runner = runner
		ws.Config = &testkit.FakeConfigFiles{Repository: config}
		if _, err := Publish(context.Background(), ws, PublishRequest{}); err != nil {
			t.Fatal(err)
		}
		return runner.Calls
	}
	override := `{"overrides": [{"paths": ["docs/**"], "ship": {"gates": {"enabled": false}}}]}`
	if calls := gateCalls("docs/guide.md", "{}"); len(calls) == 0 {
		t.Fatal("control: without the override the gates run")
	}
	if calls := gateCalls("docs/guide.md", override); len(calls) != 0 {
		t.Errorf("a docs-only branch skips the gates: %q", calls)
	}
	if calls := gateCalls("src/vat.ts", override); len(calls) == 0 {
		t.Error("a code change still runs them")
	}
}

func TestStrictRefusesSkippedGates(t *testing.T) {
	var calls []string
	git := publishableGit(&calls)
	git.WorkingFiles = map[string]string{"package.json": `{"scripts": {"test": "vitest"}}`}
	ws, _ := publishWorkspace(git, &testkit.FakeForge{})
	ws.Runner = &testkit.FakeRunner{}
	ws.Config = &testkit.FakeConfigFiles{Repository: `{"extends": ["ship-it:strict"]}`}
	_, err := Publish(context.Background(), ws, PublishRequest{SkipGates: true})
	if !errors.Is(err, &fault.Fault{Code: fault.GateSkipRefused}) || testkit.Pushed(calls) {
		t.Fatalf("err = %v, calls %q", err, calls)
	}
}

func TestAGoProjectIsCheckedBeforeThePush(t *testing.T) {
	var calls []string
	git := publishableGit(&calls)
	git.DiffStats = []ports.FileStat{{Path: "billing/vat.go", Added: 3}}
	git.WorkingFiles = map[string]string{"go.mod": "module shop\n", "billing/vat.go": "package billing\n"}
	runner := &testkit.FakeRunner{Printing: map[string]string{"gofmt -l billing/vat.go": "billing/vat.go\n"}}
	ws, _ := publishWorkspace(git, &testkit.FakeForge{})
	ws.Runner = runner
	_, err := Publish(context.Background(), ws, PublishRequest{})
	results, red := GatesIn(err)
	if !red || results[len(results)-1].Name != "gofmt" || testkit.Pushed(calls) {
		t.Fatalf("err = %v, results %+v, calls %q", err, results, calls)
	}
	if strings.Join(runner.Calls, ";") != "gofmt -l billing/vat.go" {
		t.Errorf("gofmt runs first, on the changed file only: %q", runner.Calls)
	}
}

func TestGitHubPresetPathsMayHoldAnAt(t *testing.T) {
	owner, repository, commit, ok := gitHubOrigin(config.Layer{Path: "github:acme/cfg/presets/@team/base.json@" + presetSHA})
	if !ok || owner != "acme" || repository != "cfg" || commit != presetSHA {
		t.Fatalf("gitHubOrigin = %q %q %q %v", owner, repository, commit, ok)
	}
}
