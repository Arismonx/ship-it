// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package forge

import "testing"

func TestParseRemoteURL(t *testing.T) {
	tests := []struct {
		raw  string
		want Repository
		ok   bool
	}{
		{"https://github.com/GregoireF/ship-it.git", Repository{"github.com", "GregoireF", "ship-it"}, true},
		{"https://github.com/GregoireF/ship-it", Repository{"github.com", "GregoireF", "ship-it"}, true},
		{"https://user@GitHub.com/o/r.git", Repository{"github.com", "o", "r"}, true},
		{"git@github.com:o/r.git", Repository{"github.com", "o", "r"}, true},
		{"ssh://git@github.com:22/o/r.git", Repository{"github.com", "o", "r"}, true},
		{"git@ghe.example.com:team/app.git", Repository{"ghe.example.com", "team", "app"}, true},
		{"https://acme.ghe.com/o/r", Repository{"acme.ghe.com", "o", "r"}, true},
		{"/srv/git/r.git", Repository{}, false},
		{`C:\repos\r.git`, Repository{}, false},
		{"file:///srv/git/o/r.git", Repository{}, false},
		{"https://github.com/only-owner", Repository{}, false},
		{"", Repository{}, false},
	}
	for _, tt := range tests {
		got, ok := ParseRemoteURL(tt.raw)
		if ok != tt.ok || got != tt.want {
			t.Errorf("ParseRemoteURL(%q) = %+v, %v; want %+v, %v", tt.raw, got, ok, tt.want, tt.ok)
		}
	}
}

func TestAPIBaseURL(t *testing.T) {
	for host, want := range map[string]string{
		"github.com":      "https://api.github.com",
		"acme.ghe.com":    "https://api.acme.ghe.com",
		"git.example.org": "https://git.example.org/api/v3",
	} {
		if got := (Repository{Host: host}).APIBaseURL(); got != want {
			t.Errorf("%s: APIBaseURL = %s, want %s", host, got, want)
		}
	}
}

func FuzzParseRemoteURL(f *testing.F) {
	for _, seed := range []string{"git@github.com:o/r.git", "https://github.com/o/r", "ssh://h/o/r", ":", "@:/"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if r, ok := ParseRemoteURL(raw); ok && (r.Host == "" || r.Owner == "" || r.Name == "") {
			t.Fatalf("accepted %q with an empty part: %+v", raw, r)
		}
	})
}
