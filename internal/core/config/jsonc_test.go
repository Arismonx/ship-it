// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package config

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
)

func TestStripJSONC(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{`{"a": 1}`, `{"a": 1}`},
		{"{\"a\": 1, // why\n}", "{\"a\": 1        \n}"},
		{`{"a": /* inline */ 1}`, `{"a":              1}`},
		{`{"a": [1, 2,]}`, `{"a": [1, 2 ]}`},
		{`{"a": "// not a comment, /* nor this */", "b": ",]"}`, `{"a": "// not a comment, /* nor this */", "b": ",]"}`},
		{`{"a": "escaped \" // still a string"}`, `{"a": "escaped \" // still a string"}`},
		{"{\"a\": 1 /* unterminated", "{\"a\": 1                "},
		{`{,}`, `{,}`},
		{`[1,,]`, `[1,,]`},
	} {
		if got := string(StripJSONC([]byte(tt.in))); got != tt.want {
			t.Errorf("StripJSONC(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCommentedRepositoryFile(t *testing.T) {
	eff, err := Merge([]Layer{{Name: "repository", Path: ".github/ship.json", Data: []byte(`{
  // Reviews here are slow: keep pull requests small.
  "ship": {
    "pullRequest": {
      "maxLines": 250, /* agreed on 2026-10-23 */
    },
  },
}`)}})
	if err != nil {
		t.Fatal(err)
	}
	if eff.Settings.Ship.PullRequest.MaxLines != 250 {
		t.Fatalf("maxLines = %d", eff.Settings.Ship.PullRequest.MaxLines)
	}
}

func TestJSONCErrorsKeepTheirPosition(t *testing.T) {
	_, err := Merge([]Layer{{Name: "repository", Path: "ship.json", Data: []byte("{\n  // comment\n  \"ship\": {,}\n}")}})
	var f *fault.Fault
	if !errors.As(err, &f) || f.Code != fault.ConfigInvalidJSON || f.Params["line"] != "3" {
		t.Fatalf("err = %v", err)
	}
}

// FuzzStripJSONC: stripping never changes the length nor moves a newline,
// and valid JSON stays the same value.
func FuzzStripJSONC(f *testing.F) {
	f.Add(`{"a": [1, 2,], // c` + "\n" + `"b": "/*x*/"}`)
	f.Add(`"\`)
	f.Fuzz(func(t *testing.T, in string) {
		out := StripJSONC([]byte(in))
		if len(out) != len(in) {
			t.Fatalf("length changed: %d → %d", len(in), len(out))
		}
		for i := range in {
			if (in[i] == '\n') != (out[i] == '\n') {
				t.Fatalf("newline moved at %d", i)
			}
		}
		var before, after any
		if json.Unmarshal([]byte(in), &before) == nil {
			if err := json.Unmarshal(out, &after); err != nil {
				t.Fatalf("valid JSON broken: %q → %q (%v)", in, out, err)
			}
		}
	})
}
