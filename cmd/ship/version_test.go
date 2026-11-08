// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"encoding/json"
	"runtime/debug"
	"testing"

	"github.com/GregoireF/ship-it/internal/surface/cli"
)

func TestBuildInfo(t *testing.T) {
	installed := func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}, Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "749cf69abc"}, {Key: "vcs.time", Value: "2026-11-06T20:00:00Z"},
		}}, true
	}
	tests := []struct {
		name                string
		version, commit, at string
		read                func() (*debug.BuildInfo, bool)
		want                [3]string
	}{
		{"go install", "", "", "", installed, [3]string{"v0.1.0", "749cf69abc", "2026-11-06T20:00:00Z"}},
		{"go install without vcs stamps", "", "", "", func() (*debug.BuildInfo, bool) {
			return &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}}, true
		}, [3]string{"v0.1.0", "none", "unknown"}},
		{"release", "0.1.0", "abc", "2026-11-01", installed, [3]string{"0.1.0", "abc", "2026-11-01"}},
		{"go build", "", "", "", func() (*debug.BuildInfo, bool) {
			return &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, true
		}, [3]string{"dev", "none", "unknown"}},
		{"no build information", "", "", "", func() (*debug.BuildInfo, bool) { return nil, false }, [3]string{"dev", "none", "unknown"}},
	}
	for _, tt := range tests {
		v, c, d := buildInfo(tt.version, tt.commit, tt.at, tt.read)
		if got := [3]string{v, c, d}; got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

// TestVersionPrintsTheVersionAlone: a go install records the module version
// but no commit and no date; the placeholders would only be noise. A release
// build keeps them, and --json always gives every field.
func TestVersionPrintsTheVersionAlone(t *testing.T) {
	s := newScenario(t, nil)
	installed := cli.BuildInfo{Version: "v0.1.0", Commit: "none", Date: "unknown"}
	release := cli.BuildInfo{Version: "0.1.0", Commit: "2cb822e", Date: "2026-11-07T15:23:09Z"}
	code, stdout, _ := s.shipWith(installed, "version")
	if code != cli.ExitOK || stdout != "ship v0.1.0\n" {
		t.Errorf("go install: exit %d, output %q", code, stdout)
	}
	code, stdout, _ = s.shipWith(release, "version")
	if code != cli.ExitOK || stdout != "ship 0.1.0 (2cb822e, 2026-11-07T15:23:09Z)\n" {
		t.Errorf("release: exit %d, output %q", code, stdout)
	}
	code, stdout, _ = s.shipWith(installed, "version", "--json")
	if code != cli.ExitOK {
		t.Errorf("version --json: exit %d\n%s", code, stdout)
	}
	var fields map[string]string
	if err := json.Unmarshal([]byte(stdout), &fields); err != nil {
		t.Fatalf("version --json: %v\n%s", err, stdout)
	}
	want := map[string]string{"version": "v0.1.0", "commit": "none", "date": "unknown"}
	for k, v := range want {
		if fields[k] != v {
			t.Errorf("version --json %s = %q, want %q", k, fields[k], v)
		}
	}
	if _, ok := fields["go"]; !ok {
		t.Error("version --json lost the go field")
	}
}
