// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"runtime/debug"
	"testing"
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
