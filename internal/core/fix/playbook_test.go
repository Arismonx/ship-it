// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package fix

import (
	"reflect"
	"testing"
)

func kinds(remedies []Remedy) []RemedyKind {
	var k []RemedyKind
	for _, r := range remedies {
		k = append(k, r.Kind)
	}
	return k
}

func TestRemedies(t *testing.T) {
	f := FixFacts{
		Facts:          Facts{OperatingSystem: "windows", UserName: "Ada", MirrorRemotes: []string{"backup"}},
		GitHubUser:     &GitHubUser{Login: "ada", ID: 583231, Name: "Ada Lovelace"},
		Tracked:        []string{"src/a.ts", "node_modules/x/index.js", "web/node_modules/y.js", ".env.local", ".env.example", "config/.env"},
		Gitignore:      "dist/\n.env.*\n",
		MergedBranches: 2,
	}
	got := Remedies(f)
	want := []RemedyKind{SetIdentity, EnableLongPaths, DisableMirror, UntrackFiles, TidyBranches}
	if !reflect.DeepEqual(kinds(got), want) {
		t.Fatalf("kinds = %v", kinds(got))
	}
	if got[0].Params["name"] != "Ada" || got[0].Params["email"] != "583231+ada@users.noreply.github.com" {
		t.Errorf("identity = %v (the existing name is kept)", got[0].Params)
	}
	untrack := got[3]
	if !reflect.DeepEqual(untrack.Paths, []string{"node_modules/x/index.js", "web/node_modules/y.js", ".env.local", "config/.env"}) {
		t.Errorf("paths = %v", untrack.Paths)
	}
	if !reflect.DeepEqual(untrack.Patterns, []string{"node_modules/", ".env", "!.env.example"}) {
		t.Errorf("patterns = %v (.env.* is already ignored)", untrack.Patterns)
	}
}

func TestNothingToFix(t *testing.T) {
	f := FixFacts{Facts: Facts{OperatingSystem: "linux", UserName: "Ada", UserEmail: "a@b.c"}, Tracked: []string{"go.mod", ".env.example"}}
	if r := Remedies(f); len(r) != 0 {
		t.Fatalf("remedies = %+v", r)
	}
}

func TestIdentityNeedsGitHub(t *testing.T) {
	// Without a token there is nothing to propose: the doctor shows the command.
	if r := Remedies(FixFacts{Facts: Facts{OperatingSystem: "linux"}}); len(r) != 0 {
		t.Fatalf("remedies = %+v", r)
	}
}
