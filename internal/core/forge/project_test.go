// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package forge

import "testing"

func TestParseProject(t *testing.T) {
	tests := []struct {
		raw  string
		want Project
		ok   bool
	}{
		{"https://github.com/orgs/acme/projects/7", Project{Host: "github.com", Kind: ProjectOfOrganization, Owner: "acme", Number: 7}, true},
		{"https://github.com/users/ada/projects/3/", Project{Host: "github.com", Kind: ProjectOfUser, Owner: "ada", Number: 3}, true},
		{"https://github.com/orgs/acme/projects/7/views/2", Project{Host: "github.com", Kind: ProjectOfOrganization, Owner: "acme", Number: 7}, true},
		{"https://GHE.example.org/orgs/team/projects/12", Project{Host: "ghe.example.org", Kind: ProjectOfOrganization, Owner: "team", Number: 12}, true},
		{"acme/7", Project{Owner: "acme", Number: 7}, true},
		{"", Project{}, false},
		{"acme", Project{}, false},
		{"acme/0", Project{}, false},
		{"https://github.com/acme/shop/projects/7", Project{}, false}, // classic repository project
		{"http://github.com/orgs/acme/projects/7", Project{}, false},
		{"https://github.com/orgs/acme/projects/x", Project{}, false},
	}
	for _, tt := range tests {
		got, ok := ParseProject(tt.raw)
		if ok != tt.ok || got != tt.want {
			t.Errorf("ParseProject(%q) = %+v, %v; want %+v, %v", tt.raw, got, ok, tt.want, tt.ok)
		}
	}
	if s := (Project{Owner: "acme", Number: 7}).String(); s != "acme/7" {
		t.Errorf("String = %q", s)
	}
}

func FuzzParseProject(f *testing.F) {
	for _, seed := range []string{"acme/7", "https://github.com/orgs/acme/projects/7", "/", "https://x/users/y/projects/1/views/0"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if p, ok := ParseProject(raw); ok && (p.Owner == "" || p.Number < 1) {
			t.Fatalf("accepted %q with an empty part: %+v", raw, p)
		}
	})
}
