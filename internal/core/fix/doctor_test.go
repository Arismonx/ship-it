// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package fix

import (
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
)

func healthyFacts() Facts {
	return Facts{
		OperatingSystem: "linux", GitVersion: "2.52.0", InsideRepository: true,
		UserName: "Ada", UserEmail: "ada@example.invalid", HasRemote: true,
	}
}

func codes(findings []Finding) []fault.Code {
	out := make([]fault.Code, 0, len(findings))
	for _, f := range findings {
		if f.Severity != Healthy {
			out = append(out, f.Code)
		}
	}
	return out
}

func TestParseGitVersion(t *testing.T) {
	cases := map[string]GitVersion{
		"2.52.0.windows.1": {2, 52, 0},
		"2.43.0":           {2, 43, 0},
		"2.39.5 (Apple)":   {2, 39, 0},
		"3.0":              {3, 0, 0},
	}
	for in, want := range cases {
		if got, ok := ParseGitVersion(in); !ok || got != want {
			t.Errorf("ParseGitVersion(%q) = %v, %v; want %v", in, got, ok, want)
		}
	}
	if _, ok := ParseGitVersion("garbage"); ok {
		t.Error("garbage accepted")
	}
}

func TestDiagnose(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Facts)
		want   []fault.Code
	}{
		{"healthy", func(*Facts) {}, nil},
		{"git missing stops everything", func(f *Facts) { f.GitVersion = "" }, []fault.Code{fault.GitMissing}},
		{"git 2.39 too old (Debian 12)", func(f *Facts) { f.GitVersion = "2.39.5" }, []fault.Code{fault.GitTooOld}},
		{"git 2.43 accepted (Ubuntu 24.04)", func(f *Facts) { f.GitVersion = "2.43.0" }, nil},
		{"identity missing", func(f *Facts) { f.UserEmail = "" }, []fault.Code{fault.GitIdentityMissing}},
		{"windows without long paths", func(f *Facts) { f.OperatingSystem = "windows" }, []fault.Code{fault.LongPathsDisabled}},
		{"outside a repository", func(f *Facts) { f.InsideRepository = false }, []fault.Code{fault.NotARepository}},
		{"rebase in progress", func(f *Facts) { f.OperationInProgress = "rebase" }, []fault.Code{fault.OperationInProgress}},
		{"detached and no origin", func(f *Facts) { f.DetachedHead = true; f.HasRemote = false },
			[]fault.Code{fault.DetachedHead, fault.NoOriginRemote}},
		{"mirror remote", func(f *Facts) { f.MirrorRemotes = []string{"backup"} }, []fault.Code{fault.MirrorRemote}},
		{"remote not on GitHub", func(f *Facts) { f.RemoteNotGitHub = true }, []fault.Code{fault.RemoteNotGitHub}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts := healthyFacts()
			tt.mutate(&facts)
			got := codes(Diagnose(facts))
			if len(got) != len(tt.want) {
				t.Fatalf("codes = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("code %d = %s, want %s", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestWorst(t *testing.T) {
	facts := healthyFacts()
	facts.DetachedHead = true
	if Worst(Diagnose(facts)) != Warning {
		t.Error("detached HEAD must be a warning")
	}
	facts.UserName = ""
	if Worst(Diagnose(facts)) != Blocking {
		t.Error("missing identity must block")
	}
}

func TestEveryNonHealthyFindingHasARemedy(t *testing.T) {
	facts := Facts{OperatingSystem: "windows", GitVersion: "2.52.0", InsideRepository: true,
		OperationInProgress: "merge", DetachedHead: true, MirrorRemotes: []string{"m"}, RemoteNotGitHub: true}
	for _, f := range Diagnose(facts) {
		if f.Severity != Healthy && f.Remedy == "" {
			t.Errorf("%s has no remedy", f.Code)
		}
	}
}
