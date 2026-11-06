// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package split

import (
	"reflect"
	"testing"
)

func TestSuggest(t *testing.T) {
	files := []File{
		{"src/api/vat.go", 200}, {"src/api/vat_test.go", 150},
		{"src/ui/cart.vue", 120},
		{"docs/vat.md", 40},
		{"README.md", 5},
	}
	got := Suggest(files, 400)
	want := []Group{
		{Path: "src/api/", Lines: 350, Files: []string{"src/api/vat.go", "src/api/vat_test.go"}},
		{Path: "src/ui/", Lines: 120, Files: []string{"src/ui/cart.vue"}},
		{Path: "docs/", Lines: 40, Files: []string{"docs/vat.md"}},
		{Path: "README.md", Lines: 5, Files: []string{"README.md"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Suggest =\n%+v\nwant\n%+v", got, want)
	}
	if Suggest(files, 1000) != nil {
		t.Error("small enough: no suggestion")
	}
	if Suggest([]File{{"src/api/a.go", 500}, {"src/api/b.go", 500}}, 400) != nil {
		t.Error("one topic cannot be split by directory")
	}
}

func TestTopic(t *testing.T) {
	for p, want := range map[string]string{
		"internal/core/plan/plan.go": "internal/core/",
		"cmd/ship/main.go":           "cmd/ship/",
		"src/main.go":                "src/",
		"go.mod":                     "go.mod",
		"apps/web/src/a.ts":          "apps/web/",
	} {
		if got := topic(p); got != want {
			t.Errorf("topic(%s) = %s, want %s", p, got, want)
		}
	}
}

func TestMatches(t *testing.T) {
	only := []string{"src/api/", `docs\vat.md`}
	for p, want := range map[string]bool{
		"src/api/vat.go": true, "src/apix/a.go": false, "docs/vat.md": true, "docs/other.md": false,
	} {
		if Matches(p, only) != want {
			t.Errorf("Matches(%s) != %v", p, want)
		}
	}
}
